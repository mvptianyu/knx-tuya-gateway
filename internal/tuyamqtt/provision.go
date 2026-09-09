package tuyamqtt

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const gatewayActivePath = "/v1.0/end-user/devices/ha/gateway/active"

type RegionEndpoints struct {
	Key         string
	OpenAPIBase string
	MQTTBroker  string
}

type ProvisionResult struct {
	ProductID string
	DeviceID  string
	ShortURL  string
	Region    string
	Broker    string
}

type gatewayActivationResponse struct {
	Success   bool   `json:"success"`
	ErrorCode string `json:"errorCode"`
	ErrorMsg  string `json:"errorMsg"`
	Result    struct {
		ProductID    string `json:"productId"`
		DeviceID     string `json:"deviceId"`
		DeviceSecret string `json:"deviceSecret"`
		ShortURL     string `json:"shortUrl"`
	} `json:"result"`
}

func ProvisionGateway(ctx context.Context, credentialsFile, apiKey string) (ProvisionResult, error) {
	values, err := readEnvFile(credentialsFile)
	if err != nil && !os.IsNotExist(err) {
		return ProvisionResult{}, err
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" || strings.HasPrefix(apiKey, "replace_") {
		return ProvisionResult{}, fmt.Errorf("fill tuya_mqtt.api_key in the runtime config first")
	}
	endpoints, err := EndpointsForAPIKey(apiKey)
	if err != nil {
		return ProvisionResult{}, err
	}
	clientID := strings.TrimSpace(values["TUYA_PROVISION_CLIENT_ID"])
	if clientID == "" {
		clientID = newProvisionClientID()
	}

	body, err := json.Marshal(map[string]string{"clientId": clientID})
	if err != nil {
		return ProvisionResult{}, err
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoints.OpenAPIBase+gatewayActivePath,
		bytes.NewReader(body),
	)
	if err != nil {
		return ProvisionResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return ProvisionResult{}, fmt.Errorf("activate Tuya virtual gateway: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ProvisionResult{}, fmt.Errorf("read Tuya activation response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ProvisionResult{}, fmt.Errorf("Tuya activation HTTP %d", resp.StatusCode)
	}
	var activation gatewayActivationResponse
	if err := json.Unmarshal(raw, &activation); err != nil {
		return ProvisionResult{}, fmt.Errorf("decode Tuya activation response: %w", err)
	}
	if !activation.Success {
		return ProvisionResult{}, fmt.Errorf(
			"Tuya activation rejected: %s: %s",
			activation.ErrorCode,
			activation.ErrorMsg,
		)
	}
	if activation.Result.ProductID == "" ||
		activation.Result.DeviceID == "" ||
		activation.Result.DeviceSecret == "" {
		return ProvisionResult{}, fmt.Errorf("Tuya activation response is missing gateway credentials")
	}

	values["TUYA_PROVISION_CLIENT_ID"] = clientID
	values["TUYA_PRODUCT_ID"] = activation.Result.ProductID
	values["TUYA_DEVICE_ID"] = activation.Result.DeviceID
	values["TUYA_DEVICE_SECRET"] = activation.Result.DeviceSecret
	values["TUYA_MQTT_BROKER"] = endpoints.MQTTBroker
	if err := writeCredentialEnv(credentialsFile, values); err != nil {
		return ProvisionResult{}, err
	}
	return ProvisionResult{
		ProductID: activation.Result.ProductID,
		DeviceID:  activation.Result.DeviceID,
		ShortURL:  activation.Result.ShortURL,
		Region:    endpoints.Key,
		Broker:    endpoints.MQTTBroker,
	}, nil
}

func EndpointsForAPIKey(apiKey string) (RegionEndpoints, error) {
	normalized := strings.TrimSpace(apiKey)
	if len(normalized) < 5 || !strings.EqualFold(normalized[:3], "sk-") {
		return RegionEndpoints{}, fmt.Errorf("unsupported TUYA_API_KEY format; expected sk-<region>...")
	}
	switch strings.ToUpper(normalized[3:5]) {
	case "AY":
		return RegionEndpoints{"cn", "https://openapi.tuyacn.com", "tls://m1.tuyacn.com:8883"}, nil
	case "AZ":
		return RegionEndpoints{"us", "https://openapi.tuyaus.com", "tls://m1.tuyaus.com:8883"}, nil
	case "EU":
		return RegionEndpoints{"eu", "https://openapi.tuyaeu.com", "tls://m1.tuyaeu.com:8883"}, nil
	case "IN":
		return RegionEndpoints{"in", "https://openapi.tuyain.com", "tls://m1.tuyain.com:8883"}, nil
	case "SG":
		return RegionEndpoints{"sg", "https://openapi-sg.iotbing.com", "tls://m1-sg.lifeaiot.com:8883"}, nil
	default:
		return RegionEndpoints{}, fmt.Errorf("TUYA_API_KEY region is not supported by the virtual gateway API")
	}
}

func readEnvFile(path string) (map[string]string, error) {
	values := make(map[string]string)
	file, err := os.Open(path)
	if err != nil {
		return values, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	return values, scanner.Err()
}

func writeCredentialEnv(path string, values map[string]string) error {
	keys := []string{
		"TUYA_PROVISION_CLIENT_ID",
		"TUYA_PRODUCT_ID",
		"TUYA_DEVICE_ID",
		"TUYA_DEVICE_SECRET",
		"TUYA_MQTT_BROKER",
	}
	var out strings.Builder
	out.WriteString("# Generated/updated by knx-tuya-gw. Keep this file private.\n")
	for _, key := range keys {
		value := strings.TrimSpace(values[key])
		if value != "" {
			fmt.Fprintf(&out, "%s=%s\n", key, value)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(out.String()), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func newProvisionClientID() string {
	var id [12]byte
	if _, err := rand.Read(id[:]); err != nil {
		return fmt.Sprintf("knx-tuya-gw-%d", time.Now().UnixNano())
	}
	return "knx-tuya-gw-" + hex.EncodeToString(id[:])
}

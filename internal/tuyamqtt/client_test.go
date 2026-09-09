package tuyamqtt

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"knx-tuya-gw/internal/config"
)

func TestMQTTCredentials(t *testing.T) {
	now := time.UnixMilli(1700000000123)
	clientID, username, password := MQTTCredentials("device123", "secret456", now)
	if clientID != "tuyalink_device123" {
		t.Fatalf("clientID = %q", clientID)
	}
	if username != "device123|signMethod=hmacSha256,timestamp=1700000000123,secureMode=1,accessType=1" {
		t.Fatalf("username = %q", username)
	}
	content := "deviceId=device123,timestamp=1700000000123,secureMode=1,accessType=1"
	mac := hmac.New(sha256.New, []byte("secret456"))
	_, _ = mac.Write([]byte(content))
	if want := hex.EncodeToString(mac.Sum(nil)); password != want {
		t.Fatalf("password = %q, want %q", password, want)
	}
}

func TestLoadCredentialsRejectsPlaceholders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tuya-mqtt.env")
	err := os.WriteFile(path, []byte(
		"TUYA_PRODUCT_ID=replace_with_product_id\n"+
			"TUYA_DEVICE_ID=replace_with_device_id\n"+
			"TUYA_DEVICE_SECRET=replace_with_device_secret\n",
	), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = loadCredentials(path)
	if err == nil || !strings.Contains(err.Error(), "fill TUYA_PRODUCT_ID") {
		t.Fatalf("error = %v", err)
	}
}

func TestNewGatewayDPClient(t *testing.T) {
	credentialsPath := filepath.Join(t.TempDir(), "tuya-mqtt.env")
	err := os.WriteFile(credentialsPath, []byte(
		"TUYA_PRODUCT_ID=gateway-product\n"+
			"TUYA_DEVICE_ID=gateway-device\n"+
			"TUYA_DEVICE_SECRET=gateway-secret\n",
	), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	client, err := New(config.TuyaMQTTConfig{
		GatewayNodeID:   "gateway",
		CredentialsFile: credentialsPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Report("wrong-node", "light_switch", true); err == nil ||
		!strings.Contains(err.Error(), `requires node_id "gateway"`) {
		t.Fatalf("unexpected node validation error: %v", err)
	}
	if err := client.Report("gateway", "light_switch", true); err == nil ||
		!strings.Contains(err.Error(), "not connected") {
		t.Fatalf("unexpected disconnected report error: %v", err)
	}

	_, err = New(config.TuyaMQTTConfig{
		ExpectedProductID: "another-product",
		CredentialsFile:   credentialsPath,
	})
	if err == nil || !strings.Contains(err.Error(), "Tuya product mismatch") {
		t.Fatalf("unexpected product mismatch error: %v", err)
	}
}

func TestAckRequested(t *testing.T) {
	tests := []struct {
		name string
		sys  interface{}
		want bool
	}{
		{name: "missing", sys: nil, want: false},
		{name: "disabled", sys: map[string]interface{}{"ack": float64(0)}, want: false},
		{name: "json number", sys: map[string]interface{}{"ack": float64(1)}, want: true},
		{name: "integer", sys: map[string]interface{}{"ack": 1}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ackRequested(test.sys); got != test.want {
				t.Fatalf("ackRequested(%v) = %v, want %v", test.sys, got, test.want)
			}
		})
	}
}

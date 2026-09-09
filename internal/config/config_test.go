package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadResolvesMappingRelativeToConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := []byte(`{
		"knx": {"gateway": {"host": "127.0.0.1", "port": 3671}},
		"mapping_file": "maps/devices.json"
	}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "maps/devices.json")
	if cfg.MappingFile != want {
		t.Fatalf("mapping_file = %q, want %q", cfg.MappingFile, want)
	}
}

func TestLoadTuyaAPIKeyAndDataPaths(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := []byte(`{
		"knx": {"gateway": {"host": "127.0.0.1", "port": 3671}},
		"tuya_mqtt": {
			"api_key": "sk-AY-test",
			"expected_product_id": "gateway-product",
			"credentials_file": "tuya-mqtt.env"
		}
	}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TuyaMQTT.APIKey != "sk-AY-test" {
		t.Fatalf("api_key = %q", cfg.TuyaMQTT.APIKey)
	}
	if cfg.TuyaMQTT.ExpectedProductID != "gateway-product" {
		t.Fatalf("expected_product_id = %q", cfg.TuyaMQTT.ExpectedProductID)
	}
	if cfg.TuyaMQTT.CredentialsFile != filepath.Join(dir, "tuya-mqtt.env") {
		t.Fatalf("credentials_file = %q", cfg.TuyaMQTT.CredentialsFile)
	}
}

func TestLoadGatewayDPConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := []byte(`{
		"knx": {"gateway": {"host": "127.0.0.1", "port": 3671}},
		"tuya_mqtt": {
			"gateway_node_id": "knx_gateway"
		}
	}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TuyaMQTT.GatewayNodeID != "knx_gateway" {
		t.Fatalf("unexpected Tuya gateway mode config: %+v", cfg.TuyaMQTT)
	}
	if cfg.TuyaMQTT.GatewayDPPool.PlatformTemplateFile != filepath.Join(dir, "../data/tuya-register-dp-template.xlsx") {
		t.Fatalf("unexpected platform template path: %q", cfg.TuyaMQTT.GatewayDPPool.PlatformTemplateFile)
	}
	if cfg.TuyaMQTT.GatewayDPPool.PlatformXLSXFile != filepath.Join(dir, "../data/tuya-gateway-dp-platform.xlsx") {
		t.Fatalf("unexpected platform XLSX path: %q", cfg.TuyaMQTT.GatewayDPPool.PlatformXLSXFile)
	}
}

func TestBootstrapRuntimeCreatesTemplatesWithoutOverwriting(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config", "config.json")

	created, err := BootstrapRuntime(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 3 {
		t.Fatalf("created %d files, want 3: %v", len(created), created)
	}
	for _, path := range []string{
		configPath,
		filepath.Join(root, "config", "knx-mapping.json"),
		filepath.Join(root, "config", "tuya-mqtt.env"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("generated file %s: %v", path, err)
		}
	}

	mappingPath := filepath.Join(root, "config", "knx-mapping.json")
	const customMapping = "[{\"custom\":true}]\n"
	if err := os.WriteFile(mappingPath, []byte(customMapping), 0o644); err != nil {
		t.Fatal(err)
	}
	created, err = BootstrapRuntime(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 0 {
		t.Fatalf("second bootstrap created files: %v", created)
	}
	data, err := os.ReadFile(mappingPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != customMapping {
		t.Fatalf("bootstrap overwrote existing mapping: %s", data)
	}
}

func TestBootstrapRuntimeRejectsMissingConfiguredCA(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config", "config.json")
	if _, err := BootstrapRuntime(configPath); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	tuya := cfg["tuya_mqtt"].(map[string]interface{})
	tuya["ca_file"] = "missing.pem"
	data, err = json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := BootstrapRuntime(configPath); err == nil ||
		!strings.Contains(err.Error(), "ca_file does not exist") {
		t.Fatalf("unexpected missing CA error: %v", err)
	}
}

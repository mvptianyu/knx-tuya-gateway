package tuyamqtt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEndpointsForAPIKey(t *testing.T) {
	tests := map[string]struct {
		region string
		broker string
	}{
		"sk-AY-example": {"cn", "tls://m1.tuyacn.com:8883"},
		"sk-AZ-example": {"us", "tls://m1.tuyaus.com:8883"},
		"sk-EU-example": {"eu", "tls://m1.tuyaeu.com:8883"},
		"sk-IN-example": {"in", "tls://m1.tuyain.com:8883"},
		"sk-SG-example": {"sg", "tls://m1-sg.lifeaiot.com:8883"},
	}
	for key, want := range tests {
		got, err := EndpointsForAPIKey(key)
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		if got.Key != want.region || got.MQTTBroker != want.broker {
			t.Fatalf("%s: got %+v, want region=%s broker=%s", key, got, want.region, want.broker)
		}
	}
}

func TestWriteCredentialEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tuya-mqtt.env")
	values := map[string]string{
		"TUYA_PROVISION_CLIENT_ID": "bridge-id",
		"TUYA_PRODUCT_ID":          "pid",
		"TUYA_DEVICE_ID":           "device",
		"TUYA_DEVICE_SECRET":       "secret",
		"TUYA_MQTT_BROKER":         "tls://m1.tuyacn.com:8883",
	}
	if err := writeCredentialEnv(path, values); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	loaded, err := readEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range values {
		if loaded[key] != want {
			t.Fatalf("%s = %q, want %q", key, loaded[key], want)
		}
	}
}

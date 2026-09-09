package main

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"knx-tuya-gw/internal/config"
	"knx-tuya-gw/internal/mapping"
)

func TestAcquireRuntimeLock(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	first, err := acquireRuntimeLock(configPath)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := acquireRuntimeLock(configPath); err == nil {
		first.Close()
		t.Fatal("expected a second instance to be rejected")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := acquireRuntimeLock(configPath)
	if err != nil {
		t.Fatalf("lock was not released: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestTuyaValueTransforms(t *testing.T) {
	temperature := &mapping.Item{TuyaDPType: "value", TuyaScale: 1}
	if got := normalizeTuyaValue(23.5, temperature); got != 235 {
		t.Fatalf("scaled temperature = %v, want 235", got)
	}
	if got := denormalizeKNXValue(temperature, float64(235)); got != 23.5 {
		t.Fatalf("unscaled temperature = %v, want 23.5", got)
	}

	mode := &mapping.Item{
		TuyaDPType: "enum",
		TuyaToKNX:  map[string]interface{}{"cold": float64(3)},
		KNXToTuya:  map[string]interface{}{"3": "cold"},
	}
	if got := normalizeTuyaValue(3, mode); got != "cold" {
		t.Fatalf("Tuya mode = %v, want cold", got)
	}
	if got := denormalizeKNXValue(mode, "cold"); got != float64(3) {
		t.Fatalf("KNX mode = %v, want 3", got)
	}

	scene := &mapping.Item{KNXWriteValue: float64(1)}
	if got := denormalizeKNXValue(scene, "scene"); got != float64(1) {
		t.Fatalf("scene write = %v, want 1", got)
	}
}

func TestValidateTuyaModeMapping(t *testing.T) {
	m, err := mapping.New([]mapping.Item{{
		GA: "1/2/12", StatusGA: "1/2/11", Name: "Bathroom",
		DPT: "DPT-1.001", TuyaDevID: "gateway",
		TuyaDPCode: "light_01_switch", TuyaDPType: "bool",
		VirtualDeviceID: "bathroom", Category: "light", Slot: 1, Capability: "switch",
	}})
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.TuyaMQTT.GatewayNodeID = "gateway"
	if err := validateTuyaModeMapping(cfg, m); err != nil {
		t.Fatal(err)
	}

	cfg.TuyaMQTT.GatewayNodeID = "another_gateway"
	if err := validateTuyaModeMapping(cfg, m); err == nil {
		t.Fatal("expected gateway node ID mismatch")
	}
}

func TestPollAfterWriteDelayDefault(t *testing.T) {
	cfg := config.Default()
	if cfg.Poll.AfterWriteDelayMs != 300 {
		t.Fatalf("after-write delay = %d, want 300", cfg.Poll.AfterWriteDelayMs)
	}
	if !cfg.Poll.ReportCommandOnWriteAck {
		t.Fatal("report command on write ACK should be enabled by default")
	}
}

func TestFetchRuntimeBundle(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(
				`{"schema_version":1,"version":"v2","mappings":[]}`,
			)),
			Header: make(http.Header),
		}, nil
	})}

	data, err := fetchRuntimeBundleWithClient(
		context.Background(),
		client,
		"https://config.example/runtime.json",
		1024,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("empty runtime bundle")
	}
	if _, err := fetchRuntimeBundleWithClient(
		context.Background(),
		client,
		"https://config.example/runtime.json",
		8,
	); err == nil {
		t.Fatal("expected maximum size validation error")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

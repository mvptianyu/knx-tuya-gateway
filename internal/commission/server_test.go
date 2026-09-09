package commission

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/vapourismo/knx-go/knx/dpt"

	"knx-tuya-gw/internal/config"
	"knx-tuya-gw/internal/knxclient"
)

func TestConfirmWritesCommissionReview(t *testing.T) {
	directory := t.TempDir()
	basePath := filepath.Join(directory, "base.json")
	reviewPath := filepath.Join(directory, "review.csv")
	if err := os.WriteFile(
		basePath,
		[]byte(`{"schema_version":1,"version":"test","mappings":[]}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	server := New(
		config.CommissioningConfig{ReviewFile: reviewPath, MaxEvents: 10},
		basePath,
		map[string]int{"light": 2},
	)

	postJSON(t, server.handleSession, map[string]interface{}{
		"room": "公卫", "name": "公卫灯", "category": "light", "capability": "switch",
	})
	server.Capture(knxclient.Event{
		Source: "1.1.10", GA: "1/2/12", Command: "write", RawHex: "01",
		Data: dpt.DPT_1001(true).Pack(),
	})
	postJSON(t, server.handleConfirm, map[string]interface{}{
		"control_event_id": 1,
		"status_event_id":  1,
		"dpt":              "DPT-1.001",
	})

	raw, err := os.ReadFile(reviewPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("light_01_switch")) ||
		!bytes.Contains(raw, []byte("1/2/12")) {
		t.Fatalf("unexpected review CSV: %s", raw)
	}
}

func postJSON(
	t *testing.T,
	handler http.HandlerFunc,
	body map[string]interface{},
) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw))
	recorder := httptest.NewRecorder()
	handler(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("HTTP %d: %s", recorder.Code, recorder.Body.String())
	}
	return recorder
}

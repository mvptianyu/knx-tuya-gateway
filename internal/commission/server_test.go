package commission

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
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
		"dpt": "DPT-1.001",
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

func TestSuggestEventsPrefersFirstWriteAndLatestResponse(t *testing.T) {
	events := []CapturedEvent{
		capturedEvent(1, "1/1/1", "read", knxclient.DPT1_001, false),
		capturedEvent(2, "1/1/2", "write", knxclient.DPT1_001, true),
		capturedEvent(3, "1/1/3", "write", knxclient.DPT1_001, false),
		capturedEvent(4, "1/1/4", "response", knxclient.DPT1_001, true),
		capturedEvent(5, "1/1/5", "response", knxclient.DPT1_001, false),
	}

	control, status, err := suggestEvents(events, knxclient.DPT1_001, false)
	if err != nil {
		t.Fatal(err)
	}
	if control.ID != 2 || status.ID != 5 {
		t.Fatalf("control=%d status=%d, want 2 and 5", control.ID, status.ID)
	}
}

func TestSuggestEventsReadOnlyUsesLatestMatchingEvent(t *testing.T) {
	events := []CapturedEvent{
		capturedEvent(1, "2/1/1", "response", knxclient.DPT9_001, 23.1),
		capturedEvent(2, "2/1/2", "response", knxclient.DPT1_001, true),
		capturedEvent(3, "2/1/1", "response", knxclient.DPT9_001, 23.4),
	}

	control, status, err := suggestEvents(events, knxclient.DPT9_001, true)
	if err != nil {
		t.Fatal(err)
	}
	if control.ID != 3 || status.ID != 3 {
		t.Fatalf("control=%d status=%d, want both 3", control.ID, status.ID)
	}
}

func TestConfirmBuildsEnumMappingsFromObservedState(t *testing.T) {
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
		map[string]int{"air_conditioner": 2},
	)

	postJSON(t, server.handleSession, map[string]interface{}{
		"room": "客厅", "name": "客厅空调",
		"category": "air_conditioner", "capability": "mode",
	})
	server.Capture(knxclient.Event{
		Source: "1.1.10", GA: "3/1/1", Command: "write", RawHex: "01",
		Data: dpt.DPT_20105(1).Pack(),
	})
	server.Capture(knxclient.Event{
		Source: "1.1.20", GA: "3/1/2", Command: "response", RawHex: "01",
		Data: dpt.DPT_20105(1).Pack(),
	})
	postJSON(t, server.handleConfirm, map[string]interface{}{
		"observed_value": "cool",
	})

	records, err := readCommissionReview(reviewPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 ||
		records[0]["tuya_to_knx_json"] != `{"cool":1}` ||
		records[0]["knx_to_tuya_json"] != `{"1":"cool"}` {
		t.Fatalf("unexpected enum record: %+v", records)
	}
}

type fakeBus struct {
	readGA     string
	writeGA    string
	writeDPT   string
	writeValue interface{}
	err        error
}

func (bus *fakeBus) ReadValue(ga string) error {
	bus.readGA = ga
	return bus.err
}

func (bus *fakeBus) WriteValue(ga, dptType string, value interface{}) error {
	bus.writeGA = ga
	bus.writeDPT = dptType
	bus.writeValue = value
	return bus.err
}

func TestProbeReadCapturesReadAndResponseWithoutSession(t *testing.T) {
	server := New(
		config.CommissioningConfig{MaxEvents: 10},
		"",
		nil,
	)
	bus := &fakeBus{}
	server.SetBus(bus)

	postJSON(t, server.handleProbe, map[string]interface{}{
		"action": "read",
		"ga":     "1/2/11",
		"dpt":    "DPT-1.001",
		"value":  nil,
	})
	if bus.readGA != "1/2/11" {
		t.Fatalf("read GA=%q, want 1/2/11", bus.readGA)
	}
	server.Capture(knxclient.Event{
		Source: "1.1.10", GA: "1/2/11", Command: "read",
	})
	server.Capture(knxclient.Event{
		Source: "1.1.20", GA: "1/2/11", Command: "response", RawHex: "01",
		Data: dpt.DPT_1001(true).Pack(),
	})

	probe := server.currentProbe()
	if probe == nil || !probe.HasResponse || probe.DirectMatches != 2 ||
		probe.NearbyEvents != 2 {
		t.Fatalf("unexpected probe: %+v", probe)
	}
}

func TestProbeWritePassesTypedValueToKNX(t *testing.T) {
	server := New(
		config.CommissioningConfig{MaxEvents: 10},
		"",
		nil,
	)
	bus := &fakeBus{}
	server.SetBus(bus)

	postJSON(t, server.handleProbe, map[string]interface{}{
		"action": "write",
		"ga":     "1/2/12",
		"dpt":    "DPT-1.001",
		"value":  true,
	})
	if bus.writeGA != "1/2/12" || bus.writeDPT != "DPT-1.001" ||
		bus.writeValue != true {
		t.Fatalf(
			"unexpected write: ga=%q dpt=%q value=%#v",
			bus.writeGA,
			bus.writeDPT,
			bus.writeValue,
		)
	}
}

func TestConfirmResponseUsesJSONFieldNamesExpectedByPage(t *testing.T) {
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
		map[string]int{"light": 1},
	)
	postJSON(t, server.handleSession, map[string]interface{}{
		"room": "公卫", "name": "公卫灯", "category": "light", "capability": "switch",
	})
	server.Capture(knxclient.Event{
		GA: "1/2/12", Command: "write", Data: dpt.DPT_1001(true).Pack(),
	})
	response := postJSON(t, server.handleConfirm, map[string]interface{}{})
	var payload map[string]interface{}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	record, ok := payload["record"].(map[string]interface{})
	if !ok || record["tuya_dp_code"] != "light_01_switch" {
		t.Fatalf("unexpected response: %s", response.Body.String())
	}
}

func capturedEvent(
	id int,
	ga string,
	command string,
	dptType string,
	value interface{},
) CapturedEvent {
	return CapturedEvent{
		ID: id, GA: ga, Command: command,
		Candidates: []knxclient.DecodedCandidate{{DPT: dptType, Value: value}},
	}
}

func readCommissionReview(path string) ([]map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	headers, err := reader.Read()
	if err != nil {
		return nil, err
	}
	records := make([]map[string]string, 0)
	for {
		values, readErr := reader.Read()
		if readErr == io.EOF {
			return records, nil
		}
		if readErr != nil {
			return nil, readErr
		}
		record := make(map[string]string, len(headers))
		for index := range headers {
			record[headers[index]] = values[index]
		}
		records = append(records, record)
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

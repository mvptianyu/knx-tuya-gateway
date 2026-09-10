package knxdebug

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"knx-tuya-gw/internal/knxclient"
)

type fakeBus struct {
	readGA     string
	writeGA    string
	writeDPT   string
	writeValue interface{}
}

func (b *fakeBus) ReadValue(ga string) error {
	b.readGA = ga
	return nil
}

func (b *fakeBus) WriteValue(ga, dpt string, value interface{}) error {
	b.writeGA, b.writeDPT, b.writeValue = ga, dpt, value
	return nil
}

type fakeReporter struct {
	mu      sync.Mutex
	reports map[string]interface{}
}

func (r *fakeReporter) Report(_ string, code string, value interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reports[code] = value
	return nil
}

func TestReadRequestCapturesTargetEvent(t *testing.T) {
	bus := &fakeBus{}
	reporter := &fakeReporter{reports: make(map[string]interface{})}
	controller := New(bus, reporter, "gateway")
	controller.readWait = 10 * time.Millisecond
	if err := controller.Handle(RequestDP, `{"id":"1","action":"read","ga":"1/2/11","dpt":"DPT-1.001"}`); err != nil {
		t.Fatal(err)
	}
	if err := controller.Handle(TriggerDP, true); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for bus.readGA == "" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if bus.readGA != "1/2/11" {
		t.Fatalf("read GA = %q", bus.readGA)
	}
	controller.Capture(knxclient.Event{
		GA: "1/2/11", Command: "response", RawHex: "01", Data: []byte{1},
	})

	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		reporter.mu.Lock()
		status := reporter.reports[StatusDP]
		raw, done := reporter.reports[ResultDP].(string)
		reporter.mu.Unlock()
		if status == "success" && done {
			var output result
			if err := json.Unmarshal([]byte(raw), &output); err != nil {
				t.Fatal(err)
			}
			if output.Value != true || output.Command != "response" {
				t.Fatalf("unexpected result: %+v", output)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for debug result")
}

func TestWriteRequestValidation(t *testing.T) {
	controller := New(&fakeBus{}, &fakeReporter{reports: make(map[string]interface{})}, "gateway")
	if err := controller.Handle(RequestDP, `{"action":"write","ga":"1/2/12"}`); err == nil {
		t.Fatal("expected missing DPT/value error")
	}
}

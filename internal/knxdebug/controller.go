package knxdebug

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"knx-tuya-gw/internal/knxclient"
	"knx-tuya-gw/internal/logger"
)

const (
	RequestDP = "knx_debug_request"
	TriggerDP = "knx_debug_trigger"
	StatusDP  = "knx_debug_status"
	ResultDP  = "knx_debug_result"
)

type KNXBus interface {
	ReadValue(ga string) error
	WriteValue(ga string, dptType string, value interface{}) error
}

type Reporter interface {
	Report(nodeID, dpCode string, value interface{}) error
}

type Request struct {
	ID     string          `json:"id"`
	Action string          `json:"action"`
	GA     string          `json:"ga"`
	DPT    string          `json:"dpt,omitempty"`
	Value  json.RawMessage `json:"value,omitempty"`
}

type result struct {
	ID         string      `json:"id,omitempty"`
	OK         bool        `json:"ok"`
	Action     string      `json:"action,omitempty"`
	GA         string      `json:"ga,omitempty"`
	EventCount int         `json:"event_count,omitempty"`
	Command    string      `json:"command,omitempty"`
	Raw        string      `json:"raw,omitempty"`
	DPT        string      `json:"dpt,omitempty"`
	Value      interface{} `json:"value,omitempty"`
	Candidates string      `json:"candidates,omitempty"`
	Message    string      `json:"message,omitempty"`
}

type Controller struct {
	bus       KNXBus
	reporter  Reporter
	nodeID    string
	readWait  time.Duration
	writeWait time.Duration

	mu      sync.Mutex
	request *Request
	active  *activeProbe
}

type activeProbe struct {
	request Request
	count   int
	event   *knxclient.Event
}

func New(bus KNXBus, reporter Reporter, nodeID string) *Controller {
	return &Controller{
		bus: bus, reporter: reporter, nodeID: nodeID,
		readWait: 4 * time.Second, writeWait: 2 * time.Second,
	}
}

func (c *Controller) Handles(dpCode string) bool {
	return dpCode == RequestDP || dpCode == TriggerDP
}

func (c *Controller) Handle(dpCode string, value interface{}) error {
	switch dpCode {
	case RequestDP:
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s expects a JSON string", RequestDP)
		}
		request, err := parseRequest(text)
		if err != nil {
			return err
		}
		c.mu.Lock()
		c.request = &request
		c.mu.Unlock()
		return nil
	case TriggerDP:
		c.mu.Lock()
		if c.active != nil {
			c.mu.Unlock()
			return fmt.Errorf("KNX debug request is already running")
		}
		if c.request == nil {
			c.mu.Unlock()
			return fmt.Errorf("set %s before triggering", RequestDP)
		}
		probe := &activeProbe{request: *c.request}
		c.active = probe
		c.mu.Unlock()
		go c.execute(probe)
		return nil
	default:
		return fmt.Errorf("unsupported KNX debug DP %q", dpCode)
	}
}

func (c *Controller) Capture(event knxclient.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active == nil || event.GA != c.active.request.GA {
		return
	}
	c.active.count++
	copy := event
	copy.Data = append([]byte(nil), event.Data...)
	// Prefer a GroupValueResponse for reads; otherwise retain the latest target event.
	if c.active.event == nil || event.Command == "response" || c.active.event.Command != "response" {
		c.active.event = &copy
	}
}

func (c *Controller) execute(probe *activeProbe) {
	request := probe.request
	c.report(StatusDP, "running")
	logger.Infof("KNX remote debug started: id=%s action=%s ga=%s dpt=%s",
		request.ID, request.Action, request.GA, request.DPT)

	var err error
	if request.Action == "read" {
		err = c.bus.ReadValue(request.GA)
	} else {
		var value interface{}
		if unmarshalErr := json.Unmarshal(request.Value, &value); unmarshalErr != nil {
			err = fmt.Errorf("invalid write value: %w", unmarshalErr)
		} else {
			err = c.bus.WriteValue(request.GA, request.DPT, value)
		}
	}
	if err != nil {
		c.finish(probe, result{
			ID: request.ID, OK: false, Action: request.Action, GA: request.GA,
			Message: err.Error(),
		})
		return
	}

	wait := c.writeWait
	if request.Action == "read" {
		wait = c.readWait
	}
	time.Sleep(wait)

	c.mu.Lock()
	count := probe.count
	event := probe.event
	c.mu.Unlock()
	output := result{
		ID: request.ID, OK: true, Action: request.Action, GA: request.GA,
		EventCount: count,
	}
	if event == nil {
		output.Message = "command sent; no matching bus response observed"
	} else {
		output.Command = event.Command
		output.Raw = event.RawHex
		if request.DPT != "" {
			output.DPT = request.DPT
			decoded, decodeErr := knxclient.DecodeValue(request.DPT, event.Data)
			if decodeErr == nil {
				output.Value = decoded
			} else {
				output.Message = decodeErr.Error()
			}
		} else {
			output.Candidates = compactCandidates(knxclient.DecodeCandidates(event.Data))
		}
	}
	c.finish(probe, output)
}

func (c *Controller) finish(probe *activeProbe, output result) {
	c.mu.Lock()
	if c.active == probe {
		c.active = nil
	}
	c.mu.Unlock()

	raw := compactResult(output)
	c.report(ResultDP, raw)
	if output.OK {
		c.report(StatusDP, "success")
	} else {
		c.report(StatusDP, "error")
	}
	logger.Infof("KNX remote debug finished: %s", raw)
}

func (c *Controller) report(code string, value interface{}) {
	if err := c.reporter.Report(c.nodeID, code, value); err != nil {
		logger.Warnf("KNX remote debug report %s failed: %v", code, err)
	}
}

func parseRequest(text string) (Request, error) {
	var request Request
	if err := json.Unmarshal([]byte(text), &request); err != nil {
		return Request{}, fmt.Errorf("invalid KNX debug request JSON: %w", err)
	}
	request.ID = strings.TrimSpace(request.ID)
	request.Action = strings.ToLower(strings.TrimSpace(request.Action))
	request.GA = strings.TrimSpace(request.GA)
	request.DPT = strings.TrimSpace(request.DPT)
	if request.Action != "read" && request.Action != "write" {
		return Request{}, fmt.Errorf("KNX debug action must be read or write")
	}
	if request.GA == "" {
		return Request{}, fmt.Errorf("KNX debug group address is required")
	}
	if request.Action == "write" {
		if request.DPT == "" {
			return Request{}, fmt.Errorf("KNX debug DPT is required for write")
		}
		if len(request.Value) == 0 || string(request.Value) == "null" {
			return Request{}, fmt.Errorf("KNX debug value is required for write")
		}
	}
	return request, nil
}

func compactCandidates(candidates []knxclient.DecodedCandidate) string {
	values := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		values = append(values, fmt.Sprintf("%s=%v", candidate.DPT, candidate.Value))
	}
	return strings.Join(values, ",")
}

func compactResult(output result) string {
	raw, _ := json.Marshal(output)
	if len(raw) <= 240 {
		return string(raw)
	}
	output.Candidates = ""
	output.Message = shorten(output.Message, 48)
	raw, _ = json.Marshal(output)
	if len(raw) <= 240 {
		return string(raw)
	}
	output.Raw = ""
	raw, _ = json.Marshal(output)
	return string(raw)
}

func shorten(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

package commission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"knx-tuya-gw/internal/config"
	"knx-tuya-gw/internal/knxclient"
	"knx-tuya-gw/internal/knxreview"
	"knx-tuya-gw/internal/logger"
)

type Session struct {
	Active     bool   `json:"active"`
	Room       string `json:"room"`
	Name       string `json:"name"`
	Category   string `json:"category"`
	Capability string `json:"capability"`
	StartedAt  string `json:"started_at,omitempty"`
}

type CapturedEvent struct {
	ID         int                          `json:"id"`
	Time       string                       `json:"time"`
	Source     string                       `json:"source"`
	Origin     string                       `json:"origin"`
	GA         string                       `json:"ga"`
	Command    string                       `json:"command"`
	RawHex     string                       `json:"raw_hex"`
	Candidates []knxclient.DecodedCandidate `json:"candidates"`
}

type KNXBus interface {
	ReadValue(ga string) error
	WriteValue(ga string, dptType string, value interface{}) error
}

type ProbeState struct {
	Action          string `json:"action"`
	GA              string `json:"ga"`
	DPT             string `json:"dpt,omitempty"`
	Value           string `json:"value,omitempty"`
	StartedAt       string `json:"started_at"`
	ActiveUntil     string `json:"active_until"`
	BaselineEventID int    `json:"baseline_event_id"`
	DirectMatches   int    `json:"direct_matches"`
	NearbyEvents    int    `json:"nearby_events"`
	HasResponse     bool   `json:"has_response"`
	HasWrite        bool   `json:"has_write"`
	Error           string `json:"error,omitempty"`
}

type Server struct {
	cfg        config.CommissioningConfig
	basePath   string
	capacities map[string]int

	mu      sync.RWMutex
	bus     KNXBus
	session Session
	probe   *ProbeState
	events  []CapturedEvent
	nextID  int
}

type probeRequest struct {
	Action string          `json:"action"`
	GA     string          `json:"ga"`
	DPT    string          `json:"dpt"`
	Value  json.RawMessage `json:"value"`
}

type confirmRequest struct {
	ControlEventID int    `json:"control_event_id"`
	StatusEventID  int    `json:"status_event_id"`
	DPT            string `json:"dpt"`
	ObservedValue  string `json:"observed_value"`
	KNXWriteValue  string `json:"knx_write_value"`
	TuyaToKNXJSON  string `json:"tuya_to_knx_json"`
	KNXToTuyaJSON  string `json:"knx_to_tuya_json"`
	Notes          string `json:"notes"`
}

func New(
	cfg config.CommissioningConfig,
	basePath string,
	capacities map[string]int,
) *Server {
	return &Server{cfg: cfg, basePath: basePath, capacities: capacities, nextID: 1}
}

func (s *Server) SetBus(bus KNXBus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bus = bus
}

func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/session", s.handleSession)
	mux.HandleFunc("/api/probe", s.handleProbe)
	mux.HandleFunc("/api/confirm", s.handleConfirm)
	mux.HandleFunc("/api/review.csv", s.handleReviewDownload)

	server := &http.Server{
		Addr:              s.cfg.Listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	listener, err := net.Listen("tcp", s.cfg.Listen)
	if err != nil {
		return err
	}
	logger.Infof("KNX commissioning page ready: http://%s", s.cfg.Listen)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) Capture(event knxclient.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	probeActive := s.probe != nil && time.Now().Before(parseTime(s.probe.ActiveUntil))
	if !s.session.Active && !probeActive {
		return
	}
	captured := CapturedEvent{
		ID: s.nextID, Time: time.Now().Format("15:04:05.000"),
		Source: event.Source, Origin: "bus", GA: event.GA,
		Command: event.Command, RawHex: event.RawHex,
		Candidates: knxclient.DecodeCandidates(event.Data),
	}
	s.nextID++
	s.events = append(s.events, captured)
	if probeActive && captured.ID > s.probe.BaselineEventID {
		s.probe.NearbyEvents++
		if event.GA == s.probe.GA {
			s.probe.DirectMatches++
			s.probe.HasResponse = s.probe.HasResponse || event.Command == "response"
			s.probe.HasWrite = s.probe.HasWrite || event.Command == "write"
		}
	}
	if len(s.events) > s.cfg.MaxEvents {
		s.events = append([]CapturedEvent(nil), s.events[len(s.events)-s.cfg.MaxEvents:]...)
	}
}

func (s *Server) handleIndex(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/" {
		http.NotFound(writer, request)
		return
	}
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = writer.Write([]byte(indexHTML))
}

func (s *Server) handleState(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.RLock()
	response := struct {
		Session    Session         `json:"session"`
		Probe      *ProbeState     `json:"probe,omitempty"`
		Events     []CapturedEvent `json:"events"`
		ReviewFile string          `json:"review_file"`
	}{
		Session: s.session, Events: append([]CapturedEvent(nil), s.events...),
		Probe:      cloneProbe(s.probe),
		ReviewFile: s.cfg.ReviewFile,
	}
	s.mu.RUnlock()
	writeJSON(writer, response)
}

func (s *Server) handleSession(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var session Session
	if err := decodeJSON(request, &session); err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	session.Room = strings.TrimSpace(session.Room)
	session.Name = strings.TrimSpace(session.Name)
	session.Category = strings.TrimSpace(session.Category)
	session.Capability = strings.TrimSpace(session.Capability)
	if session.Room == "" || session.Name == "" {
		http.Error(writer, "room and name are required", http.StatusBadRequest)
		return
	}
	if _, _, _, err := capabilityDefaults(session.Category, session.Capability); err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	session.Active = true
	session.StartedAt = time.Now().Format(time.RFC3339)
	s.mu.Lock()
	s.session = session
	s.events = nil
	s.mu.Unlock()
	writeJSON(writer, map[string]interface{}{"ok": true, "session": session})
}

func (s *Server) handleProbe(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var input probeRequest
	if err := decodeJSON(request, &input); err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	input.Action = strings.ToLower(strings.TrimSpace(input.Action))
	input.GA = strings.TrimSpace(input.GA)
	input.DPT = strings.TrimSpace(input.DPT)
	if input.Action != "read" && input.Action != "write" {
		http.Error(writer, "action must be read or write", http.StatusBadRequest)
		return
	}
	if input.GA == "" {
		http.Error(writer, "group address is required", http.StatusBadRequest)
		return
	}

	var value interface{}
	valueText := ""
	if input.Action == "write" {
		if input.DPT == "" {
			http.Error(writer, "DPT is required for write", http.StatusBadRequest)
			return
		}
		if len(input.Value) == 0 || string(input.Value) == "null" {
			http.Error(writer, "value is required for write", http.StatusBadRequest)
			return
		}
		if err := json.Unmarshal(input.Value, &value); err != nil {
			http.Error(writer, "value must be valid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		valueText = string(input.Value)
	}

	now := time.Now()
	probe := &ProbeState{
		Action: input.Action, GA: input.GA, DPT: input.DPT, Value: valueText,
		StartedAt:   now.Format(time.RFC3339),
		ActiveUntil: now.Add(10 * time.Second).Format(time.RFC3339),
	}
	s.mu.Lock()
	probe.BaselineEventID = s.nextID - 1
	s.probe = probe
	bus := s.bus
	s.mu.Unlock()
	if bus == nil {
		s.finishProbeError("KNX bus is not attached")
		http.Error(writer, "KNX bus is not attached", http.StatusServiceUnavailable)
		return
	}

	var err error
	if input.Action == "read" {
		err = bus.ReadValue(input.GA)
	} else {
		err = bus.WriteValue(input.GA, input.DPT, value)
	}
	if err != nil {
		s.finishProbeError(err.Error())
		http.Error(writer, err.Error(), http.StatusBadGateway)
		return
	}
	s.appendProbeRequest(input, value)
	logger.Infof(
		"KNX commissioning probe sent: action=%s ga=%s dpt=%s value=%s",
		input.Action,
		input.GA,
		input.DPT,
		valueText,
	)
	writeJSON(writer, map[string]interface{}{"ok": true, "probe": s.currentProbe()})
}

func (s *Server) handleConfirm(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var input confirmRequest
	if err := decodeJSON(request, &input); err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.RLock()
	session := s.session
	events := append([]CapturedEvent(nil), s.events...)
	s.mu.RUnlock()
	if !session.Active {
		http.Error(writer, "start a capture session first", http.StatusBadRequest)
		return
	}
	defaultDPT, dpType, _, err := capabilityDefaults(session.Category, session.Capability)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	if input.DPT == "" {
		input.DPT = defaultDPT
	}
	control, controlOK := findEvent(events, input.ControlEventID)
	status, statusOK := findEvent(events, input.StatusEventID)
	if !controlOK || !statusOK {
		control, status, err = suggestEvents(
			events,
			input.DPT,
			isReadOnlyCapability(session.Category, session.Capability),
		)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if _, ok := candidateValue(status, input.DPT); !ok {
		http.Error(writer, "selected DPT cannot decode the status event", http.StatusBadRequest)
		return
	}
	if _, ok := candidateValue(control, input.DPT); !ok {
		http.Error(writer, "selected DPT cannot decode the control event", http.StatusBadRequest)
		return
	}
	if dpType == "enum" && strings.TrimSpace(input.ObservedValue) != "" {
		controlValue, _ := candidateValue(control, input.DPT)
		statusValue, _ := candidateValue(status, input.DPT)
		input.TuyaToKNXJSON = singleMappingJSON(input.ObservedValue, controlValue)
		input.KNXToTuyaJSON = singleMappingJSON(fmt.Sprint(statusValue), input.ObservedValue)
	}
	if err := validateJSONObject(input.TuyaToKNXJSON); err != nil {
		http.Error(writer, "invalid Tuya-to-KNX JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateJSONObject(input.KNXToTuyaJSON); err != nil {
		http.Error(writer, "invalid KNX-to-Tuya JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if input.KNXWriteValue != "" && !json.Valid([]byte(input.KNXWriteValue)) {
		http.Error(writer, "KNX write value must be valid JSON", http.StatusBadRequest)
		return
	}

	capacity := s.capacities[session.Category]
	identity, err := knxreview.AllocateIdentity(
		s.basePath, s.cfg.ReviewFile, session.Category, session.Room, session.Name,
		session.Capability, capacity,
	)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	if session.Category == "scene" && input.KNXWriteValue == "" {
		if value, ok := candidateValue(control, input.DPT); ok {
			raw, _ := json.Marshal(value)
			input.KNXWriteValue = string(raw)
		}
	}
	notes := fmt.Sprintf(
		"现场采集确认；control=%s %s raw=%s；status=%s %s raw=%s",
		control.Command, control.GA, control.RawHex, status.Command, status.GA, status.RawHex,
	)
	if strings.TrimSpace(input.Notes) != "" {
		notes += "；" + strings.TrimSpace(input.Notes)
	}
	record := knxreview.Record{
		Result: "passed", SourceFile: "live-knx", SourceSheet: "commissioning",
		SourceRow: strconv.FormatInt(time.Now().UnixMilli(), 10),
		Name:      session.Name, Category: session.Category, Room: session.Room,
		Capability: session.Capability, GA: control.GA, StatusGA: status.GA,
		DPT: input.DPT, TuyaDPType: dpType, Slot: identity.Slot,
		VirtualDeviceID: identity.VirtualDeviceID, TuyaDPCode: identity.TuyaDPCode,
		KNXWriteValue: input.KNXWriteValue, TuyaToKNXJSON: input.TuyaToKNXJSON,
		KNXToTuyaJSON: input.KNXToTuyaJSON, Notes: notes,
	}
	if err := knxreview.UpsertReviewRecord(s.cfg.ReviewFile, record); err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	s.mu.Lock()
	s.session.Active = false
	s.mu.Unlock()
	writeJSON(writer, map[string]interface{}{
		"ok": true, "record": record, "review_file": s.cfg.ReviewFile,
	})
}

func (s *Server) handleReviewDownload(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writer.Header().Set("Content-Disposition", `attachment; filename="knx-commission-review.csv"`)
	writer.Header().Set("Content-Type", "text/csv; charset=utf-8")
	http.ServeFile(writer, request, s.cfg.ReviewFile)
}

func capabilityDefaults(category, capability string) (string, string, string, error) {
	key := category + "/" + capability
	switch key {
	case "light/switch", "air_conditioner/switch", "fresh_air/switch":
		return knxclient.DPT1_001, "bool", "开关", nil
	case "scene/trigger":
		return knxclient.DPT17_001, "bool", "场景号", nil
	case "air_conditioner/mode", "fresh_air/mode":
		return knxclient.DPT20_105, "enum", "模式枚举", nil
	case "air_conditioner/fan_speed", "fresh_air/fan_speed":
		return knxclient.DPT5_005, "enum", "风速枚举", nil
	case "air_conditioner/temp_set", "air_conditioner/temp_current",
		"climate_sensor/temperature":
		return knxclient.DPT9_001, "value", "温度", nil
	case "climate_sensor/humidity":
		return knxclient.DPT9_007, "value", "湿度", nil
	default:
		return "", "", "", fmt.Errorf("unsupported category/capability %s", key)
	}
}

func findEvent(events []CapturedEvent, id int) (CapturedEvent, bool) {
	for _, event := range events {
		if event.ID == id {
			return event, true
		}
	}
	return CapturedEvent{}, false
}

func suggestEvents(
	events []CapturedEvent,
	dptType string,
	readOnly bool,
) (CapturedEvent, CapturedEvent, error) {
	matching := make([]CapturedEvent, 0, len(events))
	for _, event := range events {
		if _, ok := candidateValue(event, dptType); ok {
			matching = append(matching, event)
		}
	}
	if len(matching) == 0 {
		return CapturedEvent{}, CapturedEvent{}, fmt.Errorf(
			"no captured telegram can be decoded as %s",
			dptType,
		)
	}
	if readOnly {
		event := matching[len(matching)-1]
		return event, event, nil
	}

	control := matching[0]
	for _, event := range matching {
		if event.Command == "write" {
			control = event
			break
		}
	}
	status := control
	for index := len(matching) - 1; index >= 0; index-- {
		event := matching[index]
		if event.Command == "response" {
			status = event
			return control, status, nil
		}
		if event.GA != control.GA {
			status = event
			return control, status, nil
		}
	}
	status = matching[len(matching)-1]
	return control, status, nil
}

func isReadOnlyCapability(category, capability string) bool {
	return capability == "temp_current" ||
		category == "climate_sensor"
}

func singleMappingJSON(key string, value interface{}) string {
	raw, _ := json.Marshal(map[string]interface{}{strings.TrimSpace(key): value})
	return string(raw)
}

func candidateValue(event CapturedEvent, dptType string) (interface{}, bool) {
	for _, candidate := range event.Candidates {
		if candidate.DPT == dptType {
			return candidate.Value, true
		}
	}
	return nil, false
}

func validateJSONObject(value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	var object map[string]interface{}
	return json.Unmarshal([]byte(value), &object)
}

func decodeJSON(request *http.Request, target interface{}) error {
	defer request.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(request.Body, 64*1024))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writeJSON(writer http.ResponseWriter, value interface{}) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(writer).Encode(value)
}

func (s *Server) finishProbeError(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.probe != nil {
		s.probe.Error = message
	}
}

func (s *Server) appendProbeRequest(input probeRequest, value interface{}) {
	event := CapturedEvent{
		Time: time.Now().Format("15:04:05.000"), Source: "现场识别工具",
		Origin: "probe", GA: input.GA, Command: input.Action, RawHex: "主动发送",
	}
	if input.Action == "write" {
		event.Candidates = []knxclient.DecodedCandidate{{DPT: input.DPT, Value: value}}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	event.ID = s.nextID
	s.nextID++
	s.events = append(s.events, event)
	if len(s.events) > s.cfg.MaxEvents {
		s.events = append([]CapturedEvent(nil), s.events[len(s.events)-s.cfg.MaxEvents:]...)
	}
}

func (s *Server) currentProbe() *ProbeState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneProbe(s.probe)
}

func cloneProbe(probe *ProbeState) *ProbeState {
	if probe == nil {
		return nil
	}
	copy := *probe
	return &copy
}

func parseTime(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339, value)
	return parsed
}

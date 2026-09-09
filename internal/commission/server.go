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
	GA         string                       `json:"ga"`
	Command    string                       `json:"command"`
	RawHex     string                       `json:"raw_hex"`
	Candidates []knxclient.DecodedCandidate `json:"candidates"`
}

type Server struct {
	cfg        config.CommissioningConfig
	basePath   string
	capacities map[string]int

	mu      sync.RWMutex
	session Session
	events  []CapturedEvent
	nextID  int
}

type confirmRequest struct {
	ControlEventID int    `json:"control_event_id"`
	StatusEventID  int    `json:"status_event_id"`
	DPT            string `json:"dpt"`
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

func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/session", s.handleSession)
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
	if !s.session.Active {
		return
	}
	captured := CapturedEvent{
		ID: s.nextID, Time: time.Now().Format("15:04:05.000"),
		Source: event.Source, GA: event.GA, Command: event.Command, RawHex: event.RawHex,
		Candidates: knxclient.DecodeCandidates(event.Data),
	}
	s.nextID++
	s.events = append(s.events, captured)
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
		Events     []CapturedEvent `json:"events"`
		ReviewFile string          `json:"review_file"`
	}{
		Session: s.session, Events: append([]CapturedEvent(nil), s.events...),
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
	control, controlOK := findEvent(s.events, input.ControlEventID)
	status, statusOK := findEvent(s.events, input.StatusEventID)
	s.mu.RUnlock()
	if !session.Active {
		http.Error(writer, "start a capture session first", http.StatusBadRequest)
		return
	}
	if !controlOK {
		http.Error(writer, "select a control event", http.StatusBadRequest)
		return
	}
	if input.StatusEventID == 0 {
		status, statusOK = control, true
	}
	if !statusOK {
		http.Error(writer, "select a status event", http.StatusBadRequest)
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
	if _, ok := candidateValue(status, input.DPT); !ok {
		http.Error(writer, "selected DPT cannot decode the status event", http.StatusBadRequest)
		return
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

package knxreview

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"knx-tuya-gw/internal/config"
	"knx-tuya-gw/internal/gatewaydps"
	"knx-tuya-gw/internal/mapping"
)

var reviewHeader = []string{
	"result", "source_file", "source_sheet", "source_row", "name", "category", "room",
	"capability", "ga", "status_ga", "dpt", "tuya_dp_type", "slot", "virtual_device_id",
	"tuya_dp_code", "knx_write_value", "tuya_to_knx_json", "knx_to_tuya_json", "notes",
}

type Record struct {
	Result          string `json:"result"`
	SourceFile      string `json:"source_file"`
	SourceSheet     string `json:"source_sheet"`
	SourceRow       string `json:"source_row"`
	Name            string `json:"name"`
	Category        string `json:"category"`
	Room            string `json:"room"`
	Capability      string `json:"capability"`
	GA              string `json:"ga"`
	StatusGA        string `json:"status_ga"`
	DPT             string `json:"dpt"`
	TuyaDPType      string `json:"tuya_dp_type"`
	Slot            int    `json:"slot"`
	VirtualDeviceID string `json:"virtual_device_id"`
	TuyaDPCode      string `json:"tuya_dp_code"`
	KNXWriteValue   string `json:"knx_write_value,omitempty"`
	TuyaToKNXJSON   string `json:"tuya_to_knx_json,omitempty"`
	KNXToTuyaJSON   string `json:"knx_to_tuya_json,omitempty"`
	Notes           string `json:"notes,omitempty"`
}

type Identity struct {
	Slot            int
	VirtualDeviceID string
	TuyaDPCode      string
}

type slotAllocator struct {
	next       map[string]int
	max        map[string]int
	identities map[string]string
}

func ImportXLSX(input, reviewPath, basePath string, capacities map[string]int) (map[string]int, error) {
	sheets, err := readWorkbook(input)
	if err != nil {
		return nil, err
	}
	allocator, err := newSlotAllocator(basePath, capacities)
	if err != nil {
		return nil, err
	}

	records := make([]Record, 0)
	for _, sheet := range sheets {
		switch strings.TrimSpace(sheet.Name) {
		case "开关设备":
			records = append(records, importSwitches(input, sheet, allocator)...)
		case "场景":
			records = append(records, importScenes(input, sheet, allocator)...)
		case "空调设备":
			records = append(records, importAirConditioners(input, sheet, allocator)...)
		}
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("no supported rows found in %s", input)
	}
	if err := writeReviewCSV(reviewPath, records); err != nil {
		return nil, err
	}
	counts := make(map[string]int)
	for _, record := range records {
		counts[record.Result]++
	}
	return counts, nil
}

func Finalize(reviewPath, basePath, outputPath, version string, cfg config.GatewayDPPoolConfig) (int, error) {
	records, err := readReviewCSV(reviewPath)
	if err != nil {
		return 0, err
	}
	base, err := mapping.Load(basePath)
	if err != nil {
		return 0, err
	}
	items := append([]mapping.Item(nil), base.Items...)
	indexByDP := make(map[string]int, len(items))
	for i, item := range items {
		indexByDP[item.TuyaDevID+"\x00"+item.TuyaDPCode] = i
	}

	passed := 0
	for line, record := range records {
		switch strings.ToLower(strings.TrimSpace(record.Result)) {
		case "", "pending", "needs_manual", "failed", "skip":
			continue
		case "passed":
		default:
			return 0, fmt.Errorf("review line %d has unsupported result %q", line+2, record.Result)
		}
		item, err := record.mappingItem()
		if err != nil {
			return 0, fmt.Errorf("review line %d: %w", line+2, err)
		}
		key := item.TuyaDevID + "\x00" + item.TuyaDPCode
		if index, exists := indexByDP[key]; exists {
			items[index] = item
		} else {
			indexByDP[key] = len(items)
			items = append(items, item)
		}
		passed++
	}
	if passed == 0 {
		return 0, fmt.Errorf("review contains no passed rows")
	}
	if _, err := mapping.New(items); err != nil {
		return 0, fmt.Errorf("final mapping validation: %w", err)
	}
	if _, err := gatewaydps.Build(cfg, items); err != nil {
		return 0, fmt.Errorf("gateway DP pool validation: %w", err)
	}
	if strings.TrimSpace(version) == "" {
		version = time.Now().Format("2006.01.02-150405")
	}
	bundle := mapping.Bundle{
		SchemaVersion: mapping.BundleSchemaVersion,
		Version:       version,
		UpdatedAt:     time.Now().Format(time.RFC3339),
		Mappings:      items,
	}
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return 0, err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return 0, err
	}
	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		return 0, err
	}
	return len(items), nil
}

func ValidateBundle(path string, cfg config.GatewayDPPoolConfig) (int, error) {
	loaded, err := mapping.Load(path)
	if err != nil {
		return 0, err
	}
	if _, err := gatewaydps.Build(cfg, loaded.Items); err != nil {
		return 0, err
	}
	return len(loaded.Items), nil
}

func newSlotAllocator(basePath string, capacities map[string]int) (*slotAllocator, error) {
	allocator := &slotAllocator{
		next:       map[string]int{"light": 1, "scene": 1, "air_conditioner": 1},
		max:        capacities,
		identities: make(map[string]string),
	}
	if basePath == "" {
		return allocator, nil
	}
	base, err := mapping.Load(basePath)
	if err != nil {
		return nil, err
	}
	for _, item := range base.Items {
		if item.Slot >= allocator.next[item.Category] {
			allocator.next[item.Category] = item.Slot + 1
		}
		if item.TuyaDPCode != "" && item.VirtualDeviceID != "" {
			allocator.identities[item.TuyaDPCode] = item.VirtualDeviceID
		}
	}
	return allocator, nil
}

func (a *slotAllocator) allocate(category string) (int, error) {
	slot := a.next[category]
	if slot == 0 {
		slot = 1
	}
	if maximum := a.max[category]; maximum > 0 && slot > maximum {
		return 0, fmt.Errorf("%s candidates exceed configured capacity %d", category, maximum)
	}
	a.next[category] = slot + 1
	return slot, nil
}

func (a *slotAllocator) identity(dpCode, fallback string) string {
	if existing := a.identities[dpCode]; existing != "" {
		return existing
	}
	return fallback
}

func importSwitches(input string, sheet sheetData, allocator *slotAllocator) []Record {
	records := make([]Record, 0)
	for _, row := range sheet.Rows {
		ga := row["GA地址"]
		name := strings.TrimSuffix(strings.TrimSuffix(row["设备名称"], "-主"), "-辅")
		name = strings.TrimSuffix(strings.TrimSuffix(name, "-开"), "-关")
		if ga == "" || name == "" || strings.HasSuffix(row["设备名称"], "-辅") ||
			strings.HasSuffix(row["设备名称"], "-开") {
			continue
		}
		control := row["控制方式"]
		result := "pending"
		statusGA := ga
		notes := "必须现场确认控制值以及 status_ga 是否可读；若无独立反馈地址，不要标 passed"
		slot := 0
		if strings.Contains(control, "分开") || strings.Contains(control, "双GA") ||
			strings.Contains(row["备注"], "同时") {
			result = "needs_manual"
			notes = "当前单 GA 映射模型不能表达分开开关地址或多 GA 联动，需先改 ETS 或扩展 write_actions"
		}
		if ga == "1/2/12" {
			name = "公卫灯"
			slot = 1
			statusGA = "1/2/11"
			result = "passed"
			notes = "已确认基线：1/2/12 写、公卫灯状态 1/2/11 读"
		} else {
			var err error
			slot, err = allocator.allocate("light")
			if err != nil {
				records = append(records, errorRecord(input, sheet.Name, row["_row"], name, err))
				continue
			}
		}
		dpCode := fmt.Sprintf("light_%02d_switch", slot)
		records = append(records, Record{
			Result: result, SourceFile: input, SourceSheet: sheet.Name, SourceRow: row["_row"],
			Name: name, Category: "light", Room: row["位置/区域"], Capability: "switch",
			GA: ga, StatusGA: statusGA, DPT: normalizeDPT(row["DPT类型"], "DPT-1.001"),
			TuyaDPType: "bool", Slot: slot,
			VirtualDeviceID: allocator.identity(dpCode, fmt.Sprintf("light_%02d", slot)),
			TuyaDPCode:      dpCode, Notes: notes,
		})
	}
	return records
}

func importScenes(input string, sheet sheetData, allocator *slotAllocator) []Record {
	records := make([]Record, 0, len(sheet.Rows))
	gaUse := make(map[string]int)
	for _, row := range sheet.Rows {
		if row["触发GA地址"] != "" {
			gaUse[row["触发GA地址"]]++
		}
	}
	for _, row := range sheet.Rows {
		ga, name := row["触发GA地址"], row["场景名称"]
		if ga == "" || name == "" {
			continue
		}
		slot, err := allocator.allocate("scene")
		if err != nil {
			records = append(records, errorRecord(input, sheet.Name, row["_row"], name, err))
			continue
		}
		result := "pending"
		notes := "场景只验证 Tuya->KNX 触发；确认写值和 GA 后再标 passed"
		if gaUse[ga] > 1 {
			result = "needs_manual"
			notes = "多个场景共用同一 GA、依赖不同 val；当前 status_ga 索引无法安全区分，需逐项确认"
		}
		records = append(records, Record{
			Result: result, SourceFile: input, SourceSheet: sheet.Name, SourceRow: row["_row"],
			Name: name, Category: "scene", Room: row["区域"], Capability: "trigger",
			GA: ga, StatusGA: ga, DPT: normalizeDPT(row["DPT类型"], "DPT-17.001"),
			TuyaDPType: "bool", Slot: slot, VirtualDeviceID: fmt.Sprintf("scene_%02d", slot),
			TuyaDPCode:    fmt.Sprintf("scene_%02d_trigger", slot),
			KNXWriteValue: scalarJSON(row["val值"]), Notes: notes,
		})
	}
	return records
}

func importAirConditioners(input string, sheet sheetData, allocator *slotAllocator) []Record {
	records := make([]Record, 0, len(sheet.Rows))
	slotByName := make(map[string]int)
	for _, row := range sheet.Rows {
		name, capability := row["空调名称"], acCapability(row["功能"])
		if name == "" || capability == "" || row["GA地址"] == "" || name == "空调场景" {
			continue
		}
		slot := slotByName[name]
		if slot == 0 {
			var err error
			slot, err = allocator.allocate("air_conditioner")
			if err != nil {
				records = append(records, errorRecord(input, sheet.Name, row["_row"], name, err))
				continue
			}
			slotByName[name] = slot
		}
		dpt, dpType := acTypes(capability)
		result := "needs_manual"
		notes := "空调写能力缺少独立状态 GA，且模式/风速/温度编码需现场抓包确认"
		if capability == "temp_current" {
			result = "pending"
			notes = "只读当前温度候选；确认 DPT-9.001 实际值和倍率后再标 passed"
		}
		records = append(records, Record{
			Result: result, SourceFile: input, SourceSheet: sheet.Name, SourceRow: row["_row"],
			Name: name, Category: "air_conditioner", Room: row["位置"], Capability: capability,
			GA: row["GA地址"], StatusGA: row["GA地址"], DPT: dpt, TuyaDPType: dpType,
			Slot: slot, VirtualDeviceID: fmt.Sprintf("air_conditioner_%02d", slot),
			TuyaDPCode: fmt.Sprintf("ac_%02d_%s", slot, capability), Notes: notes,
		})
	}
	return records
}

func errorRecord(input, sheet, row, name string, err error) Record {
	return Record{
		Result: "needs_manual", SourceFile: input, SourceSheet: sheet, SourceRow: row,
		Name: name, Notes: err.Error(),
	}
}

func normalizeDPT(value, fallback string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "DPT", "DPT-"))
	value = strings.ReplaceAll(value, "DPT-_", "DPT-")
	if value == "" || strings.Contains(value, "Switch") {
		return fallback
	}
	return value
}

func scalarJSON(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "-" {
		return ""
	}
	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return value
	}
	return strconv.Quote(value)
}

func acCapability(value string) string {
	switch strings.TrimSpace(value) {
	case "开关":
		return "switch"
	case "模式":
		return "mode"
	case "风速":
		return "fan_speed"
	case "目标温度":
		return "temp_set"
	case "当前温度":
		return "temp_current"
	default:
		return ""
	}
}

func acTypes(capability string) (string, string) {
	switch capability {
	case "switch":
		return "DPT-1.001", "bool"
	case "mode":
		return "DPT-20.105", "enum"
	case "fan_speed":
		return "DPT-5.005", "enum"
	default:
		return "DPT-9.001", "value"
	}
}

func writeReviewCSV(path string, records []Record) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := csv.NewWriter(file)
	defer writer.Flush()
	if err := writer.Write(reviewHeader); err != nil {
		return err
	}
	for _, record := range records {
		if err := writer.Write(record.values()); err != nil {
			return err
		}
	}
	return writer.Error()
}

// UpsertReviewRecord stores one confirmed commissioning result in the standard review CSV.
func UpsertReviewRecord(path string, record Record) error {
	records := make([]Record, 0)
	if _, err := os.Stat(path); err == nil {
		var readErr error
		records, readErr = readReviewCSV(path)
		if readErr != nil {
			return readErr
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	replaced := false
	for index, existing := range records {
		if existing.Category == record.Category && existing.Slot == record.Slot &&
			existing.Capability == record.Capability {
			var err error
			record.TuyaToKNXJSON, err = mergeJSONObjectStrings(
				existing.TuyaToKNXJSON,
				record.TuyaToKNXJSON,
			)
			if err != nil {
				return fmt.Errorf("merge tuya_to_knx_json: %w", err)
			}
			record.KNXToTuyaJSON, err = mergeJSONObjectStrings(
				existing.KNXToTuyaJSON,
				record.KNXToTuyaJSON,
			)
			if err != nil {
				return fmt.Errorf("merge knx_to_tuya_json: %w", err)
			}
			if record.KNXWriteValue == "" {
				record.KNXWriteValue = existing.KNXWriteValue
			}
			records[index] = record
			replaced = true
			break
		}
	}
	if !replaced {
		records = append(records, record)
	}
	return writeReviewCSV(path, records)
}

func mergeJSONObjectStrings(existing, incoming string) (string, error) {
	if strings.TrimSpace(incoming) == "" {
		return existing, nil
	}
	merged := make(map[string]interface{})
	if strings.TrimSpace(existing) != "" {
		if err := json.Unmarshal([]byte(existing), &merged); err != nil {
			return "", err
		}
	}
	var values map[string]interface{}
	if err := json.Unmarshal([]byte(incoming), &values); err != nil {
		return "", err
	}
	for key, value := range values {
		merged[key] = value
	}
	raw, err := json.Marshal(merged)
	return string(raw), err
}

// AllocateIdentity reuses a logical device slot by category/name/room or picks the first free slot.
func AllocateIdentity(
	basePath,
	reviewPath,
	category,
	room,
	name,
	capability string,
	capacity int,
) (Identity, error) {
	type allocated struct {
		Category        string
		Room            string
		Name            string
		Capability      string
		Slot            int
		VirtualDeviceID string
	}
	entries := make([]allocated, 0)
	if basePath != "" {
		base, err := mapping.Load(basePath)
		if err != nil {
			return Identity{}, err
		}
		for _, item := range base.Items {
			entries = append(entries, allocated{
				Category: item.Category, Room: item.Room, Name: item.Name,
				Capability: item.Capability, Slot: item.Slot,
				VirtualDeviceID: item.VirtualDeviceID,
			})
		}
	}
	if reviewPath != "" {
		if _, err := os.Stat(reviewPath); err == nil {
			records, err := readReviewCSV(reviewPath)
			if err != nil {
				return Identity{}, err
			}
			for _, record := range records {
				entries = append(entries, allocated{
					Category: record.Category, Room: record.Room, Name: record.Name,
					Capability: record.Capability, Slot: record.Slot,
					VirtualDeviceID: record.VirtualDeviceID,
				})
			}
		} else if !os.IsNotExist(err) {
			return Identity{}, err
		}
	}

	prefix, ok := categoryPrefix(category)
	if !ok {
		return Identity{}, fmt.Errorf("unsupported commissioning category %q", category)
	}
	used := make(map[int]bool)
	for _, entry := range entries {
		if entry.Category != category || entry.Slot <= 0 {
			continue
		}
		used[entry.Slot] = true
		if entry.Name == name && normalizedRoom(entry.Room) == normalizedRoom(room) {
			virtualID := entry.VirtualDeviceID
			if virtualID == "" {
				virtualID = fmt.Sprintf("%s_%02d", prefix, entry.Slot)
			}
			return Identity{
				Slot: entry.Slot, VirtualDeviceID: virtualID,
				TuyaDPCode: fmt.Sprintf("%s_%02d_%s", prefix, entry.Slot, capability),
			}, nil
		}
	}
	for slot := 1; slot <= capacity; slot++ {
		if used[slot] {
			continue
		}
		return Identity{
			Slot: slot, VirtualDeviceID: fmt.Sprintf("%s_%02d", prefix, slot),
			TuyaDPCode: fmt.Sprintf("%s_%02d_%s", prefix, slot, capability),
		}, nil
	}
	return Identity{}, fmt.Errorf("%s capacity %d is exhausted", category, capacity)
}

func categoryPrefix(category string) (string, bool) {
	switch category {
	case "light":
		return "light", true
	case "air_conditioner":
		return "ac", true
	case "scene":
		return "scene", true
	case "fresh_air":
		return "fresh_air", true
	case "climate_sensor":
		return "sensor", true
	default:
		return "", false
	}
}

func normalizedRoom(room string) string {
	room = strings.TrimSpace(room)
	if room == "" {
		return "全屋"
	}
	return room
}

func readReviewCSV(path string) ([]Record, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("review CSV is empty")
	}
	index := make(map[string]int, len(rows[0]))
	for i, name := range rows[0] {
		index[name] = i
	}
	for _, required := range reviewHeader {
		if _, ok := index[required]; !ok {
			return nil, fmt.Errorf("review CSV missing column %q", required)
		}
	}
	records := make([]Record, 0, len(rows)-1)
	for _, row := range rows[1:] {
		value := func(name string) string {
			position := index[name]
			if position >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[position])
		}
		slot, err := strconv.Atoi(value("slot"))
		if err != nil && value("slot") != "" {
			return nil, fmt.Errorf("invalid slot %q", value("slot"))
		}
		records = append(records, Record{
			Result: value("result"), SourceFile: value("source_file"),
			SourceSheet: value("source_sheet"), SourceRow: value("source_row"),
			Name: value("name"), Category: value("category"), Room: value("room"),
			Capability: value("capability"), GA: value("ga"), StatusGA: value("status_ga"),
			DPT: value("dpt"), TuyaDPType: value("tuya_dp_type"), Slot: slot,
			VirtualDeviceID: value("virtual_device_id"), TuyaDPCode: value("tuya_dp_code"),
			KNXWriteValue: value("knx_write_value"), TuyaToKNXJSON: value("tuya_to_knx_json"),
			KNXToTuyaJSON: value("knx_to_tuya_json"), Notes: value("notes"),
		})
	}
	return records, nil
}

func (record Record) values() []string {
	return []string{
		record.Result, record.SourceFile, record.SourceSheet, record.SourceRow, record.Name,
		record.Category, record.Room, record.Capability, record.GA, record.StatusGA, record.DPT,
		record.TuyaDPType, strconv.Itoa(record.Slot), record.VirtualDeviceID, record.TuyaDPCode,
		record.KNXWriteValue, record.TuyaToKNXJSON, record.KNXToTuyaJSON, record.Notes,
	}
}

func (record Record) mappingItem() (mapping.Item, error) {
	if record.Name == "" || record.GA == "" || record.StatusGA == "" || record.DPT == "" ||
		record.TuyaDPType == "" || record.Category == "" || record.Capability == "" ||
		record.Slot <= 0 || record.VirtualDeviceID == "" || record.TuyaDPCode == "" {
		return mapping.Item{}, fmt.Errorf("passed row has missing mapping fields")
	}
	var writeValue interface{}
	if record.KNXWriteValue != "" {
		if err := json.Unmarshal([]byte(record.KNXWriteValue), &writeValue); err != nil {
			return mapping.Item{}, fmt.Errorf("invalid knx_write_value JSON: %w", err)
		}
	}
	tuyaToKNX, err := decodeMap(record.TuyaToKNXJSON)
	if err != nil {
		return mapping.Item{}, fmt.Errorf("invalid tuya_to_knx_json: %w", err)
	}
	knxToTuya, err := decodeMap(record.KNXToTuyaJSON)
	if err != nil {
		return mapping.Item{}, fmt.Errorf("invalid knx_to_tuya_json: %w", err)
	}
	return mapping.Item{
		GA: record.GA, StatusGA: record.StatusGA, Name: record.Name, DPT: record.DPT,
		TuyaDevID: "gateway", TuyaDPCode: record.TuyaDPCode, TuyaDPType: record.TuyaDPType,
		VirtualDeviceID: record.VirtualDeviceID, Category: record.Category, Room: record.Room,
		Slot: record.Slot, Capability: record.Capability, KNXWriteValue: writeValue,
		TuyaToKNX: tuyaToKNX, KNXToTuya: knxToTuya,
	}, nil
}

func decodeMap(value string) (map[string]interface{}, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		return nil, err
	}
	return result, nil
}

func SortedCounts(counts map[string]int) []string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, fmt.Sprintf("%s=%d", key, counts[key]))
	}
	return result
}

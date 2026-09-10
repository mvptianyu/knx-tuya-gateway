// Package gatewaydps validates and documents the gateway-owned DP slot pool.
package gatewaydps

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"knx-tuya-gw/internal/config"
	"knx-tuya-gw/internal/mapping"
)

type capability struct {
	CodeSuffix     string
	Name           string
	Type           string
	Access         string
	SceneCondition bool
	SceneAction    bool
	EnumValues     []string
	ValueSpec      *ValueSpec
}

type category struct {
	ID           string
	Name         string
	CodePrefix   string
	Capabilities []capability
}

var categories = []category{
	{
		ID: "light", Name: "灯光", CodePrefix: "light",
		Capabilities: []capability{
			{CodeSuffix: "switch", Name: "开关", Type: "bool", Access: "rw", SceneCondition: true, SceneAction: true},
		},
	},
	{
		ID: "air_conditioner", Name: "空调", CodePrefix: "ac",
		Capabilities: []capability{
			{CodeSuffix: "switch", Name: "开关", Type: "bool", Access: "rw", SceneCondition: true, SceneAction: true},
			{
				CodeSuffix: "mode", Name: "模式", Type: "enum", Access: "rw",
				SceneCondition: true, SceneAction: true,
				EnumValues: []string{"auto", "cool", "heat", "fan", "dry"},
			},
			{
				CodeSuffix: "fan_speed", Name: "风速", Type: "enum", Access: "rw",
				SceneCondition: true, SceneAction: true,
				EnumValues: []string{"auto", "low", "middle", "high"},
			},
			{
				CodeSuffix: "temp_set", Name: "设定温度", Type: "value", Access: "rw",
				SceneCondition: true, SceneAction: true,
				ValueSpec: &ValueSpec{Min: 160, Max: 300, Step: 5, Scale: 1, Unit: "℃"},
			},
			{
				CodeSuffix: "temp_current", Name: "当前温度", Type: "value", Access: "ro",
				SceneCondition: true,
				ValueSpec:      &ValueSpec{Min: -400, Max: 1250, Step: 1, Scale: 1, Unit: "℃"},
			},
		},
	},
	{
		ID: "scene", Name: "场景", CodePrefix: "scene",
		Capabilities: []capability{
			{CodeSuffix: "trigger", Name: "触发", Type: "bool", Access: "rw", SceneAction: true},
		},
	},
	{
		ID: "fresh_air", Name: "新风", CodePrefix: "fresh_air",
		Capabilities: []capability{
			{CodeSuffix: "switch", Name: "开关", Type: "bool", Access: "rw", SceneCondition: true, SceneAction: true},
			{
				CodeSuffix: "mode", Name: "模式", Type: "enum", Access: "rw",
				SceneCondition: true, SceneAction: true,
				EnumValues: []string{"auto", "manual"},
			},
			{
				CodeSuffix: "fan_speed", Name: "风速", Type: "enum", Access: "rw",
				SceneCondition: true, SceneAction: true,
				EnumValues: []string{"low", "middle", "high"},
			},
		},
	},
	{
		ID: "climate_sensor", Name: "温湿度", CodePrefix: "sensor",
		Capabilities: []capability{
			{
				CodeSuffix: "temperature", Name: "温度", Type: "value", Access: "ro",
				SceneCondition: true,
				ValueSpec:      &ValueSpec{Min: -400, Max: 1250, Step: 1, Scale: 1, Unit: "℃"},
			},
			{
				CodeSuffix: "humidity", Name: "湿度", Type: "value", Access: "ro",
				SceneCondition: true,
				ValueSpec:      &ValueSpec{Min: 0, Max: 1000, Step: 1, Scale: 1, Unit: "%"},
			},
		},
	},
}

const maxBusinessFunctions = 76

var systemFunctions = []Function{
	{
		DPID: 177, Code: "knx_debug_request", Name: "KNX调试请求",
		Category: "system", Capability: "debug_request", Type: "string", Access: "rw",
	},
	{
		DPID: 178, Code: "knx_debug_trigger", Name: "KNX调试触发",
		Category: "system", Capability: "debug_trigger", Type: "bool", Access: "rw",
	},
	{
		DPID: 179, Code: "knx_debug_status", Name: "KNX调试状态",
		Category: "system", Capability: "debug_status", Type: "enum", Access: "ro",
		EnumValues: []string{"idle", "running", "success", "error"},
	},
	{
		DPID: 180, Code: "knx_debug_result", Name: "KNX调试结果",
		Category: "system", Capability: "debug_result", Type: "string", Access: "ro",
	},
}

type ValueSpec struct {
	Min   int    `json:"min"`
	Max   int    `json:"max"`
	Step  int    `json:"step"`
	Scale int    `json:"scale"`
	Unit  string `json:"unit"`
}

type Function struct {
	DPID           int        `json:"dp_id"`
	Code           string     `json:"code"`
	Name           string     `json:"name"`
	Category       string     `json:"category"`
	Slot           int        `json:"slot"`
	Capability     string     `json:"capability"`
	Type           string     `json:"type"`
	Access         string     `json:"access"`
	SceneCondition bool       `json:"scene_condition"`
	SceneAction    bool       `json:"scene_action"`
	EnumValues     []string   `json:"enum_values,omitempty"`
	ValueSpec      *ValueSpec `json:"value_spec,omitempty"`
	Mapped         bool       `json:"mapped"`
	MappingName    string     `json:"mapping_name,omitempty"`
	KNXWriteGA     string     `json:"knx_write_ga,omitempty"`
	KNXStatusGA    string     `json:"knx_status_ga,omitempty"`
}

type ModelPlan struct {
	Mode               string         `json:"mode"`
	MaxFunctions       int            `json:"max_functions"`
	AllocatedFunctions int            `json:"allocated_functions"`
	ReservedFunctions  int            `json:"reserved_functions"`
	Capacities         map[string]int `json:"capacities"`
	Functions          []Function     `json:"functions"`
}

type Artifacts struct {
	ModelPlan ModelPlan
}

// Build creates the full cloud product plan and validates logical device consistency.
func Build(cfg config.GatewayDPPoolConfig, items []mapping.Item) (Artifacts, error) {
	byCategory := make(map[string]category, len(categories))
	for _, category := range categories {
		byCategory[category.ID] = category
	}
	for configured := range cfg.Capacities {
		if _, ok := byCategory[configured]; !ok {
			return Artifacts{}, fmt.Errorf("unknown gateway DP category %q", configured)
		}
	}

	plan := ModelPlan{
		Mode:         config.TuyaModeGatewayDPs,
		MaxFunctions: cfg.MaxFunctions,
		Capacities:   copyCapacities(cfg.Capacities),
		Functions:    make([]Function, 0),
	}
	for _, category := range categories {
		capacity := cfg.Capacities[category.ID]
		for slot := 1; slot <= capacity; slot++ {
			for _, capability := range category.Capabilities {
				plan.Functions = append(plan.Functions, Function{
					DPID:           101 + len(plan.Functions),
					Code:           dpCode(category, slot, capability.CodeSuffix),
					Name:           fmt.Sprintf("%s%02d%s", category.Name, slot, capability.Name),
					Category:       category.ID,
					Slot:           slot,
					Capability:     capability.CodeSuffix,
					Type:           capability.Type,
					Access:         capability.Access,
					SceneCondition: capability.SceneCondition,
					SceneAction:    capability.SceneAction,
					EnumValues:     capability.EnumValues,
					ValueSpec:      capability.ValueSpec,
				})
			}
		}
	}
	if len(plan.Functions) > maxBusinessFunctions {
		return Artifacts{}, fmt.Errorf(
			"gateway business DP pool allocates %d functions, exceeding fixed diagnostic DP boundary %d",
			len(plan.Functions),
			maxBusinessFunctions,
		)
	}
	plan.Functions = append(plan.Functions, systemFunctions...)
	plan.AllocatedFunctions = len(plan.Functions)
	plan.ReservedFunctions = cfg.MaxFunctions - plan.AllocatedFunctions
	if plan.ReservedFunctions < 0 {
		return Artifacts{}, fmt.Errorf(
			"gateway DP pool allocates %d functions, exceeding max_functions=%d",
			plan.AllocatedFunctions,
			cfg.MaxFunctions,
		)
	}
	if len(plan.Functions) > 0 && plan.Functions[len(plan.Functions)-1].DPID > 499 {
		return Artifacts{}, fmt.Errorf(
			"gateway DP pool ends at DP ID %d, exceeding template maximum 499",
			plan.Functions[len(plan.Functions)-1].DPID,
		)
	}

	functionByCode := make(map[string]int, len(plan.Functions))
	for i := range plan.Functions {
		functionByCode[plan.Functions[i].Code] = i
	}

	deviceByKey := make(map[string]mapping.Item)
	for i, item := range items {
		if item.Category == "" || item.Slot <= 0 || item.Capability == "" || item.VirtualDeviceID == "" {
			if cfg.Strict {
				return Artifacts{}, fmt.Errorf(
					"mapping item %d (%q) requires virtual_device_id, category, slot and capability in strict gateway DP mode",
					i,
					item.Name,
				)
			}
			continue
		}
		category, ok := byCategory[item.Category]
		if !ok {
			return Artifacts{}, fmt.Errorf("mapping item %d (%q) has unknown category %q", i, item.Name, item.Category)
		}
		capacity := cfg.Capacities[item.Category]
		if item.Slot > capacity {
			return Artifacts{}, fmt.Errorf(
				"mapping item %d (%q) uses %s slot %d, configured capacity is %d",
				i,
				item.Name,
				item.Category,
				item.Slot,
				capacity,
			)
		}
		expected, ok := findCapability(category, item.Capability)
		if !ok {
			return Artifacts{}, fmt.Errorf(
				"mapping item %d (%q) has unsupported %s capability %q",
				i,
				item.Name,
				item.Category,
				item.Capability,
			)
		}
		expectedCode := dpCode(category, item.Slot, expected.CodeSuffix)
		if item.TuyaDPCode != expectedCode {
			return Artifacts{}, fmt.Errorf(
				"mapping item %d (%q) DP code must be %q for %s slot %d capability %s, got %q",
				i,
				item.Name,
				expectedCode,
				item.Category,
				item.Slot,
				item.Capability,
				item.TuyaDPCode,
			)
		}
		if item.TuyaDPType != expected.Type {
			return Artifacts{}, fmt.Errorf(
				"mapping item %d (%q) DP type must be %q for %s, got %q",
				i,
				item.Name,
				expected.Type,
				expectedCode,
				item.TuyaDPType,
			)
		}
		functionIndex := functionByCode[item.TuyaDPCode]
		plan.Functions[functionIndex].Mapped = true
		plan.Functions[functionIndex].MappingName = item.Name
		plan.Functions[functionIndex].KNXWriteGA = item.GA
		plan.Functions[functionIndex].KNXStatusGA = item.StatusGA

		room := strings.TrimSpace(item.Room)
		if room == "" {
			room = "全屋"
		}
		key := fmt.Sprintf("%s\x00%d", item.Category, item.Slot)
		device, exists := deviceByKey[key]
		if !exists {
			item.Room = room
			deviceByKey[key] = item
		} else if device.VirtualDeviceID != item.VirtualDeviceID ||
			device.Name != item.Name || device.Room != room {
			return Artifacts{}, fmt.Errorf(
				"%s slot %d must use one virtual_device_id, name and room across all capabilities",
				item.Category,
				item.Slot,
			)
		}
	}

	return Artifacts{ModelPlan: plan}, nil
}

func WriteArtifacts(cfg config.GatewayDPPoolConfig, artifacts Artifacts) error {
	if err := writeJSON(cfg.ModelPlanFile, artifacts.ModelPlan); err != nil {
		return fmt.Errorf("write gateway DP model plan: %w", err)
	}
	if err := writePlatformXLSX(
		cfg.PlatformTemplateFile,
		cfg.PlatformXLSXFile,
		artifacts.ModelPlan,
	); err != nil {
		return fmt.Errorf("write gateway DP platform XLSX: %w", err)
	}
	return nil
}

func findCapability(category category, name string) (capability, bool) {
	for _, capability := range category.Capabilities {
		if capability.CodeSuffix == name {
			return capability, true
		}
	}
	return capability{}, false
}

func dpCode(category category, slot int, capability string) string {
	return fmt.Sprintf("%s_%02d_%s", category.CodePrefix, slot, capability)
}

func copyCapacities(input map[string]int) map[string]int {
	output := make(map[string]int, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func writeJSON(path string, value interface{}) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return writeFileAtomically(path, raw)
}

func writeFileAtomically(path string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func Categories() []string {
	result := make([]string, 0, len(categories))
	for _, category := range categories {
		result = append(result, category.ID)
	}
	return result
}

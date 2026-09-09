package mapping

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewAndLookup(t *testing.T) {
	items := []Item{
		{GA: "1/1/1", StatusGA: "1/1/10", Name: "客厅主灯", DPT: "DPT-1.001", TuyaDevID: "sub_dev_001", TuyaDPCode: "switch_led", TuyaDPType: "bool"},
		{GA: "2/1/1", StatusGA: "2/1/10", Name: "客厅窗帘", DPT: "DPT-5.001", TuyaDevID: "sub_dev_003", TuyaDPCode: "shutter_percent", TuyaDPType: "percent"},
	}
	m, err := New(items)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// 按涂鸦网关节点+DP 查 KNX 控制地址
	it := m.ByTuya("sub_dev_001", "switch_led")
	if it == nil || it.GA != "1/1/1" {
		t.Fatalf("ByTuya wrong: %+v", it)
	}
	// 按 KNX 状态反馈地址查涂鸦
	statusItems := m.ByStatusGA("2/1/10")
	if len(statusItems) != 1 || statusItems[0].TuyaDevID != "sub_dev_003" {
		t.Fatalf("ByStatusGA wrong: %+v", statusItems)
	}
	// 全部状态地址
	gas := m.AllStatusGAs()
	if len(gas) != 2 {
		t.Fatalf("AllStatusGAs len = %d, want 2", len(gas))
	}
	// 未映射
	if len(m.ByStatusGA("9/9/9")) != 0 {
		t.Error("unmapped GA should return nil")
	}
}

func TestNewMissingField(t *testing.T) {
	items := []Item{
		{GA: "1/1/1", Name: "缺字段", DPT: "DPT-1.001", TuyaDevID: "sub_dev_001", TuyaDPCode: "switch_led"},
	}
	if _, err := New(items); err == nil {
		t.Error("missing status_ga should error")
	}
}

func TestNewRejectsDuplicateAndUnsupportedMappings(t *testing.T) {
	base := Item{
		GA: "1/1/1", StatusGA: "1/1/10", Name: "light",
		DPT: "DPT-1.001", TuyaDevID: "sub_1", TuyaDPCode: "switch", TuyaDPType: "bool",
	}
	duplicate := base
	duplicate.Name = "duplicate"
	if _, err := New([]Item{base, duplicate}); err == nil {
		t.Fatal("duplicate mapping should error")
	}

	unsupported := base
	unsupported.DPT = "DPT-99.001"
	if _, err := New([]Item{unsupported}); err == nil {
		t.Fatal("unsupported DPT should error")
	}
}

func TestNewAllowsScenesWithSharedStatusGAAndDistinctValues(t *testing.T) {
	first := Item{
		GA: "1/0/200", StatusGA: "1/0/200", Name: "总开场景",
		DPT: "DPT-17.001", TuyaDevID: "gateway", TuyaDPCode: "scene_01_trigger",
		TuyaDPType: "bool", Category: "scene", KNXWriteValue: float64(0),
	}
	second := first
	second.Name = "会客场景"
	second.TuyaDPCode = "scene_02_trigger"
	second.KNXWriteValue = float64(4)

	m, err := New([]Item{first, second})
	if err != nil {
		t.Fatalf("shared scene status_ga should be valid: %v", err)
	}
	if got := m.ByStatusGA("1/0/200"); len(got) != 2 {
		t.Fatalf("shared scene mappings = %d, want 2", len(got))
	}
	if got := m.AllStatusGAs(); len(got) != 1 {
		t.Fatalf("status GAs = %v, want one unique address", got)
	}
}

func TestNewRejectsAmbiguousSharedStatusGA(t *testing.T) {
	scene := Item{
		GA: "1/0/200", StatusGA: "1/0/200", Name: "总开场景",
		DPT: "DPT-17.001", TuyaDevID: "gateway", TuyaDPCode: "scene_01_trigger",
		TuyaDPType: "bool", Category: "scene", KNXWriteValue: float64(0),
	}
	duplicateValue := scene
	duplicateValue.Name = "另一个场景"
	duplicateValue.TuyaDPCode = "scene_02_trigger"
	duplicateValue.KNXWriteValue = 0
	if _, err := New([]Item{scene, duplicateValue}); err == nil {
		t.Fatal("duplicate scene value on shared status_ga should error")
	}

	light := scene
	light.Name = "普通灯"
	light.Category = "light"
	light.TuyaDPCode = "light_01_switch"
	if _, err := New([]Item{scene, light}); err == nil {
		t.Fatal("non-scene duplicate status_ga should error")
	}
}

func TestDecodeRuntimeBundle(t *testing.T) {
	raw, err := json.Marshal(Bundle{
		SchemaVersion: BundleSchemaVersion,
		Version:       "2026.09.09-1",
		Mappings:      []Item{bundleTestItem()},
	})
	if err != nil {
		t.Fatal(err)
	}
	m, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if m.Version != "2026.09.09-1" || len(m.Items) != 1 {
		t.Fatalf("decoded bundle = version %q items %d", m.Version, len(m.Items))
	}
}

func TestStoreKeepsLastValidMapping(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime-bundle.json")
	initial, err := json.Marshal(Bundle{
		SchemaVersion: BundleSchemaVersion,
		Version:       "v1",
		Mappings:      []Item{bundleTestItem()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, initial, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"schema_version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if changed, _, err := store.ReloadFromDisk(); err == nil || changed {
		t.Fatalf("invalid mapping reload = changed %t error %v", changed, err)
	}
	if store.Current().Version != "v1" {
		t.Fatalf("active version = %q, want v1", store.Current().Version)
	}
}

func bundleTestItem() Item {
	return Item{
		GA: "1/2/12", StatusGA: "1/2/11", Name: "公卫灯",
		DPT: "DPT-1.001", TuyaDevID: "gateway",
		TuyaDPCode: "light_01_switch", TuyaDPType: "bool",
		VirtualDeviceID: "public_bathroom_light",
		Category:        "light", Room: "公卫", Slot: 1, Capability: "switch",
	}
}

package knxreview

import (
	"os"
	"path/filepath"
	"testing"

	"knx-tuya-gw/internal/config"
	"knx-tuya-gw/internal/mapping"
)

func TestFinalizePassedRows(t *testing.T) {
	directory := t.TempDir()
	basePath := filepath.Join(directory, "base.json")
	reviewPath := filepath.Join(directory, "review.csv")
	outputPath := filepath.Join(directory, "upgrade.json")
	base := `{"schema_version":1,"version":"base","mappings":[]}`
	if err := os.WriteFile(basePath, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	records := []Record{{
		Result: "passed", Name: "公卫灯", Category: "light", Room: "公卫",
		Capability: "switch", GA: "1/2/12", StatusGA: "1/2/11", DPT: "DPT-1.001",
		TuyaDPType: "bool", Slot: 1, VirtualDeviceID: "public_bathroom_light",
		TuyaDPCode: "light_01_switch",
	}}
	if err := writeReviewCSV(reviewPath, records); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default().TuyaMQTT.GatewayDPPool
	if _, err := Finalize(reviewPath, basePath, outputPath, "test-v1", cfg); err != nil {
		t.Fatal(err)
	}
	result, err := mapping.Load(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if result.Version != "test-v1" || len(result.Items) != 1 {
		t.Fatalf("unexpected result: version=%s items=%d", result.Version, len(result.Items))
	}
}

func TestKnownBathroomLightDoesNotConsumeNextSlot(t *testing.T) {
	allocator := &slotAllocator{
		next: map[string]int{"light": 2},
		max:  map[string]int{"light": 3},
		identities: map[string]string{
			"light_01_switch": "public_bathroom_light",
		},
	}
	sheet := sheetData{
		Name: "开关设备",
		Rows: []map[string]string{
			{
				"_row": "2", "设备名称": "公卫灯", "GA地址": "1/2/12",
				"位置/区域": "公卫", "DPT类型": "DPT-1.001",
			},
			{
				"_row": "3", "设备名称": "客厅灯", "GA地址": "1/0/1",
				"位置/区域": "客厅", "DPT类型": "DPT-1.001",
			},
		},
	}

	records := importSwitches("test.xlsx", sheet, allocator)
	if len(records) != 2 {
		t.Fatalf("records=%d, want 2", len(records))
	}
	if records[0].Slot != 1 || records[0].VirtualDeviceID != "public_bathroom_light" {
		t.Fatalf("unexpected known mapping: %+v", records[0])
	}
	if records[1].Slot != 2 {
		t.Fatalf("next slot=%d, want 2", records[1].Slot)
	}
}

func TestAllocateIdentityReusesDeviceAndAllocatesNextSlot(t *testing.T) {
	directory := t.TempDir()
	basePath := filepath.Join(directory, "base.json")
	reviewPath := filepath.Join(directory, "review.csv")
	base := `{
	  "schema_version": 1,
	  "version": "test",
	  "mappings": [{
	    "ga": "1/3/1",
	    "status_ga": "1/3/2",
	    "name": "客厅空调",
	    "dpt": "DPT-1.001",
	    "tuya_dev_id": "gateway",
	    "tuya_dp_code": "ac_01_switch",
	    "tuya_dp_type": "bool",
	    "room": "客厅",
	    "category": "air_conditioner",
	    "capability": "switch",
	    "slot": 1,
	    "virtual_device_id": "living_room_ac"
	  }]
	}`
	if err := os.WriteFile(basePath, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}

	identity, err := AllocateIdentity(
		basePath, reviewPath, "air_conditioner", "客厅", "客厅空调", "mode", 2,
	)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Slot != 1 || identity.VirtualDeviceID != "living_room_ac" ||
		identity.TuyaDPCode != "ac_01_mode" {
		t.Fatalf("unexpected reused identity: %+v", identity)
	}

	identity, err = AllocateIdentity(
		basePath, reviewPath, "air_conditioner", "主卧", "主卧空调", "switch", 2,
	)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Slot != 2 || identity.TuyaDPCode != "ac_02_switch" {
		t.Fatalf("unexpected allocated identity: %+v", identity)
	}
}

func TestAllocateIdentityRejectsExhaustedCapacity(t *testing.T) {
	directory := t.TempDir()
	basePath := filepath.Join(directory, "base.json")
	base := `{
	  "schema_version": 1,
	  "version": "test",
	  "mappings": [{
	    "ga": "1/1/1",
	    "status_ga": "1/1/2",
	    "name": "灯一",
	    "dpt": "DPT-1.001",
	    "tuya_dev_id": "gateway",
	    "tuya_dp_code": "light_01_switch",
	    "tuya_dp_type": "bool",
	    "room": "客厅",
	    "category": "light",
	    "capability": "switch",
	    "slot": 1,
	    "virtual_device_id": "light_01"
	  }]
	}`
	if err := os.WriteFile(basePath, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := AllocateIdentity(
		basePath, "", "light", "卧室", "灯二", "switch", 1,
	); err == nil {
		t.Fatal("expected capacity exhaustion")
	}
}

func TestUpsertReviewRecordReplacesSameCapability(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review.csv")
	first := Record{
		Result: "passed", Category: "air_conditioner", Capability: "mode", Slot: 1,
		Name: "公卫灯", GA: "1/2/12", StatusGA: "1/2/11",
		KNXWriteValue: "1", TuyaToKNXJSON: `{"cool":1}`,
		KNXToTuyaJSON: `{"1":"cool"}`,
	}
	if err := UpsertReviewRecord(path, first); err != nil {
		t.Fatal(err)
	}
	first.GA = "1/2/13"
	first.KNXWriteValue = ""
	first.TuyaToKNXJSON = `{"heat":2}`
	first.KNXToTuyaJSON = `{"2":"heat"}`
	if err := UpsertReviewRecord(path, first); err != nil {
		t.Fatal(err)
	}
	records, err := readReviewCSV(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].GA != "1/2/13" ||
		records[0].KNXWriteValue != "1" ||
		records[0].TuyaToKNXJSON != `{"cool":1,"heat":2}` ||
		records[0].KNXToTuyaJSON != `{"1":"cool","2":"heat"}` {
		t.Fatalf("unexpected records: %+v", records)
	}
}

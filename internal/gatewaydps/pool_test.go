package gatewaydps

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"knx-tuya-gw/internal/config"
	"knx-tuya-gw/internal/mapping"
)

func testConfig() config.GatewayDPPoolConfig {
	return config.GatewayDPPoolConfig{
		Strict: true, MaxFunctions: 40,
		Capacities: map[string]int{
			"light": 1, "air_conditioner": 1, "scene": 1,
			"fresh_air": 1, "climate_sensor": 1,
		},
	}
}

func TestBuildPlan(t *testing.T) {
	items := []mapping.Item{{
		Name: "公卫灯", VirtualDeviceID: "public_bathroom_light",
		Category: "light", Room: "公卫", Slot: 1, Capability: "switch",
		TuyaDPCode: "light_01_switch", TuyaDPType: "bool",
	}}
	artifacts, err := Build(testConfig(), items)
	if err != nil {
		t.Fatal(err)
	}
	if artifacts.ModelPlan.AllocatedFunctions != 12 {
		t.Fatalf("allocated functions = %d, want 12", artifacts.ModelPlan.AllocatedFunctions)
	}
	if !artifacts.ModelPlan.Functions[0].Mapped {
		t.Fatal("mapped function was not marked")
	}
	if artifacts.ModelPlan.Functions[0].KNXWriteGA != "" {
		t.Fatalf("unexpected KNX address in partial test item: %+v", artifacts.ModelPlan.Functions[0])
	}
}

func TestBuildValidatesLogicalDeviceIdentity(t *testing.T) {
	items := []mapping.Item{
		{
			Name: "客厅空调", VirtualDeviceID: "living_room_ac",
			Category: "air_conditioner", Slot: 1, Capability: "switch",
			TuyaDPCode: "ac_01_switch", TuyaDPType: "bool",
		},
	}
	if _, err := Build(testConfig(), items); err != nil {
		t.Fatal(err)
	}

	items = append(items, mapping.Item{
		Name: "客厅空调", VirtualDeviceID: "living_room_ac",
		Category: "air_conditioner", Room: "客厅", Slot: 1, Capability: "mode",
		TuyaDPCode: "ac_01_mode", TuyaDPType: "enum",
	})
	if _, err := Build(testConfig(), items); err == nil || !strings.Contains(err.Error(), "name and room") {
		t.Fatalf("expected room consistency error, got %v", err)
	}
}

func TestBuildRejectsCapacityAndCodeMismatch(t *testing.T) {
	item := mapping.Item{
		Name: "灯", VirtualDeviceID: "light",
		Category: "light", Slot: 2, Capability: "switch",
		TuyaDPCode: "light_02_switch", TuyaDPType: "bool",
	}
	if _, err := Build(testConfig(), []mapping.Item{item}); err == nil || !strings.Contains(err.Error(), "capacity") {
		t.Fatalf("expected capacity error, got %v", err)
	}

	item.Slot = 1
	item.TuyaDPCode = "wrong"
	if _, err := Build(testConfig(), []mapping.Item{item}); err == nil || !strings.Contains(err.Error(), "must be") {
		t.Fatalf("expected DP code error, got %v", err)
	}
}

func TestBuildRejectsTooManyFunctions(t *testing.T) {
	cfg := testConfig()
	cfg.MaxFunctions = 5
	if _, err := Build(cfg, nil); err == nil || !strings.Contains(err.Error(), "exceeding") {
		t.Fatalf("expected function limit error, got %v", err)
	}
}

func TestWriteArtifactsIncludesPlatformXLSX(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig()
	cfg.ModelPlanFile = filepath.Join(dir, "plan.json")
	cfg.PlatformTemplateFile = filepath.Join(dir, "template.xlsx")
	cfg.PlatformXLSXFile = filepath.Join(dir, "platform.xlsx")
	writeTestTemplate(t, cfg.PlatformTemplateFile)
	artifacts, err := Build(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteArtifacts(cfg, artifacts); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(cfg.PlatformXLSXFile)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	var sheet string
	for _, file := range reader.File {
		if file.Name != "xl/worksheets/sheet1.xml" {
			continue
		}
		content, err := readZipFile(file)
		if err != nil {
			t.Fatal(err)
		}
		sheet = string(content)
	}
	if !strings.Contains(sheet, `<c r="A4" s="4"><v>101</v></c>`) ||
		!strings.Contains(sheet, `<t>light_01_switch</t>`) {
		t.Fatalf("unexpected platform XLSX sheet: %q", sheet)
	}
}

func writeTestTemplate(t *testing.T, path string) {
	t.Helper()
	var sheet strings.Builder
	sheet.WriteString(`<worksheet><sheetData>`)
	for row := templateFirstDataRow; row <= templateLastDataRow; row++ {
		sheet.WriteString(`<row r="` + strconv.Itoa(row) + `">`)
		for _, column := range cellColumns {
			sheet.WriteString(fmt.Sprintf(`<c r="%s%d" s="5"/>`, column, row))
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create("xl/worksheets/sheet1.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(sheet.String())); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buffer.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

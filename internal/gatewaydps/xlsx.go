package gatewaydps

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	templateFirstDataRow = 4
	templateLastDataRow  = 201
)

var cellColumns = []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L"}

func writePlatformXLSX(templatePath, outputPath string, plan ModelPlan) error {
	template, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("read template %s: %w", templatePath, err)
	}
	reader, err := zip.NewReader(bytes.NewReader(template), int64(len(template)))
	if err != nil {
		return fmt.Errorf("open template %s: %w", templatePath, err)
	}
	if len(plan.Functions) > templateLastDataRow-templateFirstDataRow+1 {
		return fmt.Errorf("template supports at most %d functions, got %d",
			templateLastDataRow-templateFirstDataRow+1, len(plan.Functions))
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(outputPath), filepath.Base(outputPath)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	writer := zip.NewWriter(tmp)
	foundSheet := false
	for _, file := range reader.File {
		content, err := readZipFile(file)
		if err != nil {
			_ = writer.Close()
			_ = tmp.Close()
			return err
		}
		if file.Name == "xl/worksheets/sheet1.xml" {
			content, err = fillTemplateSheet(content, plan)
			if err != nil {
				_ = writer.Close()
				_ = tmp.Close()
				return err
			}
			foundSheet = true
		}
		header := file.FileHeader
		entry, err := writer.CreateHeader(&header)
		if err != nil {
			_ = writer.Close()
			_ = tmp.Close()
			return err
		}
		if _, err := entry.Write(content); err != nil {
			_ = writer.Close()
			_ = tmp.Close()
			return err
		}
	}
	if !foundSheet {
		_ = writer.Close()
		_ = tmp.Close()
		return fmt.Errorf("template does not contain xl/worksheets/sheet1.xml")
	}
	if err := writer.Close(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, outputPath)
}

func readZipFile(file *zip.File) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read XLSX entry %s: %w", file.Name, err)
	}
	return content, nil
}

func fillTemplateSheet(sheet []byte, plan ModelPlan) ([]byte, error) {
	content := string(sheet)
	for row := templateFirstDataRow; row <= templateLastDataRow; row++ {
		values := make([]cellValue, len(cellColumns))
		index := row - templateFirstDataRow
		if index < len(plan.Functions) {
			values = templateValues(plan.Functions[index])
		}
		for columnIndex, column := range cellColumns {
			var err error
			content, err = replaceCell(content, column, row, values[columnIndex])
			if err != nil {
				return nil, err
			}
		}
	}
	return []byte(content), nil
}

type cellValue struct {
	text    string
	numeric bool
}

func templateValues(function Function) []cellValue {
	definition := ""
	minimum := ""
	maximum := ""
	step := ""
	scale := ""
	unit := ""
	if len(function.EnumValues) > 0 {
		definition = strings.Join(function.EnumValues, ",")
	}
	if function.ValueSpec != nil {
		minimum = strconv.Itoa(function.ValueSpec.Min)
		maximum = strconv.Itoa(function.ValueSpec.Max)
		step = strconv.Itoa(function.ValueSpec.Step)
		scale = strconv.Itoa(function.ValueSpec.Scale)
		unit = function.ValueSpec.Unit
	}
	note := "预留DP，待确认KNX映射"
	if function.Category == "system" {
		note = "网关内置KNX读写调试通道，请勿修改编号、code和类型"
	}
	if function.Mapped {
		note = fmt.Sprintf("%s；控制%s；状态%s",
			function.MappingName, function.KNXWriteGA, function.KNXStatusGA)
	}
	sceneRoles := make([]string, 0, 2)
	if function.SceneCondition {
		sceneRoles = append(sceneRoles, "条件")
	}
	if function.SceneAction {
		sceneRoles = append(sceneRoles, "任务")
	}
	if len(sceneRoles) > 0 {
		note += "；需在产品场景联动设置中启用：" + strings.Join(sceneRoles, "+")
	}
	return []cellValue{
		{text: strconv.Itoa(function.DPID), numeric: true},
		{text: function.Name},
		{text: function.Code},
		{text: function.Access},
		{text: function.Type},
		{text: definition},
		{text: note},
		{text: minimum, numeric: minimum != ""},
		{text: maximum, numeric: maximum != ""},
		{text: step, numeric: step != ""},
		{text: scale, numeric: scale != ""},
		{text: unit},
	}
}

func replaceCell(content, column string, row int, value cellValue) (string, error) {
	reference := column + strconv.Itoa(row)
	pattern := regexp.MustCompile(`(?s)<c r="` + regexp.QuoteMeta(reference) + `"[^>]*(?:/>|>.*?</c>)`)
	if !pattern.MatchString(content) {
		return "", fmt.Errorf("template cell %s not found", reference)
	}
	style := "5"
	if row == templateFirstDataRow {
		style = "4"
	}
	replacement := `<c r="` + reference + `" s="` + style + `"`
	if value.text == "" {
		replacement += `/>`
	} else if value.numeric {
		replacement += `><v>` + xmlEscape(value.text) + `</v></c>`
	} else {
		replacement += ` t="inlineStr"><is><t>` + xmlEscape(value.text) + `</t></is></c>`
	}
	return pattern.ReplaceAllString(content, replacement), nil
}

func xmlEscape(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(value)
}

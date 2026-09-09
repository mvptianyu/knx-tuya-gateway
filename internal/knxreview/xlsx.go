package knxreview

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
)

type sheetData struct {
	Name string
	Rows []map[string]string
}

type workbookXML struct {
	Sheets []struct {
		Name string `xml:"name,attr"`
		ID   string `xml:"id,attr"`
	} `xml:"sheets>sheet"`
}

type relationshipsXML struct {
	Relationships []struct {
		ID     string `xml:"Id,attr"`
		Target string `xml:"Target,attr"`
	} `xml:"Relationship"`
}

type sharedStringsXML struct {
	Items []struct {
		Text string `xml:"t"`
		Runs []struct {
			Text string `xml:"t"`
		} `xml:"r"`
	} `xml:"si"`
}

type worksheetXML struct {
	Rows []struct {
		Number int `xml:"r,attr"`
		Cells  []struct {
			Ref        string `xml:"r,attr"`
			Type       string `xml:"t,attr"`
			Value      string `xml:"v"`
			InlineText string `xml:"is>t"`
		} `xml:"c"`
	} `xml:"sheetData>row"`
}

func readWorkbook(filename string) ([]sheetData, error) {
	reader, err := zip.OpenReader(filename)
	if err != nil {
		return nil, fmt.Errorf("open xlsx: %w", err)
	}
	defer reader.Close()

	files := make(map[string]*zip.File, len(reader.File))
	for _, file := range reader.File {
		files[file.Name] = file
	}

	var workbook workbookXML
	if err := readXMLFile(files, "xl/workbook.xml", &workbook); err != nil {
		return nil, err
	}
	var relationships relationshipsXML
	if err := readXMLFile(files, "xl/_rels/workbook.xml.rels", &relationships); err != nil {
		return nil, err
	}
	targetByID := make(map[string]string, len(relationships.Relationships))
	for _, relationship := range relationships.Relationships {
		targetByID[relationship.ID] = relationship.Target
	}

	var shared sharedStringsXML
	if file := files["xl/sharedStrings.xml"]; file != nil {
		if err := readXMLFile(files, "xl/sharedStrings.xml", &shared); err != nil {
			return nil, err
		}
	}
	sharedValues := make([]string, len(shared.Items))
	for i, item := range shared.Items {
		if item.Text != "" {
			sharedValues[i] = item.Text
			continue
		}
		var value strings.Builder
		for _, run := range item.Runs {
			value.WriteString(run.Text)
		}
		sharedValues[i] = value.String()
	}

	result := make([]sheetData, 0, len(workbook.Sheets))
	for _, sheet := range workbook.Sheets {
		target := targetByID[sheet.ID]
		if target == "" {
			return nil, fmt.Errorf("worksheet %q has no relationship target", sheet.Name)
		}
		filename := path.Clean(path.Join("xl", target))
		var worksheet worksheetXML
		if err := readXMLFile(files, filename, &worksheet); err != nil {
			return nil, err
		}
		rows, err := worksheetRows(worksheet, sharedValues)
		if err != nil {
			return nil, fmt.Errorf("parse worksheet %q: %w", sheet.Name, err)
		}
		result = append(result, sheetData{Name: sheet.Name, Rows: rows})
	}
	return result, nil
}

func readXMLFile(files map[string]*zip.File, filename string, target interface{}) error {
	file := files[filename]
	if file == nil {
		return fmt.Errorf("xlsx entry not found: %s", filename)
	}
	reader, err := file.Open()
	if err != nil {
		return err
	}
	defer reader.Close()
	decoder := xml.NewDecoder(reader)
	if err := decoder.Decode(target); err != nil && err != io.EOF {
		return fmt.Errorf("decode %s: %w", filename, err)
	}
	return nil
}

func worksheetRows(worksheet worksheetXML, shared []string) ([]map[string]string, error) {
	if len(worksheet.Rows) == 0 {
		return nil, nil
	}
	headers := make(map[int]string)
	for _, cell := range worksheet.Rows[0].Cells {
		column := cellColumn(cell.Ref)
		value, err := cellValue(cell.Type, cell.Value, cell.InlineText, shared)
		if err != nil {
			return nil, err
		}
		headers[column] = strings.TrimSpace(value)
	}

	rows := make([]map[string]string, 0, len(worksheet.Rows)-1)
	for _, row := range worksheet.Rows[1:] {
		values := make(map[string]string)
		for _, cell := range row.Cells {
			column := cellColumn(cell.Ref)
			header := headers[column]
			if header == "" {
				continue
			}
			value, err := cellValue(cell.Type, cell.Value, cell.InlineText, shared)
			if err != nil {
				return nil, err
			}
			values[header] = strings.TrimSpace(value)
		}
		values["_row"] = strconv.Itoa(row.Number)
		rows = append(rows, values)
	}
	return rows, nil
}

func cellColumn(reference string) int {
	column := 0
	for _, character := range reference {
		if character < 'A' || character > 'Z' {
			break
		}
		column = column*26 + int(character-'A'+1)
	}
	return column
}

func cellValue(cellType, raw, inline string, shared []string) (string, error) {
	if cellType == "inlineStr" {
		return inline, nil
	}
	if cellType != "s" {
		return raw, nil
	}
	index, err := strconv.Atoi(raw)
	if err != nil || index < 0 || index >= len(shared) {
		return "", fmt.Errorf("invalid shared string index %q", raw)
	}
	return shared[index], nil
}

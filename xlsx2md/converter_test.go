package xlsx2md

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func createTestXlsx() []byte {
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	writeFile := func(name, content string) {
		f, _ := w.Create(name)
		f.Write([]byte(content))
	}

	contentTypes := `<?xml version="1.0" encoding="UTF-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
  <Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
  <Override PartName="/xl/sharedStrings.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml"/>
</Types>`
	writeFile("[Content_Types].xml", contentTypes)

	rels := `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`
	writeFile("_rels/.rels", rels)

	workbook := `<?xml version="1.0" encoding="UTF-8"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"
 xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets>
    <sheet name="Sheet1" sheetId="1" r:id="rId1"/>
  </sheets>
</workbook>`
	writeFile("xl/workbook.xml", workbook)

	workbookRels := `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
</Relationships>`
	writeFile("xl/_rels/workbook.xml.rels", workbookRels)

	sharedStrings := `<?xml version="1.0" encoding="UTF-8"?>
<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="3" uniqueCount="3">
  <si><t>Name</t></si>
  <si><t>Value</t></si>
  <si><t>Alice</t></si>
</sst>`
	writeFile("xl/sharedStrings.xml", sharedStrings)

	worksheet := `<?xml version="1.0" encoding="UTF-8"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <sheetData>
    <row r="1">
      <c r="A1" t="s"><v>0</v></c>
      <c r="B1" t="s"><v>1</v></c>
    </row>
    <row r="2">
      <c r="A2" t="s"><v>2</v></c>
      <c r="B2"><v>42</v></c>
    </row>
  </sheetData>
</worksheet>`
	writeFile("xl/worksheets/sheet1.xml", worksheet)

	w.Close()
	return buf.Bytes()
}

func TestConvertXLSX(t *testing.T) {
	converter := New()
	markdown, err := converter.Convert(createTestXlsx())
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	if !strings.Contains(markdown, "## Sheet1") {
		t.Errorf("Expected sheet heading, got: %s", markdown)
	}
	if !strings.Contains(markdown, "### Range: Sheet1!A1:B2") {
		t.Errorf("Expected range heading, got: %s", markdown)
	}
	if !strings.Contains(markdown, "| Row | A | B |") {
		t.Errorf("Expected row/column header row, got: %s", markdown)
	}
	if !strings.Contains(markdown, "| 1 | Name | Value |") {
		t.Errorf("Expected header data row, got: %s", markdown)
	}
	if !strings.Contains(markdown, "| 2 | Alice | 42 |") {
		t.Errorf("Expected data row, got: %s", markdown)
	}
}

func TestConvertXLSXWithoutSheetNames(t *testing.T) {
	converter := New(WithIncludeSheetNames(false))
	markdown, err := converter.Convert(createTestXlsx())
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	if strings.Contains(markdown, "## Sheet1") {
		t.Errorf("Expected no sheet heading, got: %s", markdown)
	}
}

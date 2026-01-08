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

func createStructuredXlsx() []byte {
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
  <Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>
  <Override PartName="/xl/tables/table1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.table+xml"/>
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
  <si><t>Header1</t></si>
  <si><t>Header2</t></si>
  <si><t>Label</t></si>
</sst>`
	writeFile("xl/sharedStrings.xml", sharedStrings)

	styles := `<?xml version="1.0" encoding="UTF-8"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <numFmts count="2">
    <numFmt numFmtId="165" formatCode="0.00%"/>
    <numFmt numFmtId="166" formatCode="$#,##0.00"/>
  </numFmts>
  <cellXfs count="3">
    <xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>
    <xf numFmtId="165" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>
    <xf numFmtId="166" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>
  </cellXfs>
</styleSheet>`
	writeFile("xl/styles.xml", styles)

	sheet := `<?xml version="1.0" encoding="UTF-8"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <cols><col min="2" max="2" hidden="1"/></cols>
  <sheetData>
    <row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c></row>
    <row r="2"><c r="A2" t="s"><v>2</v></c><c r="B2"><f>SUM(A2:A3)</f><v>2</v></c></row>
    <row r="3" hidden="1"><c r="A3" t="e"><v>#DIV/0!</v></c><c r="B3" s="2"><v>12.5</v></c></row>
    <row r="4"><c r="A4" s="1"><v>0.25</v></c></row>
  </sheetData>
  <mergeCells count="1"><mergeCell ref="A2:A3"/></mergeCells>
  <tableParts count="1"><tablePart r:id="rId1"/></tableParts>
</worksheet>`
	writeFile("xl/worksheets/sheet1.xml", sheet)

	sheetRels := `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/table" Target="../tables/table1.xml"/>
</Relationships>`
	writeFile("xl/worksheets/_rels/sheet1.xml.rels", sheetRels)

	table := `<?xml version="1.0" encoding="UTF-8"?>
<table xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" name="Table1" ref="A1:B3" headerRowCount="1"/>`
	writeFile("xl/tables/table1.xml", table)

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

func TestConvertXLSXStructuredOutput(t *testing.T) {
	converter := New()
	markdown, err := converter.Convert(createStructuredXlsx())
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	if !strings.Contains(markdown, "### Table: Sheet1!A1:B3") {
		t.Errorf("Expected table heading, got: %s", markdown)
	}
	if !strings.Contains(markdown, "| 2 | Label | 2 (=SUM(A2:A3)) |") {
		t.Errorf("Expected formula output, got: %s", markdown)
	}
	if !strings.Contains(markdown, "| 3 [hidden] | #DIV/0! | $12.50 |") {
		t.Errorf("Expected hidden row and error/currency output, got: %s", markdown)
	}
	if !strings.Contains(markdown, "| 4 | 25.00% |") {
		t.Errorf("Expected percent formatting, got: %s", markdown)
	}
}

func TestConvertXLSXExcludeHidden(t *testing.T) {
	converter := New(WithIncludeHidden(false))
	markdown, err := converter.Convert(createStructuredXlsx())
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	if strings.Contains(markdown, "[hidden]") {
		t.Errorf("Expected hidden rows/cols excluded, got: %s", markdown)
	}
}

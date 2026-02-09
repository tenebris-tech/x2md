package xlsx

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func TestParseEncryptedXLSX(t *testing.T) {
	data := make([]byte, 16)
	copy(data, []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1})

	_, err := Parse(data)
	if err == nil || !strings.Contains(err.Error(), "encrypted XLSX") {
		t.Fatalf("expected encrypted XLSX error, got: %v", err)
	}
}

func TestParseTableDefinition(t *testing.T) {
	data := createTableXlsx()
	wb, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(wb.Sheets) != 1 {
		t.Fatalf("expected 1 sheet, got %d", len(wb.Sheets))
	}

	sheet := wb.Sheets[0]
	if len(sheet.Tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(sheet.Tables))
	}

	table := sheet.Tables[0]
	if table.Ref != "A1:B3" {
		t.Fatalf("expected table ref A1:B3, got %s", table.Ref)
	}
	if table.StartRow != 1 || table.EndRow != 3 || table.StartCol != 1 || table.EndCol != 2 {
		t.Fatalf("unexpected table range: %+v", table)
	}
	if table.HeaderRowCount != 1 {
		t.Fatalf("expected header row count 1, got %d", table.HeaderRowCount)
	}
}

func createTableXlsx() []byte {
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	writeFile := func(name, content string) {
		f, _ := w.Create(name)
		_, _ = f.Write([]byte(content))
	}

	contentTypes := `<?xml version="1.0" encoding="UTF-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
  <Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
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

	sheet := `<?xml version="1.0" encoding="UTF-8"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <sheetData>
    <row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c></row>
    <row r="2"><c r="A2" t="s"><v>2</v></c><c r="B2"><v>2</v></c></row>
    <row r="3"><c r="A3" t="e"><v>#DIV/0!</v></c><c r="B3"><v>3</v></c></row>
  </sheetData>
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

	sharedStrings := `<?xml version="1.0" encoding="UTF-8"?>
<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="3" uniqueCount="3">
  <si><t>Header1</t></si>
  <si><t>Header2</t></si>
  <si><t>Value</t></si>
</sst>`
	writeFile("xl/sharedStrings.xml", sharedStrings)

	_ = w.Close()
	return buf.Bytes()
}

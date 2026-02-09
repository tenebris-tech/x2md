package docx2md

import (
	"archive/zip"
	"bytes"
	"testing"
)

// createTestDocx creates a minimal valid DOCX for testing
func createTestDocx(content string) []byte {
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	// [Content_Types].xml
	contentTypes := `<?xml version="1.0" encoding="UTF-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`
	f, _ := w.Create("[Content_Types].xml")
	_, _ = f.Write([]byte(contentTypes))

	// _rels/.rels
	rels := `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`
	f, _ = w.Create("_rels/.rels")
	_, _ = f.Write([]byte(rels))

	// word/document.xml
	document := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>` + content + `</w:body>
</w:document>`
	f, _ = w.Create("word/document.xml")
	_, _ = f.Write([]byte(document))

	_ = w.Close()
	return buf.Bytes()
}

func TestConvertSimpleParagraph(t *testing.T) {
	docx := createTestDocx(`
    <w:p>
      <w:r>
        <w:t>Hello World</w:t>
      </w:r>
    </w:p>`)

	converter := New()
	md, err := converter.Convert(docx)
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	if md == "" {
		t.Error("Expected non-empty markdown output")
	}

	if !bytes.Contains([]byte(md), []byte("Hello World")) {
		t.Errorf("Expected markdown to contain 'Hello World', got: %s", md)
	}
}

func TestConvertBoldText(t *testing.T) {
	docx := createTestDocx(`
    <w:p>
      <w:r>
        <w:rPr><w:b/></w:rPr>
        <w:t>Bold Text</w:t>
      </w:r>
    </w:p>`)

	converter := New()
	md, err := converter.Convert(docx)
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	if !bytes.Contains([]byte(md), []byte("**Bold Text**")) {
		t.Errorf("Expected bold markdown formatting, got: %s", md)
	}
}

func TestConvertItalicText(t *testing.T) {
	docx := createTestDocx(`
    <w:p>
      <w:r>
        <w:rPr><w:i/></w:rPr>
        <w:t>Italic Text</w:t>
      </w:r>
    </w:p>`)

	converter := New()
	md, err := converter.Convert(docx)
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	if !bytes.Contains([]byte(md), []byte("_Italic Text_")) {
		t.Errorf("Expected italic markdown formatting, got: %s", md)
	}
}

func TestConvertHeading(t *testing.T) {
	// Add styles.xml
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	// Copy the test docx but add styles
	contentTypes := `<?xml version="1.0" encoding="UTF-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
  <Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>
</Types>`
	f, _ := w.Create("[Content_Types].xml")
	_, _ = f.Write([]byte(contentTypes))

	rels := `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`
	f, _ = w.Create("_rels/.rels")
	_, _ = f.Write([]byte(rels))

	document := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p>
      <w:pPr><w:pStyle w:val="Heading1"/></w:pPr>
      <w:r>
        <w:t>My Heading</w:t>
      </w:r>
    </w:p>
  </w:body>
</w:document>`
	f, _ = w.Create("word/document.xml")
	_, _ = f.Write([]byte(document))

	styles := `<?xml version="1.0" encoding="UTF-8"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:style w:type="paragraph" w:styleId="Heading1">
    <w:name w:val="Heading 1"/>
    <w:pPr><w:outlineLvl w:val="0"/></w:pPr>
  </w:style>
</w:styles>`
	f, _ = w.Create("word/styles.xml")
	_, _ = f.Write([]byte(styles))

	_ = w.Close()

	converter := New()
	md, err := converter.Convert(buf.Bytes())
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	if !bytes.Contains([]byte(md), []byte("# My Heading")) {
		t.Errorf("Expected H1 markdown formatting, got: %s", md)
	}
}

func TestConvertTable(t *testing.T) {
	docx := createTestDocx(`
    <w:tbl>
      <w:tr>
        <w:tc><w:p><w:r><w:t>Header1</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>Header2</w:t></w:r></w:p></w:tc>
      </w:tr>
      <w:tr>
        <w:tc><w:p><w:r><w:t>Cell1</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>Cell2</w:t></w:r></w:p></w:tc>
      </w:tr>
    </w:tbl>`)

	converter := New()
	md, err := converter.Convert(docx)
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	// Should contain table separators
	if !bytes.Contains([]byte(md), []byte("|")) {
		t.Errorf("Expected table markdown with pipes, got: %s", md)
	}
	if !bytes.Contains([]byte(md), []byte("---")) {
		t.Errorf("Expected table header separator, got: %s", md)
	}
}

func TestConverterOptions(t *testing.T) {
	converter := New(
		WithPreserveFormatting(false),
		WithPreserveImages(false),
		WithPageSeparator("\n\n"),
	)

	if converter.options.PreserveFormatting {
		t.Error("Expected PreserveFormatting to be false")
	}
	if converter.options.PreserveImages {
		t.Error("Expected PreserveImages to be false")
	}
	if converter.options.PageSeparator != "\n\n" {
		t.Error("Expected PageSeparator to be \\n\\n")
	}
}

func TestInvalidDocx(t *testing.T) {
	converter := New()

	// Not a ZIP file
	_, err := converter.Convert([]byte("not a docx file"))
	if err == nil {
		t.Error("Expected error for invalid DOCX")
	}

	// Valid ZIP but missing document.xml
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	f, _ := w.Create("dummy.txt")
	_, _ = f.Write([]byte("dummy"))
	_ = w.Close()

	_, err = converter.Convert(buf.Bytes())
	if err == nil {
		t.Error("Expected error for DOCX without document.xml")
	}
}

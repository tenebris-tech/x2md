package docx2md

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// createTestDocx creates a minimal valid DOCX for testing
func createTestDocx(content string) []byte {
	return createTestDocxWithParts(content, "", "")
}

func createTestDocxWithParts(content, styles, numbering string) []byte {
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	// [Content_Types].xml
	contentTypes := `<?xml version="1.0" encoding="UTF-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>`
	if styles != "" {
		contentTypes += `
  <Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>`
	}
	if numbering != "" {
		contentTypes += `
  <Override PartName="/word/numbering.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"/>`
	}
	contentTypes += `
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

	if styles != "" {
		f, _ = w.Create("word/styles.xml")
		_, _ = f.Write([]byte(styles))
	}

	if numbering != "" {
		f, _ = w.Create("word/numbering.xml")
		_, _ = f.Write([]byte(numbering))
	}

	_ = w.Close()
	return buf.Bytes()
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected markdown to contain %q, got:\n%s", want, got)
	}
}

func assertNotContains(t *testing.T, got, want string) {
	t.Helper()
	if strings.Contains(got, want) {
		t.Fatalf("expected markdown not to contain %q, got:\n%s", want, got)
	}
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

func TestConvertDocxTypographyPreservesInlineHTML(t *testing.T) {
	docx := createTestDocx(`
    <w:p>
      <w:r><w:t>Prefix </w:t></w:r>
      <w:r><w:rPr><w:u w:val="single"/></w:rPr><w:t>under</w:t></w:r>
      <w:r><w:t> </w:t></w:r>
      <w:r><w:rPr><w:strike/></w:rPr><w:t>delete</w:t></w:r>
      <w:r><w:t> </w:t></w:r>
      <w:r><w:rPr><w:dstrike/></w:rPr><w:t>double</w:t></w:r>
      <w:r><w:t> </w:t></w:r>
      <w:r><w:rPr><w:vertAlign w:val="superscript"/></w:rPr><w:t>sup</w:t></w:r>
      <w:r><w:t> </w:t></w:r>
      <w:r><w:rPr><w:vertAlign w:val="subscript"/></w:rPr><w:t>sub</w:t></w:r>
      <w:r><w:t> </w:t></w:r>
      <w:r><w:rPr><w:highlight w:val="yellow"/></w:rPr><w:t>mark</w:t></w:r>
      <w:r><w:t> </w:t></w:r>
      <w:r><w:rPr><w:color w:val="FF0000"/></w:rPr><w:t>red</w:t></w:r>
      <w:r><w:t> </w:t></w:r>
      <w:r><w:rPr><w:b/><w:i/></w:rPr><w:t>both</w:t></w:r>
    </w:p>`)

	converter := New()
	md, err := converter.Convert(docx)
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	assertContains(t, md, `<u>under</u>`)
	assertContains(t, md, `~~delete~~`)
	assertContains(t, md, `~~double~~`)
	assertContains(t, md, `<sup>sup</sup>`)
	assertContains(t, md, `<sub>sub</sub>`)
	assertContains(t, md, `<span style="background-color: yellow">mark</span>`)
	assertContains(t, md, `<span style="color: #FF0000">red</span>`)
	assertContains(t, md, `**_both_**`)
}

func TestConvertDocxTypographyCanDisableInlineHTML(t *testing.T) {
	docx := createTestDocx(`
    <w:p>
      <w:r><w:t>Prefix </w:t></w:r>
      <w:r><w:rPr><w:u/></w:rPr><w:t>under</w:t></w:r>
      <w:r><w:t> </w:t></w:r>
      <w:r><w:rPr><w:strike/></w:rPr><w:t>delete</w:t></w:r>
      <w:r><w:t> </w:t></w:r>
      <w:r><w:rPr><w:vertAlign w:val="superscript"/></w:rPr><w:t>sup</w:t></w:r>
      <w:r><w:t> </w:t></w:r>
      <w:r><w:rPr><w:highlight w:val="yellow"/><w:color w:val="00AA00"/></w:rPr><w:t>styled</w:t></w:r>
    </w:p>`)

	converter := New(WithInlineHTML(false))
	md, err := converter.Convert(docx)
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	assertContains(t, md, `Prefix under ~~delete~~ sup styled`)
	assertNotContains(t, md, `<u>`)
	assertNotContains(t, md, `<sup>`)
	assertNotContains(t, md, `<span`)
}

func TestConvertDocxRunBoundariesDoNotInsertSpaces(t *testing.T) {
	docx := createTestDocx(`
    <w:p>
      <w:r><w:t>Application Programming Interfac</w:t></w:r>
      <w:r><w:rPr><w:u/></w:rPr><w:t>e</w:t></w:r>
      <w:r><w:t> O.</w:t></w:r>
      <w:r><w:rPr><w:b/></w:rPr><w:t>A</w:t></w:r>
      <w:r><w:t>DMIN P</w:t></w:r>
      <w:r><w:rPr><w:i/></w:rPr><w:t>.</w:t></w:r>
      <w:r><w:t>RETAIN</w:t></w:r>
    </w:p>`)

	converter := New(WithPreserveFormatting(false))
	md, err := converter.Convert(docx)
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	assertContains(t, md, `Application Programming Interface O.ADMIN P.RETAIN`)
	assertNotContains(t, md, `Interfac e`)
	assertNotContains(t, md, `O. A`)
	assertNotContains(t, md, `P .`)
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

func TestConvertDocxCharacterStylesAndValFalse(t *testing.T) {
	styles := `<?xml version="1.0" encoding="UTF-8"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:style w:type="character" w:styleId="Strong">
    <w:name w:val="Strong"/>
    <w:rPr><w:b/></w:rPr>
  </w:style>
  <w:style w:type="character" w:styleId="Emphasis">
    <w:name w:val="Emphasis"/>
    <w:rPr><w:i/></w:rPr>
  </w:style>
  <w:style w:type="paragraph" w:styleId="BoldPara">
    <w:name w:val="BoldPara"/>
    <w:rPr><w:b/></w:rPr>
  </w:style>
</w:styles>`
	docx := createTestDocxWithParts(`
    <w:p>
      <w:r><w:rPr><w:rStyle w:val="Strong"/></w:rPr><w:t>Strong</w:t></w:r>
      <w:r><w:t> </w:t></w:r>
      <w:r><w:rPr><w:rStyle w:val="Emphasis"/></w:rPr><w:t>Emphasis</w:t></w:r>
    </w:p>
    <w:p>
      <w:pPr><w:pStyle w:val="BoldPara"/></w:pPr>
      <w:r><w:t>Inherited </w:t></w:r>
      <w:r><w:rPr><w:b w:val="0"/></w:rPr><w:t>plain</w:t></w:r>
    </w:p>`, styles, "")

	converter := New()
	md, err := converter.Convert(docx)
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	assertContains(t, md, `**Strong** _Emphasis_`)
	assertContains(t, md, `**Inherited **plain`)
	assertNotContains(t, md, `**plain**`)
}

func TestConvertDocxNumberedHeadingsAndListReset(t *testing.T) {
	styles := `<?xml version="1.0" encoding="UTF-8"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:style w:type="paragraph" w:styleId="Heading1">
    <w:name w:val="Heading 1"/>
    <w:pPr><w:outlineLvl w:val="0"/></w:pPr>
  </w:style>
  <w:style w:type="paragraph" w:styleId="Heading2">
    <w:name w:val="Heading 2"/>
    <w:pPr><w:outlineLvl w:val="1"/></w:pPr>
  </w:style>
</w:styles>`
	numbering := `<?xml version="1.0" encoding="UTF-8"?>
<w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:abstractNum w:abstractNumId="1">
    <w:lvl w:ilvl="0">
      <w:start w:val="1"/>
      <w:numFmt w:val="decimal"/>
      <w:lvlText w:val="%1."/>
    </w:lvl>
    <w:lvl w:ilvl="1">
      <w:start w:val="1"/>
      <w:numFmt w:val="decimal"/>
      <w:lvlText w:val="%1.%2"/>
    </w:lvl>
  </w:abstractNum>
  <w:num w:numId="1"><w:abstractNumId w:val="1"/></w:num>
  <w:abstractNum w:abstractNumId="2">
    <w:lvl w:ilvl="0">
      <w:start w:val="1"/>
      <w:numFmt w:val="lowerLetter"/>
      <w:lvlText w:val="%1."/>
    </w:lvl>
  </w:abstractNum>
  <w:num w:numId="2"><w:abstractNumId w:val="2"/></w:num>
</w:numbering>`
	docx := createTestDocxWithParts(`
    <w:p>
      <w:pPr>
        <w:pStyle w:val="Heading1"/>
        <w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr>
      </w:pPr>
      <w:r><w:t>Overview</w:t></w:r>
    </w:p>
    <w:p><w:r><w:t>Body text</w:t></w:r></w:p>
    <w:p>
      <w:pPr>
        <w:pStyle w:val="Heading2"/>
        <w:numPr><w:ilvl w:val="1"/><w:numId w:val="1"/></w:numPr>
      </w:pPr>
      <w:r><w:t>Scope</w:t></w:r>
    </w:p>
    <w:p>
      <w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="2"/></w:numPr></w:pPr>
      <w:r><w:t>First</w:t></w:r>
    </w:p>
    <w:p>
      <w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="2"/></w:numPr></w:pPr>
      <w:r><w:t>Second</w:t></w:r>
    </w:p>
    <w:p><w:r><w:t>Break</w:t></w:r></w:p>
    <w:p>
      <w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="2"/></w:numPr></w:pPr>
      <w:r><w:t>Third</w:t></w:r>
    </w:p>
    <w:p>
      <w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="2"/></w:numPr></w:pPr>
      <w:r><w:t>Fourth</w:t></w:r>
    </w:p>`, styles, numbering)

	converter := New()
	md, err := converter.Convert(docx)
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	assertContains(t, md, "# 1. Overview")
	assertContains(t, md, "## 1.1 Scope")
	assertContains(t, md, "a. First\nb. Second")
	assertContains(t, md, "Break\n\na. Third\nb. Fourth")
	assertNotContains(t, md, "c. Third")
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
		WithInlineHTML(false),
		WithPreserveImages(false),
		WithPageSeparator("\n\n"),
	)

	if converter.options.PreserveFormatting {
		t.Error("Expected PreserveFormatting to be false")
	}
	if converter.options.PreserveInlineHTML {
		t.Error("Expected PreserveInlineHTML to be false")
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

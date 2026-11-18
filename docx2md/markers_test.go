package docx2md

import (
	"fmt"
	"strings"
	"testing"
)

const testStyles = `<?xml version="1.0" encoding="UTF-8"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/></w:style>
</w:styles>`

const testNumbering = `<?xml version="1.0" encoding="UTF-8"?>
<w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:abstractNum w:abstractNumId="0"><w:lvl w:ilvl="0"><w:numFmt w:val="bullet"/></w:lvl></w:abstractNum>
  <w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num>
</w:numbering>`

func para(text string) string {
	return `<w:p><w:r><w:t>` + text + `</w:t></w:r></w:p>`
}

func heading(text string) string {
	return `<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>` + text + `</w:t></w:r></w:p>`
}

func cell(paras ...string) string {
	return `<w:tc>` + strings.Join(paras, "") + `</w:tc>`
}

func convertBody(t *testing.T, body string, opts ...Option) string {
	t.Helper()
	md, err := New(opts...).Convert(createTestDocxWithParts(body, map[string]string{
		"word/styles.xml":    testStyles,
		"word/numbering.xml": testNumbering,
	}))
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	return md
}

// markerBefore returns the paragraph marker nearest above the given text,
// which is how a reader of the Markdown locates a passage
func markerBefore(t *testing.T, md, text string) string {
	t.Helper()
	idx := strings.Index(md, text)
	if idx < 0 {
		t.Fatalf("text %q not found in:\n%s", text, md)
	}
	start := strings.LastIndex(md[:idx], "<!-- para ")
	if start < 0 {
		t.Fatalf("no marker before %q in:\n%s", text, md)
	}
	end := strings.Index(md[start:], "-->")
	return md[start : start+end+3]
}

func TestParagraphMarkers_ConventionHeaderAndFirstParagraph(t *testing.T) {
	md := convertBody(t, para("Hello")+para("World"))
	if !strings.HasPrefix(md, "<!-- x2md: converted from a DOCX.") {
		t.Errorf("expected convention header first, got:\n%s", md)
	}
	if !strings.Contains(md, "<!-- para 1 -->\n\nHello\n\nWorld\n\n") {
		t.Errorf("expected marker before first paragraph only, got:\n%s", md)
	}
}

func TestParagraphMarkers_Disabled(t *testing.T) {
	md := convertBody(t, heading("Title")+para("Text"), WithParagraphMarkers(false))
	if strings.Contains(md, "<!--") {
		t.Errorf("expected no comments, got:\n%s", md)
	}
}

func TestParagraphMarkers_EveryHeading(t *testing.T) {
	md := convertBody(t, heading("One")+para("a")+para("b")+heading("Two")+para("c"))
	for text, want := range map[string]string{
		"# One": "<!-- para 1 -->",
		"# Two": "<!-- para 4 -->",
		"c":     "<!-- para 4 -->",
	} {
		if got := markerBefore(t, md, text+"\n"); got != want {
			t.Errorf("marker before %q = %s, want %s\n%s", text, got, want, md)
		}
	}
}

func TestParagraphMarkers_EmptyParagraphsAreCounted(t *testing.T) {
	// Empty paragraphs produce no output but still occupy a position
	md := convertBody(t, heading("One")+`<w:p/>`+`<w:p></w:p>`+heading("Two"))
	if got := markerBefore(t, md, "# Two"); got != "<!-- para 4 -->" {
		t.Errorf("got %s, want <!-- para 4 -->\n%s", got, md)
	}
}

func TestParagraphMarkers_Interval(t *testing.T) {
	var body strings.Builder
	for i := 1; i <= 25; i++ {
		body.WriteString(para(fmt.Sprintf("Line-%02d.", i)))
	}
	md := convertBody(t, body.String())
	for _, want := range []string{"<!-- para 1 -->\n\nLine-01.", "<!-- para 11 -->\n\nLine-11.", "<!-- para 21 -->\n\nLine-21."} {
		if !strings.Contains(md, want) {
			t.Errorf("expected %q in:\n%s", want, md)
		}
	}
	if n := strings.Count(md, "<!-- para "); n != 3 {
		t.Errorf("expected 3 markers, got %d:\n%s", n, md)
	}
}

func TestParagraphMarkers_TableCellsAreCounted(t *testing.T) {
	// Paragraph 1, then a 2x2 table whose cells hold 5 paragraphs (2-6), then paragraph 7
	body := para("Before") +
		`<w:tbl>` +
		`<w:tr>` + cell(para("H1")) + cell(para("H2")) + `</w:tr>` +
		`<w:tr>` + cell(para("A"), para("B")) + cell(para("C")) + `</w:tr>` +
		`</w:tbl>` +
		heading("After")
	md := convertBody(t, body)
	if got := markerBefore(t, md, "| H1 | H2 |"); got != "<!-- para 2 -->" {
		t.Errorf("table marker = %s, want <!-- para 2 -->\n%s", got, md)
	}
	if got := markerBefore(t, md, "# After"); got != "<!-- para 7 -->" {
		t.Errorf("marker after table = %s, want <!-- para 7 -->\n%s", got, md)
	}
	// The marker must not split the table from the blank line Markdown needs before it
	if !strings.Contains(md, "<!-- para 2 -->\n\n| H1 | H2 |") {
		t.Errorf("expected marker directly before table, got:\n%s", md)
	}
}

func TestParagraphMarkers_FallbackContentNotCounted(t *testing.T) {
	// A text box is stored twice: once in mc:Choice and again in mc:Fallback.
	// A viewer renders one of them, so only the first is counted.
	textBox := `<w:p><w:r><mc:AlternateContent xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006">` +
		`<mc:Choice Requires="wps"><w:drawing><w:txbxContent>` + para("Box") + `</w:txbxContent></w:drawing></mc:Choice>` +
		`<mc:Fallback><w:pict><w:txbxContent>` + para("Box") + `</w:txbxContent></w:pict></mc:Fallback>` +
		`</mc:AlternateContent></w:r></w:p>`
	md := convertBody(t, heading("One")+textBox+heading("Two"))
	// 1 = One, 2 = paragraph holding the text box, 3 = paragraph in mc:Choice, 4 = Two
	if got := markerBefore(t, md, "# Two"); got != "<!-- para 4 -->" {
		t.Errorf("got %s, want <!-- para 4 -->\n%s", got, md)
	}
}

func TestParagraphMarkers_DrawingMLParagraphsNotCounted(t *testing.T) {
	// DrawingML has its own a:p element, which is not a document paragraph
	shape := `<w:p><w:r><w:drawing><a:txBody xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">` +
		`<a:p><a:r><a:t>shape text</a:t></a:r></a:p></a:txBody></w:drawing></w:r></w:p>`
	md := convertBody(t, heading("One")+shape+heading("Two"))
	if got := markerBefore(t, md, "# Two"); got != "<!-- para 3 -->" {
		t.Errorf("got %s, want <!-- para 3 -->\n%s", got, md)
	}
}

func TestParagraphMarkers_OwnLineAfterList(t *testing.T) {
	list := `<w:p><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr></w:pPr><w:r><w:t>Item</w:t></w:r></w:p>`
	md := convertBody(t, para("Intro")+list+heading("Next"))
	if !strings.Contains(md, "\n\n<!-- para 3 -->\n\n# Next") {
		t.Errorf("expected marker on its own line with blank lines around it, got:\n%q", md)
	}
}

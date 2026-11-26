/******************************************************************************
 * Copyright (c) 2026 Tenebris Technologies Inc.                              *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package docx2md

import (
	"strings"
	"testing"
)

const (
	relNS    = `xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`
	testRels = `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rLink" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="http://x.example/" TargetMode="External"/>
  <Relationship Id="rImg1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/one.png"/>
  <Relationship Id="rImg2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/two.png"/>
</Relationships>`
	pngHeader = "\x89PNG\r\n\x1a\n"
)

// convertWithRels converts a body that uses the test relationships
func convertWithRels(t *testing.T, body string) string {
	t.Helper()
	md, err := New(WithParagraphMarkers(false)).Convert(createTestDocxWithParts(body, map[string]string{
		"word/styles.xml":              testStyles,
		"word/_rels/document.xml.rels": testRels,
		"word/media/one.png":           pngHeader + "one",
		"word/media/two.png":           pngHeader + "two",
	}))
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	return md
}

func expectLine(t *testing.T, md, want string) {
	t.Helper()
	if !strings.Contains(md, "\n"+want+"\n") && !strings.HasPrefix(md, want+"\n") {
		t.Errorf("expected the line %q in:\n%s", want, md)
	}
}

func TestBreaksSeparateWords(t *testing.T) {
	body := `<w:p><w:r><w:t>DR/BCP</w:t></w:r><w:r><w:br/><w:t>Marco</w:t></w:r>` +
		`<w:r><w:cr/><w:t>Polo</w:t></w:r><w:r><w:tab/><w:t>end</w:t></w:r></w:p>` +
		table(cell(`<w:p><w:r><w:t>c1</w:t><w:br/><w:t>c2</w:t><w:cr/><w:t>c3</w:t></w:r></w:p>`), cell(para("x"))) +
		`<w:p><w:r><w:t>Before</w:t></w:r><w:r><w:pict><v:shape ` + vmlNS + `><v:textbox><w:txbxContent>` +
		`<w:p><w:r><w:t>b1</w:t><w:br/><w:t>b2</w:t></w:r></w:p></w:txbxContent></v:textbox></v:shape></w:pict></w:r></w:p>` +
		`<w:p><w:r><w:t>Shape</w:t></w:r><w:r><w:drawing><w:txbxContent>` +
		`<w:p><w:r><w:t>d1</w:t><w:br/><w:t>d2</w:t><w:tab/><w:t>d3</w:t></w:r></w:p></w:txbxContent></w:drawing></w:r></w:p>`
	md := convertBody(t, body, WithParagraphMarkers(false))
	expectLine(t, md, "DR/BCP Marco Polo end")
	expectLine(t, md, "| c1 c2 c3 | x |")
	expectLine(t, md, "Before b1 b2")
	expectLine(t, md, "Shape d1 d2 d3")
}

func TestHyperlinkSpacing(t *testing.T) {
	link := func(runs ...string) string {
		return `<w:hyperlink r:id="rLink" ` + relNS + `>` + strings.Join(runs, "") + `</w:hyperlink>`
	}
	md := convertWithRels(t,
		`<w:p>`+run("Please ")+link(run("click here "))+run("now.")+`</w:p>`+
			`<w:p>`+run("See ")+link(run("click"), run(" here"))+run(".")+`</w:p>`)
	expectLine(t, md, "Please [click here](http://x.example/) now.")
	expectLine(t, md, "See [click here](http://x.example/).")
}

func TestNoBreakSpaceIsKept(t *testing.T) {
	md := convertBody(t, para("10 kg")+`<w:p>`+run("20 ")+run("kg")+`</w:p>`+
		table(cell(para("30 kg"))), WithParagraphMarkers(false))
	expectLine(t, md, "10 kg")
	expectLine(t, md, "20 kg")
	expectLine(t, md, "| 30 kg |")
}

func TestNestedTableStaysInItsCell(t *testing.T) {
	inner := `<w:tbl><w:tr>` + cell(para("in1")) + cell(para("in2")) + `</w:tr></w:tbl>`
	md := convertBody(t, table(cell(para("outer"), inner, para("after")), cell(para("next")))+heading("Next"))
	expectLine(t, md, "| outer in1 in2 after | next |")
	// 1 = outer, 2 = in1, 3 = in2, 4 = after, 5 = next, 6 = heading
	if got := markerBefore(t, md, "# Next"); got != "<!-- para 6 -->" {
		t.Errorf("got %s, want <!-- para 6 -->\n%s", got, md)
	}
}

func TestDrawingWithSeveralImages(t *testing.T) {
	blip := func(id string) string {
		return `<a:blip xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" r:embed="` + id + `" ` + relNS + `/>`
	}
	md := convertWithRels(t, `<w:p><w:r><w:drawing><wp:docPr xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" name="Group"/>`+
		blip("rImg1")+blip("rImg2")+`</w:drawing></w:r></w:p>`)
	if !strings.Contains(md, "![image_001]") || !strings.Contains(md, "![image_002]") {
		t.Errorf("expected both images, got:\n%s", md)
	}
}

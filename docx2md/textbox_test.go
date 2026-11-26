/******************************************************************************
 * Copyright (c) 2026 Tenebris Technologies Inc.                              *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package docx2md

import (
	"strings"
	"testing"
)

// run is a run of text that keeps its surrounding spaces
func run(text string) string {
	return `<w:r><w:t xml:space="preserve">` + text + `</w:t></w:r>`
}

const (
	vmlNS = `xmlns:v="urn:schemas-microsoft-com:vml"`
	mcNS  = `xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006"`
	wpsNS = `xmlns:wps="http://schemas.microsoft.com/office/word/2010/wordprocessingShape"`
)

func TestTextBox_VMLKeepsTheParagraphWhole(t *testing.T) {
	box := `<w:r><w:pict><v:shape ` + vmlNS + `><v:textbox><w:txbxContent>` +
		heading("Box text") + `</w:txbxContent></v:textbox></v:shape></w:pict></w:r>`
	md := convertBody(t, `<w:p>`+run("Before ")+box+run(" after")+`</w:p>`+heading("Next"))

	if !strings.Contains(md, "\n\nBefore Box text after\n\n") {
		t.Errorf("expected the whole paragraph as plain text, got:\n%s", md)
	}
	// 1 = outer paragraph, 2 = paragraph in the text box, 3 = Next
	if got := markerBefore(t, md, "# Next"); got != "<!-- para 3 -->" {
		t.Errorf("got %s, want <!-- para 3 -->\n%s", got, md)
	}
}

func TestTextBox_AlternateContentTextOnce(t *testing.T) {
	box := `<w:r><mc:AlternateContent ` + mcNS + `>` +
		`<mc:Choice Requires="wps"><w:drawing><wps:wsp ` + wpsNS + `><wps:txbx><w:txbxContent>` + para("Box text") +
		`</w:txbxContent></wps:txbx></wps:wsp></w:drawing></mc:Choice>` +
		`<mc:Fallback><w:pict><v:shape ` + vmlNS + `><v:textbox><w:txbxContent>` + para("Box text") +
		`</w:txbxContent></v:textbox></v:shape></w:pict></mc:Fallback>` +
		`</mc:AlternateContent></w:r>`
	md := convertBody(t, `<w:p>`+run("Before")+box+run("after")+`</w:p>`+heading("Next"))

	if !strings.Contains(md, "\n\nBefore Box text after\n\n") || strings.Count(md, "Box text") != 1 {
		t.Errorf("expected the text box text once inside the paragraph, got:\n%s", md)
	}
	// 1 = outer paragraph, 2 = paragraph in mc:Choice, 3 = Next
	if got := markerBefore(t, md, "# Next"); got != "<!-- para 3 -->" {
		t.Errorf("got %s, want <!-- para 3 -->\n%s", got, md)
	}
}

/******************************************************************************
 * Copyright (c) 2026 Tenebris Technologies Inc.                              *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package docx2md

import (
	"strings"
	"testing"
)

func table(cells ...string) string {
	return `<w:tbl><w:tr>` + strings.Join(cells, "") + `</w:tr></w:tbl>`
}

func TestTableCell_RunSpacing(t *testing.T) {
	md := convertBody(t, table(
		cell(`<w:p>`+run("ISO 27001 ")+run("certification is expected")+`</w:p>`),
		cell(`<w:p>`+run("an")+run("d")+`</w:p>`, para("second paragraph")),
	))
	want := "| ISO 27001 certification is expected | and second paragraph |"
	if !strings.Contains(md, want) {
		t.Errorf("expected %q in:\n%s", want, md)
	}
}

func TestTableCell_FallbackTextOnce(t *testing.T) {
	box := `<w:p><w:r><mc:AlternateContent ` + mcNS + `>` +
		`<mc:Choice Requires="wps"><w:drawing><w:txbxContent>` + para("Box") + `</w:txbxContent></w:drawing></mc:Choice>` +
		`<mc:Fallback><w:pict><w:txbxContent>` + para("Box") + `</w:txbxContent></w:pict></mc:Fallback>` +
		`</mc:AlternateContent></w:r></w:p>`
	md := convertBody(t, table(cell(box), cell(para("Other")))+heading("Next"))
	if !strings.Contains(md, "| Box | Other |") {
		t.Errorf("expected the text box text once in its cell:\n%s", md)
	}
	// 1 = cell paragraph, 2 = paragraph in mc:Choice, 3 = Other, 4 = Next
	if got := markerBefore(t, md, "# Next"); got != "<!-- para 4 -->" {
		t.Errorf("got %s, want <!-- para 4 -->\n%s", got, md)
	}
}

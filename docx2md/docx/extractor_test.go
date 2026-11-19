/******************************************************************************
 * Copyright (c) 2026 Tenebris Technologies Inc.                              *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package docx

import (
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

const wordNS = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"`

// countParagraphs reads a whole document through a bodyReader
func countParagraphs(t *testing.T, body string) int {
	t.Helper()
	r := &bodyReader{Decoder: xml.NewDecoder(strings.NewReader(`<w:document ` + wordNS + `><w:body>` + body + `</w:body></w:document>`))}
	for {
		_, err := r.Token()
		if err == io.EOF {
			return r.paragraphs
		}
		if err != nil {
			t.Fatalf("reading document: %v", err)
		}
	}
}

func TestBodyReader_CountsParagraphs(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"empty body", ``, 0},
		{"plain paragraphs", `<w:p/><w:p></w:p><w:p><w:r><w:t>x</w:t></w:r></w:p>`, 3},
		{"paragraphs in table cells", `<w:p/><w:tbl><w:tr><w:tc><w:p/><w:p/></w:tc><w:tc><w:p/></w:tc></w:tr></w:tbl>`, 4},
		{"nested table", `<w:tbl><w:tr><w:tc><w:p/><w:tbl><w:tr><w:tc><w:p/></w:tc></w:tr></w:tbl></w:tc></w:tr></w:tbl>`, 2},
		{
			"fallback copy of a text box is not counted",
			`<w:p><mc:AlternateContent xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006">` +
				`<mc:Choice><w:txbxContent><w:p/></w:txbxContent></mc:Choice>` +
				`<mc:Fallback><w:txbxContent><w:p/></w:txbxContent></mc:Fallback>` +
				`</mc:AlternateContent></w:p><w:p/>`,
			3,
		},
		{
			"DrawingML paragraphs are not counted",
			`<w:p><a:txBody xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:p/><a:p/></a:txBody></w:p>`,
			1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countParagraphs(t, tt.body); got != tt.want {
				t.Errorf("counted %d paragraphs, want %d", got, tt.want)
			}
		})
	}
}

func TestIsWordNamespace(t *testing.T) {
	tests := map[string]bool{
		"http://schemas.openxmlformats.org/wordprocessingml/2006/main": true,
		"http://purl.oclc.org/ooxml/wordprocessingml/main":             true, // strict OOXML
		"w": true, // undeclared prefix
		"http://schemas.openxmlformats.org/drawingml/2006/main": false,
		"": false,
	}
	for space, want := range tests {
		if got := isWordNamespace(space); got != want {
			t.Errorf("isWordNamespace(%q) = %v, want %v", space, got, want)
		}
	}
}

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		n      int
		format string
		want   string
	}{
		{1, "decimal", "1"},
		{9, "decimal", "9"},
		{10, "decimal", "10"},
		{27, "decimal", "27"},
		{123, "decimal", "123"},
		{1, "lowerLetter", "a"},
		{26, "lowerLetter", "z"},
		{3, "upperLetter", "C"},
		{4, "lowerRoman", "iv"},
		{9, "upperRoman", "IX"},
		{14, "upperRoman", "XIV"},
		{2024, "lowerRoman", "mmxxiv"},
		{12, "unknown", "12"},
	}
	for _, tt := range tests {
		if got := formatNumber(tt.n, tt.format); got != tt.want {
			t.Errorf("formatNumber(%d, %q) = %q, want %q", tt.n, tt.format, got, tt.want)
		}
	}
}

func TestToRoman_OutOfRange(t *testing.T) {
	for _, n := range []int{0, -1, 4000} {
		if got := toRoman(n, false); got != "" {
			t.Errorf("toRoman(%d) = %q, want empty", n, got)
		}
	}
}

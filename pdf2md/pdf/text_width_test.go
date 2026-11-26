/******************************************************************************
 * Copyright (c) 2026 Tenebris Technologies Inc.                              *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package pdf

import (
	"math"
	"testing"
)

// testFont is a simple TrueType font with widths for A, B and C only, and a
// MissingWidth of 250 for every other code
func testFont() map[string]interface{} {
	return map[string]interface{}{
		"Subtype":   "/TrueType",
		"BaseFont":  "/AAAAAA+Test",
		"FirstChar": 65.0,
		"LastChar":  67.0,
		"Widths":    []interface{}{600.0, 700.0, 800.0},
		"FontDescriptor": map[string]interface{}{
			"MissingWidth": 250.0,
		},
	}
}

// extractWith runs a page content stream with the given fonts and returns
// the text items
func extractWith(t *testing.T, fonts map[string]map[string]interface{}, content string) []TextItem {
	t.Helper()
	e := NewTextExtractor(NewParser(nil))
	for name, dict := range fonts {
		font := e.parseFont(dict)
		font.Name = name
		e.fonts[name] = font
	}
	items, err := e.parseContentStream([]byte(content), [4]float64{0, 0, 612, 792})
	if err != nil {
		t.Fatalf("parsing content: %v", err)
	}
	return items
}

func oneItem(t *testing.T, items []TextItem) TextItem {
	t.Helper()
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1: %+v", len(items), items)
	}
	return items[0]
}

func near(got, want float64) bool {
	return math.Abs(got-want) < 0.01
}

func TestTextWidth_InPageUnits(t *testing.T) {
	fonts := map[string]map[string]interface{}{"F1": testFont()}

	tests := []struct {
		name    string
		content string
		wantX   float64
		wantW   float64
	}{
		{
			// Font size 1, scaled to 10 by the text matrix
			name:    "Tj scaled by text matrix",
			content: "BT /F1 1 Tf 10 0 0 10 100 700 Tm (ABC) Tj ET",
			wantX:   100,
			wantW:   21,
		},
		{
			name:    "Tj scaled by CTM",
			content: "q 0.5 0 0 0.5 0 0 cm BT /F1 20 Tf 200 1400 Td (ABC) Tj ET Q",
			wantX:   100,
			wantW:   21,
		},
		{
			// AB (26) + 500/1000 em (10) + C (16) = 52 in text space, halved
			name:    "TJ scaled by CTM, with adjustment",
			content: "q 0.5 0 0 0.5 0 0 cm BT /F1 20 Tf 1 0 0 1 200 1400 Tm [(AB) -500 (C)] TJ ET Q",
			wantX:   100,
			wantW:   26,
		},
		{
			// Size 1, scaled to 10 by the text matrix
			name:    "TJ scaled by text matrix",
			content: "BT /F1 1 Tf 10 0 0 10 100 700 Tm [(A) -1000 (B)] TJ ET",
			wantX:   100,
			wantW:   23,
		},
		{
			// The text starts after the leading adjustment and ends with the
			// last glyph, not with the trailing adjustment
			name:    "TJ leading and trailing adjustments",
			content: "BT /F1 10 Tf 100 700 Td [-1000 (A) 200] TJ ET",
			wantX:   110,
			wantW:   6,
		},
		{
			// A: 6+2, space: MissingWidth 2.5+2 plus Tw 3, A: 6+2 = 23.5,
			// times Tz 50%, times the text matrix scale 2
			name:    "Tc, Tw and Tz",
			content: "BT /F1 10 Tf 2 0 0 2 100 700 Tm 2 Tc 3 Tw 50 Tz (A A) Tj ET",
			wantX:   100,
			wantW:   23.5,
		},
		{
			// D is outside FirstChar..LastChar, so it has the MissingWidth
			name:    "code outside the Widths array",
			content: "BT /F1 10 Tf 100 700 Td (AD) Tj ET",
			wantX:   100,
			wantW:   8.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := oneItem(t, extractWith(t, fonts, tt.content))
			if !near(item.X, tt.wantX) || !near(item.Width, tt.wantW) {
				t.Errorf("X = %v, Width = %v; want X = %v, Width = %v", item.X, item.Width, tt.wantX, tt.wantW)
			}
		})
	}
}

// Fonts without a Widths array get their widths from built-in metrics: the
// standard 14 fonts exactly, other fonts from the family they resemble
func TestTextWidth_FontsWithoutWidths(t *testing.T) {
	tests := []struct {
		name     string
		subtype  string
		baseFont string
		want     float64
	}{
		// H 722, e 556, l 222, l 222, o 556
		{"Helvetica", "/Type1", "/Helvetica", 22.78},
		{"Helvetica-Oblique", "/Type1", "/Helvetica-Oblique", 22.78},
		// H 722, e 556, l 278, l 278, o 611
		{"Helvetica-Bold", "/Type1", "/Helvetica-Bold", 24.45},
		// H 722, e 444, l 278, l 278, o 500
		{"Times-Roman", "/Type1", "/Times-Roman", 22.22},
		// H 778, e 444, l 278, l 278, o 500
		{"Times-Bold", "/Type1", "/Times-Bold", 22.78},
		{"Courier", "/Type1", "/Courier", 30},
		{"Arial has Helvetica metrics", "/TrueType", "/ArialMT", 22.78},
		{"Times New Roman has Times metrics", "/TrueType", "/TimesNewRomanPSMT", 22.22},
		{"Courier New is fixed pitch", "/TrueType", "/CourierNewPSMT", 30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fonts := map[string]map[string]interface{}{
				"F1": {"Subtype": tt.subtype, "BaseFont": tt.baseFont},
			}
			item := oneItem(t, extractWith(t, fonts, "BT /F1 10 Tf 100 700 Td (Hello) Tj ET"))
			if !near(item.Width, tt.want) {
				t.Errorf("Width = %v, want %v", item.Width, tt.want)
			}
		})
	}
}

// A Type0 font has 2-byte codes with Identity-H; its widths are the /W array
// and /DW of the descendant CIDFont, by CID
func TestTextWidth_Type0Font(t *testing.T) {
	fonts := map[string]map[string]interface{}{
		"F1": {
			"Subtype":  "/Type0",
			"BaseFont": "/AAAAAA+ArialMT",
			"Encoding": "/Identity-H",
			"DescendantFonts": []interface{}{
				map[string]interface{}{
					"Subtype": "/CIDFontType2",
					"DW":      1000.0,
					"W":       []interface{}{3.0, []interface{}{278.0}, 36.0, 38.0, 667.0},
				},
			},
		},
	}

	tests := []struct {
		name    string
		content string
		want    float64
	}{
		// CID 3 (278), 36 (667), 38 (667), 255 (DW 1000)
		{"W array and DW", "BT /F1 10 Tf 100 700 Td <00030024002600FF> Tj ET", 26.12},
		// Word spacing applies only to the single-byte code 32, never to a
		// 2-byte code that contains the byte 32
		{"no word spacing on 2-byte codes", "BT /F1 10 Tf 100 700 Td 5 Tw <00200003> Tj ET", 12.78},
		{"TJ", "BT /F1 10 Tf 100 700 Td [<0003> -1000 <0024>] TJ ET", 19.45},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := oneItem(t, extractWith(t, fonts, tt.content))
			if !near(item.Width, tt.want) {
				t.Errorf("Width = %v, want %v", item.Width, tt.want)
			}
		})
	}
}

// A Type3 font's widths are in glyph space, which /FontMatrix maps to text
// space
func TestTextWidth_Type3FontMatrix(t *testing.T) {
	fonts := map[string]map[string]interface{}{
		"F1": {
			"Subtype":    "/Type3",
			"FontMatrix": []interface{}{1.0 / 2048, 0.0, 0.0, -1.0 / 2048, 0.0, 0.0},
			"FirstChar":  65.0,
			"LastChar":   66.0,
			"Widths":     []interface{}{1024.0, 2048.0},
		},
	}
	item := oneItem(t, extractWith(t, fonts, "BT /F1 10 Tf 100 700 Td (AB) Tj ET"))
	if !near(item.Width, 15) {
		t.Errorf("Width = %v, want 15", item.Width)
	}
}

// Text in a Form XObject is placed by the form's /Matrix as well as the CTM
// at the point the form is drawn
func TestTextWidth_FormXObjectMatrix(t *testing.T) {
	e := NewTextExtractor(NewParser(nil))
	font := e.parseFont(testFont())
	font.Name = "F1"
	e.fonts["F1"] = font
	e.xobjects["Fm1"] = &Object{
		Dict: map[string]interface{}{
			"Subtype": "/Form",
			"Matrix":  []interface{}{2.0, 0.0, 0.0, 2.0, 0.0, 0.0},
		},
		Stream: []byte("BT /F1 10 Tf 50 300 Td (ABC) Tj ET"),
	}

	items, err := e.parseContentStream([]byte("q /Fm1 Do Q"), [4]float64{0, 0, 612, 792})
	if err != nil {
		t.Fatalf("parsing content: %v", err)
	}
	item := oneItem(t, items)
	if !near(item.X, 100) || !near(item.Y, 192) || !near(item.Width, 42) {
		t.Errorf("X = %v, Y = %v, Width = %v; want 100, 192, 42", item.X, item.Y, item.Width)
	}
}

/******************************************************************************
 * Copyright (c) 2026 Tenebris Technologies Inc.                              *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package pdf

import (
	"math"
	"testing"
)

// A standard 14 font with no /Encoding uses its built-in encoding
// (StandardEncoding), where code 0x27 is quoteright (222 in Helvetica);
// with WinAnsiEncoding it is quotesingle (191). Symbol has its own.
func TestStandardFontEncoding(t *testing.T) {
	tests := []struct {
		name string
		dict map[string]interface{}
		text string
		want float64
	}{
		{"built-in encoding", map[string]interface{}{"Subtype": "/Type1", "BaseFont": "/Helvetica"}, "(')", 2.22},
		{"WinAnsiEncoding", map[string]interface{}{"Subtype": "/Type1", "BaseFont": "/Helvetica", "Encoding": "/WinAnsiEncoding"}, "(')", 1.91},
		{"encoding dictionary based on WinAnsi", map[string]interface{}{
			"Subtype": "/Type1", "BaseFont": "/Helvetica",
			"Encoding": map[string]interface{}{"BaseEncoding": "/WinAnsiEncoding"},
		}, "(')", 1.91},
		// alpha in Symbol is 631
		{"Symbol", map[string]interface{}{"Subtype": "/Type1", "BaseFont": "/Symbol"}, "(a)", 6.31},
		// a1 in ZapfDingbats (code 0x21) is 974
		{"ZapfDingbats", map[string]interface{}{"Subtype": "/Type1", "BaseFont": "/ZapfDingbats"}, "(!)", 9.74},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fonts := map[string]map[string]interface{}{"F1": tt.dict}
			item := oneItem(t, extractWith(t, fonts, "BT /F1 10 Tf 100 700 Td "+tt.text+" Tj ET"))
			if !near(item.Width, tt.want) {
				t.Errorf("Width = %v, want %v", item.Width, tt.want)
			}
		})
	}
}

// A Type3 font without /Widths gets built-in widths in thousandths of an
// em, not scaled by its FontMatrix
func TestType3WithoutWidths(t *testing.T) {
	fonts := map[string]map[string]interface{}{"F1": {
		"Subtype":    "/Type3",
		"FontMatrix": []interface{}{1.0 / 2048, 0.0, 0.0, 1.0 / 2048, 0.0, 0.0},
	}}
	item := oneItem(t, extractWith(t, fonts, "BT /F1 10 Tf 100 700 Td (A) Tj ET"))
	if !near(item.Width, 6.67) {
		t.Errorf("Width = %v, want 6.67", item.Width)
	}
}

// An odd byte left at the end of a string in a 2-byte font gets no word
// spacing, even when it is 0x20
func TestTwoByteFontTrailingSpaceByte(t *testing.T) {
	fonts := map[string]map[string]interface{}{"F1": {
		"Subtype": "/Type0", "Encoding": "/Identity-H",
		"DescendantFonts": []interface{}{map[string]interface{}{
			"DW": 1000.0, "W": []interface{}{3.0, []interface{}{278.0}},
		}},
	}}
	// CID 3 (278), then the lone byte 0x20 (DW 1000)
	item := oneItem(t, extractWith(t, fonts, "BT /F1 10 Tf 100 700 Td 5 Tw <000320> Tj ET"))
	if !near(item.Width, 12.78) {
		t.Errorf("Width = %v, want 12.78", item.Width)
	}
}

// FirstChar and LastChar may be indirect objects
func TestFirstCharIndirect(t *testing.T) {
	e := NewTextExtractor(NewParser(nil))
	e.parser.objects[7] = &Object{Type: "number", Number: 65}
	e.parser.objects[8] = &Object{Type: "number", Number: 66}
	font := e.parseFont(map[string]interface{}{
		"Subtype":   "/TrueType",
		"FirstChar": &Reference{ObjectNum: 7},
		"LastChar":  &Reference{ObjectNum: 8},
		"Widths":    []interface{}{600.0, 700.0},
	})
	font.Name = "F1"
	e.fonts["F1"] = font
	items, err := e.parseContentStream([]byte("BT /F1 10 Tf 100 700 Td (B) Tj ET"), [4]float64{0, 0, 612, 792})
	if err != nil {
		t.Fatal(err)
	}
	if item := oneItem(t, items); !near(item.Width, 7) {
		t.Errorf("Width = %v, want 7", item.Width)
	}
}

// A Form XObject inherits the text state of the page that draws it
func TestFormXObjectInheritsTextState(t *testing.T) {
	e := NewTextExtractor(NewParser(nil))
	font := e.parseFont(testFont())
	font.Name = "F1"
	e.fonts["F1"] = font
	e.xobjects["Fm1"] = &Object{
		Dict:   map[string]interface{}{"Subtype": "/Form"},
		Stream: []byte("BT 100 700 Td (AB) Tj ET"),
	}
	items, err := e.parseContentStream([]byte("BT /F1 10 Tf 2 Tc ET q /Fm1 Do Q"), [4]float64{0, 0, 612, 792})
	if err != nil {
		t.Fatal(err)
	}
	// A 6 + 2, B 7 + 2
	if item := oneItem(t, items); item.Font != "F1" || !near(item.Width, 17) {
		t.Errorf("font %q, Width = %v; want F1, 17", item.Font, item.Width)
	}
}

// A Form /Matrix with an entry that is not a number is the identity
func TestFormXObjectMalformedMatrix(t *testing.T) {
	e := NewTextExtractor(NewParser(nil))
	font := e.parseFont(testFont())
	font.Name = "F1"
	e.fonts["F1"] = font
	e.xobjects["Fm1"] = &Object{
		Dict: map[string]interface{}{
			"Subtype": "/Form",
			"Matrix":  []interface{}{2.0, 0.0, 0.0, "/Bad", 0.0, 0.0},
		},
		Stream: []byte("BT /F1 10 Tf 50 300 Td (A) Tj ET"),
	}
	items, err := e.parseContentStream([]byte("/Fm1 Do"), [4]float64{0, 0, 612, 792})
	if err != nil {
		t.Fatal(err)
	}
	if item := oneItem(t, items); !near(item.X, 50) || !near(item.Width, 6) {
		t.Errorf("X = %v, Width = %v; want 50, 6", item.X, item.Width)
	}
}

// A space set wider than two ems with word spacing separates columns: the
// text either side becomes a separate item
func TestTjWideSpaceSplitsItems(t *testing.T) {
	fonts := map[string]map[string]interface{}{"F1": testFont()}
	items := extractWith(t, fonts, "BT /F1 10 Tf 100 700 Td 30 Tw (A A) Tj ET")
	// A 6, space 2.5 + 30
	if len(items) != 2 || items[0].Text != "A " || items[1].Text != "A" || !near(items[1].X, 138.5) {
		t.Fatalf("got %+v, want items \"A \" and \"A\" at 138.5", items)
	}
	if words := extractWith(t, fonts, "BT /F1 10 Tf 100 700 Td 5 Tw (A A) Tj ET"); len(words) != 1 {
		t.Errorf("got %d items with a normal word space, want 1", len(words))
	}
}

// Text drawn along a slanted or vertical baseline is marked as rotated
func TestRotatedText(t *testing.T) {
	fonts := map[string]map[string]interface{}{"F1": testFont()}
	slanted := oneItem(t, extractWith(t, fonts, "BT /F1 10 Tf 0.7071 0.7071 -0.7071 0.7071 100 100 Tm (AB) Tj ET"))
	upright := oneItem(t, extractWith(t, fonts, "BT /F1 10 Tf 100 700 Td (AB) Tj ET"))
	if !slanted.Rotated || upright.Rotated {
		t.Errorf("rotated = %v / %v, want true / false", slanted.Rotated, upright.Rotated)
	}
}

// The same in a TJ string
func TestTJWideSpaceSplitsItems(t *testing.T) {
	fonts := map[string]map[string]interface{}{"F1": testFont()}
	items := extractWith(t, fonts, "BT /F1 10 Tf 100 700 Td 30 Tw [(A A)] TJ ET")
	if len(items) != 2 || items[0].Text != "A " || items[1].Text != "A" || !near(items[1].X, 138.5) {
		t.Fatalf("got %+v, want items \"A \" and \"A\" at 138.5", items)
	}
}

// A Type0 font's codes are 2 bytes even when its ToUnicode CMap also
// declares a 1-byte codespace
func TestType0ToUnicodeWithOneByteCodespace(t *testing.T) {
	e := NewTextExtractor(NewParser(nil))
	font := e.parseFont(map[string]interface{}{
		"Subtype": "/Type0", "Encoding": "/Identity-H",
		"DescendantFonts": []interface{}{map[string]interface{}{"DW": 1000.0}},
	})
	font.ToUnicode = ParseCMap([]byte("2 begincodespacerange\n<00> <FF>\n<0100> <FFFF>\nendcodespacerange\n" +
		"3 beginbfchar\n<0000> <FFFD>\n<0024> <0041>\n<0026> <0043>\nendbfchar\n"))
	font.Name = "F1"
	e.fonts["F1"] = font
	if got := e.decodeText("\x00\x24\x00\x26", "F1"); got != "AC" {
		t.Errorf("decoded %q, want \"AC\"", got)
	}
}

// Text whose position is not a finite number (a degenerate matrix) or that
// lies far off the page is not extracted
func TestVisibleText(t *testing.T) {
	items := []TextItem{
		{X: 50, Y: 100, Width: 10, Height: 10, Text: "kept"},
		{X: math.Inf(1), Y: 100, Width: 10, Height: 10, Text: "inf"},
		{X: 50, Y: 100, Width: math.NaN(), Height: 10, Text: "nan"},
		{X: 1e9, Y: 100, Width: 10, Height: 10, Text: "far"},
		{X: 50, Y: -5000, Width: 10, Height: 10, Text: "above"},
	}
	got := visibleText(items, [4]float64{0, 0, 612, 792})
	if len(got) != 1 || got[0].Text != "kept" {
		t.Errorf("got %+v, want only \"kept\"", got)
	}
}

// Text in a Form XObject whose BBox is smaller than the page is placed on the
// page like the page's own text: Y is flipped about the page height
func TestFormXObjectYOnPage(t *testing.T) {
	e := NewTextExtractor(NewParser(nil))
	font := e.parseFont(testFont())
	font.Name = "F1"
	e.fonts["F1"] = font
	e.xobjects["Fm1"] = &Object{
		Dict: map[string]interface{}{
			"Subtype": "/Form",
			"BBox":    []interface{}{0.0, 0.0, 100.0, 50.0},
		},
		Stream: []byte("BT /F1 10 Tf 50 300 Td (A) Tj ET"),
	}
	items, err := e.parseContentStream([]byte("/Fm1 Do"), [4]float64{0, 0, 612, 792})
	if err != nil {
		t.Fatal(err)
	}
	if item := oneItem(t, items); !near(item.Y, 492) {
		t.Errorf("Y = %v, want 492", item.Y)
	}
}

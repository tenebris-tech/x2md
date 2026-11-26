/******************************************************************************
 * Copyright (c) 2026 Tenebris Technologies Inc.                              *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package pdf

import "strings"

// maxCIDRange is the largest CID range a /W entry may cover, so that a
// malformed array cannot allocate without bound
const maxCIDRange = 0xFFFF

var courier = func() (widths [224]uint16) {
	for i := range widths {
		widths[i] = 600
	}
	return widths
}()

// defaultFont measures text whose font is not declared in the resources
var defaultFont = &Font{std: &helvetica, widthScale: 0.001}

// loadWidths sets a font's width metrics from its font dictionary
// (ISO 32000-1 §9.6.2, §9.6.5, §9.7.4.3)
func (e *TextExtractor) loadWidths(font *Font, dict map[string]interface{}) {
	font.widthScale = 0.001

	switch strings.TrimPrefix(nameValue(dict["Subtype"]), "/") {
	case "Type0":
		e.loadCIDWidths(font, dict)
		return
	case "Type3":
		// Type3 widths are in glyph space, which FontMatrix maps to text
		// space
		if m := e.resolveArray(dict["FontMatrix"]); len(m) > 0 {
			if a, ok := e.resolveNumber(m[0]); ok && a != 0 {
				font.widthScale = a
			}
		}
	}

	if desc := e.resolveDict(dict["FontDescriptor"]); desc != nil {
		if w, ok := e.resolveNumber(desc["MissingWidth"]); ok {
			font.missingWidth = w
		}
	}

	for i, v := range e.resolveArray(dict["Widths"]) {
		if w, ok := e.resolveNumber(v); ok {
			font.Widths[font.FirstChar+i] = w
		}
	}

	// Without a Widths array (the standard 14 fonts may omit it), use the
	// built-in metrics of the font or of the family it resembles
	if len(font.Widths) == 0 {
		font.std = standardWidths(font.BaseFont, e.encodingName(dict))
	}
}

// loadCIDWidths sets the widths of a Type0 font from its descendant CIDFont:
// /W by CID, /DW (default 1000) for every other CID
func (e *TextExtractor) loadCIDWidths(font *Font, dict map[string]interface{}) {
	font.twoByte = true
	font.missingWidth = 1000

	descendants := e.resolveArray(dict["DescendantFonts"])
	if len(descendants) == 0 {
		return
	}
	cidFont := e.resolveDict(descendants[0])
	if cidFont == nil {
		return
	}
	if dw, ok := e.resolveNumber(cidFont["DW"]); ok {
		font.missingWidth = dw
	}

	// With Identity-H or Identity-V the 2-byte code is the CID. Other CMaps
	// map codes to different CIDs, which needs the CMap itself; those fonts
	// keep the default width for every code.
	encoding := strings.TrimPrefix(nameValue(dict["Encoding"]), "/")
	if encoding != "Identity-H" && encoding != "Identity-V" {
		return
	}

	w := e.resolveArray(cidFont["W"])
	for i := 0; i+1 < len(w); {
		first, ok := e.resolveNumber(w[i])
		if !ok {
			return
		}
		// c [w1 w2 ...]: consecutive CIDs from c
		if list := e.resolveArray(w[i+1]); list != nil {
			for j, v := range list {
				if width, ok := e.resolveNumber(v); ok {
					font.Widths[int(first)+j] = width
				}
			}
			i += 2
			continue
		}
		// cfirst clast w: one width for a range of CIDs
		if i+2 >= len(w) {
			return
		}
		last, ok1 := e.resolveNumber(w[i+1])
		width, ok2 := e.resolveNumber(w[i+2])
		if !ok1 || !ok2 || last < first || last-first > maxCIDRange {
			return
		}
		for cid := int(first); cid <= int(last); cid++ {
			font.Widths[cid] = width
		}
		i += 3
	}
}

// standardWidths returns the built-in widths for a font by its base font
// name and encoding: the standard 14 fonts exactly, and for any other font
// the metrics of the family it resembles (Arial and other sans fonts
// Helvetica, serif fonts Times, monospaced fonts Courier). The tables are by
// character code, so they follow the encoding: WinAnsiEncoding (and, for its
// ASCII range, MacRomanEncoding) or the font's built-in encoding, which is
// StandardEncoding for the text fonts. Symbol and ZapfDingbats have only
// their own.
func standardWidths(baseFont, encoding string) *[224]uint16 {
	name := strings.ToLower(baseFont)
	if i := strings.IndexByte(name, '+'); i >= 0 {
		name = name[i+1:]
	}
	switch {
	case strings.Contains(name, "symbol"):
		return &symbolBuiltin
	case strings.Contains(name, "dingbats"):
		return &zapfDingbatsBuiltin
	}

	winAnsi := encoding == "WinAnsiEncoding" || encoding == "MacRomanEncoding"
	pick := func(byWinAnsi, byBuiltin *[224]uint16) *[224]uint16 {
		if winAnsi {
			return byWinAnsi
		}
		return byBuiltin
	}
	bold := strings.Contains(name, "bold")
	italic := strings.Contains(name, "italic") || strings.Contains(name, "oblique")

	switch {
	case strings.Contains(name, "courier") || strings.Contains(name, "mono"):
		return pick(&courier, &courierBuiltin)
	case strings.Contains(name, "times") || strings.Contains(name, "roman") ||
		(strings.Contains(name, "serif") && !strings.Contains(name, "sans")):
		switch {
		case bold && italic:
			return pick(&timesBoldItalic, &timesBoldItalicBuiltin)
		case bold:
			return pick(&timesBold, &timesBoldBuiltin)
		case italic:
			return pick(&timesItalic, &timesItalicBuiltin)
		}
		return pick(&timesRoman, &timesRomanBuiltin)
	case bold:
		return pick(&helveticaBold, &helveticaBoldBuiltin)
	}
	return pick(&helvetica, &helveticaBuiltin)
}

// encodingName returns the name of a simple font's encoding: the /Encoding
// name, or the /BaseEncoding of an encoding dictionary; "" for the font's
// built-in encoding
func (e *TextExtractor) encodingName(dict map[string]interface{}) string {
	if name := nameValue(dict["Encoding"]); name != "" {
		return strings.TrimPrefix(name, "/")
	}
	if enc := e.resolveDict(dict["Encoding"]); enc != nil {
		return strings.TrimPrefix(nameValue(enc["BaseEncoding"]), "/")
	}
	return ""
}

// glyphWidth returns the width of the glyph for a character code in text
// space units for a font size of 1
func (f *Font) glyphWidth(code int) float64 {
	if w, ok := f.Widths[code]; ok {
		return w * f.widthScale
	}
	if f.std != nil && code >= 32 && code-32 < len(f.std) && f.std[code-32] != 0 {
		// Built-in widths are in thousandths of an em whatever the font's
		// matrix
		return float64(f.std[code-32]) / 1000
	}
	return f.missingWidth * f.widthScale
}

// textAdvance returns how far showing text moves the text position, in
// text space (ISO 32000-1 §9.4.4): per glyph (w0 * Tfs + Tc + Tw) * Th, where
// Tw applies only to the single-byte code 32
func (e *TextExtractor) textAdvance(text string, gs *GraphicsState) float64 {
	font, ok := e.fonts[gs.FontName]
	if !ok {
		font = defaultFont
	}
	codeLen := 1
	if font.twoByte {
		codeLen = 2
	}

	var advance float64
	for i := 0; i < len(text); i += codeLen {
		n := min(codeLen, len(text)-i)
		code := 0
		for j := 0; j < n; j++ {
			code = code<<8 | int(text[i+j])
		}
		advance += font.glyphWidth(code)*gs.FontSize + gs.CharSpacing
		// Word spacing applies to the single-byte code 32, never to a byte
		// of a 2-byte font, even an odd byte left at the end
		if !font.twoByte && code == ' ' {
			advance += gs.WordSpacing
		}
	}
	return advance * gs.HorizScaling / 100
}

// nameValue returns a PDF name operand as a string, or "" if it is not one
func nameValue(v interface{}) string {
	s, _ := v.(string)
	return s
}

// resolve follows an indirect reference to the value it names
func (e *TextExtractor) resolve(v interface{}) interface{} {
	ref, ok := v.(*Reference)
	if !ok {
		return v
	}
	obj, err := e.parser.GetObject(ref.ObjectNum)
	if err != nil {
		return nil
	}
	switch obj.Type {
	case "dict":
		return obj.Dict
	case "array":
		return obj.Array
	case "number":
		return obj.Number
	}
	if obj.Dict != nil {
		return obj.Dict
	}
	return nil
}

func (e *TextExtractor) resolveDict(v interface{}) map[string]interface{} {
	d, _ := e.resolve(v).(map[string]interface{})
	return d
}

func (e *TextExtractor) resolveArray(v interface{}) []interface{} {
	a, _ := e.resolve(v).([]interface{})
	return a
}

func (e *TextExtractor) resolveNumber(v interface{}) (float64, bool) {
	switch n := e.resolve(v).(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	}
	return 0, false
}

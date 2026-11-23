/******************************************************************************
 * Copyright (c) 2026 Tenebris Technologies Inc.                              *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package pdf

import "testing"

// glyphItems lays out words one glyph per item, 6 units per glyph at font
// size 12, with a 3.3 unit gap (a typical word space) between words
func glyphItems(y float64, words ...string) []TextItem {
	var items []TextItem
	x := 72.0
	for _, word := range words {
		for _, r := range word {
			items = append(items, TextItem{X: x, Y: y, Width: 6, Height: 12, Text: string(r), Font: "F0"})
			x += 6
		}
		x += 3.3
	}
	return items
}

func texts(items []TextItem) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.Text
	}
	return out
}

func TestMergeGlyphRuns_JoinsGlyphsAndRestoresSpaces(t *testing.T) {
	items := append(glyphItems(100, "The", "dog's", "vacation"), glyphItems(114, "was", "approved")...)
	got := texts(mergeGlyphRuns(items))
	want := []string{"The dog's vacation", "was approved"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMergeGlyphRuns_WidthCoversJoinedText(t *testing.T) {
	merged := mergeGlyphRuns(glyphItems(100, "abcdefghij", "klmnopqrst"))
	if len(merged) != 1 {
		t.Fatalf("expected one item, got %q", texts(merged))
	}
	// 20 glyphs of 6 units plus one 3.3 unit gap
	if w := merged[0].Width; w < 123.2 || w > 123.4 {
		t.Errorf("width = %v, want 123.3", w)
	}
}

func TestMergeGlyphRuns_ColumnGapIsNotASpace(t *testing.T) {
	left := glyphItems(100, "Controlnumber", "one")
	right := glyphItems(100, "Ownerofthecontrol")
	for i := range right {
		right[i].X += 300
	}
	got := texts(mergeGlyphRuns(append(left, right...)))
	if len(got) != 2 || got[0] != "Controlnumber one" || got[1] != "Ownerofthecontrol" {
		t.Errorf("got %q, want two separate items", got)
	}
}

func TestMergeGlyphRuns_LeavesWordPerItemPagesAlone(t *testing.T) {
	// A normal page draws words or lines per operation. Short fragments on
	// such a page (brackets, digits, hyphenated parts) must stay separate,
	// because table and reference detection depend on them.
	var items []TextItem
	x := 72.0
	for i := 0; i < 30; i++ {
		items = append(items, TextItem{X: x, Y: 100, Width: 40, Height: 12, Text: "Evaluation", Font: "F0"})
		x += 43
	}
	items = append(items,
		TextItem{X: x, Y: 100, Width: 3, Height: 12, Text: "[", Font: "F0"},
		TextItem{X: x + 3, Y: 100, Width: 6, Height: 12, Text: "1", Font: "F0"},
		TextItem{X: x + 9, Y: 100, Width: 3, Height: 12, Text: "]", Font: "F0"},
	)
	got := mergeGlyphRuns(items)
	if len(got) != len(items) {
		t.Errorf("expected %d items unchanged, got %d: %q", len(items), len(got), texts(got))
	}
}

func TestMergeGlyphRuns_DifferentFontOrSizeNotJoined(t *testing.T) {
	items := glyphItems(100, "Headingtextthatislongenough", "bodytextthatfollows")
	for i := 27; i < len(items); i++ {
		items[i].Font = "F1"
	}
	got := texts(mergeGlyphRuns(items))
	if len(got) != 2 {
		t.Errorf("expected a break at the font change, got %q", got)
	}
}

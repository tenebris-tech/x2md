/******************************************************************************
 * Copyright (c) 2026 Tenebris Technologies Inc.                              *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package transform

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/tenebris-tech/x2md/pdf2md/models"
)

func item11(x, y, w float64, text string) *models.TextItem {
	return &models.TextItem{X: x, Y: y, Width: w, Height: 11, Text: text}
}

// tableTexts returns the cells of the table rows among line groups
func tableTexts(groups []lineGroup) [][]string {
	c := NewCompactLines()
	var rows [][]string
	for _, g := range groups {
		if g.isTableRow {
			rows = append(rows, c.extractColumnTexts(g.items, g.columns, g.extents))
		}
	}
	return rows
}

func asInterfaces(items []*models.TextItem) []interface{} {
	out := make([]interface{}, len(items))
	for i, item := range items {
		out[i] = item
	}
	return out
}

// Lines of running text drawn as several items (a font change, justified
// fragments) separated by word spaces are not table rows, even where the
// items start at the same places on several lines
func TestRunningTextIsNotATable(t *testing.T) {
	var items []*models.TextItem
	for i := 0; i < 6; i++ {
		y := 100 + 13*float64(i)
		items = append(items,
			item11(72, y, 67.7, "The evaluator "),
			item11(139.7, y, 60, "shall examine"),
			item11(202, y, 300, "the presentation of the interfaces to determine that"))
	}
	if rows := tableTexts(NewCompactLines().groupByLineWithTableDetection(asInterfaces(items), nil, 13, 700)); len(rows) != 0 {
		t.Errorf("got table rows %q, want none", rows)
	}
}

// Numbered headings line up (number, title) but the paragraphs between them
// run across the columns: they are not a table
func TestNumberedHeadingsAreNotATable(t *testing.T) {
	var items []*models.TextItem
	y := 100.0
	for i := 0; i < 4; i++ {
		items = append(items, item11(72, y, 42.8, "9.4.5.2."+string(rune('1'+i))), item11(129.6, y, 120, "Heading title"))
		y += 25
		for j := 0; j < 3; j++ {
			items = append(items, item11(72, y, 450, "Running text of a paragraph under the heading that fills the line"))
			y += 13
		}
		y += 10
	}
	if rows := tableTexts(NewCompactLines().groupByLineWithTableDetection(asInterfaces(items), nil, 13, 700)); len(rows) != 0 {
		t.Errorf("got table rows %q, want none", rows)
	}
}

// Leader dots in a table of contents are not cell content: a contents line
// is its title and its page number
func TestContentsLeadersAreNotCells(t *testing.T) {
	var items []*models.TextItem
	for i, title := range []string{"Terminology", "Introduction", "Scope", "References"} {
		y := 100 + 14*float64(i)
		items = append(items,
			item11(72, y, 60, title),
			item11(140, y, 300, strings.Repeat(".", 60)),
			item11(450, y, 11, "1"+string(rune('0'+i))))
	}
	rows := tableTexts(NewCompactLines().groupByLineWithTableDetection(asInterfaces(items), nil, 14, 700))
	want := [][]string{{"Terminology", "10"}, {"Introduction", "11"}, {"Scope", "12"}, {"References", "13"}}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("rows = %q, want %q", rows, want)
	}
}

// The line of column labels directly above aligned rows is their header, and
// right-aligned values go in the column whose text they end with
func TestAlignedTableHeaderAndRightAlignedValues(t *testing.T) {
	items := []*models.TextItem{item11(27.6, 195, 24, "What"), item11(191.9, 195, 54, "Date / Time"), item11(356.9, 195, 42, "Hours old")}
	// The first date is right-aligned in its column, the others start at it
	items = append(items, item11(27.6, 210, 64, "Servers' config"), item11(273.1, 210, 79.9, "10/20/2025 18:00"), item11(392.7, 210, 11, "21"))
	values := [][3]string{{"DB1 data", "251021-095902", "4"}, {"DB2 data", "251021-132901", "0.5"}, {"SMFU data", "251021-121900", "1.5"}}
	for i, v := range values {
		y := 224 + 14*float64(i)
		w := 5.5 * float64(len(v[2]))
		items = append(items, item11(27.6, y, 40, v[0]), item11(191.9, y, 69.7, v[1]), item11(403.7-w, y, w, v[2]))
	}
	// A second block with dates in the same right-aligned position
	items = append(items,
		item11(27.6, 300, 30, "Phase"), item11(191.9, 300, 54, "Date / Time"), item11(356.9, 300, 42, "Hours ET"),
		item11(27.6, 314, 150, "Start bootup of internal servers"), item11(273.1, 314, 79.9, "10/21/2025 20:35"),
		item11(27.6, 328, 150, "Ready to begin data processing"), item11(273.1, 328, 79.9, "10/21/2025 21:18"))
	rows := tableTexts(NewCompactLines().groupByLineWithTableDetection(asInterfaces(items), nil, 14, 700))
	want := [][]string{
		{"What", "Date / Time", "Hours old"}, {"Servers' config", "10/20/2025 18:00", "21"},
		{"DB1 data", "251021-095902", "4"}, {"DB2 data", "251021-132901", "0.5"}, {"SMFU data", "251021-121900", "1.5"},
		{"Phase", "Date / Time", "Hours ET"},
		{"Start bootup of internal servers", "10/21/2025 20:35", ""},
		{"Ready to begin data processing", "10/21/2025 21:18", ""},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("rows = %q, want %q", rows, want)
	}
}

// In a narrow justified column, a line of two long words is spread to both
// margins. Lines like that line up left and right, but their only
// boundaries are the paragraph's margins: they are not a table.
func TestJustifiedTwoWordLinesAreNotATable(t *testing.T) {
	words := []string{"Internationalization", "Responsibilities", "Telecommunications", "Infrastructure",
		"Characteristically", "Unrecognizable", "Administratively", "Interoperability", "Acknowledgements"}
	var items []*models.TextItem
	for i := 0; i < 24; i++ {
		y := 100 + 13*float64(i)
		first, second := words[i%len(words)], words[(i*4+3)%len(words)]
		w1, w2 := 2.8*float64(len(first)), 2.8*float64(len(second))
		if i%4 == 3 {
			// a line of three short words, also spread to both margins
			items = append(items, item11(72, y, 30, "and"), item11(127, y, 30, "the"), item11(182, y, 30, "for"))
			continue
		}
		items = append(items, item11(72, y, w1, first), item11(212-w2, y, w2, second))
	}
	if rows := tableTexts(NewCompactLines().groupByLineWithTableDetection(asInterfaces(items), nil, 13, 700)); len(rows) != 0 {
		t.Errorf("got table rows %q, want none", rows)
	}
}

// A title on its own line between two groups of rows ends the first table;
// the second group is a table of its own, under its own header
func TestTitleBetweenTablesSplitsThem(t *testing.T) {
	items := []*models.TextItem{
		item11(27.6, 195, 24, "What"), item11(191.9, 195, 54, "Date / Time"), item11(356.9, 195, 42, "Hours old"),
		item11(27.6, 210, 64, "Servers' config"), item11(273.1, 210, 79.9, "10/20/2025 18:00"), item11(392.7, 210, 11, "21"),
		item11(27.6, 224, 40, "DB1 data"), item11(191.9, 224, 69.7, "251021-095902"), item11(398.2, 224, 5.5, "4"),
		item11(27.6, 238, 40, "DB2 data"), item11(191.9, 238, 69.7, "251021-132901"), item11(389.9, 238, 13.8, "0.5"),
		// a title centred over the second group
		item11(181.6, 255, 69, "Recovery Times"),
		item11(27.6, 270, 30, "Phase"), item11(191.9, 270, 54, "Date / Time"), item11(356.9, 270, 42, "Hours ET"),
		item11(27.6, 284, 150, "Start bootup of internal servers"), item11(273.1, 284, 79.9, "10/21/2025 20:35"),
		item11(27.6, 298, 150, "Ready to begin data processing"), item11(273.1, 298, 79.9, "10/21/2025 21:18"),
	}
	groups := NewCompactLines().groupByLineWithTableDetection(asInterfaces(items), nil, 14, 700)
	var got []string
	c := NewCompactLines()
	for _, g := range groups {
		switch {
		case g.isTableRow && g.isHeader:
			got = append(got, "header:"+strings.Join(c.extractColumnTexts(g.items, g.columns, g.extents), "|"))
		case g.isTableRow:
			got = append(got, "row:"+strings.Join(c.extractColumnTexts(g.items, g.columns, g.extents), "|"))
		default:
			got = append(got, "text:"+g.items[0].Text)
		}
	}
	want := []string{
		"header:What|Date / Time|Hours old",
		"row:Servers' config|10/20/2025 18:00|21",
		"row:DB1 data|251021-095902|4",
		"row:DB2 data|251021-132901|0.5",
		"text:Recovery Times",
		"header:Phase|Date / Time|Hours ET",
		"row:Start bootup of internal servers|10/21/2025 20:35|",
		"row:Ready to begin data processing|10/21/2025 21:18|",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("lines =\n%q\nwant\n%q", got, want)
	}
}

// A cell whose only content is dots ("…", "...") keeps it: dots are leaders
// only between a title and its page number
func TestDotsAloneAreACellValue(t *testing.T) {
	c := NewCompactLines()
	columns := []float64{50, 250}
	items := []*models.TextItem{item11(50, 100, 40, "Status"), item11(250, 100, 8, "…")}
	if got := c.extractColumnTexts(items, columns, nil); !reflect.DeepEqual(got, []string{"Status", "…"}) {
		t.Errorf("cells = %q, want [Status …]", got)
	}
	items = []*models.TextItem{item11(50, 100, 40, "Pending"), item11(250, 100, 12, "...")}
	if got := c.extractColumnTexts(items, columns, nil); !reflect.DeepEqual(got, []string{"Pending", "..."}) {
		t.Errorf("cells = %q, want [Pending ...]", got)
	}
}

// Items with no height on one baseline are one line: they take the typical
// height of the text around them
func TestZeroHeightItemsOnOneLine(t *testing.T) {
	items := []*models.TextItem{
		{X: 50, Y: 100, Width: 30, Height: 10, Text: "measured"},
		{X: 50, Y: 120, Width: 30, Height: 0, Text: "first"},
		{X: 90, Y: 120.5, Width: 30, Height: 0, Text: "second"},
	}
	want := [][]string{{"measured"}, {"first", "second"}}
	if got := lineTexts(visualLines(items)); !reflect.DeepEqual(got, want) {
		t.Errorf("lines = %q, want %q", got, want)
	}
}

// Text with a position or size that is not a finite number, or far off the
// page, is not visible text; an item without a height takes the page's
// typical height
func TestVisibleItems(t *testing.T) {
	page := &models.Page{Width: 612, Height: 792, Items: []interface{}{
		&models.TextItem{X: 50, Y: 100, Width: 30, Height: 10, Text: "kept"},
		&models.TextItem{X: math.Inf(1), Y: 300, Width: 10, Height: 10, Text: "inf"},
		&models.TextItem{X: 50, Y: math.NaN(), Width: 10, Height: 10, Text: "nan"},
		&models.TextItem{X: 1e9, Y: 300, Width: 10, Height: 10, Text: "far"},
		&models.TextItem{X: 90, Y: 100, Width: 30, Height: 0, Text: "flat"},
	}}
	var got []string
	for _, item := range visibleItems(page) {
		ti := item.(*models.TextItem)
		got = append(got, ti.Text)
		if ti.Height != 10 {
			t.Errorf("%q has height %v, want 10", ti.Text, ti.Height)
		}
	}
	if !reflect.DeepEqual(got, []string{"kept", "flat"}) {
		t.Errorf("items = %q, want [kept flat]", got)
	}
}

// The gutter is found by sweeping item edges, so a coordinate far off the
// page does not make the search step across it
func TestFindGutterFarItem(t *testing.T) {
	var items []*models.TextItem
	for i := 0; i < 12; i++ {
		y := 100 + 14*float64(i)
		items = append(items, item11(50, y, 100, "left column"), item11(300, y, 100, "right column"))
	}
	items = append(items, item11(1e9, 300, 10, "far"))
	// the middle third of 50..1e9+10 holds nothing; the call must return
	if x := findGutter(items, 50, 1e9+10); x <= 400 {
		t.Errorf("gutter = %v, want a position right of the text", x)
	}
	if x := findGutter(items[:24], 50, 400); x <= 150 || x >= 300 {
		t.Errorf("gutter = %v, want between the columns", x)
	}
}

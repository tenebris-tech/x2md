/******************************************************************************
 * Copyright (c) 2026 Tenebris Technologies Inc.                              *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package transform

import (
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

/******************************************************************************
 * Copyright (c) 2026 Tenebris Technologies Inc.                              *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package transform

import (
	"reflect"
	"testing"

	"github.com/tenebris-tech/x2md/pdf2md/models"
)

func lineTexts(lines [][]*models.TextItem) [][]string {
	var out [][]string
	for _, line := range lines {
		var texts []string
		for _, item := range line {
			texts = append(texts, item.Text)
		}
		out = append(out, texts)
	}
	return out
}

func TestVisualLines(t *testing.T) {
	item := func(x, y, h float64, text string) *models.TextItem {
		return &models.TextItem{X: x, Y: y, Width: 30, Height: h, Text: text}
	}
	tests := []struct {
		name  string
		items []*models.TextItem
		want  [][]string
	}{
		{
			name:  "baselines 0.1 apart are one line",
			items: []*models.TextItem{item(100, 100.05, 10, "b"), item(50, 99.95, 10, "a")},
			want:  [][]string{{"a", "b"}},
		},
		{
			// The mark is 6 high and 3.5 above the baseline of 10 high text
			name:  "superscript joins its line",
			items: []*models.TextItem{item(50, 100, 10, "word"), item(80, 96.5, 6, "1"), item(90, 100, 10, "more")},
			want:  [][]string{{"word", "1", "more"}},
		},
		{
			name:  "next line is a new line",
			items: []*models.TextItem{item(50, 100, 10, "one"), item(50, 111, 10, "two")},
			want:  [][]string{{"one"}, {"two"}},
		},
		{
			// Two page columns with offset baselines: a 10 high line between
			// two 9 high lines 10.8 apart does not chain them into one line
			name: "offset columns do not chain",
			items: []*models.TextItem{
				item(312, 301.7, 9, "right1"), item(57, 306, 10, "left"), item(312, 312.5, 9, "right2"),
			},
			want: [][]string{{"right1"}, {"left"}, {"right2"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lineTexts(visualLines(tt.items)); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("lines = %q, want %q", got, tt.want)
			}
		})
	}
}

// A bucket boundary used to split rows whose cells are a fraction of a unit
// apart: with a line distance of 12, Y 101.9 and 102.1 fell in different
// buckets
func TestDetectAlignedTables_RowsAcrossBucketBoundary(t *testing.T) {
	var items []*models.TextItem
	for i, row := range [][2]string{{"Name", "Value"}, {"Alpha", "100"}, {"Beta", "200"}} {
		y := 102 + 12*float64(i)
		items = append(items, tableItem(50, y-0.1, row[0]), tableItem(150, y+0.1, row[1]))
	}
	if regions := NewCompactLines().detectAlignedTables(items, 700); len(regions) != 1 {
		t.Errorf("detected %d table region(s), want 1", len(regions))
	}
}

// A borderless table whose cells are centred against each other: the left
// cell's four lines and the right cell's three lines are half a line off.
// Both cells belong to one row.
func TestTableRows_OffsetCellsAreOneRow(t *testing.T) {
	line := func(x, y float64, text string) *models.TextItem {
		return &models.TextItem{X: x, Y: y, Width: 90, Height: 8, Text: text}
	}
	var items []interface{}
	items = append(items, line(43, 104.5, "Address:"), line(225, 104.5, "Activities:"))
	for i, top := range []float64{158.3, 276.4} {
		for j := 0; j < 4; j++ {
			items = append(items, line(43, top+11.2*float64(j), []string{"Office", "Street", "City", "Country"}[j]+string(rune('A'+i))))
		}
		for j := 0; j < 3; j++ {
			items = append(items, line(243, top+5.3+11.7*float64(j), []string{"• Control", "• Operations", "• Facilities"}[j]+string(rune('A'+i))))
		}
	}

	groups := NewCompactLines().groupByLineWithTableDetection(items, nil, 12, 700)
	var got [][]string
	for _, g := range groups {
		if !g.isTableRow {
			t.Fatalf("line %q is not a table row", g.items[0].Text)
		}
		got = append(got, NewCompactLines().extractColumnTexts(g.items, g.columns, g.extents))
	}
	want := [][]string{
		{"Address:", "Activities:"},
		{"OfficeA StreetA CityA CountryA", "• ControlA • OperationsA • FacilitiesA"},
		{"OfficeB StreetB CityB CountryB", "• ControlB • OperationsB • FacilitiesB"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rows = %q, want %q", got, want)
	}
}

// Two columns of running text on a page are read column by column, never
// paired into table rows, even where their lines are offset
func TestTwoColumnPageIsNotATable(t *testing.T) {
	var items []interface{}
	for i := 0; i < 12; i++ {
		y := 100 + 15*float64(i)
		items = append(items,
			&models.TextItem{X: 57, Y: y, Width: 222, Height: 10, Text: "left column running text line"},
			&models.TextItem{X: 297, Y: y + 4, Width: 222, Height: 10, Text: "continues the right column here"},
		)
	}
	groups := NewCompactLines().groupByLineWithTableDetection(items, nil, 15, 700)
	if len(groups) != 24 {
		t.Fatalf("got %d lines, want 24", len(groups))
	}
	for i, g := range groups {
		if g.isTableRow {
			t.Fatalf("line %d is a table row", i)
		}
		wantLeft := i < 12
		if (g.items[0].X < 200) != wantLeft {
			t.Fatalf("line %d is %q, want the left column first", i, g.items[0].Text)
		}
	}
}

// Horizontal rules drawn between the rows of a text table decide its rows:
// the lines between two rules are one row, even where both cells wrap
func TestRowsBetweenRules(t *testing.T) {
	c := NewCompactLines()
	columns := []float64{50, 250}
	var items []*models.TextItem
	for i, row := range [][2][]string{
		{{"100 First Street", "San Francisco"}, {"Headquarters:", "Security, Legal"}},
		{{"1 Second Street", "Seattle"}, {"Engineering:", "Platform"}},
	} {
		top := 100 + 40*float64(i)
		for j := 0; j < 2; j++ {
			y := top + 12 + 12*float64(j)
			items = append(items, tableItem(50, y, row[0][j]), tableItem(250, y, row[1][j]))
		}
	}
	rules := []models.Rule{
		{X1: 40, Y1: 100, X2: 400, Y2: 100},
		{X1: 40, Y1: 140, X2: 400, Y2: 140},
		{X1: 40, Y1: 180, X2: 400, Y2: 180},
	}
	rows := c.rowsBetweenRules(items, columns, nil, rules)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	got := c.extractColumnTexts(rows[0], columns, nil)
	want := []string{"100 First Street San Francisco", "Headquarters: Security, Legal"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("first row = %q, want %q", got, want)
	}
}

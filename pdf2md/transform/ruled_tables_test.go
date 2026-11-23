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

func hRule(y, x1, x2 float64) models.Rule { return models.Rule{X1: x1, Y1: y, X2: x2, Y2: y} }
func vRule(x, y1, y2 float64) models.Rule { return models.Rule{X1: x, Y1: y1, X2: x, Y2: y2} }

// gridRules draws a full grid: a horizontal rule at every y and a vertical
// rule at every x
func gridRules(xs, ys []float64) []models.Rule {
	var rules []models.Rule
	for _, y := range ys {
		rules = append(rules, hRule(y, xs[0], xs[len(xs)-1]))
	}
	for _, x := range xs {
		rules = append(rules, vRule(x, ys[0], ys[len(ys)-1]))
	}
	return rules
}

// cellItem is a 10 point text item whose glyphs sit inside the cell whose
// top left corner is (x, y)
func cellItem(x, y float64, text string) *models.TextItem {
	return &models.TextItem{X: x + 4, Y: y + 14, Width: 6 * float64(len(text)), Height: 10, Text: text, Font: "F1"}
}

func TestFindRuledGrids(t *testing.T) {
	tests := []struct {
		name     string
		rules    []models.Rule
		wantXs   []float64
		wantYs   []float64
		wantNone bool
	}{
		{
			name:   "2x2 grid",
			rules:  gridRules([]float64{100, 200, 300}, []float64{100, 120, 140}),
			wantXs: []float64{100, 200, 300},
			wantYs: []float64{100, 120, 140},
		},
		{
			// Ends up to 1.5 units short of, or past, the rules they meet
			name: "near-miss corners snap",
			rules: []models.Rule{
				hRule(100, 101.5, 298.5), hRule(120.8, 100, 300), hRule(140, 99, 301),
				vRule(100, 101, 139), vRule(200.6, 100, 140), vRule(299.2, 98.5, 141.5),
			},
			wantXs: []float64{100, 200.6, 299.2},
			wantYs: []float64{100, 120.8, 140},
		},
		{
			name:     "single box",
			rules:    gridRules([]float64{100, 300}, []float64{100, 200}),
			wantNone: true,
		},
		{
			name:     "box split by one horizontal rule",
			rules:    gridRules([]float64{100, 300}, []float64{100, 150, 200}),
			wantNone: true,
		},
		{
			name:     "box split by one vertical rule",
			rules:    gridRules([]float64{100, 200, 300}, []float64{100, 200}),
			wantNone: true,
		},
		{
			name:     "lone separators and an underline",
			rules:    []models.Rule{hRule(100, 72, 540), hRule(400, 72, 540), hRule(300, 100, 160)},
			wantNone: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grids := findRuledGrids(tt.rules)
			if tt.wantNone {
				if len(grids) != 0 {
					t.Fatalf("got %d grids, want none", len(grids))
				}
				return
			}
			if len(grids) != 1 {
				t.Fatalf("got %d grids, want 1", len(grids))
			}
			if !reflect.DeepEqual(grids[0].xs, tt.wantXs) || !reflect.DeepEqual(grids[0].ys, tt.wantYs) {
				t.Errorf("xs = %v, ys = %v; want %v, %v", grids[0].xs, grids[0].ys, tt.wantXs, tt.wantYs)
			}
		})
	}
}

// A cell with a missing interior boundary spans several units; its text
// goes in its first column and row
func TestRuledTable_MergedCells(t *testing.T) {
	xs := []float64{100, 200, 300}
	ys := []float64{100, 120, 140, 160}
	rules := []models.Rule{
		// Outer box and all row rules; the column rule is missing in the
		// top row (a title spanning both columns)
		hRule(100, 100, 300), hRule(120, 100, 300), hRule(140, 100, 300), hRule(160, 100, 300),
		vRule(100, 100, 160), vRule(300, 100, 160), vRule(200, 120, 160),
	}
	items := []*models.TextItem{
		cellItem(150, ys[0], "Title"),
		cellItem(xs[0], ys[1], "a1"), cellItem(xs[1], ys[1], "b1"),
		cellItem(xs[0], ys[2], "a2"), cellItem(xs[1], ys[2], "b2"),
	}

	tables, inTable := NewCompactLines().detectRuledTables(items, rules)
	if len(tables) != 1 {
		t.Fatalf("got %d tables, want 1", len(tables))
	}
	want := [][]string{{"Title", ""}, {"a1", "b1"}, {"a2", "b2"}}
	if got := tableCells(tables[0]); !reflect.DeepEqual(got, want) {
		t.Errorf("cells = %q, want %q", got, want)
	}
	for i, taken := range inTable {
		if !taken {
			t.Errorf("item %d (%q) not taken by the table", i, items[i].Text)
		}
	}
	if !tables[0].rows[0].isHeader || tables[0].rows[1].isHeader {
		t.Errorf("only the first row should be the header")
	}
}

// Text is placed by where it starts and the middle of its glyphs: the
// baseline of text in a cell can be close to the rule below it
func TestRuledTable_CellAssignment(t *testing.T) {
	rules := gridRules([]float64{100, 200, 300}, []float64{100, 120, 140})
	items := []*models.TextItem{
		// Baseline 1 unit above the rule at 120
		{X: 104, Y: 119, Width: 30, Height: 10, Text: "Name"},
		{X: 204, Y: 119, Width: 30, Height: 10, Text: "Value"},
		// Two lines in one cell, joined in reading order
		{X: 104, Y: 131, Width: 30, Height: 8, Text: "first"},
		{X: 104, Y: 139.5, Width: 30, Height: 8, Text: "second"},
		{X: 204, Y: 131, Width: 30, Height: 8, Text: "x"},
	}
	tables, _ := NewCompactLines().detectRuledTables(items, rules)
	if len(tables) != 1 {
		t.Fatalf("got %d tables, want 1", len(tables))
	}
	want := [][]string{{"Name", "Value"}, {"first second", "x"}}
	if got := tableCells(tables[0]); !reflect.DeepEqual(got, want) {
		t.Errorf("cells = %q, want %q", got, want)
	}
}

// A ruled header over a body drawn as one box: the body's items go in the
// columns they start in, and its rows are told apart by the gaps between
// them, while the lines of one wrapped cell stay together
func TestRuledTable_UnruledBody(t *testing.T) {
	rules := []models.Rule{
		hRule(100, 100, 400), hRule(120, 100, 400), hRule(200, 100, 400),
		vRule(100, 100, 200), vRule(400, 100, 200), vRule(200, 100, 120), vRule(300, 100, 120),
	}
	line := func(x, y float64, text string) *models.TextItem {
		return &models.TextItem{X: x, Y: y, Width: 40, Height: 10, Text: text}
	}
	items := []*models.TextItem{
		line(104, 115, "Host"), line(204, 115, "Name"), line(304, 115, "Score"),
		// The name wraps with lines 12 units (1.2 heights) apart; the next
		// row starts 18 units (1.8 heights) below the last line of the one
		// before
		line(104, 140, "h1"), line(204, 134, "alpha-"), line(204, 146, "one"), line(304, 140, "0"),
		line(104, 170, "h2"), line(204, 164, "beta-"), line(204, 176, "two"), line(304, 170, "3"),
	}
	tables, _ := NewCompactLines().detectRuledTables(items, rules)
	if len(tables) != 1 {
		t.Fatalf("got %d tables, want 1", len(tables))
	}
	want := [][]string{{"Host", "Name", "Score"}, {"h1", "alpha-one", "0"}, {"h2", "beta-two", "3"}}
	if got := tableCells(tables[0]); !reflect.DeepEqual(got, want) {
		t.Errorf("cells = %q, want %q", got, want)
	}
}

// Text inside a ruled grid is the ruled table's: the text-based detectors
// do not see it, and the prose around it stays in order around the table
func TestRuledTable_PageWithProse(t *testing.T) {
	rules := gridRules([]float64{100, 250, 400}, []float64{200, 220, 240, 260})
	var items []interface{}
	items = append(items,
		&models.TextItem{X: 100, Y: 150, Width: 200, Height: 10, Text: "Prose above the table."},
		&models.TextItem{X: 100, Y: 300, Width: 200, Height: 10, Text: "Prose below the table."},
	)
	for r, row := range [][2]string{{"Version", "Date"}, {"1.0", "2024"}, {"2.0", "2025"}} {
		y := 200 + 20*float64(r)
		items = append(items, cellItem(100, y, row[0]), cellItem(250, y, row[1]))
	}

	c := NewCompactLines()
	groups := c.groupByLineWithTableDetection(items, rules, 12, 700)

	var got []string
	for _, g := range groups {
		if g.isTableRow {
			got = append(got, "row:"+g.cells[0]+"|"+g.cells[1])
		} else {
			got = append(got, "text:"+g.items[0].Text)
		}
	}
	want := []string{
		"text:Prose above the table.",
		"row:Version|Date", "row:1.0|2024", "row:2.0|2025",
		"text:Prose below the table.",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("lines = %q, want %q", got, want)
	}
}

// Where a ruled grid and the text-based detectors could both claim text,
// the ruled grid decides its columns; text outside it is still detected by
// the text-based detectors
func TestRuledTable_TakesPrecedence(t *testing.T) {
	// Three columns drawn with rules, with text spaced so that the
	// header-based detector would read only two columns
	rules := gridRules([]float64{100, 300, 320, 500}, []float64{100, 120, 140, 160})
	var items []interface{}
	for r, row := range [][3]string{{"Name", "", "Owner"}, {"Alice", "", "Bob"}, {"Carol", "x", "Dave"}} {
		y := 100 + 20*float64(r)
		for col, x := range []float64{100, 300, 320} {
			if row[col] != "" {
				items = append(items, cellItem(x, y, row[col]))
			}
		}
	}

	groups := NewCompactLines().groupByLineWithTableDetection(items, rules, 12, 700)
	var got [][]string
	for _, g := range groups {
		if !g.isTableRow || g.cells == nil {
			t.Fatalf("line %+v is not a ruled table row", g)
		}
		got = append(got, g.cells)
	}
	want := [][]string{{"Name", "", "Owner"}, {"Alice", "", "Bob"}, {"Carol", "x", "Dave"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rows = %q, want %q", got, want)
	}
}

func tableCells(t *ruledTable) [][]string {
	var cells [][]string
	for _, row := range t.rows {
		cells = append(cells, row.cells)
	}
	return cells
}

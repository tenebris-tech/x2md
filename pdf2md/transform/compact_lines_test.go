/******************************************************************************
 * Copyright (c) 2025-2026 Tenebris Technologies Inc.                         *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package transform

import (
	"testing"

	"github.com/tenebris-tech/x2md/pdf2md/models"
)

func TestNeedsSpaceBetween(t *testing.T) {
	c := NewCompactLines()

	tests := []struct {
		name      string
		prevText  string
		nextText  string
		xDistance float64
		want      bool
	}{
		// Date formatting - hyphens should not add spaces
		{
			name:      "date with hyphen before month",
			prevText:  "06",
			nextText:  "-December",
			xDistance: 5,
			want:      false,
		},
		{
			name:      "date with hyphen after month",
			prevText:  "December",
			nextText:  "-2023",
			xDistance: 5,
			want:      false,
		},
		{
			name:      "hyphen at end of previous",
			prevText:  "06-",
			nextText:  "December",
			xDistance: 5,
			want:      false,
		},
		{
			name:      "en-dash connecting words",
			prevText:  "pages",
			nextText:  "–",
			xDistance: 5,
			want:      false,
		},

		// Punctuation attachment
		{
			name:      "period attaches to previous",
			prevText:  "word",
			nextText:  ".",
			xDistance: 2,
			want:      false,
		},
		{
			name:      "comma attaches to previous",
			prevText:  "word",
			nextText:  ",",
			xDistance: 2,
			want:      false,
		},
		{
			name:      "closing paren attaches",
			prevText:  "text",
			nextText:  ")",
			xDistance: 2,
			want:      false,
		},
		{
			name:      "opening paren attaches to next",
			prevText:  "(",
			nextText:  "text",
			xDistance: 2,
			want:      false,
		},

		// Large gaps need spaces (alphanumeric chars need gap > 25px)
		{
			name:      "large gap between alphanumeric words",
			prevText:  "word1",
			nextText:  "word2",
			xDistance: 30,
			want:      true,
		},
		{
			name:      "medium gap between alphanumeric words - no space",
			prevText:  "word1",
			nextText:  "word2",
			xDistance: 20,
			want:      false, // Below alphanumericGapThreshold (25)
		},

		// Small gaps with alphanumeric
		{
			name:      "small gap alphanumeric sequence",
			prevText:  "abc",
			nextText:  "123",
			xDistance: 10,
			want:      false,
		},

		// Negative distance (overlap)
		{
			name:      "overlapping items",
			prevText:  "over",
			nextText:  "lap",
			xDistance: -5,
			want:      false,
		},

		// Alphanumeric word spacing - need gaps > 25px for spaces
		{
			name:      "alphanumeric gap 20 - no space",
			prevText:  "hello",
			nextText:  "world",
			xDistance: 20,
			want:      false, // Below alphanumericGapThreshold (25)
		},
		{
			name:      "alphanumeric gap 12 - no space",
			prevText:  "hello",
			nextText:  "world",
			xDistance: 12,
			want:      false, // Both alphanumeric, below threshold
		},
		// Non-alphanumeric characters use lower thresholds
		{
			name:      "symbol to word - large gap needs space",
			prevText:  "?",
			nextText:  "What",
			xDistance: 35,
			want:      true, // Symbol to alphanumeric > largeGapThreshold
		},
		{
			name:      "word to symbol - small gap",
			prevText:  "text",
			nextText:  "?",
			xDistance: 15,
			want:      true, // Non-matching pattern, > minSpaceGapThreshold
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.needsSpaceBetween(tt.prevText, tt.nextText, tt.xDistance)
			if got != tt.want {
				t.Errorf("needsSpaceBetween(%q, %q, %.1f) = %v, want %v",
					tt.prevText, tt.nextText, tt.xDistance, got, tt.want)
			}
		})
	}
}

func TestExtractColumnTexts(t *testing.T) {
	c := NewCompactLines()

	tests := []struct {
		name    string
		items   []*models.TextItem
		columns []float64
		want    []string
	}{
		{
			name: "two column table",
			items: []*models.TextItem{
				{X: 50, Y: 100, Text: "[CC1]"},
				{X: 150, Y: 100, Text: "Description of CC1"},
			},
			columns: []float64{50, 150},
			want:    []string{"[CC1]", "Description of CC1"},
		},
		{
			name: "multi-line cell in second column",
			items: []*models.TextItem{
				{X: 50, Y: 100, Text: "[CC1]"},
				{X: 150, Y: 100, Text: "First line"},
				{X: 150, Y: 115, Text: "Second line"},
			},
			columns: []float64{50, 150},
			want:    []string{"[CC1]", "First line Second line"},
		},
		{
			name: "three column table",
			items: []*models.TextItem{
				{X: 50, Y: 100, Text: "1.0"},
				{X: 150, Y: 100, Text: "2023-01-01"},
				{X: 300, Y: 100, Text: "Initial release"},
			},
			columns: []float64{50, 150, 300},
			want:    []string{"1.0", "2023-01-01", "Initial release"},
		},
		{
			name:    "empty columns",
			items:   []*models.TextItem{},
			columns: []float64{50, 150},
			want:    []string{"", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.extractColumnTexts(tt.items, tt.columns)
			if len(got) != len(tt.want) {
				t.Errorf("extractColumnTexts() returned %d columns, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("extractColumnTexts()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestGetColumn(t *testing.T) {
	c := NewCompactLines()

	columns := []float64{50, 150, 300}

	tests := []struct {
		name string
		x    float64
		want int
	}{
		{"exactly at first column", 50, 0},
		{"exactly at second column", 150, 1},
		{"exactly at third column", 300, 2},
		{"between first and second", 100, 0},
		{"between second and third", 200, 1},
		{"after third column", 400, 2},
		{"before first column (within tolerance)", 40, 0},
		{"slightly before second column", 145, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.getColumn(tt.x, columns)
			if got != tt.want {
				t.Errorf("getColumn(%.1f) = %d, want %d", tt.x, got, tt.want)
			}
		})
	}
}

func TestVisualRowAddItem(t *testing.T) {
	row := &visualRow{
		minY:  100,
		maxY:  100,
		items: []*models.TextItem{{X: 50, Y: 100, Text: "first"}},
	}

	// Add item below
	row.addItemToRow(&models.TextItem{X: 50, Y: 115, Text: "second"})
	if row.maxY != 115 {
		t.Errorf("maxY = %.1f, want 115", row.maxY)
	}

	// Add item above
	row.addItemToRow(&models.TextItem{X: 50, Y: 90, Text: "third"})
	if row.minY != 90 {
		t.Errorf("minY = %.1f, want 90", row.minY)
	}

	if len(row.items) != 3 {
		t.Errorf("items count = %d, want 3", len(row.items))
	}
}

func TestVisualRowIsInYRange(t *testing.T) {
	row := &visualRow{minY: 100, maxY: 120}

	tests := []struct {
		name      string
		itemY     float64
		threshold float64
		want      bool
	}{
		{"within range", 110, 10, true},
		{"at min boundary", 100, 10, true},
		{"at max boundary", 120, 10, true},
		{"just below threshold", 89, 10, false},
		{"just above threshold", 131, 10, false},
		{"within threshold below min", 95, 10, true},
		{"within threshold above max", 125, 10, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := row.isInYRange(tt.itemY, tt.threshold)
			if got != tt.want {
				t.Errorf("isInYRange(%.1f, %.1f) = %v, want %v",
					tt.itemY, tt.threshold, got, tt.want)
			}
		})
	}
}

func TestDetectReferenceTable(t *testing.T) {
	c := NewCompactLines()

	tests := []struct {
		name     string
		items    []*models.TextItem
		wantNil  bool
		wantCols int
	}{
		{
			name: "valid reference table with 3 IDs",
			items: []*models.TextItem{
				{X: 50, Y: 100, Text: "[CC1]"},
				{X: 150, Y: 100, Text: "Description 1"},
				{X: 50, Y: 120, Text: "[CC2]"},
				{X: 150, Y: 120, Text: "Description 2"},
				{X: 50, Y: 140, Text: "[CC3]"},
				{X: 150, Y: 140, Text: "Description 3"},
			},
			wantNil:  false,
			wantCols: 2,
		},
		{
			name: "not enough reference IDs",
			items: []*models.TextItem{
				{X: 50, Y: 100, Text: "[CC1]"},
				{X: 150, Y: 100, Text: "Description 1"},
				{X: 50, Y: 120, Text: "[CC2]"},
				{X: 150, Y: 120, Text: "Description 2"},
			},
			wantNil: true,
		},
		{
			name: "reference IDs not aligned",
			items: []*models.TextItem{
				{X: 50, Y: 100, Text: "[CC1]"},
				{X: 100, Y: 120, Text: "[CC2]"}, // Different X
				{X: 150, Y: 140, Text: "[CC3]"}, // Different X
			},
			wantNil: true,
		},
		{
			name: "no reference IDs",
			items: []*models.TextItem{
				{X: 50, Y: 100, Text: "Regular text"},
				{X: 50, Y: 120, Text: "More text"},
			},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.detectReferenceTable(tt.items, 12)
			if tt.wantNil {
				if got != nil {
					t.Errorf("detectReferenceTable() = %v, want nil", got)
				}
			} else {
				if got == nil {
					t.Error("detectReferenceTable() = nil, want non-nil")
				} else if len(got.columns) != tt.wantCols {
					t.Errorf("detectReferenceTable() columns = %d, want %d",
						len(got.columns), tt.wantCols)
				}
			}
		})
	}
}

func TestCombineText(t *testing.T) {
	c := NewCompactLines()

	tests := []struct {
		name  string
		items []*models.TextItem
		want  string
	}{
		{
			name: "word space gap",
			items: []*models.TextItem{
				{X: 0, Y: 100, Width: 30, Height: 12, Text: "Hello"},
				{X: 33, Y: 100, Width: 30, Height: 12, Text: "World"}, // Gap of 3 (0.25 em at 12) > 2.4 (12*0.2) threshold
			},
			want: "Hello World",
		},
		{
			name: "text items with small gap - same word",
			items: []*models.TextItem{
				{X: 0, Y: 100, Width: 30, Height: 12, Text: "Hello"},
				{X: 31, Y: 100, Width: 30, Height: 12, Text: "World"}, // Gap of 1 < 2.4 (12*0.2) threshold
			},
			want: "HelloWorld", // No space - considered same word in PDF
		},
		{
			// A narrow word's width is its real width, not an estimate
			name: "narrow text followed by a word space",
			items: []*models.TextItem{
				{X: 0, Y: 100, Width: 8, Height: 12, Text: "ill"},
				{X: 11, Y: 100, Width: 30, Height: 12, Text: "World"},
			},
			want: "ill World",
		},
		{
			name: "date with hyphens",
			items: []*models.TextItem{
				{X: 0, Y: 100, Width: 15, Text: "06"},
				{X: 16, Y: 100, Width: 5, Text: "-"},
				{X: 22, Y: 100, Width: 50, Text: "December"},
				{X: 73, Y: 100, Width: 5, Text: "-"},
				{X: 79, Y: 100, Width: 30, Text: "2023"},
			},
			want: "06-December-2023",
		},
		{
			name: "punctuation attachment",
			items: []*models.TextItem{
				{X: 0, Y: 100, Width: 30, Text: "word"},
				{X: 32, Y: 100, Width: 5, Text: "."},
			},
			want: "word.",
		},
		{
			name: "multi-line cell content",
			items: []*models.TextItem{
				{X: 0, Y: 100, Width: 50, Text: "First line"},
				{X: 0, Y: 115, Width: 60, Text: "Second line"},
			},
			want: "First line Second line",
		},
		{
			name: "hyphenated word across lines",
			items: []*models.TextItem{
				{X: 0, Y: 100, Width: 50, Text: "hyphen-"},
				{X: 0, Y: 115, Width: 30, Text: "ated"},
			},
			want: "hyphen-ated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.combineText(tt.items)
			if got != tt.want {
				t.Errorf("combineText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectAlignedTables(t *testing.T) {
	c := NewCompactLines()

	tests := []struct {
		name           string
		items          []*models.TextItem
		wantRegions    int
		wantMinColumns int
	}{
		{
			name: "simple 3-row 2-column table",
			items: []*models.TextItem{
				// Row 1 (header)
				{X: 50, Y: 100, Width: 40, Text: "Name"},
				{X: 150, Y: 100, Width: 40, Text: "Value"},
				// Row 2
				{X: 50, Y: 120, Width: 40, Text: "Alpha"},
				{X: 150, Y: 120, Width: 40, Text: "100"},
				// Row 3
				{X: 50, Y: 140, Width: 40, Text: "Beta"},
				{X: 150, Y: 140, Width: 40, Text: "200"},
			},
			wantRegions:    1,
			wantMinColumns: 2,
		},
		{
			name: "4-row 3-column table",
			items: []*models.TextItem{
				// Row 1
				{X: 50, Y: 100, Width: 40, Text: "ID"},
				{X: 150, Y: 100, Width: 40, Text: "Name"},
				{X: 250, Y: 100, Width: 40, Text: "Status"},
				// Row 2
				{X: 50, Y: 120, Width: 40, Text: "1"},
				{X: 150, Y: 120, Width: 40, Text: "Item A"},
				{X: 250, Y: 120, Width: 40, Text: "Active"},
				// Row 3
				{X: 50, Y: 140, Width: 40, Text: "2"},
				{X: 150, Y: 140, Width: 40, Text: "Item B"},
				{X: 250, Y: 140, Width: 40, Text: "Pending"},
				// Row 4
				{X: 50, Y: 160, Width: 40, Text: "3"},
				{X: 150, Y: 160, Width: 40, Text: "Item C"},
				{X: 250, Y: 160, Width: 40, Text: "Done"},
			},
			wantRegions:    1,
			wantMinColumns: 3,
		},
		{
			name: "not a table - only 2 rows",
			items: []*models.TextItem{
				{X: 50, Y: 100, Width: 40, Text: "Name"},
				{X: 150, Y: 100, Width: 40, Text: "Value"},
				{X: 50, Y: 120, Width: 40, Text: "Alpha"},
				{X: 150, Y: 120, Width: 40, Text: "100"},
			},
			wantRegions:    0,
			wantMinColumns: 0,
		},
		{
			name: "not a table - misaligned columns",
			items: []*models.TextItem{
				{X: 50, Y: 100, Width: 40, Text: "Name"},
				{X: 150, Y: 100, Width: 40, Text: "Value"},
				{X: 110, Y: 120, Width: 40, Text: "Alpha"}, // 60 pixels off, outside tolerance
				{X: 210, Y: 120, Width: 40, Text: "100"},
				{X: 50, Y: 140, Width: 40, Text: "Beta"},
				{X: 150, Y: 140, Width: 40, Text: "200"},
			},
			wantRegions:    0,
			wantMinColumns: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			regions := c.detectAlignedTables(tt.items, 12, 700.0)

			if len(regions) != tt.wantRegions {
				t.Errorf("detectAlignedTables() got %d regions, want %d", len(regions), tt.wantRegions)
			}

			if tt.wantMinColumns > 0 && len(regions) > 0 {
				if len(regions[0].columns) < tt.wantMinColumns {
					t.Errorf("detectAlignedTables() got %d columns, want at least %d",
						len(regions[0].columns), tt.wantMinColumns)
				}
			}
		})
	}
}

func TestColumnsMatch(t *testing.T) {
	c := NewCompactLines()

	tests := []struct {
		name  string
		cols1 []float64
		cols2 []float64
		want  bool
	}{
		{
			name:  "exact match",
			cols1: []float64{50.0, 150.0, 250.0},
			cols2: []float64{50.0, 150.0, 250.0},
			want:  true,
		},
		{
			name:  "within tolerance",
			cols1: []float64{50.0, 150.0, 250.0},
			cols2: []float64{55.0, 155.0, 255.0},
			want:  true,
		},
		{
			name:  "outside tolerance",
			cols1: []float64{50.0, 150.0, 250.0},
			cols2: []float64{50.0, 200.0, 250.0}, // 50 pixels off
			want:  false,
		},
		{
			name:  "different column count",
			cols1: []float64{50.0, 150.0},
			cols2: []float64{50.0, 150.0, 250.0},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.columnsMatch(tt.cols1, tt.cols2)
			if got != tt.want {
				t.Errorf("columnsMatch() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMergeColumnPositions(t *testing.T) {
	c := NewCompactLines()

	columnSets := [][]float64{
		{50.0, 150.0, 250.0},
		{52.0, 148.0, 252.0},
		{48.0, 152.0, 248.0},
	}

	result := c.mergeColumnPositions(columnSets)

	if len(result) != 3 {
		t.Errorf("mergeColumnPositions() got %d columns, want 3", len(result))
	}

	// Average of 50, 52, 48 = 50
	if result[0] != 50.0 {
		t.Errorf("mergeColumnPositions()[0] = %f, want 50.0", result[0])
	}
}

// tableItem builds a text item for the table detection tests
func tableItem(x, y float64, text string) *models.TextItem {
	return &models.TextItem{X: x, Y: y, Width: 5 * float64(len(text)), Height: 10, Text: text, Font: "F1"}
}

// A cell wrapped over several lines puts lines on the page that start at an
// interior column only. They belong to the table and must not end it.
func TestBoundTableRegionKeepsWrappedCells(t *testing.T) {
	c := NewCompactLines()
	columns := []float64{80, 140, 215}

	items := []*models.TextItem{
		tableItem(80, 100, "Version"), tableItem(140, 100, "Date"), tableItem(215, 100, "Description"),
	}
	// Three rows 60 apart. The date is broken over two lines, above and below
	// the line that holds the version and the description.
	for i := 0; i < 3; i++ {
		y := 160 + float64(i)*60
		items = append(items,
			tableItem(140, y-8, "06-Dec-"),
			tableItem(80, y, "3.0"), tableItem(215, y, "Released for use"),
			tableItem(140, y+8, "2023"),
		)
	}

	maxY, dataRows, consistentCols, interiorCols := c.boundTableRegionByColumns(items, 100, columns, 16)

	if dataRows != 3 {
		t.Errorf("dataRows = %d, want 3", dataRows)
	}
	// The last row's date ends on a second line at 288, below the row itself
	if maxY != 288 {
		t.Errorf("maxY = %.0f, want 288, the lowest line of the last row", maxY)
	}
	if consistentCols < 2 || interiorCols < 1 {
		t.Errorf("consistentCols = %d, interiorCols = %d, want at least 2 and 1", consistentCols, interiorCols)
	}
}

// Text below a table starts at the left margin and must not be drawn in.
func TestBoundTableRegionStopsAtProse(t *testing.T) {
	c := NewCompactLines()
	columns := []float64{80, 215}

	items := []*models.TextItem{tableItem(80, 100, "Name"), tableItem(215, 100, "Value")}
	for i := 0; i < 3; i++ {
		y := 116 + float64(i)*16
		items = append(items, tableItem(80, y, "key"), tableItem(215, y, "value"))
	}
	for i := 0; i < 6; i++ {
		items = append(items, tableItem(80, 200+float64(i)*16, "A paragraph of text below the table."))
	}

	maxY, dataRows, _, _ := c.boundTableRegionByColumns(items, 100, columns, 16)

	if dataRows != 3 {
		t.Errorf("dataRows = %d, want 3", dataRows)
	}
	if maxY >= 200 {
		t.Errorf("maxY = %.0f, want the region to end above the paragraph at 200", maxY)
	}
}

// On a 2-column page, a few lines that happen to start with a capital in both
// columns must not be taken for a table when the rest of the page shows the
// same two columns to be page layout.
func TestDetectAlignedTablesRejectsPageColumns(t *testing.T) {
	c := NewCompactLines()

	var items []*models.TextItem
	// Lines of running text: the right column continues sentences
	for i := 0; i < 5; i++ {
		y := 100 + float64(i)*15
		items = append(items, tableItem(56, y, "Left column text"), tableItem(297, y, "continues a sentence here"))
	}
	// A line with an indented right half separates the two runs
	items = append(items, tableItem(56, 175, "Left column text"), tableItem(420, 175, "Indented line"))
	// Three lines that start with a capital on both sides
	for i := 0; i < 3; i++ {
		y := 190 + float64(i)*15
		items = append(items, tableItem(56, y, "Left column text"), tableItem(297, y, "To be or not to be"))
	}

	if regions := c.detectAlignedTables(items, 15, 700); len(regions) != 0 {
		t.Errorf("detected %d table region(s) on a 2-column page, want none", len(regions))
	}
}

// The same three lines on their own, with no sign of page layout, are a table.
func TestDetectAlignedTablesKeepsTable(t *testing.T) {
	c := NewCompactLines()

	var items []*models.TextItem
	for i := 0; i < 3; i++ {
		y := 190 + float64(i)*15
		items = append(items, tableItem(56, y, "Setting"), tableItem(297, y, "Enabled"))
	}

	if regions := c.detectAlignedTables(items, 15, 700); len(regions) != 1 {
		t.Errorf("detected %d table region(s), want 1", len(regions))
	}
}

// Indented text below a table starts at an interior column like a wrapped
// cell does, but runs on for many lines. It must not be drawn into the table.
func TestBoundTableRegionStopsAtIndentedText(t *testing.T) {
	c := NewCompactLines()
	columns := []float64{80, 215}

	items := []*models.TextItem{tableItem(80, 100, "Name"), tableItem(215, 100, "Value")}
	for i := 0; i < 3; i++ {
		y := 116 + float64(i)*16
		items = append(items, tableItem(80, y, "key"), tableItem(215, y, "value"))
	}
	for i := 0; i < 20; i++ {
		items = append(items, tableItem(215, 200+float64(i)*16, "An indented block of text below the table."))
	}

	maxY, dataRows, _, _ := c.boundTableRegionByColumns(items, 100, columns, 16)

	if dataRows != 3 {
		t.Errorf("dataRows = %d, want 3", dataRows)
	}
	if maxY > 200+16*float64(maxWrappedCellRows) {
		t.Errorf("maxY = %.0f, want the region to end within %d lines of the table", maxY, maxWrappedCellRows)
	}
}

// The last row of a table continued from the previous page sits alone under
// a repeated header. A known header makes that one row a table.
func TestDetectTableRegionsKnownHeaderWithOneRow(t *testing.T) {
	c := NewCompactLines()

	items := []*models.TextItem{
		tableItem(80, 100, "Version"), tableItem(140, 100, "Date"), tableItem(215, 100, "Description"),
		tableItem(80, 130, "0.1"), tableItem(140, 130, "05-Sep-2014"), tableItem(215, 130, "Draft published for review"),
	}

	regions := c.detectTableRegions(items, 16, 700)
	if len(regions) != 1 {
		t.Fatalf("detected %d table region(s), want 1", len(regions))
	}
	if len(regions[0].columns) != 3 {
		t.Errorf("table has %d columns, want 3", len(regions[0].columns))
	}
}

// The same single row under a header that is not a known one is not enough.
func TestDetectTableRegionsUnknownHeaderWithOneRow(t *testing.T) {
	c := NewCompactLines()

	items := []*models.TextItem{
		tableItem(80, 100, "Item"), tableItem(140, 100, "Owner"), tableItem(215, 100, "Comment"),
		tableItem(80, 130, "0.1"), tableItem(140, 130, "05-Sep-2014"), tableItem(215, 130, "Draft published for review"),
	}

	if regions := c.detectTableRegions(items, 16, 700); len(regions) != 0 {
		t.Errorf("detected %d table region(s), want none", len(regions))
	}
}

// Lines whose text runs from the left half across the middle of the page
// are single-column lines, even when the only item that starts in the right
// half is a dot leader or other text that looks like a sentence fragment
func TestDetectAndSplitMultiColumnLayout_TextAcrossMiddle(t *testing.T) {
	// 20 lines starting at X 60; the first 6 also have a dot leader that
	// starts at X 400, so the page's middle is at X 230
	page := func(leftWidth float64) []*models.TextItem {
		var items []*models.TextItem
		for i := 0; i < 20; i++ {
			y := 100 + 20*float64(i)
			items = append(items, &models.TextItem{X: 60, Y: y, Width: leftWidth, Height: 10, Text: "Entry"})
			if i < 6 {
				items = append(items, &models.TextItem{X: 400, Y: y, Width: 150, Height: 10, Text: ".....5"})
			}
		}
		return items
	}

	tests := []struct {
		name      string
		leftWidth float64
		want      bool
	}{
		{"left text crosses the middle", 300, false},
		{"left text ends before the middle", 150, true},
	}

	c := NewCompactLines()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, got := c.detectAndSplitMultiColumnLayout(page(tt.leftWidth))
			if got != tt.want {
				t.Errorf("multi-column = %v, want %v", got, tt.want)
			}
		})
	}
}

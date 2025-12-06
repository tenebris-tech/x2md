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
			name: "simple text combination with large gap",
			items: []*models.TextItem{
				{X: 0, Y: 100, Width: 30, Height: 12, Text: "Hello"},
				{X: 80, Y: 100, Width: 30, Height: 12, Text: "World"}, // Gap of 50 (80-30) > 36 (12*3) threshold
			},
			want: "Hello World",
		},
		{
			name: "text items with small gap - same word",
			items: []*models.TextItem{
				{X: 0, Y: 100, Width: 30, Height: 12, Text: "Hello"},
				{X: 50, Y: 100, Width: 30, Height: 12, Text: "World"}, // Gap of 20 (50-30) < 36 (12*3) threshold
			},
			want: "HelloWorld", // No space - considered same word in PDF
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

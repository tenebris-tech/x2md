package models

import (
	"strings"
	"testing"
)

func TestHeadlineByLevel(t *testing.T) {
	tests := []struct {
		level int
		want  *BlockType
	}{
		{1, BlockTypeH1},
		{2, BlockTypeH2},
		{3, BlockTypeH3},
		{4, BlockTypeH4},
		{5, BlockTypeH5},
		{6, BlockTypeH6},
		{7, BlockTypeH6}, // Capped at H6
		{0, BlockTypeH1}, // Below 1 defaults to H1
		{-1, BlockTypeH1},
	}

	for _, tt := range tests {
		t.Run(tt.want.Name, func(t *testing.T) {
			got := HeadlineByLevel(tt.level)
			if got != tt.want {
				t.Errorf("HeadlineByLevel(%d) = %v, want %v", tt.level, got, tt.want)
			}
		})
	}
}

func TestIsHeadline(t *testing.T) {
	tests := []struct {
		name      string
		blockType *BlockType
		want      bool
	}{
		{"H1 is headline", BlockTypeH1, true},
		{"H6 is headline", BlockTypeH6, true},
		{"TOC is not headline", BlockTypeTOC, false},
		{"LIST is not headline", BlockTypeList, false},
		{"nil is not headline", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsHeadline(tt.blockType)
			if got != tt.want {
				t.Errorf("IsHeadline() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBlockTypeByName(t *testing.T) {
	tests := []struct {
		name string
		want *BlockType
	}{
		{"H1", BlockTypeH1},
		{"H2", BlockTypeH2},
		{"H3", BlockTypeH3},
		{"H4", BlockTypeH4},
		{"H5", BlockTypeH5},
		{"H6", BlockTypeH6},
		{"TOC", BlockTypeTOC},
		{"FOOTNOTES", BlockTypeFootnotes},
		{"CODE", BlockTypeCode},
		{"LIST", BlockTypeList},
		{"PARAGRAPH", BlockTypeParagraph},
		{"UNKNOWN", nil},
		{"", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BlockTypeByName(tt.name)
			if got != tt.want {
				t.Errorf("BlockTypeByName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestLinesToTextTableRendering(t *testing.T) {
	tests := []struct {
		name  string
		lines []*LineItem
		want  string
	}{
		{
			name: "simple table row",
			lines: []*LineItem{
				{
					IsTableRow:   true,
					TableColumns: []string{"col1", "col2", "col3"},
				},
			},
			want: "| col1 | col2 | col3 |\n",
		},
		{
			name: "table with header",
			lines: []*LineItem{
				{
					IsTableRow:    true,
					IsTableHeader: true,
					TableColumns:  []string{"Name", "Value"},
				},
				{
					IsTableRow:   true,
					TableColumns: []string{"foo", "bar"},
				},
			},
			want: "| Name | Value |\n| --- | --- |\n| foo | bar |\n",
		},
		{
			name: "empty table row skipped",
			lines: []*LineItem{
				{
					IsTableRow:    true,
					IsTableHeader: true,
					TableColumns:  []string{"Name", "Value"},
				},
				{
					IsTableRow:   true,
					TableColumns: []string{"", "   "}, // Empty row
				},
				{
					IsTableRow:   true,
					TableColumns: []string{"foo", "bar"},
				},
			},
			want: "| Name | Value |\n| --- | --- |\n| foo | bar |\n",
		},
		{
			name: "whitespace-only columns skipped",
			lines: []*LineItem{
				{
					IsTableRow:   true,
					TableColumns: []string{"  ", "\t", "   "},
				},
			},
			want: "", // Entire row skipped
		},
		{
			name: "table columns trimmed",
			lines: []*LineItem{
				{
					IsTableRow:   true,
					TableColumns: []string{"  padded  ", "  text  "},
				},
			},
			want: "| padded | text |\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LinesToText(tt.lines, false)
			if got != tt.want {
				t.Errorf("LinesToText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLinesToTextRegularText(t *testing.T) {
	lines := []*LineItem{
		{
			Words: []*Word{
				{String: "Hello"},
				{String: "world"},
			},
		},
		{
			Words: []*Word{
				{String: "Second"},
				{String: "line"},
			},
		},
	}

	got := LinesToText(lines, false)
	want := "Hello world\nSecond line\n"

	if got != want {
		t.Errorf("LinesToText() = %q, want %q", got, want)
	}
}

func TestLinesToTextFormatting(t *testing.T) {
	boldFormat := &WordFormat{
		Name:        "Bold",
		StartSymbol: "**",
		EndSymbol:   "**",
	}

	lines := []*LineItem{
		{
			Words: []*Word{
				{String: "Normal"},
				{String: "bold", Format: boldFormat},
				{String: "normal"},
			},
		},
	}

	got := LinesToText(lines, false)
	if !strings.Contains(got, "**bold**") {
		t.Errorf("LinesToText() = %q, should contain **bold**", got)
	}

	// Test with formatting disabled
	got = LinesToText(lines, true)
	if strings.Contains(got, "**") {
		t.Errorf("LinesToText(disableInlineFormats=true) = %q, should not contain **", got)
	}
}

func TestBlockToText(t *testing.T) {
	tests := []struct {
		name      string
		block     *LineItemBlock
		wantStart string
	}{
		{
			name: "H1 block",
			block: &LineItemBlock{
				Type: BlockTypeH1,
				Items: []*LineItem{
					{Words: []*Word{{String: "Title"}}},
				},
			},
			wantStart: "# ",
		},
		{
			name: "H2 block",
			block: &LineItemBlock{
				Type: BlockTypeH2,
				Items: []*LineItem{
					{Words: []*Word{{String: "Subtitle"}}},
				},
			},
			wantStart: "## ",
		},
		{
			name: "H3 block",
			block: &LineItemBlock{
				Type: BlockTypeH3,
				Items: []*LineItem{
					{Words: []*Word{{String: "Section"}}},
				},
			},
			wantStart: "### ",
		},
		{
			name: "Code block",
			block: &LineItemBlock{
				Type: BlockTypeCode,
				Items: []*LineItem{
					{Words: []*Word{{String: "code"}}},
				},
			},
			wantStart: "```\n",
		},
		{
			name: "nil type block",
			block: &LineItemBlock{
				Items: []*LineItem{
					{Words: []*Word{{String: "text"}}},
				},
			},
			wantStart: "text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BlockToText(tt.block)
			if !strings.HasPrefix(got, tt.wantStart) {
				t.Errorf("BlockToText() = %q, want prefix %q", got, tt.wantStart)
			}
		})
	}
}

func TestLinesToTextMixedTableAndText(t *testing.T) {
	lines := []*LineItem{
		{
			Words: []*Word{{String: "Intro"}},
		},
		{
			IsTableRow:    true,
			IsTableHeader: true,
			TableColumns:  []string{"A", "B"},
		},
		{
			IsTableRow:   true,
			TableColumns: []string{"1", "2"},
		},
		{
			Words: []*Word{{String: "Outro"}},
		},
	}

	got := LinesToText(lines, false)

	// Should contain intro text
	if !strings.Contains(got, "Intro\n") {
		t.Error("Output should contain 'Intro'")
	}

	// Should contain table
	if !strings.Contains(got, "| A | B |") {
		t.Error("Output should contain table header")
	}

	// Should contain separator
	if !strings.Contains(got, "| --- | --- |") {
		t.Error("Output should contain table separator")
	}

	// Should contain outro text
	if !strings.Contains(got, "Outro\n") {
		t.Error("Output should contain 'Outro'")
	}
}

func TestLinesToTextNestedLists(t *testing.T) {
	lines := []*LineItem{
		{
			Type:      BlockTypeList,
			ListLevel: 0,
			Words:     []*Word{{String: "-"}, {String: "Level"}, {String: "0"}},
		},
		{
			Type:      BlockTypeList,
			ListLevel: 1,
			Words:     []*Word{{String: "-"}, {String: "Level"}, {String: "1"}},
		},
		{
			Type:      BlockTypeList,
			ListLevel: 2,
			Words:     []*Word{{String: "-"}, {String: "Level"}, {String: "2"}},
		},
		{
			Type:      BlockTypeList,
			ListLevel: 0,
			Words:     []*Word{{String: "-"}, {String: "Back"}, {String: "to"}, {String: "0"}},
		},
	}

	got := LinesToText(lines, false)
	want := "- Level 0\n  - Level 1\n    - Level 2\n- Back to 0\n"

	if got != want {
		t.Errorf("LinesToText() = %q, want %q", got, want)
	}
}

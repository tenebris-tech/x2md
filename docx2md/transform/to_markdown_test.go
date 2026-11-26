/******************************************************************************
 * Copyright (c) 2026 Tenebris Technologies Inc.                              *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package transform

import (
	"strings"
	"testing"

	"github.com/tenebris-tech/x2md/pdf2md/models"
)

func TestNeedsMarker(t *testing.T) {
	tests := []struct {
		name       string
		block      models.TextBlock
		lastMarked int
		want       bool
	}{
		{"unknown position never gets a marker", models.TextBlock{Category: "H1", Paragraph: 0}, 0, false},
		{"first block always gets one", models.TextBlock{Category: "paragraph", Paragraph: 3}, 0, true},
		{"heading", models.TextBlock{Category: "H3", Paragraph: 5}, 4, true},
		{"table", models.TextBlock{Category: "TABLE", Paragraph: 5}, 4, true},
		{"paragraph just after a marker", models.TextBlock{Category: "paragraph", Paragraph: 5}, 4, false},
		{"paragraph one short of the interval", models.TextBlock{Category: "paragraph", Paragraph: 13}, 4, false},
		{"paragraph at the interval", models.TextBlock{Category: "paragraph", Paragraph: 14}, 4, true},
		{"list at the interval", models.TextBlock{Category: "LIST", Paragraph: 30}, 4, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := needsMarker(&tt.block, tt.lastMarked); got != tt.want {
				t.Errorf("needsMarker = %v, want %v", got, tt.want)
			}
		})
	}
}

func render(markers bool, blocks ...*models.TextBlock) string {
	page := &models.Page{}
	for _, b := range blocks {
		page.Items = append(page.Items, b)
	}
	result := NewToMarkdown(markers).Transform(&models.ParseResult{Pages: []*models.Page{page}})
	var out strings.Builder
	for _, item := range result.Pages[0].Items {
		out.WriteString(item.(string))
	}
	return out.String()
}

func TestToMarkdown_MarkerPlacement(t *testing.T) {
	got := render(true,
		&models.TextBlock{Category: "H1", Text: "# Title", Paragraph: 1},
		&models.TextBlock{Category: "paragraph", Text: "Body", Paragraph: 2},
		&models.TextBlock{Category: "LIST", Text: "- one\n- two", Paragraph: 3},
		&models.TextBlock{Category: "TABLE", Text: "| A |\n| --- |\n| 1 |", Paragraph: 5},
		&models.TextBlock{Category: "paragraph", Text: "After", Paragraph: 7},
	)
	want := "<!-- para 1 -->\n\n# Title\n\nBody\n\n- one\n- two\n" +
		"\n<!-- para 5 -->\n\n| A |\n| --- |\n| 1 |\n\nAfter\n\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestToMarkdown_ListEndsWithBlankLine(t *testing.T) {
	got := render(false,
		&models.TextBlock{Category: "LIST", Text: "1. one\n    - two", Paragraph: 3},
		&models.TextBlock{Category: "paragraph", Text: "After", Paragraph: 5},
	)
	want := "1. one\n    - two\n\nAfter\n\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestToMarkdown_MarkersDisabled(t *testing.T) {
	got := render(false,
		&models.TextBlock{Category: "H1", Text: "# Title", Paragraph: 1},
		&models.TextBlock{Category: "TABLE", Text: "| A |\n| --- |", Paragraph: 2},
	)
	if strings.Contains(got, "<!--") {
		t.Errorf("expected no markers, got:\n%s", got)
	}
}

func TestToTextBlocks_CarriesParagraphAndTableCategory(t *testing.T) {
	table := &models.LineItemBlock{}
	table.AddItem(&models.LineItem{IsTableRow: true, IsTableHeader: true, TableColumns: []string{"A", "B"}, Paragraph: 9,
		Words: []*models.Word{{String: "A | B"}}})
	para := &models.LineItemBlock{}
	para.AddItem(&models.LineItem{Words: []*models.Word{{String: "text"}}, Paragraph: 12})

	result := NewToTextBlocks().Transform(&models.ParseResult{Pages: []*models.Page{{Items: []interface{}{table, para}}}})
	items := result.Pages[0].Items
	if tb := items[0].(*models.TextBlock); tb.Category != "TABLE" || tb.Paragraph != 9 {
		t.Errorf("table block = %+v, want category TABLE at paragraph 9", tb)
	}
	if tb := items[1].(*models.TextBlock); tb.Category != "paragraph" || tb.Paragraph != 12 {
		t.Errorf("paragraph block = %+v, want category paragraph at paragraph 12", tb)
	}
}

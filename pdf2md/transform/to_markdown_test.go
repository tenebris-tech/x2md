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

func textPage(index int, blocks ...string) *models.Page {
	page := &models.Page{Index: index}
	for _, b := range blocks {
		page.Items = append(page.Items, &models.TextBlock{Text: b})
	}
	return page
}

func runToMarkdown(pageMarkers bool, pages ...*models.Page) string {
	result := NewToMarkdown(pageMarkers).Transform(&models.ParseResult{Pages: pages})
	var out strings.Builder
	for _, page := range result.Pages {
		for _, item := range page.Items {
			out.WriteString(item.(string))
		}
	}
	return out.String()
}

func TestPageMarkers_PhysicalPageNumbers(t *testing.T) {
	// Page index 1 is missing (e.g. stripped as blank): numbering must not shift
	got := runToMarkdown(true,
		textPage(0, "First page text"),
		textPage(2, "Third page text"),
	)
	want := "<!-- page 1 -->\n\nFirst page text\n\n<!-- page 3 -->\n\nThird page text\n\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestPageMarkers_Disabled(t *testing.T) {
	got := runToMarkdown(false, textPage(0, "First"), textPage(1, "Second"))
	if strings.Contains(got, "<!--") {
		t.Errorf("expected no markers, got:\n%s", got)
	}
}

func TestPageMarkers_EmptyPageHasNoMarker(t *testing.T) {
	got := runToMarkdown(true, textPage(0, "First"), textPage(1), textPage(2, "Third"))
	if strings.Contains(got, "<!-- page 2 -->") {
		t.Errorf("empty page should not get a marker, got:\n%s", got)
	}
}

func TestPageMarkers_ScannedPageInOrder(t *testing.T) {
	got := runToMarkdown(true,
		textPage(0, "First"),
		&models.Page{Index: 1, IsScanned: true, ScanImage: "page_002"},
		textPage(2, "Third"),
	)
	want := "<!-- page 1 -->\n\nFirst\n\n<!-- page 2 -->\n\n![page_002]\n\n<!-- page 3 -->\n\nThird\n\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestPageMarkers_TableAcrossPages(t *testing.T) {
	got := runToMarkdown(true,
		textPage(0, "Intro", "| A | B |\n| --- | --- |\n| 1 | 2 |"),
		textPage(1, "| A | B |\n| --- | --- |\n| 3 | 4 |"),
		textPage(2, "| 5 | 6 |\n| 7 | 8 |", "After the table"),
		textPage(3, "Last page"),
	)
	want := strings.Join([]string{
		"<!-- page 1 -->",
		"",
		"Intro",
		"",
		"<!-- pages 1-3 -->",
		"",
		"| A | B |",
		"| --- | --- |",
		"| 1 | 2 |",
		"| 3 | 4 |",
		"| 5 | 6 |",
		"| 7 | 8 |",
		"",
		"<!-- page 3 -->",
		"",
		"After the table",
		"",
		"<!-- page 4 -->",
		"",
		"Last page",
		"",
		"",
	}, "\n")
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestPageMarkers_TableWithinOnePage(t *testing.T) {
	got := runToMarkdown(true,
		textPage(0, "| A | B |\n| --- | --- |\n| 1 | 2 |"),
		textPage(1, "Next page"),
	)
	if strings.Contains(got, "<!-- pages") {
		t.Errorf("single-page table should not get a range marker, got:\n%s", got)
	}
	if !strings.Contains(got, "| 1 | 2 |\n\n<!-- page 2 -->\n\nNext page") {
		t.Errorf("unexpected output:\n%s", got)
	}
}

// A table continued on the next page without its header has its first row
// there taken as a header; the merged table keeps that row as data and has
// one header separator
func TestTableAcrossPages_FirstRowNotRepeatedHeader(t *testing.T) {
	got := runToMarkdown(false,
		textPage(0, "| Term | Meaning |\n| --- | --- |\n| A | First |"),
		textPage(1, "| B | Second |\n| --- | --- |\n| C | Third |"),
	)
	want := "| Term | Meaning |\n| --- | --- |\n| A | First |\n| B | Second |\n| C | Third |\n\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// A new table that has the same header as an earlier one keeps its header;
// only a table continued at the top of the next page loses the repeat
func TestTableHeader_RepeatedOnlyRemovedOnContinuation(t *testing.T) {
	got := runToMarkdown(false,
		textPage(0, "| A | B |\n| --- | --- |\n| 1 | 2 |", "Some prose"),
		textPage(1, "More prose", "| A | B |\n| --- | --- |\n| 3 | 4 |"),
	)
	if strings.Count(got, "| A | B |") != 2 || strings.Count(got, "| --- | --- |") != 2 {
		t.Errorf("both tables should keep their header, got:\n%s", got)
	}
}

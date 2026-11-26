/******************************************************************************
 * Copyright (c) 2025-2026 Tenebris Technologies Inc.                         *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package transform

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/tenebris-tech/x2md/pdf2md/models"
)

// ToMarkdown converts text blocks to final markdown
type ToMarkdown struct {
	pageMarkers bool
}

// NewToMarkdown creates a new ToMarkdown transformation.
// When pageMarkers is true, each page's content is preceded by a page marker.
func NewToMarkdown(pageMarkers bool) *ToMarkdown {
	return &ToMarkdown{pageMarkers: pageMarkers}
}

// Page markers are HTML comments on their own line. The number is the
// 1-based physical page of the source document (what a viewer's "go to page"
// uses), not the number printed on the page.
//
//	<!-- page 12 -->     content from here on starts on page 12
//	<!-- pages 3-5 -->   the table that follows spans pages 3 to 5
var pageMarkerPattern = regexp.MustCompile(`^<!-- pages? (\d+)(?:-\d+)? -->$`)

// PageMarker returns the marker for a single 1-based page number
func PageMarker(page int) string {
	return fmt.Sprintf("<!-- page %d -->", page)
}

func pageRangeMarker(first, last int) string {
	return fmt.Sprintf("<!-- pages %d-%d -->", first, last)
}

// parsePageMarker returns the (first) page number of a marker line
func parsePageMarker(line string) (int, bool) {
	m := pageMarkerPattern.FindStringSubmatch(line)
	if m == nil {
		return 0, false
	}
	var page int
	fmt.Sscanf(m[1], "%d", &page)
	return page, true
}

var newlinePattern = regexp.MustCompile(`(\r\n|\n|\r)`)

// Transform converts blocks to markdown
func (t *ToMarkdown) Transform(result *models.ParseResult) *models.ParseResult {
	// Track table headers we've seen to deduplicate across pages
	seenTableHeaders := make(map[string]bool)

	for _, page := range result.Pages {
		var text strings.Builder
		// A table continued from the previous page comes before any other
		// content of its page; only there is a repeated header a repeat
		firstContent := true

		for _, item := range page.Items {
			block, ok := item.(*models.TextBlock)
			if !ok {
				continue
			}

			var concatText string
			if block.Category == "TOC" {
				concatText = block.Text
			} else if isTableContent(block.Text) {
				// Preserve newlines in table content
				// Deduplicate table headers that repeat across pages
				concatText = deduplicateTableHeader(block.Text, seenTableHeaders, firstContent)
			} else {
				concatText = newlinePattern.ReplaceAllString(block.Text, " ")
			}

			// Concatenate words that were previously broken by newline
			if block.Category != "LIST" && !isTableContent(concatText) {
				concatText = strings.ReplaceAll(concatText, "- ", "")
			}

			// Remove backticks from code blocks (assume no actual code blocks)
			if block.Category == "CODE" {
				concatText = strings.ReplaceAll(concatText, "`", "")
			}

			// Skip empty content (e.g., removed duplicate headers)
			if strings.TrimSpace(concatText) == "" {
				continue
			}

			text.WriteString(concatText)
			text.WriteString("\n\n")
			firstContent = false
		}

		// Scanned pages have no text; reference the page image instead
		if page.IsScanned && page.ScanImage != "" {
			text.WriteString(fmt.Sprintf("![%s]\n\n", page.ScanImage))
		}

		// Set page items to just the text, preceded by the page marker
		pageText := text.String()
		if t.pageMarkers && pageText != "" {
			pageText = PageMarker(page.Index+1) + "\n\n" + pageText
		}
		page.Items = []interface{}{pageText}
	}

	// Merge continuation tables across pages
	// This joins table rows that were split across page boundaries
	result = t.mergeTablesCrossPages(result)

	return result
}

// mergeTablesCrossPages merges continuation table rows across page boundaries
func (t *ToMarkdown) mergeTablesCrossPages(result *models.ParseResult) *models.ParseResult {
	// Collect all page content
	// A page break line between pages marks where one ends whether or not
	// page markers are written; it is never emitted
	var allContent strings.Builder
	for i, page := range result.Pages {
		if i > 0 {
			if text := allContent.String(); text != "" && !strings.HasSuffix(text, "\n") {
				allContent.WriteString("\n")
			}
			allContent.WriteString(pageBreakLine + "\n")
		}
		for _, item := range page.Items {
			if text, ok := item.(string); ok {
				allContent.WriteString(text)
			}
		}
	}

	fullText := allContent.String()

	// Merge consecutive table rows that are separated by empty lines
	// Pattern: table row, newlines, table row (without header separator)
	//
	// A page marker cannot sit between the rows of a table without breaking
	// it, so a marker that falls inside a continued table is held back: the
	// table gets a page range marker in front of it, and the held marker is
	// emitted after the table so that following content is attributed to
	// the page the table ended on.
	lines := strings.Split(fullText, "\n")
	var mergedLines []string
	var inContinuationTable bool
	currentPage := 0      // Page of the last marker seen
	tableStart := -1      // Index in mergedLines of the current table's first row
	tableFirstPage := 0   // Page the current table started on
	heldPage := 0         // Page of a marker held back inside the current table
	hasSeparator := false // Whether the current table has its header separator
	tableColumns := 0     // Number of columns of the current table's last row
	pageEnded := false    // Whether a page has ended since the table's last row

	// continues reports whether the current table goes on after line i: the
	// next line with content is a data row with as many columns, and a page
	// ends in between. Two tables on one page are never one table.
	continues := func(i int) bool {
		crossed := pageEnded
		for j := i; j < len(lines); j++ {
			nextTrimmed := strings.TrimSpace(lines[j])
			if _, ok := parsePageMarker(nextTrimmed); ok || nextTrimmed == pageBreakLine {
				crossed = true
				continue
			}
			if j == i || nextTrimmed == "" {
				continue
			}
			return crossed && strings.HasPrefix(nextTrimmed, "|") && !strings.Contains(nextTrimmed, "---") &&
				tableRowColumns(nextTrimmed) == tableColumns
		}
		return false
	}

	// endTable closes the current table, emitting markers held back inside it
	endTable := func() {
		if heldPage > 0 && tableStart >= 0 {
			rangeLines := []string{pageRangeMarker(tableFirstPage, heldPage), ""}
			mergedLines = append(mergedLines[:tableStart],
				append(rangeLines, mergedLines[tableStart:]...)...)
			mergedLines = append(mergedLines, "", PageMarker(heldPage))
		}
		inContinuationTable = false
		tableStart = -1
		heldPage = 0
		hasSeparator = false
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check if this is a table data row (starts with | but not a separator)
		isTableRow := strings.HasPrefix(trimmed, "|") && !strings.Contains(trimmed, "---")
		isSeparator := strings.HasPrefix(trimmed, "|") && strings.Contains(trimmed, "---")

		if trimmed == pageBreakLine {
			if inContinuationTable && !continues(i) {
				endTable()
			}
			pageEnded = true
			continue
		}

		if page, ok := parsePageMarker(trimmed); ok {
			pageEnded = true
			if inContinuationTable && continues(i) {
				// Table continues onto this page - hold the marker back
				heldPage = page
				currentPage = page
				continue
			}
			if inContinuationTable {
				endTable()
			}
			currentPage = page
			mergedLines = append(mergedLines, line)
		} else if isSeparator && inContinuationTable && hasSeparator {
			// A table continued on the next page without repeating its
			// header has its first row there taken as a header; a table
			// has one header, so that row stays a data row
			continue
		} else if isTableRow || isSeparator {
			if isTableRow && inContinuationTable && len(mergedLines) > 0 {
				// Remove trailing empty lines before appending this row
				for len(mergedLines) > 0 && strings.TrimSpace(mergedLines[len(mergedLines)-1]) == "" {
					mergedLines = mergedLines[:len(mergedLines)-1]
				}
			}
			if !inContinuationTable {
				tableStart = len(mergedLines)
				tableFirstPage = currentPage
			}
			mergedLines = append(mergedLines, line)
			inContinuationTable = true
			if isSeparator {
				hasSeparator = true
			} else {
				tableColumns = tableRowColumns(trimmed)
			}
			pageEnded = false
		} else if trimmed == "" && inContinuationTable {
			if !continues(i) {
				// End of table - add blank line
				endTable()
				mergedLines = append(mergedLines, line)
			}
			// If next is table row, skip this empty line (continuation)
		} else {
			if inContinuationTable {
				endTable()
			}
			mergedLines = append(mergedLines, line)
		}
	}
	if inContinuationTable {
		endTable()
	}

	mergedText := strings.Join(mergedLines, "\n")

	// Replace all page content with the merged content
	// Create a single "page" with all content
	if len(result.Pages) > 0 {
		result.Pages = []*models.Page{{
			Index: 0,
			Items: []interface{}{mergedText},
		}}
	}

	return result
}

// pageBreakLine separates the text of two pages while tables are merged
// across pages; it is never part of the output
const pageBreakLine = "\x00page-break"

// tableRowColumns returns the number of cells in a Markdown table row: its
// pipes less one, not counting pipes escaped inside cells (\|)
func tableRowColumns(row string) int {
	return strings.Count(row, "|") - strings.Count(row, `\|`) - 1
}

// deduplicateTableHeader removes duplicate table headers from continuation tables.
// The first occurrence of a header is kept; a later one is stripped when the
// table starts its page (continuation), and kept otherwise (a new table that
// happens to have the same header).
func deduplicateTableHeader(text string, seenHeaders map[string]bool, startsPage bool) string {
	lines := strings.Split(text, "\n")
	if len(lines) < 2 {
		return text
	}

	// Find the header row (first line starting with |)
	headerIdx := -1
	separatorIdx := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "|") && strings.Contains(trimmed[1:], "|") {
			if headerIdx < 0 {
				headerIdx = i
			} else if strings.Contains(trimmed, "---") {
				separatorIdx = i
				break
			}
		}
	}

	if headerIdx < 0 || separatorIdx < 0 {
		return text
	}

	// Normalize the header for comparison (strip spacing)
	headerLine := strings.TrimSpace(lines[headerIdx])
	normalizedHeader := normalizeTableHeader(headerLine)

	if seenHeaders[normalizedHeader] && startsPage {
		// This is a duplicate header - remove header and separator
		var result []string
		for i, line := range lines {
			if i != headerIdx && i != separatorIdx {
				result = append(result, line)
			}
		}
		return strings.Join(result, "\n")
	}

	// First occurrence - remember it
	seenHeaders[normalizedHeader] = true
	return text
}

// normalizeTableHeader creates a canonical form of a table header for comparison
func normalizeTableHeader(header string) string {
	// Remove | and extra spaces, lowercase
	parts := strings.Split(header, "|")
	var normalized []string
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		if p != "" {
			normalized = append(normalized, p)
		}
	}
	return strings.Join(normalized, "|")
}

// isTableContent checks if the text contains markdown table formatting
func isTableContent(text string) bool {
	// Table rows start with | and contain | somewhere
	lines := strings.Split(text, "\n")
	tableLineCount := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "|") && strings.Contains(trimmed[1:], "|") {
			tableLineCount++
		}
	}
	// Consider it table content if at least 2 lines look like table rows
	return tableLineCount >= 2
}

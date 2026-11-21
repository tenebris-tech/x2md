/******************************************************************************
 * Copyright (c) 2025-2026 Tenebris Technologies Inc.                         *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package transform

import (
	"fmt"
	"strings"

	"github.com/tenebris-tech/x2md/pdf2md/models"
)

// ToMarkdown converts TextBlocks to final markdown strings
type ToMarkdown struct {
	paragraphMarkers bool
}

// NewToMarkdown creates a new ToMarkdown transformation.
// When paragraphMarkers is true, paragraph markers are added to the output.
func NewToMarkdown(paragraphMarkers bool) *ToMarkdown {
	return &ToMarkdown{paragraphMarkers: paragraphMarkers}
}

// ParagraphMarkerInterval is the largest number of paragraphs that may pass
// without a marker. Headings and tables always get one.
const ParagraphMarkerInterval = 10

// ParagraphMarker returns the marker for a 1-based paragraph number.
// The content after the marker starts at that paragraph of the document body,
// counting every paragraph in document order including those in table cells.
func ParagraphMarker(paragraph int) string {
	return fmt.Sprintf("<!-- para %d -->", paragraph)
}

// needsMarker decides whether a block gets a paragraph marker
func needsMarker(block *models.TextBlock, lastMarked int) bool {
	if block.Paragraph <= 0 {
		return false
	}
	if lastMarked == 0 {
		return true
	}
	switch block.Category {
	case "H1", "H2", "H3", "H4", "H5", "H6", "TABLE":
		return true
	}
	return block.Paragraph-lastMarked >= ParagraphMarkerInterval
}

// Transform converts TextBlocks to markdown strings
func (t *ToMarkdown) Transform(result *models.ParseResult) *models.ParseResult {
	lastMarked := 0 // Paragraph number of the last marker emitted

	for _, page := range result.Pages {
		var newItems []interface{}

		for _, item := range page.Items {
			textBlock, ok := item.(*models.TextBlock)
			if !ok {
				// Already a string or unknown type
				if s, ok := item.(string); ok {
					newItems = append(newItems, s)
				}
				continue
			}

			text := textBlock.Text

			// Ensure proper spacing between blocks
			text = strings.TrimRight(text, "\n")

			// Every block ends with a blank line. A list block holds all its
			// items, so this also ends the list: without it the following
			// paragraph would be read as part of the last item.
			text = text + "\n\n"

			if t.paragraphMarkers && needsMarker(textBlock, lastMarked) {
				newItems = append(newItems, ParagraphMarker(textBlock.Paragraph)+"\n\n")
				lastMarked = textBlock.Paragraph
			}

			newItems = append(newItems, text)
		}

		page.Items = newItems
	}

	return result
}

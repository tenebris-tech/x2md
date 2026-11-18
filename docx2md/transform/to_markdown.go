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
	lastMarked := 0    // Paragraph number of the last marker emitted
	previous := "\n\n" // Text of the previous item, to keep markers on their own line

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

			// Add appropriate newlines based on block type
			// Markdown requires blank lines between paragraphs (\n\n)
			switch textBlock.Category {
			case "H1", "H2", "H3", "H4", "H5", "H6":
				// Headings need blank line after
				text = text + "\n\n"
			case "LIST":
				// List items - single newline within lists
				text = text + "\n"
			default:
				// Regular paragraphs need blank line after
				text = text + "\n\n"
			}

			if t.paragraphMarkers && needsMarker(textBlock, lastMarked) {
				// A marker needs a blank line before it (lists end with one newline)
				marker := ParagraphMarker(textBlock.Paragraph) + "\n\n"
				if !strings.HasSuffix(previous, "\n\n") {
					marker = "\n" + marker
				}
				newItems = append(newItems, marker)
				lastMarked = textBlock.Paragraph
			}

			newItems = append(newItems, text)
			previous = text
		}

		page.Items = newItems
	}

	return result
}

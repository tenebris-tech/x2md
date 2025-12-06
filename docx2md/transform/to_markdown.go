package transform

import (
	"strings"

	"github.com/tenebris-tech/x2md/pdf2md/models"
)

// ToMarkdown converts TextBlocks to final markdown strings
type ToMarkdown struct{}

// NewToMarkdown creates a new ToMarkdown transformation
func NewToMarkdown() *ToMarkdown {
	return &ToMarkdown{}
}

// Transform converts TextBlocks to markdown strings
func (t *ToMarkdown) Transform(result *models.ParseResult) *models.ParseResult {
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
			switch textBlock.Category {
			case "H1", "H2", "H3", "H4", "H5", "H6":
				// Headings need blank line before and after
				text = text + "\n"
			case "LIST":
				// List items
				text = text + "\n"
			default:
				// Regular paragraphs
				text = text + "\n"
			}

			newItems = append(newItems, text)
		}

		page.Items = newItems
	}

	return result
}

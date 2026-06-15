package transform

import (
	"github.com/tenebris-tech/x2md/pdf2md/models"
)

// ToTextBlocks converts LineItemBlocks to text strings.
type ToTextBlocks struct {
	options *PipelineOptions
}

// NewToTextBlocks creates a new ToTextBlocks transformation
func NewToTextBlocks(opts *PipelineOptions) *ToTextBlocks {
	if opts == nil {
		opts = &PipelineOptions{
			PreserveFormatting: true,
			PreserveInlineHTML: true,
		}
	}
	return &ToTextBlocks{options: opts}
}

// Transform converts LineItemBlocks to text
func (t *ToTextBlocks) Transform(result *models.ParseResult) *models.ParseResult {
	for _, page := range result.Pages {
		var newItems []interface{}

		for _, item := range page.Items {
			block, ok := item.(*models.LineItemBlock)
			if !ok {
				newItems = append(newItems, item)
				continue
			}

			// DOCX runs concatenate directly. The renderer must not insert
			// PDF-style token spaces between adjacent runs.
			text := models.BlockToTextWithOptions(block, models.TextRenderOptions{
				DisableInlineFormats: !t.options.PreserveFormatting,
				PreserveInlineHTML:   t.options.PreserveInlineHTML,
				NoImplicitWhitespace: true,
				PreserveWordTypes:    true,
			})

			// Create text block with category for tracking
			category := "paragraph"
			if block.Type != nil {
				category = block.Type.Name
			}

			newItems = append(newItems, &models.TextBlock{
				Category: category,
				Text:     text,
			})
		}

		page.Items = newItems
	}

	return result
}

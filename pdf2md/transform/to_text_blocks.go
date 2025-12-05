package transform

import (
	"github.com/tenebris-tech/x2md/pdf2md/models"
)

// ToTextBlocks converts blocks to text blocks
type ToTextBlocks struct{}

// NewToTextBlocks creates a new ToTextBlocks transformation
func NewToTextBlocks() *ToTextBlocks {
	return &ToTextBlocks{}
}

// Transform converts blocks to text blocks
func (t *ToTextBlocks) Transform(result *models.ParseResult) *models.ParseResult {
	for _, page := range result.Pages {
		var textBlocks []interface{}

		for _, item := range page.Items {
			block, ok := item.(*models.LineItemBlock)
			if !ok {
				continue
			}

			category := "Unknown"
			if block.Type != nil {
				category = block.Type.Name
			}

			textBlock := &models.TextBlock{
				Category: category,
				Text:     models.BlockToText(block),
			}
			textBlocks = append(textBlocks, textBlock)
		}

		page.Items = textBlocks
	}

	return result
}

/******************************************************************************
 * Copyright (c) 2025-2026 Tenebris Technologies Inc.                         *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package transform

import (
	"github.com/tenebris-tech/x2md/pdf2md/models"
)

// ToTextBlocks converts LineItemBlocks to text strings
type ToTextBlocks struct{}

// NewToTextBlocks creates a new ToTextBlocks transformation
func NewToTextBlocks() *ToTextBlocks {
	return &ToTextBlocks{}
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

			// Convert block to text using the shared models function
			text := models.BlockToText(block)

			// Create text block with category for tracking
			category := "paragraph"
			if block.Type != nil {
				category = block.Type.Name
			} else if isTableBlock(block) {
				category = "TABLE"
			}

			paragraph := 0
			if len(block.Items) > 0 {
				paragraph = block.Items[0].Paragraph
			}

			newItems = append(newItems, &models.TextBlock{
				Category:  category,
				Text:      text,
				Paragraph: paragraph,
			})
		}

		page.Items = newItems
	}

	return result
}

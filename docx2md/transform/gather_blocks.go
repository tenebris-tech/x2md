package transform

import (
	"github.com/tenebris-tech/x2md/pdf2md/models"
)

// GatherBlocks groups LineItems into LineItemBlocks
type GatherBlocks struct{}

// NewGatherBlocks creates a new GatherBlocks transformation
func NewGatherBlocks() *GatherBlocks {
	return &GatherBlocks{}
}

// Transform groups LineItems into blocks
func (t *GatherBlocks) Transform(result *models.ParseResult) *models.ParseResult {
	for _, page := range result.Pages {
		var newItems []interface{}
		var currentBlock *models.LineItemBlock

		for _, item := range page.Items {
			lineItem, ok := item.(*models.LineItem)
			if !ok {
				// Non-LineItem, flush current block and add
				if currentBlock != nil && len(currentBlock.Items) > 0 {
					newItems = append(newItems, currentBlock)
					currentBlock = nil
				}
				newItems = append(newItems, item)
				continue
			}

			// Handle table rows - keep them together
			if lineItem.IsTableRow {
				if currentBlock == nil || !isTableBlock(currentBlock) {
					// Start new table block
					if currentBlock != nil && len(currentBlock.Items) > 0 {
						newItems = append(newItems, currentBlock)
					}
					currentBlock = &models.LineItemBlock{}
				}
				currentBlock.AddItem(lineItem)
				continue
			}

			// Non-table content after table - flush table block
			if currentBlock != nil && isTableBlock(currentBlock) {
				newItems = append(newItems, currentBlock)
				currentBlock = nil
			}

			// Handle headings - each heading is its own block
			if lineItem.Type != nil && lineItem.Type.Headline {
				if currentBlock != nil && len(currentBlock.Items) > 0 {
					newItems = append(newItems, currentBlock)
				}
				currentBlock = &models.LineItemBlock{Type: lineItem.Type}
				currentBlock.AddItem(lineItem)
				newItems = append(newItems, currentBlock)
				currentBlock = nil
				continue
			}

			// Handle list items
			if lineItem.Type == models.BlockTypeList {
				if currentBlock == nil || currentBlock.Type != models.BlockTypeList {
					if currentBlock != nil && len(currentBlock.Items) > 0 {
						newItems = append(newItems, currentBlock)
					}
					currentBlock = &models.LineItemBlock{Type: models.BlockTypeList}
				}
				currentBlock.AddItem(lineItem)
				continue
			}

			// Regular paragraph - each paragraph is its own block
			if currentBlock != nil && len(currentBlock.Items) > 0 {
				newItems = append(newItems, currentBlock)
			}
			currentBlock = &models.LineItemBlock{}
			currentBlock.AddItem(lineItem)
		}

		// Flush remaining block
		if currentBlock != nil && len(currentBlock.Items) > 0 {
			newItems = append(newItems, currentBlock)
		}

		page.Items = newItems
	}

	return result
}

// isTableBlock checks if a block contains table rows
func isTableBlock(block *models.LineItemBlock) bool {
	if len(block.Items) == 0 {
		return false
	}
	return block.Items[0].IsTableRow
}

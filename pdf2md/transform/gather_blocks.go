package transform

import (
	"github.com/tenebris-tech/x2md/pdf2md/models"
)

// GatherBlocks gathers lines into blocks
type GatherBlocks struct{}

// NewGatherBlocks creates a new GatherBlocks transformation
func NewGatherBlocks() *GatherBlocks {
	return &GatherBlocks{}
}

// Transform gathers lines into blocks
func (g *GatherBlocks) Transform(result *models.ParseResult) *models.ParseResult {
	mostUsedDistance := result.Globals.MostUsedDistance

	for _, page := range result.Pages {
		var blocks []interface{}
		stashedBlock := &models.LineItemBlock{}

		flushStashedItems := func() {
			if len(stashedBlock.Items) > 1 {
				stashedBlock.Annotation = models.DetectedAnnotation
			}
			blocks = append(blocks, stashedBlock)
			stashedBlock = &models.LineItemBlock{}
		}

		// Find minimum X
		minX := g.findMinX(page.Items)

		for _, item := range page.Items {
			lineItem, ok := item.(*models.LineItem)
			if !ok {
				continue
			}

			if len(stashedBlock.Items) > 0 && g.shouldFlushBlock(stashedBlock, lineItem, minX, mostUsedDistance) {
				flushStashedItems()
			}
			stashedBlock.AddItem(lineItem)
		}

		if len(stashedBlock.Items) > 0 {
			flushStashedItems()
		}

		page.Items = blocks
	}

	return result
}

func (g *GatherBlocks) findMinX(items []interface{}) float64 {
	minX := float64(999999)
	for _, item := range items {
		if lineItem, ok := item.(*models.LineItem); ok {
			if lineItem.X < minX {
				minX = lineItem.X
			}
		}
	}
	return minX
}

func (g *GatherBlocks) shouldFlushBlock(stashedBlock *models.LineItemBlock, item *models.LineItem, minX float64, mostUsedDistance int) bool {
	if stashedBlock.Type != nil && stashedBlock.Type.MergeFollowingNonTypedItems && item.Type == nil {
		return false
	}

	lastItem := stashedBlock.Items[len(stashedBlock.Items)-1]
	hasBigDistance := g.bigDistance(lastItem, item, minX, mostUsedDistance)

	// Keep table rows together in the same block
	// (both current and previous items are table rows)
	if lastItem.IsTableRow && item.IsTableRow {
		return false
	}

	// Flush if transitioning between table and non-table content
	if lastItem.IsTableRow != item.IsTableRow {
		return true
	}

	if stashedBlock.Type != nil && stashedBlock.Type.MergeFollowingNonTypedItemsWithSmallDistance && item.Type == nil && !hasBigDistance {
		return false
	}

	if item.Type != stashedBlock.Type {
		return true
	}

	if item.Type != nil {
		return !item.Type.MergeToBlock
	}

	return hasBigDistance
}

func (g *GatherBlocks) bigDistance(lastItem, item *models.LineItem, minX float64, mostUsedDistance int) bool {
	distance := lastItem.Y - item.Y

	// Distance is negative - and not only a bit
	if distance < float64(-mostUsedDistance)/2 {
		return true
	}

	allowedDistance := float64(mostUsedDistance + 1)

	// Indented elements like lists often have greater spacing
	if lastItem.X > minX && item.X > minX {
		allowedDistance = float64(mostUsedDistance) + float64(mostUsedDistance)/2
	}

	if distance > allowedDistance {
		return true
	}

	return false
}

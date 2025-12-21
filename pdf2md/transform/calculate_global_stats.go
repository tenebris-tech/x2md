package transform

import (
	"strings"

	"github.com/tenebris-tech/x2md/pdf2md/models"
	"github.com/tenebris-tech/x2md/pdf2md/pdf"
)

// CalculateGlobalStats calculates global document statistics
type CalculateGlobalStats struct {
	fontMap map[string]*pdf.Font
}

// NewCalculateGlobalStats creates a new CalculateGlobalStats transformation
func NewCalculateGlobalStats(fontMap map[string]*pdf.Font) *CalculateGlobalStats {
	return &CalculateGlobalStats{fontMap: fontMap}
}

// Transform calculates global stats
func (c *CalculateGlobalStats) Transform(result *models.ParseResult) *models.ParseResult {
	heightToOccurrence := make(map[int]int)
	fontToOccurrence := make(map[string]int)
	var maxHeight int
	var maxHeightFont string

	// Parse heights and fonts
	for _, page := range result.Pages {
		for _, item := range page.Items {
			textItem, ok := item.(*models.TextItem)
			if !ok || textItem.Height == 0 {
				continue
			}

			height := int(textItem.Height)
			heightToOccurrence[height]++
			fontToOccurrence[textItem.Font]++

			if height > maxHeight {
				maxHeight = height
				maxHeightFont = textItem.Font
			}
		}
	}

	mostUsedHeight := getMostUsedKey(heightToOccurrence)
	mostUsedFont := getMostUsedKeyStr(fontToOccurrence)

	// Parse line distances
	distanceToOccurrence := make(map[int]int)
	for _, page := range result.Pages {
		var lastItemOfMostUsedHeight *models.TextItem
		for _, item := range page.Items {
			textItem, ok := item.(*models.TextItem)
			if !ok {
				continue
			}

			if int(textItem.Height) == mostUsedHeight && strings.TrimSpace(textItem.Text) != "" {
				if lastItemOfMostUsedHeight != nil && textItem.Y != lastItemOfMostUsedHeight.Y {
					distance := int(lastItemOfMostUsedHeight.Y - textItem.Y)
					if distance > 0 {
						distanceToOccurrence[distance]++
					}
				}
				lastItemOfMostUsedHeight = textItem
			} else {
				lastItemOfMostUsedHeight = nil
			}
		}
	}

	mostUsedDistance := getMostUsedKey(distanceToOccurrence)
	if mostUsedDistance == 0 {
		mostUsedDistance = 12 // Default
	}

	// Build font to formats map
	fontToFormats := make(map[string]*models.WordFormat)
	for fontID, font := range c.fontMap {
		if fontID == mostUsedFont {
			continue
		}

		fontName := strings.ToLower(font.BaseFont)
		var format *models.WordFormat

		if strings.Contains(fontName, "bold") && (strings.Contains(fontName, "oblique") || strings.Contains(fontName, "italic")) {
			format = models.WordFormatBoldOblique
		} else if strings.Contains(fontName, "bold") {
			format = models.WordFormatBold
		} else if strings.Contains(fontName, "oblique") || strings.Contains(fontName, "italic") {
			format = models.WordFormatOblique
		} else if fontID == maxHeightFont {
			format = models.WordFormatBold
		}

		if format != nil {
			fontToFormats[fontID] = format
		}
	}

	// Make copies of pages with copied items
	newPages := make([]*models.Page, len(result.Pages))
	for i, page := range result.Pages {
		newItems := make([]interface{}, len(page.Items))
		for j, item := range page.Items {
			if textItem, ok := item.(*models.TextItem); ok {
				copied := *textItem
				newItems[j] = &copied
			} else {
				newItems[j] = item
			}
		}
		newPages[i] = &models.Page{
			Index:     page.Index,
			Items:     newItems,
			Width:     page.Width,
			Height:    page.Height,
			IsScanned: page.IsScanned,
		}
	}

	return &models.ParseResult{
		Pages: newPages,
		Globals: &models.Globals{
			MostUsedHeight:   mostUsedHeight,
			MostUsedFont:     mostUsedFont,
			MostUsedDistance: mostUsedDistance,
			MaxHeight:        maxHeight,
			MaxHeightFont:    maxHeightFont,
			FontToFormats:    fontToFormats,
		},
		Messages: result.Messages,
	}
}

func getMostUsedKey(m map[int]int) int {
	var maxOccurrence int
	var maxKey int
	for key, occurrence := range m {
		if occurrence > maxOccurrence {
			maxOccurrence = occurrence
			maxKey = key
		}
	}
	return maxKey
}

func getMostUsedKeyStr(m map[string]int) string {
	var maxOccurrence int
	var maxKey string
	for key, occurrence := range m {
		if occurrence > maxOccurrence {
			maxOccurrence = occurrence
			maxKey = key
		}
	}
	return maxKey
}

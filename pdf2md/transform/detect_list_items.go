package transform

import (
	"regexp"

	"github.com/tenebris-tech/x2md/pdf2md/models"
)

// DetectListItems detects list items
type DetectListItems struct{}

// NewDetectListItems creates a new DetectListItems transformation
func NewDetectListItems() *DetectListItems {
	return &DetectListItems{}
}

var numberedListPattern = regexp.MustCompile(`^[\s]*[\d]+[.][\s].*$`)

// indentUnit is the approximate points per indent level in PDFs
const indentUnit = 20.0

// maxListLevel caps the nesting depth to prevent unreasonable values
const maxListLevel = 6

// Transform detects list items
func (d *DetectListItems) Transform(result *models.ParseResult) *models.ParseResult {
	for _, page := range result.Pages {
		// First pass: find minimum X position for potential list items
		minListX := findMinListX(page.Items)

		var newItems []interface{}

		for _, item := range page.Items {
			lineItem, ok := item.(*models.LineItem)
			if !ok {
				newItems = append(newItems, item)
				continue
			}

			if lineItem.Type != nil {
				newItems = append(newItems, item)
				continue
			}

			if len(lineItem.Words) == 0 {
				newItems = append(newItems, item)
				continue
			}

			text := lineItem.Text()
			firstWord := lineItem.Words[0].String

			// Check for bullet list items
			if isListItemCharacter(firstWord) {
				listLevel := calculateListLevel(lineItem.X, minListX)
				if firstWord == "-" {
					lineItem.Type = models.BlockTypeList
					lineItem.ListLevel = listLevel
					lineItem.Annotation = models.DetectedAnnotation
					newItems = append(newItems, lineItem)
				} else {
					// Replace bullet with dash
					lineItem.Annotation = models.RemovedAnnotation
					newItems = append(newItems, lineItem)

					// Create new item with dash
					newWords := make([]*models.Word, len(lineItem.Words))
					for i, w := range lineItem.Words {
						newWords[i] = &models.Word{
							String: w.String,
							Type:   w.Type,
							Format: w.Format,
						}
					}
					newWords[0].String = "-"

					newItem := &models.LineItem{
						X:              lineItem.X,
						Y:              lineItem.Y,
						Width:          lineItem.Width,
						Height:         lineItem.Height,
						Words:          newWords,
						Type:           models.BlockTypeList,
						ListLevel:      listLevel,
						Annotation:     models.AddedAnnotation,
						ParsedElements: lineItem.ParsedElements,
						Font:           lineItem.Font,
					}
					newItems = append(newItems, newItem)
				}
			} else if numberedListPattern.MatchString(text) {
				lineItem.Type = models.BlockTypeList
				lineItem.ListLevel = calculateListLevel(lineItem.X, minListX)
				lineItem.Annotation = models.DetectedAnnotation
				newItems = append(newItems, lineItem)
			} else {
				newItems = append(newItems, lineItem)
			}
		}

		page.Items = newItems
	}

	return result
}

// findMinListX finds the minimum X position among potential list items on a page
func findMinListX(items []interface{}) float64 {
	minX := -1.0
	for _, item := range items {
		lineItem, ok := item.(*models.LineItem)
		if !ok || lineItem.Type != nil || len(lineItem.Words) == 0 {
			continue
		}
		firstWord := lineItem.Words[0].String
		text := lineItem.Text()
		if isListItemCharacter(firstWord) || numberedListPattern.MatchString(text) {
			if minX < 0 || lineItem.X < minX {
				minX = lineItem.X
			}
		}
	}
	return minX
}

// calculateListLevel determines nesting level based on X offset from minimum
func calculateListLevel(x, minX float64) int {
	if minX < 0 {
		return 0
	}
	offset := x - minX
	if offset <= 0 {
		return 0
	}
	level := int(offset / indentUnit)
	if level > maxListLevel {
		level = maxListLevel
	}
	return level
}

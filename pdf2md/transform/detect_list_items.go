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

// Transform detects list items
func (d *DetectListItems) Transform(result *models.ParseResult) *models.ParseResult {
	for _, page := range result.Pages {
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
				if firstWord == "-" {
					lineItem.Type = models.BlockTypeList
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
						Annotation:     models.AddedAnnotation,
						ParsedElements: lineItem.ParsedElements,
						Font:           lineItem.Font,
					}
					newItems = append(newItems, newItem)
				}
			} else if numberedListPattern.MatchString(text) {
				lineItem.Type = models.BlockTypeList
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

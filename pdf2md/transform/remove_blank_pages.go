package transform

import (
	"github.com/tenebris-tech/x2md/pdf2md/models"
)

// RemoveBlankPages removes empty or near-empty pages
type RemoveBlankPages struct{}

// NewRemoveBlankPages creates a new RemoveBlankPages transformation
func NewRemoveBlankPages() *RemoveBlankPages {
	return &RemoveBlankPages{}
}

// Transform removes blank pages
func (r *RemoveBlankPages) Transform(result *models.ParseResult) *models.ParseResult {
	var filteredPages []*models.Page

	for _, page := range result.Pages {
		if !r.isBlankPage(page) {
			filteredPages = append(filteredPages, page)
		}
	}

	result.Pages = filteredPages
	return result
}

func (r *RemoveBlankPages) isBlankPage(page *models.Page) bool {
	// Scanned pages have no text items but are not blank
	if page.IsScanned {
		return false
	}

	if len(page.Items) == 0 {
		return true
	}

	// Count meaningful content
	// For blocks, count the number of lines inside, not just the block itself
	meaningfulItems := 0
	for _, item := range page.Items {
		switch v := item.(type) {
		case *models.LineItemBlock:
			// Count lines within the block
			meaningfulItems += len(v.Items)
		case *models.LineItem:
			if len(v.Words) > 0 {
				meaningfulItems++
			}
		case *models.TextItem:
			if len(v.Text) > 0 {
				meaningfulItems++
			}
		default:
			meaningfulItems++
		}
	}

	// Consider a page blank if it has very few meaningful items
	return meaningfulItems < 2
}

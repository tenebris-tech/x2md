package transform

import (
	"strings"

	"github.com/tenebris-tech/x2md/pdf2md/models"
)

// RemoveRepetitiveElements removes headers and footers that repeat across pages
type RemoveRepetitiveElements struct{}

// NewRemoveRepetitiveElements creates a new RemoveRepetitiveElements transformation
func NewRemoveRepetitiveElements() *RemoveRepetitiveElements {
	return &RemoveRepetitiveElements{}
}

// Transform removes repetitive elements
func (r *RemoveRepetitiveElements) Transform(result *models.ParseResult) *models.ParseResult {
	if len(result.Pages) < 3 {
		return result // Need at least 3 pages to detect patterns
	}

	// Collect first and last lines per page
	type pageInfo struct {
		minElements []*models.LineItem
		maxElements []*models.LineItem
		minY        float64
		maxY        float64
		minLineHash int
		maxLineHash int
	}

	pageStore := make([]pageInfo, len(result.Pages))
	minLineHashRepetitions := make(map[int]int)
	maxLineHashRepetitions := make(map[int]int)

	for i, page := range result.Pages {
		info := pageInfo{
			minY: 999999,
			maxY: -999999,
		}

		for _, item := range page.Items {
			lineItem, ok := item.(*models.LineItem)
			if !ok {
				continue
			}

			if lineItem.Y < info.minY {
				info.minElements = []*models.LineItem{lineItem}
				info.minY = lineItem.Y
			} else if lineItem.Y == info.minY {
				info.minElements = append(info.minElements, lineItem)
			}

			if lineItem.Y > info.maxY {
				info.maxElements = []*models.LineItem{lineItem}
				info.maxY = lineItem.Y
			} else if lineItem.Y == info.maxY {
				info.maxElements = append(info.maxElements, lineItem)
			}
		}

		// Calculate hashes
		info.minLineHash = hashCodeIgnoringSpacesAndNumbers(combineLineTexts(info.minElements))
		info.maxLineHash = hashCodeIgnoringSpacesAndNumbers(combineLineTexts(info.maxElements))

		pageStore[i] = info
		minLineHashRepetitions[info.minLineHash]++
		maxLineHashRepetitions[info.maxLineHash]++
	}

	// Mark repetitive elements as removed
	threshold := max(3, len(result.Pages)*2/3)

	for i, page := range result.Pages {
		info := pageStore[i]

		if minLineHashRepetitions[info.minLineHash] >= threshold {
			for _, elem := range info.minElements {
				elem.Annotation = models.RemovedAnnotation
			}
		}

		if maxLineHashRepetitions[info.maxLineHash] >= threshold {
			for _, elem := range info.maxElements {
				elem.Annotation = models.RemovedAnnotation
			}
		}

		// Filter out removed items
		var filtered []interface{}
		for _, item := range page.Items {
			if lineItem, ok := item.(*models.LineItem); ok {
				if lineItem.Annotation != models.RemovedAnnotation {
					filtered = append(filtered, item)
				}
			} else {
				filtered = append(filtered, item)
			}
		}
		page.Items = filtered
	}

	return result
}

func combineLineTexts(lines []*models.LineItem) string {
	var texts []string
	for _, line := range lines {
		texts = append(texts, strings.ToUpper(line.Text()))
	}
	return strings.Join(texts, "")
}

func hashCodeIgnoringSpacesAndNumbers(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	hash := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !isDigit(c) && c != ' ' && c != 160 { // 160 is non-breaking space
			hash = ((hash << 5) - hash) + int(c)
			hash = hash & 0x7FFFFFFF // Keep positive
		}
	}
	return hash
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

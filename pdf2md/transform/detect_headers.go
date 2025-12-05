package transform

import (
	"regexp"
	"sort"
	"strings"

	"github.com/tenebris-tech/x2md/pdf2md/models"
)

// DetectHeaders detects headlines based on text heights
type DetectHeaders struct{}

// NewDetectHeaders creates a new DetectHeaders transformation
func NewDetectHeaders() *DetectHeaders {
	return &DetectHeaders{}
}

// Transform detects headers
func (d *DetectHeaders) Transform(result *models.ParseResult) *models.ParseResult {
	mostUsedHeight := result.Globals.MostUsedHeight
	mostUsedFont := result.Globals.MostUsedFont
	mostUsedDistance := result.Globals.MostUsedDistance
	maxHeight := result.Globals.MaxHeight

	// If mostUsedHeight is too small (unreliable height extraction), skip height-based detection
	// and rely only on font-based detection
	heightBasedDetection := mostUsedHeight >= 8

	if heightBasedDetection {
		// Find pages with maximum height items (title pages)
		pagesWithMaxHeight := d.findPagesWithMaxHeight(result.Pages, maxHeight)

		// Handle title pages
		min2ndLevelHeight := mostUsedHeight + (maxHeight-mostUsedHeight)/4
		for _, page := range pagesWithMaxHeight {
			for _, item := range page.Items {
				lineItem, ok := item.(*models.LineItem)
				if !ok || lineItem.Type != nil {
					continue
				}

				height := int(lineItem.Height)
				if height > min2ndLevelHeight {
					if height == maxHeight {
						lineItem.Type = models.BlockTypeH1
					} else {
						lineItem.Type = models.BlockTypeH2
					}
					lineItem.Annotation = models.DetectedAnnotation
				}
			}
		}

		// Categorize headlines by text heights
		heights := d.collectHeights(result.Pages, mostUsedHeight)
		sort.Sort(sort.Reverse(sort.IntSlice(heights)))

		for i, height := range heights {
			headlineLevel := i + 2 // Start at H2 since H1 is for max height
			if headlineLevel > 6 {
				break
			}

			headlineType := models.HeadlineByLevel(headlineLevel)

			for _, page := range result.Pages {
				for _, item := range page.Items {
					lineItem, ok := item.(*models.LineItem)
					if !ok || lineItem.Type != nil {
						continue
					}

					if int(lineItem.Height) == height && !isListItem(lineItem.Text()) {
						lineItem.Type = headlineType
						lineItem.Annotation = models.DetectedAnnotation
					}
				}
			}
		}
	}

	// Find headlines with paragraph height but different font (all caps)
	var smallestHeadlineLevel int = 1
	for _, page := range result.Pages {
		for _, item := range page.Items {
			lineItem, ok := item.(*models.LineItem)
			if !ok || lineItem.Type == nil || !lineItem.Type.Headline {
				continue
			}
			if lineItem.Type.HeadlineLevel > smallestHeadlineLevel {
				smallestHeadlineLevel = lineItem.Type.HeadlineLevel
			}
		}
	}

	if smallestHeadlineLevel < 6 {
		nextHeadlineType := models.HeadlineByLevel(smallestHeadlineLevel + 1)

		for _, page := range result.Pages {
			var lastItem *models.LineItem
			for _, item := range page.Items {
				lineItem, ok := item.(*models.LineItem)
				if !ok {
					continue
				}

				text := lineItem.Text()
				if lineItem.Type == nil &&
					int(lineItem.Height) == mostUsedHeight &&
					lineItem.Font != mostUsedFont &&
					(lastItem == nil ||
						lastItem.Y < lineItem.Y ||
						(lastItem.Type != nil && lastItem.Type.Headline) ||
						lastItem.Y-lineItem.Y > float64(mostUsedDistance*2)) &&
					text == strings.ToUpper(text) &&
					len(text) > 0 {

					lineItem.Type = nextHeadlineType
					lineItem.Annotation = models.DetectedAnnotation
				}
				lastItem = lineItem
			}
		}
	}

	return result
}

func (d *DetectHeaders) findPagesWithMaxHeight(pages []*models.Page, maxHeight int) []*models.Page {
	seen := make(map[*models.Page]bool)
	var result []*models.Page

	for _, page := range pages {
		for _, item := range page.Items {
			lineItem, ok := item.(*models.LineItem)
			if !ok || lineItem.Type != nil {
				continue
			}

			if int(lineItem.Height) == maxHeight {
				if !seen[page] {
					seen[page] = true
					result = append(result, page)
				}
			}
		}
	}

	return result
}

func (d *DetectHeaders) collectHeights(pages []*models.Page, mostUsedHeight int) []int {
	heightSet := make(map[int]bool)

	for _, page := range pages {
		for _, item := range page.Items {
			lineItem, ok := item.(*models.LineItem)
			if !ok || lineItem.Type != nil {
				continue
			}

			height := int(lineItem.Height)
			if height > mostUsedHeight && !isListItem(lineItem.Text()) {
				heightSet[height] = true
			}
		}
	}

	var heights []int
	for h := range heightSet {
		heights = append(heights, h)
	}
	return heights
}

var listItemPattern = regexp.MustCompile(`^[\s]*[-•–][\s].*$`)

func isListItem(s string) bool {
	return listItemPattern.MatchString(s)
}

/******************************************************************************
 * Copyright (c) 2026 Tenebris Technologies Inc.                              *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package transform

import (
	"strings"
	"testing"

	"github.com/tenebris-tech/x2md/pdf2md/models"
)

func headerTestLine(y, height float64, text string) *models.LineItem {
	var words []*models.Word
	for _, w := range strings.Fields(text) {
		words = append(words, &models.Word{String: w})
	}
	return &models.LineItem{X: 72, Y: y, Height: height, Words: words, Font: "F1"}
}

// A bulleted line is a list item even when its bullet makes it the tallest
// text in the document; only the other tall lines are headings
func TestDetectHeaders_TallestListItemIsNotHeading(t *testing.T) {
	heading := headerTestLine(100, 14, "Contents")
	bullet := headerTestLine(130, 14, "• First entry")
	body := headerTestLine(160, 8, "Body text")

	result := &models.ParseResult{
		Pages: []*models.Page{{Items: []interface{}{heading, bullet, body}}},
		Globals: &models.Globals{
			MostUsedHeight:   8,
			MostUsedFont:     "F1",
			MostUsedDistance: 10,
			MaxHeight:        14,
		},
	}
	NewDetectHeaders().Transform(result)

	if heading.Type != models.BlockTypeH1 {
		t.Errorf("heading type = %v, want H1", heading.Type)
	}
	if bullet.Type != nil {
		t.Errorf("list item type = %v, want none", bullet.Type.Name)
	}
}

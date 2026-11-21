/******************************************************************************
 * Copyright (c) 2026 Tenebris Technologies Inc.                              *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package transform

import (
	"testing"

	"github.com/tenebris-tech/x2md/pdf2md/models"
)

// Lines are extracted top to bottom with Y growing down the page, so the
// most used distance is the gap to the next line below.
func TestMostUsedDistanceIsLineSpacing(t *testing.T) {
	var items []interface{}
	for i := 0; i < 10; i++ {
		items = append(items, &models.TextItem{X: 72, Y: 100 + float64(i)*14, Height: 11, Text: "line", Font: "F1"})
	}
	// A second column starts back at the top: an upward jump, not a line gap
	items = append(items, &models.TextItem{X: 300, Y: 100, Height: 11, Text: "line", Font: "F1"})

	result := NewCalculateGlobalStats(nil).Transform(&models.ParseResult{
		Pages: []*models.Page{{Items: items}},
	})

	if got := result.Globals.MostUsedDistance; got != 14 {
		t.Errorf("MostUsedDistance = %d, want 14", got)
	}
}

func TestGetMostUsedKeyTieIsStable(t *testing.T) {
	ints := map[int]int{18: 3, 6: 3, 40: 3, 12: 1}
	strs := map[string]int{"F3": 5, "F1": 5, "F2": 5, "F0": 1}
	for i := 0; i < 100; i++ {
		if got := getMostUsedKey(ints); got != 6 {
			t.Fatalf("getMostUsedKey = %d, want 6", got)
		}
		if got := getMostUsedKeyStr(strs); got != "F1" {
			t.Fatalf("getMostUsedKeyStr = %q, want F1", got)
		}
	}
}

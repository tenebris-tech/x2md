/******************************************************************************
 * Copyright (c) 2026 Tenebris Technologies Inc.                              *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package transform

import (
	"testing"

	"github.com/tenebris-tech/x2md/pdf2md/models"
)

func TestGatherBlocks_ListLevelNeverSkipsALevel(t *testing.T) {
	item := func(level int) *models.LineItem {
		return &models.LineItem{Type: models.BlockTypeList, ListLevel: level, Words: []*models.Word{{String: "x"}}}
	}
	heading := &models.LineItem{Type: models.BlockTypeH1, Words: []*models.Word{{String: "1. X"}}}
	para := &models.LineItem{Words: []*models.Word{{String: "text"}}}
	page := &models.Page{Items: []interface{}{
		heading, item(1), item(3), item(1), item(0), item(1),
		para, item(2),
	}}

	result := NewGatherBlocks().Transform(&models.ParseResult{Pages: []*models.Page{page}})

	var got []int
	for _, it := range result.Pages[0].Items {
		block := it.(*models.LineItemBlock)
		if block.Type != models.BlockTypeList {
			continue
		}
		for _, li := range block.Items {
			got = append(got, li.ListLevel)
		}
	}
	// after a heading the list starts at level 0; level 3 under level 0 becomes 1;
	// genuine nesting under the item above is kept; after a paragraph it restarts at 0
	want := []int{0, 1, 1, 0, 1, 0}
	if len(got) != len(want) {
		t.Fatalf("levels %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("levels %v, want %v", got, want)
		}
	}
}

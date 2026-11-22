/******************************************************************************
 * Copyright (c) 2026 Tenebris Technologies Inc.                              *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package docx

import (
	"testing"

	"github.com/tenebris-tech/x2md/pdf2md/models"
)

func TestCreateLineItem_RunSpacing(t *testing.T) {
	plain := func(s string) *models.Word { return &models.Word{String: s} }
	bold := func(s string) *models.Word { return &models.Word{String: s, Format: models.WordFormatBold} }
	italic := func(s string) *models.Word { return &models.Word{String: s, Format: models.WordFormatOblique} }

	tests := []struct {
		name  string
		words []*models.Word
		want  string
	}{
		{"spaces outside formatted runs", []*models.Word{plain("defines "), bold("mandatory"), plain(" steps and "), italic("optional"), plain(" guidance")},
			"defines **mandatory** steps and _optional_ guidance"},
		{"spaces inside formatted runs", []*models.Word{plain("defines"), bold(" mandatory "), plain("steps")}, "defines **mandatory** steps"},
		{"runs concatenate without a space", []*models.Word{plain("an"), plain("d more")}, "and more"},
		{"formatted run inside a word", []*models.Word{plain("un"), bold("believ"), plain("able")}, "un**believ**able"},
		{"repeated spaces become one", []*models.Word{plain("a  "), plain("  b")}, "a b"},
		{"whitespace-only run", []*models.Word{plain("a"), bold(" "), plain("b")}, "a b"},
		{"tab separates words", []*models.Word{plain("a"), plain("\t"), plain("b")}, "a b"},
		{"edges are trimmed", []*models.Word{plain("  a"), plain("b  ")}, "ab"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestExtractor(t)
			item := e.createLineItem(tt.words, "", nil)
			if got := models.LinesToText([]*models.LineItem{item}, false); got != tt.want+"\n" {
				t.Errorf("got %q, want %q", got, tt.want+"\n")
			}
		})
	}
}

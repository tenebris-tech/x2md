/******************************************************************************
 * Copyright (c) 2025-2026 Tenebris Technologies Inc.                         *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package transform

import (
	"testing"

	"github.com/tenebris-tech/x2md/pdf2md/models"
)

func TestIsOrderedListItem(t *testing.T) {
	tests := []struct {
		text     string
		expected bool
	}{
		// Numbered lists
		{"1. First item", true},
		{"2) Second item", true},
		{"10. Tenth item", true},
		{"123. Large number", true},

		// Lowercase letter lists
		{"a. Item a", true},
		{"b) Item b", true},
		{"z. Last letter", true},

		// Uppercase letter lists
		{"A. Item A", true},
		{"B) Item B", true},
		{"Z. Last letter", true},

		// Lowercase roman numerals
		{"i. First", true},
		{"ii. Second", true},
		{"iii. Third", true},
		{"iv. Fourth", true},
		{"v. Fifth", true},
		{"vi. Sixth", true},
		{"ix. Ninth", true},
		{"x. Tenth", true},

		// Uppercase roman numerals
		{"I. First", true},
		{"II. Second", true},
		{"III. Third", true},
		{"IV. Fourth", true},
		{"V. Fifth", true},
		{"IX. Ninth", true},
		{"X. Tenth", true},

		// With leading whitespace
		{"  1. Indented", true},
		{"  a) Indented letter", true},
		{"  i. Indented roman", true},

		// Non-list items
		{"Regular paragraph text", false},
		{"No period or paren", false},
		{"1", false},                          // No period/paren
		{"a", false},                          // No period/paren
		{"1.NoSpace", false},                  // No space after
		{"ab. Double letter", false},          // Not single letter
		{"- Bullet item", false},              // Bullet, not ordered
		{"Introduction to chapter 1.", false}, // Period at end, not list marker
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			result := isOrderedListItem(tt.text)
			if result != tt.expected {
				t.Errorf("isOrderedListItem(%q) = %v, want %v", tt.text, result, tt.expected)
			}
		})
	}
}

// A table row whose first cell starts with a bullet stays a table row
func TestDetectListItems_TableRowIsNotListItem(t *testing.T) {
	row := &models.LineItem{
		X: 72, Y: 100, Height: 10, Font: "F1",
		Words:        []*models.Word{{String: "•"}, {String: "Item"}, {String: "Yes"}},
		IsTableRow:   true,
		TableColumns: []string{"• Item", "Yes"},
	}
	result := &models.ParseResult{
		Pages:   []*models.Page{{Items: []interface{}{row}}},
		Globals: &models.Globals{},
	}
	NewDetectListItems().Transform(result)

	got, ok := result.Pages[0].Items[0].(*models.LineItem)
	if !ok || !got.IsTableRow || got.Type != nil {
		t.Errorf("got %+v, want the table row unchanged", result.Pages[0].Items[0])
	}
}

package transform

import "testing"

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
		{"1", false},                        // No period/paren
		{"a", false},                        // No period/paren
		{"1.NoSpace", false},                // No space after
		{"ab. Double letter", false},        // Not single letter
		{"- Bullet item", false},            // Bullet, not ordered
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

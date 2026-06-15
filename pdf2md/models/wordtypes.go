package models

import "fmt"

// WordType represents a special word type
type WordType struct {
	Name                    string
	AttachWithoutWhitespace bool
	PlainTextFormat         bool
	toTextFunc              func(string) string
}

// ToText converts the word to its text representation
func (w *WordType) ToText(s string) string {
	if w.toTextFunc != nil {
		return w.toTextFunc(s)
	}
	return s
}

// Word types
var (
	WordTypeLink = &WordType{
		Name: "LINK",
		toTextFunc: func(s string) string {
			return fmt.Sprintf("[%s](%s)", s, s)
		},
	}
	WordTypeFootnoteLink = &WordType{
		Name:                    "FOOTNOTE_LINK",
		AttachWithoutWhitespace: true,
		PlainTextFormat:         true,
		toTextFunc: func(s string) string {
			return fmt.Sprintf("[^%s]", s)
		},
	}
	// WordTypeImage represents an embedded image reference
	// The String field contains the image ID, which maps to an ImageItem
	WordTypeImage = &WordType{
		Name: "IMAGE",
		toTextFunc: func(s string) string {
			// s is the image ID; actual path substitution happens in converter
			return fmt.Sprintf("![%s]", s)
		},
	}
)

// WordFormat represents text formatting
type WordFormat struct {
	Name        string
	StartSymbol string
	EndSymbol   string
}

// TextStyle captures DOCX-style run formatting that cannot be represented by a
// single Markdown delimiter pair.
type TextStyle struct {
	Bold        bool
	Italic      bool
	Strike      bool
	Underline   bool
	Superscript bool
	Subscript   bool
	Highlight   string
	Color       string
}

// IsZero reports whether the style has no active formatting.
func (s *TextStyle) IsZero() bool {
	if s == nil {
		return true
	}
	return !s.Bold &&
		!s.Italic &&
		!s.Strike &&
		!s.Underline &&
		!s.Superscript &&
		!s.Subscript &&
		s.Highlight == "" &&
		s.Color == ""
}

// Equal reports whether two styles are equivalent. Nil and zero-value styles
// are treated as equivalent so unformatted runs can be merged.
func (s *TextStyle) Equal(other *TextStyle) bool {
	if s.IsZero() && other.IsZero() {
		return true
	}
	if s == nil || other == nil {
		return false
	}
	return s.Bold == other.Bold &&
		s.Italic == other.Italic &&
		s.Strike == other.Strike &&
		s.Underline == other.Underline &&
		s.Superscript == other.Superscript &&
		s.Subscript == other.Subscript &&
		s.Highlight == other.Highlight &&
		s.Color == other.Color
}

// Copy returns a deep copy of the style, or nil for an empty style.
func (s *TextStyle) Copy() *TextStyle {
	if s.IsZero() {
		return nil
	}
	return &TextStyle{
		Bold:        s.Bold,
		Italic:      s.Italic,
		Strike:      s.Strike,
		Underline:   s.Underline,
		Superscript: s.Superscript,
		Subscript:   s.Subscript,
		Highlight:   s.Highlight,
		Color:       s.Color,
	}
}

// Word formats
var (
	WordFormatBold = &WordFormat{
		Name:        "BOLD",
		StartSymbol: "**",
		EndSymbol:   "**",
	}
	WordFormatOblique = &WordFormat{
		Name:        "OBLIQUE",
		StartSymbol: "_",
		EndSymbol:   "_",
	}
	WordFormatBoldOblique = &WordFormat{
		Name:        "BOLD_OBLIQUE",
		StartSymbol: "**_",
		EndSymbol:   "_**",
	}
)

package models

import "fmt"

// WordType represents a special word type
type WordType struct {
	Name                   string
	AttachWithoutWhitespace bool
	PlainTextFormat        bool
	toTextFunc             func(string) string
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
		Name:                   "FOOTNOTE_LINK",
		AttachWithoutWhitespace: true,
		PlainTextFormat:        true,
		toTextFunc: func(s string) string {
			return fmt.Sprintf("^%s", s)
		},
	}
	WordTypeFootnote = &WordType{
		Name: "FOOTNOTE",
		toTextFunc: func(s string) string {
			return fmt.Sprintf("(^%s)", s)
		},
	}
)

// WordFormat represents text formatting
type WordFormat struct {
	Name        string
	StartSymbol string
	EndSymbol   string
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

// WordFormatByName returns a word format by name
func WordFormatByName(name string) *WordFormat {
	switch name {
	case "BOLD":
		return WordFormatBold
	case "OBLIQUE":
		return WordFormatOblique
	case "BOLD_OBLIQUE":
		return WordFormatBoldOblique
	default:
		return nil
	}
}

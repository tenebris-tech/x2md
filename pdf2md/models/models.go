// Package models contains the data structures for PDF to Markdown conversion
package models

import (
	"strings"
)

// TextItem represents a text element extracted from PDF
type TextItem struct {
	X             float64
	Y             float64
	Width         float64
	Height        float64
	Text          string
	Font          string
	LineFormat    *WordFormat
	UnopenedFormat *WordFormat
	UnclosedFormat *WordFormat
}

// Page represents a page in the document
type Page struct {
	Index int
	Items []interface{} // Can be TextItem, LineItem, or LineItemBlock
}

// Word represents a word with optional formatting
type Word struct {
	String string
	Type   *WordType
	Format *WordFormat
}

// LineItem represents a line of text
type LineItem struct {
	X              float64
	Y              float64
	Width          float64
	Height         float64
	Words          []*Word
	Type           *BlockType
	Annotation     *Annotation
	ParsedElements *ParsedElements
	Font           string
	// Table-related fields
	IsTableRow     bool      // Whether this line is part of a table
	IsTableHeader  bool      // Whether this is a table header row
	TableColumns   []string  // Text content of each column (for table rows)
}

// Text returns the text content of the line
func (l *LineItem) Text() string {
	return strings.Join(l.WordStrings(), " ")
}

// WordStrings returns the strings of all words
func (l *LineItem) WordStrings() []string {
	result := make([]string, len(l.Words))
	for i, w := range l.Words {
		result[i] = w.String
	}
	return result
}

// LineItemBlock represents a block of lines
type LineItemBlock struct {
	Items          []*LineItem
	Type           *BlockType
	Annotation     *Annotation
	ParsedElements *ParsedElements
}

// AddItem adds a line item to the block
func (b *LineItemBlock) AddItem(item *LineItem) {
	if b.Type != nil && item.Type != nil && b.Type != item.Type {
		return // Don't mix types
	}
	if b.Type == nil {
		b.Type = item.Type
	}
	if item.ParsedElements != nil {
		if b.ParsedElements != nil {
			b.ParsedElements.Add(item.ParsedElements)
		} else {
			b.ParsedElements = item.ParsedElements.Copy()
		}
	}

	copiedItem := &LineItem{
		X:              item.X,
		Y:              item.Y,
		Width:          item.Width,
		Height:         item.Height,
		Words:          item.Words,
		Annotation:     item.Annotation,
		ParsedElements: item.ParsedElements,
		Font:           item.Font,
		IsTableRow:     item.IsTableRow,
		IsTableHeader:  item.IsTableHeader,
		TableColumns:   item.TableColumns,
	}
	b.Items = append(b.Items, copiedItem)
}

// ParsedElements contains parsed metadata for a line
type ParsedElements struct {
	FootnoteLinks  []int
	Footnotes      []string
	ContainLinks   bool
	FormattedWords int
}

// Add combines parsed elements
func (p *ParsedElements) Add(other *ParsedElements) {
	if other == nil {
		return
	}
	p.FootnoteLinks = append(p.FootnoteLinks, other.FootnoteLinks...)
	p.Footnotes = append(p.Footnotes, other.Footnotes...)
	p.ContainLinks = p.ContainLinks || other.ContainLinks
	p.FormattedWords += other.FormattedWords
}

// Copy creates a copy of parsed elements
func (p *ParsedElements) Copy() *ParsedElements {
	if p == nil {
		return nil
	}
	return &ParsedElements{
		FootnoteLinks:  append([]int{}, p.FootnoteLinks...),
		Footnotes:      append([]string{}, p.Footnotes...),
		ContainLinks:   p.ContainLinks,
		FormattedWords: p.FormattedWords,
	}
}

// ParseResult holds the result of parsing
type ParseResult struct {
	Pages    []*Page
	Globals  *Globals
	Messages []string
}

// Globals contains global statistics about the document
type Globals struct {
	MostUsedHeight           int
	MostUsedFont             string
	MostUsedDistance         int
	MaxHeight                int
	MaxHeightFont            string
	FontToFormats            map[string]*WordFormat
	TOCPages                 []int
	HeadlineTypeToHeightRange map[string]*HeightRange
}

// HeightRange represents a range of heights
type HeightRange struct {
	Min int
	Max int
}

// Annotation marks the status of an item
type Annotation struct {
	Category string
	Color    string
}

// Standard annotations
var (
	AddedAnnotation     = &Annotation{Category: "Added", Color: "green"}
	RemovedAnnotation   = &Annotation{Category: "Removed", Color: "red"}
	UnchangedAnnotation = &Annotation{Category: "Unchanged", Color: "brown"}
	DetectedAnnotation  = &Annotation{Category: "Detected", Color: "green"}
	ModifiedAnnotation  = &Annotation{Category: "Modified", Color: "green"}
)

// TextBlock represents a text block for final output
type TextBlock struct {
	Category string
	Text     string
}

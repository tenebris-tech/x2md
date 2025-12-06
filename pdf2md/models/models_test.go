package models

import (
	"testing"
)

func TestLineItemCopy(t *testing.T) {
	original := &LineItem{
		X:             100,
		Y:             200,
		Width:         50,
		Height:        12,
		Words:         []*Word{{String: "test"}},
		Type:          BlockTypeH1,
		Font:          "Arial",
		IsTableRow:    true,
		IsTableHeader: true,
		TableColumns:  []string{"col1", "col2", "col3"},
	}

	copied := original.Copy()

	// Verify all fields are copied
	if copied.X != original.X {
		t.Errorf("X = %.1f, want %.1f", copied.X, original.X)
	}
	if copied.Y != original.Y {
		t.Errorf("Y = %.1f, want %.1f", copied.Y, original.Y)
	}
	if copied.Width != original.Width {
		t.Errorf("Width = %.1f, want %.1f", copied.Width, original.Width)
	}
	if copied.Height != original.Height {
		t.Errorf("Height = %.1f, want %.1f", copied.Height, original.Height)
	}
	if copied.Font != original.Font {
		t.Errorf("Font = %q, want %q", copied.Font, original.Font)
	}
	if copied.IsTableRow != original.IsTableRow {
		t.Errorf("IsTableRow = %v, want %v", copied.IsTableRow, original.IsTableRow)
	}
	if copied.IsTableHeader != original.IsTableHeader {
		t.Errorf("IsTableHeader = %v, want %v", copied.IsTableHeader, original.IsTableHeader)
	}

	// Verify TableColumns is a deep copy
	if len(copied.TableColumns) != len(original.TableColumns) {
		t.Errorf("TableColumns length = %d, want %d",
			len(copied.TableColumns), len(original.TableColumns))
	}

	// Modify original's TableColumns and verify copy is unaffected
	original.TableColumns[0] = "modified"
	if copied.TableColumns[0] == "modified" {
		t.Error("TableColumns was not deep copied - modification affected copy")
	}
}

func TestLineItemCopyNil(t *testing.T) {
	var nilItem *LineItem
	copied := nilItem.Copy()
	if copied != nil {
		t.Error("Copy of nil should return nil")
	}
}

func TestLineItemText(t *testing.T) {
	line := &LineItem{
		Words: []*Word{
			{String: "Hello"},
			{String: "World"},
			{String: "Test"},
		},
	}

	got := line.Text()
	want := "Hello World Test"
	if got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}
}

func TestLineItemBlockAddItem(t *testing.T) {
	block := &LineItemBlock{}

	item1 := &LineItem{
		X:            100,
		Y:            200,
		Type:         BlockTypeH1,
		IsTableRow:   true,
		TableColumns: []string{"a", "b"},
	}

	block.AddItem(item1)

	if len(block.Items) != 1 {
		t.Errorf("Items count = %d, want 1", len(block.Items))
	}

	// Verify the item was copied, not referenced
	item1.TableColumns[0] = "modified"
	if block.Items[0].TableColumns[0] == "modified" {
		t.Error("AddItem should copy the item, not reference it")
	}

	// Verify type was set from item
	if block.Type != BlockTypeH1 {
		t.Errorf("Block type = %v, want H1", block.Type)
	}
}

func TestLineItemBlockAddItemTypeMismatch(t *testing.T) {
	block := &LineItemBlock{Type: BlockTypeH1}

	item := &LineItem{Type: BlockTypeH2}
	block.AddItem(item)

	// Item should not be added due to type mismatch
	if len(block.Items) != 0 {
		t.Error("Item with mismatched type should not be added")
	}
}

func TestParsedElementsCopy(t *testing.T) {
	original := &ParsedElements{
		FootnoteLinks:  []int{1, 2, 3},
		Footnotes:      []string{"note1", "note2"},
		ContainLinks:   true,
		FormattedWords: 5,
	}

	copied := original.Copy()

	// Verify values
	if copied.ContainLinks != original.ContainLinks {
		t.Errorf("ContainLinks = %v, want %v", copied.ContainLinks, original.ContainLinks)
	}
	if copied.FormattedWords != original.FormattedWords {
		t.Errorf("FormattedWords = %d, want %d", copied.FormattedWords, original.FormattedWords)
	}

	// Verify deep copy of slices
	original.FootnoteLinks[0] = 999
	if copied.FootnoteLinks[0] == 999 {
		t.Error("FootnoteLinks was not deep copied")
	}

	original.Footnotes[0] = "modified"
	if copied.Footnotes[0] == "modified" {
		t.Error("Footnotes was not deep copied")
	}
}

func TestParsedElementsCopyNil(t *testing.T) {
	var nilElements *ParsedElements
	copied := nilElements.Copy()
	if copied != nil {
		t.Error("Copy of nil should return nil")
	}
}

func TestParsedElementsAdd(t *testing.T) {
	p1 := &ParsedElements{
		FootnoteLinks:  []int{1, 2},
		Footnotes:      []string{"a"},
		ContainLinks:   false,
		FormattedWords: 3,
	}

	p2 := &ParsedElements{
		FootnoteLinks:  []int{3},
		Footnotes:      []string{"b", "c"},
		ContainLinks:   true,
		FormattedWords: 2,
	}

	p1.Add(p2)

	if len(p1.FootnoteLinks) != 3 {
		t.Errorf("FootnoteLinks count = %d, want 3", len(p1.FootnoteLinks))
	}
	if len(p1.Footnotes) != 3 {
		t.Errorf("Footnotes count = %d, want 3", len(p1.Footnotes))
	}
	if !p1.ContainLinks {
		t.Error("ContainLinks should be true after Add")
	}
	if p1.FormattedWords != 5 {
		t.Errorf("FormattedWords = %d, want 5", p1.FormattedWords)
	}
}

func TestParsedElementsAddNil(t *testing.T) {
	p := &ParsedElements{FormattedWords: 5}
	p.Add(nil)
	if p.FormattedWords != 5 {
		t.Error("Add(nil) should not modify the receiver")
	}
}

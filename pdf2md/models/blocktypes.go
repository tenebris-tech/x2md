/******************************************************************************
 * Copyright (c) 2025-2026 Tenebris Technologies Inc.                         *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package models

import (
	"strings"
)

// BlockType represents a markdown block type
type BlockType struct {
	Name                                         string
	Headline                                     bool
	HeadlineLevel                                int
	MergeToBlock                                 bool
	MergeFollowingNonTypedItems                  bool
	MergeFollowingNonTypedItemsWithSmallDistance bool
}

// Block types
var (
	BlockTypeH1 = &BlockType{
		Name:          "H1",
		Headline:      true,
		HeadlineLevel: 1,
	}
	BlockTypeH2 = &BlockType{
		Name:          "H2",
		Headline:      true,
		HeadlineLevel: 2,
	}
	BlockTypeH3 = &BlockType{
		Name:          "H3",
		Headline:      true,
		HeadlineLevel: 3,
	}
	BlockTypeH4 = &BlockType{
		Name:          "H4",
		Headline:      true,
		HeadlineLevel: 4,
	}
	BlockTypeH5 = &BlockType{
		Name:          "H5",
		Headline:      true,
		HeadlineLevel: 5,
	}
	BlockTypeH6 = &BlockType{
		Name:          "H6",
		Headline:      true,
		HeadlineLevel: 6,
	}
	BlockTypeTOC = &BlockType{
		Name:         "TOC",
		MergeToBlock: true,
	}
	BlockTypeFootnotes = &BlockType{
		Name:                        "FOOTNOTES",
		MergeToBlock:                true,
		MergeFollowingNonTypedItems: true,
	}
	BlockTypeCode = &BlockType{
		Name:         "CODE",
		MergeToBlock: true,
	}
	BlockTypeList = &BlockType{
		Name:         "LIST",
		MergeToBlock: false,
		MergeFollowingNonTypedItemsWithSmallDistance: true,
	}
	BlockTypeParagraph = &BlockType{
		Name: "PARAGRAPH",
	}
)

// HeadlineByLevel returns the block type for a headline level
func HeadlineByLevel(level int) *BlockType {
	switch level {
	case 1:
		return BlockTypeH1
	case 2:
		return BlockTypeH2
	case 3:
		return BlockTypeH3
	case 4:
		return BlockTypeH4
	case 5:
		return BlockTypeH5
	case 6:
		return BlockTypeH6
	default:
		if level > 6 {
			return BlockTypeH6
		}
		return BlockTypeH1
	}
}

// IsHeadline checks if a block type is a headline
func IsHeadline(blockType *BlockType) bool {
	return blockType != nil && blockType.Headline
}

// BlockTypeByName returns a block type by its name
func BlockTypeByName(name string) *BlockType {
	switch name {
	case "H1":
		return BlockTypeH1
	case "H2":
		return BlockTypeH2
	case "H3":
		return BlockTypeH3
	case "H4":
		return BlockTypeH4
	case "H5":
		return BlockTypeH5
	case "H6":
		return BlockTypeH6
	case "TOC":
		return BlockTypeTOC
	case "FOOTNOTES":
		return BlockTypeFootnotes
	case "CODE":
		return BlockTypeCode
	case "LIST":
		return BlockTypeList
	case "PARAGRAPH":
		return BlockTypeParagraph
	default:
		return nil
	}
}

// defaultListMarkerWidth is the width of a "- " marker
const defaultListMarkerWidth = 2

// listIndenter works out how far to indent nested list items. Markdown nests
// an item only when it is indented to where its parent's text starts, which
// is 2 columns under "- " but 3 under "1. " and 4 under "10. ".
type listIndenter struct {
	markerWidths []int // Marker width of the open item at each level
}

// next returns the indentation for a list line and records the line as the
// open item at its level
func (l *listIndenter) next(line *LineItem) int {
	level := line.ListLevel
	if level < 0 {
		level = 0
	}

	indent := 0
	for i := 0; i < level; i++ {
		if i < len(l.markerWidths) {
			indent += l.markerWidths[i]
		} else {
			// No item was seen at this level
			indent += defaultListMarkerWidth
		}
	}

	for len(l.markerWidths) < level {
		l.markerWidths = append(l.markerWidths, defaultListMarkerWidth)
	}
	l.markerWidths = append(l.markerWidths[:level], listMarkerWidth(line))
	return indent
}

// listMarkerWidth returns the width of a list line's marker including the
// space after it. The marker is the first word, or the part of it before a
// space when the marker and text share a word.
func listMarkerWidth(line *LineItem) int {
	if len(line.Words) == 0 {
		return defaultListMarkerWidth
	}
	marker := line.Words[0].String
	if i := strings.Index(marker, " "); i >= 0 {
		marker = marker[:i]
	}
	// A marker is short ("-", "12.", "iii."); anything longer is not one
	if width := len(marker) + 1; width <= 6 {
		return width
	}
	return defaultListMarkerWidth
}

// LinesToText converts line items to text with formatting
func LinesToText(lineItems []*LineItem, disableInlineFormats bool) string {
	var text strings.Builder
	var openFormat *WordFormat
	var inTable bool
	var headerWritten bool
	var listIndent listIndenter

	closeFormat := func() {
		if openFormat != nil {
			text.WriteString(openFormat.EndSymbol)
			openFormat = nil
		}
	}

	for lineIndex, line := range lineItems {
		// Handle table rows
		if line.IsTableRow && len(line.TableColumns) > 0 {
			// Skip empty table rows (all columns are whitespace-only)
			hasContent := false
			for _, col := range line.TableColumns {
				if strings.TrimSpace(col) != "" {
					hasContent = true
					break
				}
			}
			if !hasContent {
				continue
			}

			if !inTable {
				inTable = true
				headerWritten = false
			}

			// Write table row with | separators
			text.WriteString("| ")
			for i, col := range line.TableColumns {
				if i > 0 {
					text.WriteString(" | ")
				}
				text.WriteString(strings.TrimSpace(col))
			}
			text.WriteString(" |\n")

			// Write separator after header row
			if line.IsTableHeader && !headerWritten {
				text.WriteString("|")
				for range line.TableColumns {
					text.WriteString(" --- |")
				}
				text.WriteString("\n")
				headerWritten = true
			}
			continue
		}

		// End of table - add blank line after table
		if inTable && !line.IsTableRow {
			inTable = false
			headerWritten = false
		}

		// Add indentation for nested list items
		if line.Type == BlockTypeList {
			text.WriteString(strings.Repeat(" ", listIndent.next(line)))
		} else {
			listIndent = listIndenter{}
		}

		// Regular line processing
		for i, word := range line.Words {
			wordFormat := word.Format

			if openFormat != nil && (wordFormat == nil || wordFormat != openFormat) {
				closeFormat()
			}

			if i > 0 && (word.Type == nil || !word.Type.AttachWithoutWhitespace) && !isPunctuation(word.String) {
				text.WriteString(" ")
			}

			if wordFormat != nil && openFormat == nil && !disableInlineFormats {
				openFormat = wordFormat
				text.WriteString(openFormat.StartSymbol)
			}

			if word.Type != nil && (!disableInlineFormats || word.Type.PlainTextFormat) {
				text.WriteString(word.Type.ToText(word.String))
			} else {
				text.WriteString(word.String)
			}
		}

		nextLineFormat := (*WordFormat)(nil)
		if lineIndex+1 < len(lineItems) && len(lineItems[lineIndex+1].Words) > 0 {
			nextLineFormat = lineItems[lineIndex+1].Words[0].Format
		}

		if openFormat != nil && (lineIndex == len(lineItems)-1 || nextLineFormat != openFormat) {
			closeFormat()
		}
		text.WriteString("\n")
	}

	return text.String()
}

func isPunctuation(s string) bool {
	if len(s) != 1 {
		return false
	}
	c := s[0]
	return c == '.' || c == '!' || c == '?'
}

// BlockToText converts a block to text
func BlockToText(block *LineItemBlock) string {
	if block.Type == nil {
		return LinesToText(block.Items, false)
	}

	switch block.Type {
	case BlockTypeH1:
		return "# " + LinesToText(block.Items, true)
	case BlockTypeH2:
		return "## " + LinesToText(block.Items, true)
	case BlockTypeH3:
		return "### " + LinesToText(block.Items, true)
	case BlockTypeH4:
		return "#### " + LinesToText(block.Items, true)
	case BlockTypeH5:
		return "##### " + LinesToText(block.Items, true)
	case BlockTypeH6:
		return "###### " + LinesToText(block.Items, true)
	case BlockTypeTOC:
		return LinesToText(block.Items, true)
	case BlockTypeFootnotes:
		return LinesToText(block.Items, false)
	case BlockTypeCode:
		return "```\n" + LinesToText(block.Items, true) + "```"
	case BlockTypeList:
		return LinesToText(block.Items, false)
	default:
		return LinesToText(block.Items, false)
	}
}

package models

import (
	"fmt"
	"html"
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

// TextRenderOptions controls how line items are rendered as Markdown.
type TextRenderOptions struct {
	DisableInlineFormats bool
	PreserveInlineHTML   bool
	NoImplicitWhitespace bool
	PreserveWordTypes    bool
}

// LinesToText converts line items to text with formatting.
func LinesToText(lineItems []*LineItem, disableInlineFormats bool) string {
	return LinesToTextWithOptions(lineItems, TextRenderOptions{
		DisableInlineFormats: disableInlineFormats,
	})
}

// LinesToTextWithOptions converts line items to text with explicit rendering
// controls. NoImplicitWhitespace is intended for DOCX runs, where adjacent runs
// concatenate directly and whitespace must come from the document content.
func LinesToTextWithOptions(lineItems []*LineItem, opts TextRenderOptions) string {
	var text strings.Builder
	var openFormat *WordFormat
	var inTable bool
	var headerWritten bool

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
		if line.Type == BlockTypeList && line.ListLevel > 0 {
			text.WriteString(strings.Repeat("  ", line.ListLevel))
		}

		// Regular line processing
		for i, word := range line.Words {
			wordFormat := word.Format

			if word.Style != nil && openFormat != nil {
				closeFormat()
			}

			if word.Style == nil && openFormat != nil && (wordFormat == nil || wordFormat != openFormat) {
				closeFormat()
			}

			if !opts.NoImplicitWhitespace && i > 0 && (word.Type == nil || !word.Type.AttachWithoutWhitespace) && !isPunctuation(word.String) {
				text.WriteString(" ")
			}

			if word.Style != nil {
				text.WriteString(renderStyledWord(word, opts))
				continue
			}

			if wordFormat != nil && openFormat == nil && !opts.DisableInlineFormats {
				openFormat = wordFormat
				text.WriteString(openFormat.StartSymbol)
			}

			if word.Type != nil && (!opts.DisableInlineFormats || word.Type.PlainTextFormat || opts.PreserveWordTypes) {
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

func renderStyledWord(word *Word, opts TextRenderOptions) string {
	content := word.String
	if word.Type != nil && (!opts.DisableInlineFormats || word.Type.PlainTextFormat || opts.PreserveWordTypes) {
		content = word.Type.ToText(word.String)
	}

	if opts.DisableInlineFormats || word.Style == nil || word.Style.IsZero() {
		return content
	}

	style := word.Style
	if opts.PreserveInlineHTML {
		content = applyHTMLStyle(content, style)
	}

	if style.Bold && style.Italic {
		content = "**_" + content + "_**"
	} else if style.Bold {
		content = "**" + content + "**"
	} else if style.Italic {
		content = "_" + content + "_"
	}

	if style.Strike {
		content = "~~" + content + "~~"
	}

	return content
}

func applyHTMLStyle(content string, style *TextStyle) string {
	if style == nil {
		return content
	}

	wrap := func(start, end string) {
		content = start + content + end
	}

	var css []string
	if style.Highlight != "" {
		css = append(css, fmt.Sprintf("background-color: %s", html.EscapeString(style.Highlight)))
	}
	if style.Color != "" {
		css = append(css, fmt.Sprintf("color: %s", html.EscapeString(style.Color)))
	}
	if len(css) > 0 {
		wrap(`<span style="`+strings.Join(css, "; ")+`">`, "</span>")
	}
	if style.Subscript {
		wrap("<sub>", "</sub>")
	}
	if style.Superscript {
		wrap("<sup>", "</sup>")
	}
	if style.Underline {
		wrap("<u>", "</u>")
	}

	return content
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

// BlockToTextWithOptions converts a block to text using explicit rendering
// controls. Unlike BlockToText, it does not automatically suppress inline
// formatting inside headings.
func BlockToTextWithOptions(block *LineItemBlock, opts TextRenderOptions) string {
	if block.Type == nil {
		return LinesToTextWithOptions(block.Items, opts)
	}

	switch block.Type {
	case BlockTypeH1:
		return "# " + LinesToTextWithOptions(block.Items, opts)
	case BlockTypeH2:
		return "## " + LinesToTextWithOptions(block.Items, opts)
	case BlockTypeH3:
		return "### " + LinesToTextWithOptions(block.Items, opts)
	case BlockTypeH4:
		return "#### " + LinesToTextWithOptions(block.Items, opts)
	case BlockTypeH5:
		return "##### " + LinesToTextWithOptions(block.Items, opts)
	case BlockTypeH6:
		return "###### " + LinesToTextWithOptions(block.Items, opts)
	case BlockTypeTOC:
		return LinesToTextWithOptions(block.Items, opts)
	case BlockTypeFootnotes:
		return LinesToTextWithOptions(block.Items, opts)
	case BlockTypeCode:
		codeOpts := opts
		codeOpts.DisableInlineFormats = true
		return "```\n" + LinesToTextWithOptions(block.Items, codeOpts) + "```"
	case BlockTypeList:
		return LinesToTextWithOptions(block.Items, opts)
	default:
		return LinesToTextWithOptions(block.Items, opts)
	}
}

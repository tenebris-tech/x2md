# pdf2mdgo Development Notes

## Overview
This library is a pure Go port of the @opendocsg/pdf2md JavaScript library for converting PDF files to Markdown.

## Architecture

### PDF Parsing Layer (`pdf/`)
- `parser.go` - Low-level PDF parsing (xref, objects, streams)
- `extractor.go` - Text extraction from content streams

### Models (`models/`)
- `TextItem` - Individual text element with position (x, y, width, height, text, font)
- `Page` - Collection of items on a page
- `LineItem` - Multiple text items grouped into a line
- `LineItemBlock` - Multiple lines grouped into a block
- `Word` - Individual word with type and format
- `ParseResult` - Result of parsing with globals and messages

### Transformations Pipeline
Based on pdf2md, the transformation order is:
1. `CalculateGlobalStats` - Find most used height, font, line distance
2. `CompactLines` - Group text items on same Y into lines
3. `RemoveRepetitiveElements` - Remove headers/footers
4. `VerticalToHorizontal` - Handle vertical text
5. `DetectTOC` - Find table of contents
6. `DetectHeaders` - Identify headings by height/font
7. `DetectListItems` - Find bullet/numbered lists
8. `GatherBlocks` - Group lines into blocks
9. `DetectCodeQuoteBlocks` - Find code blocks
10. `DetectListLevels` - Determine list nesting
11. `ToTextBlocks` - Convert to text blocks
12. `ToMarkdown` - Final markdown output

### Block Types
- H1-H6 (headings)
- PARAGRAPH
- LIST
- CODE
- TOC
- FOOTNOTES

### Word Formats
- BOLD (`**text**`)
- OBLIQUE/ITALIC (`_text_`)
- BOLD_OBLIQUE (`**_text_**`)

## Progress

### Completed
- [x] Project structure created
- [x] PDF parser (xref, objects, streams, filters)
- [x] Content stream text extraction
- [x] Models implementation
- [x] Transformation pipeline
- [x] Functional options pattern
- [x] Main testing program
- [x] Initial testing with 245-page PDF

### Known Limitations
- Some character encoding issues with non-standard fonts (ToUnicode CMap parsing could be improved)
- LZW compression not fully implemented
- Complex table layouts may not convert perfectly

## Key Insights from pdf2md

1. **Text extraction** relies on pdf.js which extracts text items with:
   - x, y coordinates (transform[4], transform[5])
   - width, height
   - text string
   - font name

2. **Line detection** groups items by similar Y coordinate (within mostUsedDistance/2)

3. **Header detection** uses:
   - Height comparison (taller = more likely header)
   - Font differences
   - TOC entries if available

4. **List detection** looks for:
   - Characters: `-`, `•`, `–`
   - Numbered patterns: `1.`, `2.`, etc.

## PDF Content Stream Operators

Key operators for text extraction:
- `BT` - Begin text object
- `ET` - End text object
- `Tf` - Set font and size
- `Tm` - Set text matrix (position)
- `Td`, `TD` - Move text position
- `Tj` - Show text
- `TJ` - Show text with positioning
- `'` - Move to next line and show text
- `"` - Set spacing, move to next line, show text

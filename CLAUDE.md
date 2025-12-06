# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

x2md is a pure Go application that converts multiple file types to Markdown. Currently supports PDF and DOCX formats.

**Status**: Unreleased - do not retain legacy code or implement backward compatibility

## Commands

```bash
# Build CLI tool
go build

# Run CLI
./x2md input.pdf                    # Output: input.md
./x2md input.docx                   # Output: input.md
./x2md -output out.md input.pdf     # Specify output
./x2md -v input.pdf                 # Verbose mode

# Run tests
go test ./...

# Static analysis
go vet ./...
```

## Architecture

### Package Structure

```
x2md/
├── main.go                 # CLI entry point, format detection
├── pdf2md/                 # PDF conversion package
│   ├── converter.go        # Main API with functional options
│   ├── pdf/                # Low-level PDF parsing
│   ├── models/             # Shared data structures
│   └── transform/          # Transformation pipeline
└── docx2md/                # DOCX conversion package
    ├── converter.go        # Main API with functional options
    ├── docx/               # DOCX parsing (ZIP/XML)
    └── transform/          # Transformation pipeline
```

---

## PDF Conversion (`pdf2md/`)

### Package Structure
- `pdf2md/pdf/` - Low-level PDF parsing
  - `parser.go` - PDF structure parsing (xref, objects, streams)
  - `extractor.go` - Text extraction from content streams
- `pdf2md/models/` - Data structures
  - `models.go` - Core types (TextItem, Page, LineItem, LineItemBlock, etc.)
  - `blocktypes.go` - Markdown block types (H1-H6, LIST, CODE, etc.) and table rendering
  - `wordtypes.go` - Word formatting (BOLD, ITALIC)
- `pdf2md/transform/` - Transformation pipeline
  - Each file is one transformation step
- `pdf2md/converter.go` - Main API with functional options pattern

### Transformation Pipeline Order
1. CalculateGlobalStats - Find most used height/font/distance
2. CompactLines - Group text items on same Y into lines, detect tables
3. RemoveRepetitiveElements - Remove headers/footers
4. DetectTOC - Find table of contents
5. DetectHeaders - Identify headings by height
6. DetectListItems - Find bullet/numbered lists
7. GatherBlocks - Group lines into blocks (keeps table rows together)
8. RemoveBlankPages - Filter out empty pages
9. ToTextBlocks - Convert to text blocks
10. ToMarkdown - Final markdown output, table header deduplication, cross-page merging

### Key Implementation Details

#### Table Detection (`pdf2md/transform/compact_lines.go`)
Tables are detected using multiple methods:
1. **Traditional header-based**: Looks for rows with well-spaced column headers
2. **Reference-style**: Detects bracketed IDs like [CC1], [CC2] aligned in columns
3. **Known header patterns**: Recognizes specific headers for continuation tables

Key fields on `LineItem`:
- `IsTableRow` - Whether this line is part of a table
- `IsTableHeader` - Whether this is a table header row
- `TableColumns` - Text content of each column

#### Table Rendering (`pdf2md/models/blocktypes.go`)
`LinesToText()` renders table rows with markdown syntax:
- `| col1 | col2 | col3 |` for data rows
- `| --- | --- | --- |` separator after header rows

#### Cross-Page Table Handling (`pdf2md/transform/to_markdown.go`)
- `deduplicateTableHeader()` - Removes repeated headers from continuation tables
- `mergeTablesCrossPages()` - Joins table rows separated by page breaks

### PDF Testing
Test with reference PDF: `CPP_ND_V3.0E.pdf` (245 pages)
- Verify table detection on pages 3-5 (Revision History table)
- Verify reference table on page 3 ([CC1], [CC2], etc.)
- Verify no false header detection on "academia." text

---

## DOCX Conversion (`docx2md/`)

### Package Structure
- `docx2md/docx/` - DOCX file parsing
  - `parser.go` - ZIP archive handling, XML parsing
  - `extractor.go` - Content extraction (paragraphs, tables, lists)
  - `elements.go` - DOCX XML element structures
  - `styles.go` - Style and numbering definitions
  - `relationships.go` - Hyperlink and image relationships
- `docx2md/transform/` - Transformation pipeline
  - `transform.go` - Pipeline orchestration
  - `gather_blocks.go` - Group LineItems into blocks
  - `to_text_blocks.go` - Convert blocks to text
  - `to_markdown.go` - Final markdown generation
- `docx2md/converter.go` - Main API with functional options pattern

### Key Implementation Details

#### DOCX File Structure
DOCX files are ZIP archives containing:
- `word/document.xml` - Main document content
- `word/styles.xml` - Style definitions (Heading 1, Normal, etc.)
- `word/numbering.xml` - List numbering definitions
- `word/_rels/document.xml.rels` - Relationships (hyperlinks, images)

#### Text Run Handling (`docx2md/docx/extractor.go`)
Per OOXML specification, text runs (`<w:r>`) concatenate directly without implicit spaces.
- `xml:space="preserve"` preserves leading/trailing whitespace in `<w:t>` elements
- `mergeConsecutiveFormattedWords()` combines adjacent runs with same formatting
- Fixes fragmented text from Word editing (e.g., "an" + "d" → "and")

#### Style Resolution (`docx2md/docx/styles.go`)
- `IsHeading()` - Detects Heading 1-6 styles by name or outline level
- `IsBold()` / `IsItalic()` - Checks style formatting properties
- Styles can inherit from parent styles via `basedOn`

#### List Processing (`docx2md/docx/styles.go`)
- `Numbering.GetListPrefix()` - Returns prefix based on numFmt (bullet, decimal, etc.)
- Supports: bullet, decimal, lowerLetter, upperLetter, lowerRoman, upperRoman
- Nested lists use indentation level (`ilvl`)

#### Hyperlink Handling (`docx2md/docx/relationships.go`)
- Hyperlinks reference relationships by ID (`r:id`)
- `GetTarget()` resolves relationship ID to URL
- External links have `TargetMode="External"`

### DOCX Testing
Test with sample DOCX files in `private/`:
- Verify text run merging (no split words like "an d")
- Verify heading detection from styles
- Verify list formatting (bullets, numbers)
- Verify table rendering
- Verify hyperlink conversion

---

## Shared Models (`pdf2md/models/`)

Both PDF and DOCX converters use the shared models package:

- `TextItem` - Raw text with position (used by PDF)
- `Page` - Collection of items
- `LineItem` - Line with words, formatting, table flags
- `LineItemBlock` - Group of lines with block type
- `Word` - Individual word with format and type
- `WordFormat` - BOLD, OBLIQUE, BOLD_OBLIQUE
- `WordType` - LINK, FOOTNOTE, etc.
- `BlockType` - H1-H6, LIST, PARAGRAPH, etc.
- `ParseResult` - Pipeline result with pages and globals

---

## Adding New Format Support

To add a new format (e.g., RTF):

1. Create package `rtf2md/`
2. Implement parser in `rtf2md/rtf/`
3. Create `rtf2md/converter.go` with functional options
4. Implement transformations in `rtf2md/transform/`
5. Add format detection in `main.go`
6. Reuse `pdf2md/models/` where applicable

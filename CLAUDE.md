# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

pdf2mdgo is a pure Go library for converting PDF files to Markdown, ported from the @opendocsg/pdf2md JavaScript library. It uses no CGO and can be cross-compiled.

**Status**: Unreleased - do not retain legacy code or implement backward compatibility

## Commands

```bash
# Build everything
go build ./...

# Build CLI tool
go build -o pdf2md ./cmd/pdf2md

# Run CLI
./pdf2md input.pdf                    # Output: input.md
./pdf2md -output out.md input.pdf     # Specify output
./pdf2md -v input.pdf                 # Verbose mode

# Run tests
go test ./...

# Static analysis
go vet ./...
```

## Architecture

### Package Structure
- `pdf/` - Low-level PDF parsing
  - `parser.go` - PDF structure parsing (xref, objects, streams)
  - `extractor.go` - Text extraction from content streams
- `models/` - Data structures
  - `models.go` - Core types (TextItem, Page, LineItem, LineItemBlock, etc.)
  - `blocktypes.go` - Markdown block types (H1-H6, LIST, CODE, etc.) and table rendering
  - `wordtypes.go` - Word formatting (BOLD, ITALIC)
- `transform/` - Transformation pipeline
  - Each file is one transformation step
- `converter.go` - Main API with functional options pattern

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

### Library Usage
```go
converter := pdf2mdgo.New(
    pdf2mdgo.WithRemoveHeaders(true),
    pdf2mdgo.WithDetectLists(true),
)
markdown, err := converter.ConvertFile("input.pdf")
```

## Key Implementation Details

### Table Detection (transform/compact_lines.go)
Tables are detected using multiple methods:
1. **Traditional header-based**: Looks for rows with well-spaced column headers (e.g., "Version Date Description")
2. **Reference-style**: Detects bracketed IDs like [CC1], [CC2] aligned in columns
3. **Known header patterns**: Recognizes specific headers for continuation tables across pages

Key fields added to `LineItem`:
- `IsTableRow` - Whether this line is part of a table
- `IsTableHeader` - Whether this is a table header row
- `TableColumns` - Text content of each column

### Table Rendering (models/blocktypes.go)
`LinesToText()` renders table rows with markdown syntax:
- `| col1 | col2 | col3 |` for data rows
- `| --- | --- | --- |` separator after header rows

### Cross-Page Table Handling (transform/to_markdown.go)
- `deduplicateTableHeader()` - Removes repeated headers from continuation tables
- `mergeTablesCrossPages()` - Joins table rows separated by page breaks
- `normalizeTableHeader()` - Creates canonical form for header comparison

### Header Detection (transform/detect_headers.go)
- Skips lines ending with punctuation (period, comma, etc.) to avoid false positives
- Example: "academia." was incorrectly detected as H4; now excluded

### Date/Text Spacing (transform/compact_lines.go)
- `needsSpaceBetween()` checks for hyphens before considering gap size
- Prevents unwanted spaces in dates like "06-December-2023"

## Testing
Test with the reference PDF: `CPP_ND_V3.0E.pdf` (245 pages)
- Verify table detection on pages 3-5 (Revision History table)
- Verify reference table on page 3 ([CC1], [CC2], etc.)
- Verify no false header detection on "academia." text

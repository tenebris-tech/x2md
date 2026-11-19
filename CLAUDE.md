# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

x2md is a pure Go application that converts multiple file types to Markdown. Currently supports PDF, DOCX, and XLSX formats.

**Status**: Unreleased - do not retain legacy code or implement backward compatibility

## Commands

```bash
# Build CLI tool
go build -o x2md ./cli

# Run CLI
./x2md input.pdf                    # Output: input.md
./x2md input.docx                   # Output: input.md
./x2md -output out.md input.pdf     # Specify output
./x2md -v input.pdf                 # Verbose mode

# Full test suite - the gate. "The tests pass" means this passes.
make test

# Test then build (bin/x2md); build only; clean
make
make build
make clean

# Inner loop while iterating
go test ./some/package
```

## Development Standards

This project follows the standards in https://github.com/securityguy/coding (`standards/go/`, `standards/makefile/`). In particular:

- `make test` (implemented by `test.sh`) is the single test gate and what CI runs. It must run everything, never modify the working tree, and print the standard summary. New test sections go in `test.sh` using its numbering scheme.
- Tests must not skip silently. Tests that need the private corpus are behind the `private` build tag and run with `make test-private`, which fails if a file is missing.
- Every Go source file starts with the copyright header (holder as in `LICENSE`).
- Code must be `gofmt`-clean; `make test` verifies it and `make fmt` applies it.
- No new external dependencies without approval.

Decisions and known deviations:
- No `app/` identity package and no `--version` output. Decided by the owner: x2md is primarily a library, so it does not need one. Do not add it or ask again.
- The CLI's `main.go` lives in `cli/`, not at the repository root. Do not move it without asking.

## Architecture

### Package Structure

```
x2md/
├── cli/
│   └── main.go             # CLI entry point, format detection
├── pdf2md/                 # PDF conversion package
│   ├── converter.go        # Main API with functional options
│   ├── pdf/                # Low-level PDF parsing
│   ├── models/             # Shared data structures
│   └── transform/          # Transformation pipeline
└── docx2md/                # DOCX conversion package
    ├── converter.go        # Main API with functional options
    ├── docx/               # DOCX parsing (ZIP/XML)
    └── transform/          # Transformation pipeline
└── xlsx2md/                # XLSX conversion package
    ├── converter.go        # Main API with functional options
    └── xlsx/               # XLSX parsing (ZIP/XML)
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

#### 2-Column Page Layout Detection (`pdf2md/transform/compact_lines.go`)
PDFs with 2-column layouts are automatically detected and processed with correct reading order:
- `detectAndSplitMultiColumnLayout()` - Detects 2-column layouts by checking if right column items start with lowercase letters or punctuation (indicating sentence continuation)
- When detected, left column is processed first (top to bottom), then right column
- Fragment ratio threshold: >30% of right column items starting with lowercase/punctuation triggers detection
- `looksLikeMultiColumnLayout()` - Used by both table detection and layout detection to avoid false positives

#### Table Rendering (`pdf2md/models/blocktypes.go`)
`LinesToText()` renders table rows with markdown syntax:
- `| col1 | col2 | col3 |` for data rows
- `| --- | --- | --- |` separator after header rows

#### Cross-Page Table Handling (`pdf2md/transform/to_markdown.go`)
- `deduplicateTableHeader()` - Removes repeated headers from continuation tables
- `mergeTablesCrossPages()` - Joins table rows separated by page breaks

#### Page Markers (`pdf2md/transform/to_markdown.go`)
Enabled by default (`WithPageMarkers`, CLI `-no-location-markers`):
- `<!-- page N -->` precedes each page's content; N is `Page.Index+1` (physical page), so stripped pages do not shift numbering
- `mergeTablesCrossPages()` holds back markers that fall inside a continued table and emits `<!-- pages N-M -->` before the table instead
- Scanned pages are emitted in page order by ToMarkdown via `Page.ScanImage`

#### Glyph-by-Glyph Text (`pdf2md/pdf/extractor.go`)
`mergeGlyphRuns()` handles PDFs that draw every glyph with its own show-text operation and no space characters:
- Applies only to pages where at least 80% of text items are fragments of 3 runes or fewer
- Joins touching fragments and inserts a space where the gap is 0.18-0.6 of the font size
- Pages that draw words or lines per operation are returned unchanged (table and reference detection depend on their short fragments staying separate)

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

#### Paragraph Markers (`docx2md/docx/extractor.go`, `docx2md/transform/to_markdown.go`)
Enabled by default (`WithParagraphMarkers`, CLI `-no-location-markers`):
- `bodyReader` wraps the XML decoder and counts every `w:p` in document order, including table cells and empty paragraphs, skipping `mc:Fallback`
- `LineItem.Paragraph` / `TextBlock.Paragraph` carry the number through the pipeline (`LineItem.Copy()` must copy it)
- `<!-- para N -->` precedes every heading and table, and at least every `ParagraphMarkerInterval` paragraphs elsewhere
- The count matches the paragraph order of a browser DOCX renderer (verified against docx-preview), so a viewer can scroll to the Nth paragraph

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

## Source Path Header (`internal/source/`)

File-based conversions in all formats (`ConvertFile`, `ConvertFileToFile`) start the Markdown with `<!-- source: /absolute/path -->`. Data-based conversions (`Convert`) have no path, so no header. New formats should call `source.Prepend()` the same way.

---

## Location Contract and Tests

The Markdown must let a reader trace any passage back to its place in the original: source path (first line), then page (PDF), paragraph (DOCX) or sheet/range/row (XLSX). XLSX output already carries `### Range: Sheet!A1:D8` headings and a `Row` column.

- `cli/main_test.go` builds the CLI and runs it on the fixtures in `testdata/`, asserting where known passages are located.
- `test.sh` sections 3-5 drive the compiled CLI and assert on output files, stdout, stderr and exit codes.
- CI (`.github/workflows/ci.yml`) runs `make test`.
- `testdata/README.md` describes each fixture and how it was made.
- `integration_test.go` uses `private/`, which is not in the repository; it is built only with the `private` tag (`make test-private`).

---

## Adding New Format Support

To add a new format (e.g., RTF):

1. Create package `rtf2md/`
2. Implement parser in `rtf2md/rtf/`
3. Create `rtf2md/converter.go` with functional options
4. Implement transformations in `rtf2md/transform/`
5. Add format detection in `cli/main.go`
6. Reuse `pdf2md/models/` where applicable

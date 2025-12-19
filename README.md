# x2md

x2md is a utility and set of libraries for converting various file types to Markdown. It is written in pure Go with no CGO dependencies, making it easy to cross-compile for any platform.

## Supported Formats

- [PDF](#pdf-conversion) - Text-based PDF documents
- [DOCX](#docx-conversion) - Microsoft Word documents (.docx)

## Installation

```bash
# Clone and build
git clone https://github.com/tenebris-tech/x2md.git
cd x2md
go build

# Or install directly
go install github.com/tenebris-tech/x2md@latest
```

## CLI Usage

```bash
# Convert a file (outputs to same name with .md extension)
./x2md input.pdf
./x2md input.docx

# Specify output file
./x2md -output out.md input.pdf

# Verbose mode
./x2md -v input.pdf

# Disable image extraction
./x2md -no-images input.pdf
```

### CLI Options

| Option | Description |
|--------|-------------|
| `-output` | Specify output file path |
| `-v` | Verbose mode with progress output |
| `-no-images` | Disable image extraction |
| `-strip-headers` | Remove repetitive headers/footers (PDF only) |
| `-strip-page-numbers` | Remove page numbers (PDF only) |
| `-strip-toc` | Remove table of contents (PDF only) |
| `-strip-footnotes` | Remove footnotes (PDF only) |
| `-strip-blank-pages` | Remove blank pages (PDF only) |
| `-no-lists` | Disable list detection (PDF only) |
| `-no-headings` | Disable heading detection (PDF only) |
| `-no-formatting` | Disable bold/italic formatting |

---

## PDF Conversion

### Features

- No external dependencies or CGO
- Converts PDF text content to well-formatted Markdown
- Detects and preserves document structure:
  - Headings (H1-H6) based on font size
  - Lists (bulleted and numbered) with nesting
  - Tables with proper markdown formatting
  - Table of contents
  - Bold and italic text formatting
  - Footnotes with superscript detection
- Image extraction (JPEG and PNG)
- Handles multi-page tables with header deduplication
- Removes repetitive headers/footers
- Supports linearized PDFs and object streams
- Detects and rejects encrypted PDFs with clear error message
- Cross-platform support

### How It Works

The PDF converter uses a multi-stage transformation pipeline:

```
PDF Binary → Parser → Text Items → Lines → Blocks → Markdown
```

**Stage 1: PDF Parsing** (`pdf2md/pdf/parser.go`)
- Parses PDF structure: xref tables, object streams, content streams
- Handles linearized PDFs (uses last startxref for compatibility)
- Supports FlateDecode compression with PNG predictor
- Detects encryption and returns clear error message

**Stage 2: Text Extraction** (`pdf2md/pdf/extractor.go`)
- Extracts text from content streams with position, size, and font data
- Handles various text operators (Tj, TJ, ', ")
- Tracks font changes and text matrices

**Stage 3: Transformation Pipeline** (`pdf2md/transform/`)
1. `CalculateGlobalStats` - Determine baseline font size and line spacing
2. `CompactLines` - Group text items into lines, detect tables
3. `RemoveRepetitiveElements` - Strip headers/footers that repeat on each page
4. `DetectTOC` - Identify table of contents
5. `DetectHeaders` - Mark headings based on font size difference from body text
6. `DetectListItems` - Find bullet points and numbered lists
7. `GatherBlocks` - Organize lines into logical blocks
8. `RemoveBlankPages` - Filter out empty pages
9. `ToTextBlocks` - Convert to text representation
10. `ToMarkdown` - Generate final markdown with table merging

### Table Detection

The library detects tables using multiple strategies:
- **Header-based**: Rows with well-spaced column headers
- **Reference-style**: Bracketed IDs like [CC1], [CC2] aligned in columns
- **Known headers**: Recognizes common table headers (Version, Date, Description, etc.)
- **Cross-page merging**: Automatically joins tables split across pages
- **Header deduplication**: Removes repeated headers from continuation pages

### Image Extraction

Images are extracted and saved to a subdirectory:
```
input.pdf → input.md + input_images/image_001.png, image_002.jpg, ...
```

Markdown references are automatically inserted: `![alt](input_images/image_001.png)`

Supported formats: JPEG (direct extraction), PNG (for raw pixel data)

### Limitations

- Scanned PDFs (image-only) produce no text output
- Complex multi-column layouts may interleave columns
- Non-standard font encodings may cause character issues
- Mathematical formulas are converted as plain text
- LZW compression not implemented (gracefully skipped)
- Encrypted PDFs are not supported (clear error message provided)

### Library Usage (PDF)

```go
import "github.com/tenebris-tech/x2md/pdf2md"

// Basic usage
converter := pdf2md.New()
markdown, err := converter.ConvertFile("input.pdf")

// With options
converter := pdf2md.New(
    pdf2md.WithDetectLists(true),
    pdf2md.WithDetectHeadings(true),
    pdf2md.WithPreserveFormatting(true),
)
markdown, err := converter.ConvertFile("input.pdf")

// With image extraction
data, _ := os.ReadFile("input.pdf")
markdown, images, err := converter.ConvertWithImages(data)
// images is []ImageData with Name, Data, Format fields
```

---

## DOCX Conversion

### Features

- Pure Go implementation - no CGO or external dependencies
- Parses DOCX (Office Open XML) format directly
- Preserves document structure:
  - Headings (H1-H6) from Word styles
  - Lists (bulleted and numbered) with proper nesting
  - Tables with markdown formatting
  - Bold and italic text formatting
  - Hyperlinks converted to markdown link syntax
  - Footnotes and endnotes with proper markdown syntax
- Image extraction from embedded media
- Handles text runs split across XML elements (common in edited documents)
- Cross-platform support

### How It Works

DOCX files are ZIP archives containing XML files:

```
document.docx (ZIP)
├── word/document.xml      # Main content
├── word/styles.xml        # Style definitions
├── word/numbering.xml     # List numbering
├── word/footnotes.xml     # Footnotes
├── word/endnotes.xml      # Endnotes
├── word/_rels/document.xml.rels  # Relationships (links, images)
└── word/media/            # Embedded images
```

**Stage 1: ZIP Extraction** (`docx2md/docx/parser.go`)
- Opens DOCX as ZIP archive
- Parses XML with namespace handling
- Critical: Copies CharData tokens (xml.Decoder reuses internal buffer)

**Stage 2: Content Extraction** (`docx2md/docx/extractor.go`)
- Extracts paragraphs with text runs
- Resolves styles to determine formatting
- Processes hyperlinks from relationships
- Extracts footnote and endnote content

**Stage 3: Style Resolution** (`docx2md/docx/styles.go`)
- Maps Word styles to markdown: Heading 1 → `# `, Heading 2 → `## `
- Resolves style inheritance (basedOn)
- Detects bold/italic from run properties or style

**Stage 4: List Processing** (`docx2md/docx/styles.go`)
- Parses numbering definitions (numFmt: bullet, decimal, lowerLetter, etc.)
- Handles nested lists with indentation levels (ilvl)

**Stage 5: Transformation** (`docx2md/transform/`)
- Groups content into blocks (each paragraph is its own block)
- Adds proper spacing between paragraphs (`\n\n`)
- Generates markdown table syntax for Word tables

### Footnotes and Endnotes

Footnotes are converted to markdown footnote syntax:
```markdown
This is text with a footnote[^1].

## Footnotes

[^1]: This is the footnote content with [links](https://example.com).
```

### Image Extraction

Images are extracted from `word/media/` and saved to a subdirectory:
```
input.docx → input.md + input_images/image_001.png, image_002.jpg, ...
```

### Limitations

- Complex nested tables may not render perfectly
- Advanced formatting (text boxes, shapes, SmartArt) is ignored
- Comments and tracked changes are not included
- Document headers/footers are not extracted

### Library Usage (DOCX)

```go
import "github.com/tenebris-tech/x2md/docx2md"

// Basic usage
converter := docx2md.New()
markdown, err := converter.ConvertFile("input.docx")

// With options
converter := docx2md.New(
    docx2md.WithPreserveFormatting(true),
)
markdown, err := converter.ConvertFile("input.docx")

// With image extraction
data, _ := os.ReadFile("input.docx")
markdown, images, err := converter.ConvertWithImages(data)
```

---

## Architecture

```
x2md/
├── main.go                 # CLI entry point, format detection
├── integration_test.go     # Integration/regression tests
├── pdf2md/                 # PDF conversion package
│   ├── converter.go        # Public API with functional options
│   ├── pdf/                # Low-level PDF parsing
│   │   ├── parser.go       # PDF structure (xref, objects, streams)
│   │   └── extractor.go    # Text extraction from content streams
│   ├── models/             # Shared data structures
│   │   ├── models.go       # Core types (TextItem, Page, LineItem, etc.)
│   │   ├── blocktypes.go   # Block types and table rendering
│   │   └── wordtypes.go    # Word formatting types
│   └── transform/          # Transformation pipeline
│       ├── calculate_global_stats.go
│       ├── compact_lines.go
│       ├── detect_headers.go
│       ├── detect_list_items.go
│       ├── detect_toc.go
│       ├── gather_blocks.go
│       ├── remove_blank_pages.go
│       ├── remove_repetitive_elements.go
│       ├── to_markdown.go
│       └── to_text_blocks.go
├── docx2md/                # DOCX conversion package
│   ├── converter.go        # Public API with functional options
│   ├── docx/               # DOCX parsing
│   │   ├── parser.go       # ZIP/XML handling
│   │   ├── extractor.go    # Content extraction
│   │   ├── elements.go     # XML element structures
│   │   ├── styles.go       # Style and numbering definitions
│   │   └── relationships.go # Hyperlinks and images
│   └── transform/          # Transformation pipeline
│       ├── transform.go
│       ├── gather_blocks.go
│       ├── to_text_blocks.go
│       └── to_markdown.go
└── imageutil/              # Shared image utilities
```

---

## Testing

### Unit Tests

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run in short mode (skips large file tests)
go test -short ./...
```

### Integration Tests

Integration tests in `integration_test.go` test the converter API against real documents. These tests use files in the `private/` directory (not included in the repository).

| Test | Description |
|------|-------------|
| `TestPDFConversion` | Converts 3 PDF files, checks for expected content |
| `TestDOCXConversion` | Converts 7 DOCX files, checks for expected content |
| `TestDOCXParagraphSeparation` | Regression test: paragraphs have blank lines between them |
| `TestDOCXFootnoteContent` | Regression test: footnotes contain correct URLs, not corrupted text |
| `TestEncryptedPDFRejection` | Verifies encrypted PDFs are rejected with clear error |
| `TestPDFTableDetection` | Verifies markdown table syntax in output |
| `TestPDFHeaderDetection` | Verifies H1/H2 headers are detected |
| `TestImageExtraction` | Tests image extraction for both PDF and DOCX |
| `TestLargePDFPerformance` | Performance test with large file (skipped with `-short`) |

Tests skip gracefully if test files are not available:
```
=== RUN   TestPDFConversion/basic-text
    integration_test.go:48: Test file not available: private/basic-text.pdf
--- SKIP: TestPDFConversion/basic-text
```

### Adding Test Files

To run integration tests, add test documents to the `private/` directory:
```bash
mkdir -p private
cp your-test.pdf private/
cp your-test.docx private/
```

### Static Analysis

```bash
go vet ./...
```

---

## Development

### Building

```bash
# Build for current platform
go build

# Build for multiple platforms
GOOS=linux GOARCH=amd64 go build -o x2md-linux
GOOS=darwin GOARCH=amd64 go build -o x2md-macos
GOOS=darwin GOARCH=arm64 go build -o x2md-macos-arm
GOOS=windows GOARCH=amd64 go build -o x2md.exe
```

### Adding New Format Support

To add a new format (e.g., RTF):

1. Create package `rtf2md/`
2. Implement parser in `rtf2md/rtf/`
3. Create `rtf2md/converter.go` with functional options pattern
4. Implement transformations in `rtf2md/transform/`
5. Add format detection in `main.go`
6. Reuse `pdf2md/models/` where applicable

---

## Acknowledgments

This software was written from scratch in pure Go with the assistance of Claude Code.

We gratefully acknowledge inspiration from the following open source projects:

- https://github.com/opendocsg/pdf2md (MIT License)
- https://github.com/mozilla/pdf.js (Apache 2.0 License)

## Copyright and Licensing

Copyright (c) 2025 Tenebris Technologies Inc. All rights reserved.
Please contact us for a licence if you wish to use this software.

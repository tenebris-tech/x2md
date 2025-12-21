# x2md

x2md is a utility and set of libraries for converting various file types to Markdown. It is written in pure Go with no CGO dependencies, making it easy to cross-compile for any platform.

## Supported Formats

- [PDF](#pdf2md) - Text-based PDF documents
- [DOCX](#docx2md) - Microsoft Word documents (.docx)

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

### Single File Conversion

```bash
# Convert a file (outputs to same name with .md extension)
./x2md input.pdf
./x2md input.docx

# Specify output file
./x2md -output out.md input.pdf

# Verbose mode
./x2md -v input.pdf
```

### Batch Conversion

```bash
# Recursively convert all PDF and DOCX files in a directory
./x2md -r ./documents

# Convert to a flat output directory
./x2md -r -output-dir ./markdown ./documents

# Don't skip existing .md files (creates numbered versions)
./x2md -r -skip-existing=false ./documents
```

### CLI Options

| Option | Description |
|--------|-------------|
| `-r` | Recursively process directories |
| `-output` | Specify output file path (single file mode only) |
| `-output-dir` | Output directory for all converted files (flat structure) |
| `-skip-existing` | Skip files where .md already exists (default: true) |
| `-v` | Verbose mode with progress output |
| `-no-images` | Disable image extraction |
| `-no-formatting` | Disable bold/italic formatting |
| `-strip-headers` | Remove repetitive headers/footers (PDF only) |
| `-strip-page-numbers` | Remove page numbers (PDF only) |
| `-strip-toc` | Remove table of contents (PDF only) |
| `-strip-footnotes` | Remove footnotes (PDF only) |
| `-strip-blank-pages` | Remove blank pages (PDF only) |
| `-no-lists` | Disable list detection (PDF only) |
| `-no-headings` | Disable heading detection (PDF only) |

---

## Convert Package

The `convert` package provides a unified API for batch document conversion with support for recursive directory traversal, output management, and pass-through options to the underlying converters.

### Basic Usage

```go
import "github.com/tenebris-tech/x2md/convert"

// Convert a single file (output placed next to source)
c := convert.New()
result, err := c.Convert("document.pdf")

// Convert a directory recursively
c := convert.New(convert.WithRecursion(true))
result, err := c.Convert("./documents")

// Check results
fmt.Printf("Converted: %d, Skipped: %d, Failed: %d\n",
    result.Converted, result.Skipped, result.Failed)
```

### Options

#### WithRecursion(bool)

Enable recursive directory traversal. When false (default), Convert() only accepts file paths. When true, it accepts both files and directories.

```go
c := convert.New(convert.WithRecursion(true))
result, _ := c.Convert("./documents")  // Processes all subdirectories
```

#### WithExtensions([]string)

Specify which file extensions to convert. Default: `[]string{".pdf", ".docx"}`

```go
// Only convert PDF files
c := convert.New(
    convert.WithRecursion(true),
    convert.WithExtensions([]string{".pdf"}),
)
```

#### WithSkipExisting(bool)

Skip files where the output .md file already exists. Default: true

When false, existing files are not overwritten. Instead, numbered versions are created (e.g., `document-1.md`, `document-2.md`).

```go
// Create numbered versions instead of skipping
c := convert.New(convert.WithSkipExisting(false))
```

#### WithOutputDirectory(string)

Write all output files to a specified directory with a flat structure. By default, output files are placed next to their source files.

```go
// All .md files go to ./output, regardless of source location
c := convert.New(
    convert.WithRecursion(true),
    convert.WithOutputDirectory("./output"),
)
```

When files from different directories have the same name, numbered versions are created automatically.

#### WithVerbose(bool)

Enable verbose output (primarily for debugging).

```go
c := convert.New(convert.WithVerbose(true))
```

#### WithPDFOptions(...pdf2md.Option)

Pass options through to the PDF converter.

```go
import "github.com/tenebris-tech/x2md/pdf2md"

c := convert.New(
    convert.WithRecursion(true),
    convert.WithPDFOptions(
        pdf2md.WithExtractImages(false),
        pdf2md.WithDetectHeadings(true),
        pdf2md.WithStrip(pdf2md.HeadersFooters, pdf2md.BlankPages),
    ),
)
```

#### WithDOCXOptions(...docx2md.Option)

Pass options through to the DOCX converter.

```go
import "github.com/tenebris-tech/x2md/docx2md"

c := convert.New(
    convert.WithRecursion(true),
    convert.WithDOCXOptions(
        docx2md.WithPreserveImages(false),
        docx2md.WithPreserveFormatting(true),
    ),
)
```

#### WithOnFileStart / WithOnFileComplete

Set callbacks for conversion progress monitoring.

```go
c := convert.New(
    convert.WithRecursion(true),
    convert.WithOnFileStart(func(path string) {
        fmt.Printf("Converting: %s\n", path)
    }),
    convert.WithOnFileComplete(func(path, outputPath string, err error) {
        if err != nil {
            fmt.Printf("  Failed: %v\n", err)
        } else {
            fmt.Printf("  Created: %s\n", outputPath)
        }
    }),
)
```

### Result Structure

```go
type Result struct {
    Converted int      // Number of files successfully converted
    Skipped   int      // Number of files skipped (already exist)
    Failed    int      // Number of files that failed
    Errors    []error  // Details of failures
}
```

### Edge Cases Handled

- **Broken symlinks**: Reported as failures, processing continues
- **Symlink loops**: Detected and avoided via real path tracking
- **Duplicate files**: Files reached via different symlinks are converted only once
- **Name conflicts**: When output files would conflict, numbered versions are created
- **Directories with .pdf/.docx extensions**: Correctly identified as directories, not files

---

## Converters

### pdf2md

The `pdf2md` package converts PDF documents to Markdown.

#### Features

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

#### Library Usage

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
    pdf2md.WithExtractImages(true),
)
markdown, err := converter.ConvertFile("input.pdf")

// File to file conversion (handles image extraction automatically)
err := converter.ConvertFileToFile("input.pdf", "output.md")

// With image extraction from bytes
data, _ := os.ReadFile("input.pdf")
markdown, images, err := converter.ConvertWithImages(data)
// images is []*models.ImageItem with ID, Data, Format fields
```

#### Options

| Option | Description | Default |
|--------|-------------|---------|
| `WithStrip(...StripOption)` | Content to strip (HeadersFooters, PageNumbers, TOC, Footnotes, BlankPages) | HeadersFooters, BlankPages |
| `WithDetectLists(bool)` | Enable list detection | true |
| `WithDetectHeadings(bool)` | Enable heading detection | true |
| `WithPreserveFormatting(bool)` | Preserve bold/italic | true |
| `WithExtractImages(bool)` | Extract images | true |
| `WithPageSeparator(string)` | Separator between pages | "\n" |
| `WithOnPageParsed(func)` | Callback for page progress | nil |
| `WithOnFontParsed(func)` | Callback for font discovery | nil |

#### How It Works

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

#### Table Detection

The library detects tables using multiple strategies:
- **Header-based**: Rows with well-spaced column headers
- **Reference-style**: Bracketed IDs like [CC1], [CC2] aligned in columns
- **Known headers**: Recognizes common table headers (Version, Date, Description, etc.)
- **Cross-page merging**: Automatically joins tables split across pages
- **Header deduplication**: Removes repeated headers from continuation pages

#### Image Extraction

Images are extracted and saved to a subdirectory:
```
input.pdf → input.md + input_images/image_001.png, image_002.jpg, ...
```

Markdown references are automatically inserted: `![alt](input_images/image_001.png)`

Supported formats: JPEG (direct extraction), PNG (for raw pixel data)

#### Limitations

- Scanned PDFs (image-only) produce no text output
- Complex multi-column layouts may interleave columns
- Non-standard font encodings may cause character issues
- Mathematical formulas are converted as plain text
- LZW compression supported for image extraction
- Encrypted PDFs are not supported (clear error message provided)

---

### docx2md

The `docx2md` package converts Microsoft Word documents to Markdown.

#### Features

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

#### Library Usage

```go
import "github.com/tenebris-tech/x2md/docx2md"

// Basic usage
converter := docx2md.New()
markdown, err := converter.ConvertFile("input.docx")

// With options
converter := docx2md.New(
    docx2md.WithPreserveFormatting(true),
    docx2md.WithPreserveImages(true),
)
markdown, err := converter.ConvertFile("input.docx")

// File to file conversion (handles image extraction automatically)
err := converter.ConvertFileToFile("input.docx", "output.md")

// With image extraction from bytes
data, _ := os.ReadFile("input.docx")
markdown, images, err := converter.ConvertWithImages(data)
```

#### Options

| Option | Description | Default |
|--------|-------------|---------|
| `WithPreserveFormatting(bool)` | Preserve bold/italic | true |
| `WithPreserveImages(bool)` | Extract and include images | true |
| `WithImageLinkFormat(string)` | Template for image links | "![%s](%s)" |
| `WithPageSeparator(string)` | Separator between sections | "\n" |
| `WithOnDocumentParsed(func)` | Callback when document is parsed | nil |
| `WithOnStylesParsed(func)` | Callback with style count | nil |

#### How It Works

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

#### Footnotes and Endnotes

Footnotes are converted to markdown footnote syntax:
```markdown
This is text with a footnote[^1].

## Footnotes

[^1]: This is the footnote content with [links](https://example.com).
```

#### Image Extraction

Images are extracted from `word/media/` and saved to a subdirectory:
```
input.docx → input.md + input_images/image_001.png, image_002.jpg, ...
```

#### Limitations

- Complex nested tables may not render perfectly
- Advanced formatting (text boxes, shapes, SmartArt) is ignored
- Comments and tracked changes are not included
- Document headers/footers are not extracted

---

## Architecture

```
x2md/
├── main.go                 # CLI entry point
├── integration_test.go     # Integration/regression tests
├── convert/                # Unified conversion package
│   ├── converter.go        # Batch conversion with options
│   └── converter_test.go   # Unit tests
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

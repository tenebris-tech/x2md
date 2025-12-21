# x2md

x2md is a pure Go library and CLI for converting documents to Markdown. No CGO dependencies, making it easy to cross-compile for any platform.

## CLI

### Installation

```bash
go install github.com/tenebris-tech/x2md@latest
```

Or build from source:

```bash
git clone https://github.com/tenebris-tech/x2md.git
cd x2md
go build
```

### Usage

```bash
# Convert a file (outputs to same name with .md extension)
x2md document.pdf
x2md document.docx

# Specify output file
x2md -output out.md document.pdf

# Batch convert a directory recursively
x2md -r ./documents

# Verbose mode
x2md -v document.pdf
```

### Options

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
| `-no-scan-mode` | Disable automatic scanned page detection (PDF only) |

---

## Library

The `convert` package provides a unified API for document conversion. It handles format detection and delegates to the appropriate format-specific converter.

### Simple Usage

```go
import "github.com/tenebris-tech/x2md/convert"

// Convert a single file - output placed next to source
c := convert.New()
result, err := c.Convert("document.pdf")
```

### Batch Conversion

```go
// Convert all documents in a directory recursively
c := convert.New(convert.WithRecursion(true))
result, err := c.Convert("./documents")

fmt.Printf("Converted: %d, Skipped: %d, Failed: %d\n",
    result.Converted, result.Skipped, result.Failed)
```

### Options

```go
c := convert.New(
    convert.WithRecursion(true),              // Process directories recursively
    convert.WithSkipExisting(false),          // Don't skip existing .md files
    convert.WithOutputDirectory("./output"),  // Write all output to one directory
    convert.WithExtensions([]string{".pdf"}), // Only convert PDF files
)
```

### Pass-Through Options

Pass options to the underlying format converters:

```go
import (
    "github.com/tenebris-tech/x2md/convert"
    "github.com/tenebris-tech/x2md/pdf2md"
    "github.com/tenebris-tech/x2md/docx2md"
)

c := convert.New(
    convert.WithRecursion(true),
    convert.WithPDFOptions(
        pdf2md.WithScanMode(false),
        pdf2md.WithStrip(pdf2md.HeadersFooters, pdf2md.BlankPages),
    ),
    convert.WithDOCXOptions(
        docx2md.WithPreserveImages(false),
    ),
)
```

### Progress Callbacks

```go
c := convert.New(
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
    Converted int      // Files successfully converted
    Skipped   int      // Files skipped (already exist)
    Failed    int      // Files that failed
    Errors    []error  // Details of failures
}
```

---

## Document Formats

x2md supports multiple document formats. Each format has its own package that can be used directly for more control, or through the unified `convert` package.

- **PDF** - Text-based and scanned PDF documents (`pdf2md`)
- **DOCX** - Microsoft Word documents (`docx2md`)

---

### PDF

The `pdf2md` package converts PDF documents to Markdown.

#### Features

- Text extraction with structure detection (headings, lists, tables)
- Automatic scanned page detection - extracts page images for OCR/LLM processing
- Image extraction (JPEG and PNG)
- Multi-page table handling with header deduplication
- Footnote detection
- Bold/italic text formatting
- Encrypted PDF detection with clear error message

#### Simple Usage

```go
import "github.com/tenebris-tech/x2md/pdf2md"

converter := pdf2md.New()
err := converter.ConvertFileToFile("input.pdf", "output.md")
```

#### With Options

```go
converter := pdf2md.New(
    pdf2md.WithScanMode(false),           // Disable scanned page detection
    pdf2md.WithExtractImages(false),      // Skip image extraction
    pdf2md.WithStrip(pdf2md.HeadersFooters, pdf2md.PageNumbers),
)
markdown, err := converter.ConvertFile("input.pdf")
```

#### Get Markdown and Images Separately

```go
data, _ := os.ReadFile("input.pdf")
markdown, images, err := converter.ConvertWithImages(data)
// images is []*models.ImageItem with ID, Data, Format fields
```

#### Options

| Option | Description | Default |
|--------|-------------|---------|
| `WithScanMode(bool)` | Auto-detect scanned pages and extract as images | true |
| `WithStrip(...StripOption)` | Content to strip (HeadersFooters, PageNumbers, TOC, Footnotes, BlankPages) | HeadersFooters, BlankPages |
| `WithDetectLists(bool)` | Enable list detection | true |
| `WithDetectHeadings(bool)` | Enable heading detection | true |
| `WithPreserveFormatting(bool)` | Preserve bold/italic | true |
| `WithExtractImages(bool)` | Extract images | true |
| `WithPageSeparator(string)` | Separator between pages | "\n" |

#### Scanned PDF Handling

Scanned pages are automatically detected and extracted as images:

```
scanned.pdf → scanned.md + scanned_images/page_001.jpg, page_002.jpg, ...
```

The markdown contains image references ready for OCR or LLM processing:

```markdown
![Page 1](scanned_images/page_001.jpg)

![Page 2](scanned_images/page_002.jpg)
```

Detection criteria: pages with <100 characters of text and large images (>50% of page size or >500×500 pixels). Mixed documents (some scanned, some text) are handled automatically.

#### Image Extraction

Images are extracted and saved to a subdirectory:

```
input.pdf → input.md + input_images/image_001.png, image_002.jpg, ...
```

#### Limitations

- Complex multi-column layouts may interleave columns
- Non-standard font encodings may cause character issues
- Mathematical formulas are converted as plain text
- Encrypted PDFs are not supported

---

### DOCX

The `docx2md` package converts Microsoft Word documents to Markdown.

#### Features

- Heading detection from Word styles (Heading 1-6)
- List support (bulleted, numbered, lettered, roman numerals) with nesting
- Table extraction with markdown formatting
- Bold/italic text formatting
- Hyperlink conversion to markdown syntax
- Footnotes and endnotes
- Image extraction from embedded media

#### Simple Usage

```go
import "github.com/tenebris-tech/x2md/docx2md"

converter := docx2md.New()
err := converter.ConvertFileToFile("input.docx", "output.md")
```

#### With Options

```go
converter := docx2md.New(
    docx2md.WithPreserveFormatting(false),  // Skip bold/italic
    docx2md.WithPreserveImages(false),      // Skip image extraction
)
markdown, err := converter.ConvertFile("input.docx")
```

#### Get Markdown and Images Separately

```go
data, _ := os.ReadFile("input.docx")
markdown, images, err := converter.ConvertWithImages(data)
```

#### Options

| Option | Description | Default |
|--------|-------------|---------|
| `WithPreserveFormatting(bool)` | Preserve bold/italic | true |
| `WithPreserveImages(bool)` | Extract and include images | true |
| `WithExtractHeadersFooters(bool)` | Include document headers/footers | false |
| `WithPageSeparator(string)` | Separator between sections | "\n" |

#### Image Extraction

Images are extracted from the DOCX media folder:

```
input.docx → input.md + input_images/image_001.png, image_002.jpg, ...
```

#### Limitations

- Complex nested tables may not render perfectly
- Advanced formatting (text boxes, shapes, SmartArt) is ignored
- Comments and tracked changes are not included

---

## Development

### Building

```bash
go build
go test ./...
go vet ./...
```

### Cross-Compilation

```bash
GOOS=linux GOARCH=amd64 go build -o x2md-linux
GOOS=darwin GOARCH=amd64 go build -o x2md-macos
GOOS=darwin GOARCH=arm64 go build -o x2md-macos-arm
GOOS=windows GOARCH=amd64 go build -o x2md.exe
```

### Adding New Format Support

To add a new format (e.g., RTF):

1. Create package `rtf2md/` with `converter.go` using functional options pattern
2. Implement parser in `rtf2md/rtf/`
3. Implement transformations in `rtf2md/transform/`
4. Add format detection to `convert/converter.go`
5. Add CLI flags to `main.go`

---

## Acknowledgments

This software was written in pure Go with the assistance of Claude Code.

Inspiration from:
- https://github.com/opendocsg/pdf2md (MIT License)
- https://github.com/mozilla/pdf.js (Apache 2.0 License)

## Copyright and Licensing

Copyright (c) 2025 Tenebris Technologies Inc. All rights reserved.
Please contact us for a licence if you wish to use this software.

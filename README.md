# x2md

x2md is a utility and set of libraries for converting various file types to Markdown. It is written in pure Go with no CGO dependencies, making it easy to cross-compile for any platform.

## Supported Formats

- [PDF](#pdf-conversion) - Text-based PDF documents
- [DOCX](#docx-conversion) - Microsoft Word documents (.docx)

## CLI Usage

```bash
# Build the CLI tool
go build

# Convert a file (outputs to same name with .md extension)
./x2md input.pdf
./x2md input.docx

# Specify output file
./x2md -output out.md input.pdf

# Verbose mode
./x2md -v input.pdf
```

### CLI Options

| Option | Description |
|--------|-------------|
| `-output` | Specify output file path |
| `-v` | Verbose mode with progress output |
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
  - Headings (H1-H6)
  - Lists (bulleted and numbered)
  - Tables with proper markdown formatting
  - Table of contents
  - Bold and italic text formatting
- Handles multi-page tables with header deduplication
- Removes repetitive headers/footers
- Cross-platform support

### How It Works

The conversion happens through a pipeline of transformations:

1. **PDF Parsing** - Extract text items with position, size, and font information
2. **Line Grouping** - Group text items on the same Y coordinate into lines
3. **Table Detection** - Identify tables by column alignment and structure
4. **Header Detection** - Identify headings by font size and style
5. **List Detection** - Find bullet points and numbered lists
6. **Block Grouping** - Organize lines into logical blocks
7. **Markdown Generation** - Convert blocks to markdown syntax

### Table Support

The library detects tables using multiple methods:
- Traditional tables with header rows (e.g., "Version | Date | Description")
- Reference-style tables with bracketed IDs (e.g., [CC1], [CC2])
- Multi-page tables are automatically merged with duplicate headers removed

### Limitations

- Scanned PDFs (images) are not supported - only text-based PDFs
- Complex layouts with overlapping text regions may not convert perfectly
- Some non-standard font encodings may cause character display issues
- Mathematical formulas and equations are converted as plain text

### Library Usage (PDF)

```go
import "github.com/tenebris-tech/x2md/pdf2md"

converter := pdf2md.New(
    pdf2md.WithDetectLists(true),
    pdf2md.WithDetectHeadings(true),
    pdf2md.WithPreserveFormatting(true),
)
markdown, err := converter.ConvertFile("input.pdf")
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
- Handles text runs split across XML elements (common in edited documents)
- Cross-platform support

### How It Works

DOCX files are ZIP archives containing XML files. The conversion process:

1. **ZIP Extraction** - Open the DOCX archive
2. **XML Parsing** - Parse `word/document.xml` (content), `word/styles.xml` (styles), `word/numbering.xml` (lists)
3. **Style Resolution** - Map Word styles to markdown elements (Heading 1 → H1, etc.)
4. **Text Extraction** - Extract text runs with formatting, concatenate per OOXML spec
5. **List Processing** - Apply numbering definitions to create proper list prefixes
6. **Table Extraction** - Convert Word tables to markdown table syntax
7. **Markdown Generation** - Output final markdown

### Limitations

- Images are not extracted (only referenced)
- Complex nested tables may not render perfectly
- Some advanced formatting (text boxes, shapes) is ignored
- Comments and tracked changes are not included

### Library Usage (DOCX)

```go
import "github.com/tenebris-tech/x2md/docx2md"

converter := docx2md.New(
    docx2md.WithPreserveFormatting(true),
)
markdown, err := converter.ConvertFile("input.docx")
```

---

## Development

```bash
# Run tests
go test ./...

# Static analysis
go vet ./...

# Build for multiple platforms
GOOS=linux GOARCH=amd64 go build -o x2md-linux
GOOS=darwin GOARCH=amd64 go build -o x2md-macos
GOOS=windows GOARCH=amd64 go build -o x2md.exe
```

## Acknowledgments

This software was written from scratch in pure Go with the assistance of Claude Code.

We gratefully acknowledge inspiration from the following open source projects:

- https://github.com/opendocsg/pdf2md (MIT License)
- https://github.com/mozilla/pdf.js (Apache 2.0 License)

## Copyright and Licensing

Copyright (c) 2025 Tenebris Technologies Inc. All rights reserved.
Please contact us for a licence if you wish to use this software.

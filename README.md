# x2md

x2md is a utility and set of libraries for converting various file types to Markdown. It is a work in progress and will be expanded over time to handle additional file types.

## Currently supported

### pdf

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

#### Limitations

- Scanned PDFs (images) are not supported - only text-based PDFs
- Complex layouts with overlapping text regions may not convert perfectly
- Some non-standard font encodings may cause character display issues
- Mathematical formulas and equations are converted as plain text


#### How It Works

The conversion happens through a pipeline of transformations:

1. **PDF Parsing** - Extract text items with position, size, and font information
2. **Line Grouping** - Group text items on the same Y coordinate into lines
3. **Table Detection** - Identify tables by column alignment and structure
4. **Header Detection** - Identify headings by font size and style
5. **List Detection** - Find bullet points and numbered lists
6. **Block Grouping** - Organize lines into logical blocks
7. **Markdown Generation** - Convert blocks to markdown syntax

#### Table Support

The library detects tables using multiple methods:
- Traditional tables with header rows (e.g., "Version | Date | Description")
- Reference-style tables with bracketed IDs (e.g., [CC1], [CC2])
- Multi-page tables are automatically merged with duplicate headers removed

## CLI Usage

```bash
# Build the CLI tool
go build -o

# Convert a PDF (outputs to same name with .md extension)
./x2md input.pdf

# Specify output file
./x2md -output out.md input.pdf

# Verbose mode
./x2md -v input.pdf
```

## Acknowledgments

This software was written from scratch in pure Go with the assistance of Claude Code and Codex.

We gratefully acknowledge inspiration from the following open source projects:

- https://github.com/opendocsg/pdf2md (MIT License)

- https://github.com/mozilla/pdf.js (Apache 2.0 License)

## Copyright and Licensing
Copyright (c) 2025 Tenebris Technologies Inc. All rights reserved.
Please contact us for a licence if you wish to use this software.


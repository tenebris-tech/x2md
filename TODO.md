# TODO.md

This file tracks pending improvements and known issues for x2md.

## Current Status

### PDF Conversion - Stable
All major features implemented and tested.

### DOCX Conversion - Initial Release
Core functionality implemented.

### Image Extraction - Implemented
Both PDF and DOCX now support image extraction:
- Images saved to `{output}_images/` subdirectory
- Markdown image links generated: `![alt](path)`
- JPEG images extracted directly
- PNG wrapping for raw pixel data
- Use `--no-images` flag to disable

---

## PDF Conversion

### Completed Work

#### Issues Fixed
1. **Date Spacing** - Fixed extra space in "06-December -2023"
   - Modified `needsSpaceBetween()` in `transform/compact_lines.go`

2. **False Header Detection** - Fixed "academia." being detected as H4
   - Modified `transform/detect_headers.go` to skip punctuation-ending lines

3. **Reference Table Grouping** - Fixed [CC1], [CC2] table rendering
   - Added `detectReferenceTable()` in `transform/compact_lines.go`

4. **Revision History Table** - Full table support implemented
   - Tables render with `| col1 | col2 |` markdown syntax
   - Cross-page tables merged into single continuous table

#### Code Quality Improvements
- Extracted magic numbers to constants
- Made known table headers configurable
- Added `Copy()` method to LineItem
- Added documentation to complex functions
- Refactored deeply nested code with helper methods
- Added validation for empty table rows
- Optimized string building performance

#### Test Suite
- `pdf2md/transform/compact_lines_test.go`
- `pdf2md/models/models_test.go`
- `pdf2md/models/blocktypes_test.go`

### Known Limitations

1. **Scanned PDFs**: Only text-based PDFs supported
2. **Complex Layouts**: Overlapping text regions may not convert perfectly
3. **Font Encodings**: Some non-standard fonts may have character issues
4. **Math Formulas**: Converted as plain text

### PDF Test Cases

Test with `CPP_ND_V3.0E.pdf` (245 pages):

| Page | Feature | Expected Result |
|------|---------|-----------------|
| 1 | Date | "06-December-2023" (no extra space) |
| 2 | Text | "academia." not a header |
| 3 | Reference table | [CC1], [CC2], etc. with \| separators |
| 3-5 | Revision History | Single merged table, 13 rows, one header |

---

## DOCX Conversion

### Completed Work

1. **Core Parser** (`docx2md/docx/parser.go`)
   - ZIP archive extraction
   - XML parsing with namespace handling

2. **Content Extraction** (`docx2md/docx/extractor.go`)
   - Paragraph processing
   - Text run extraction with formatting
   - Table cell parsing
   - Hyperlink resolution

3. **Style Support** (`docx2md/docx/styles.go`)
   - Heading style detection (Heading 1-6, Title, Subtitle)
   - Bold/italic formatting from styles
   - Numbering definitions for lists

4. **Text Run Merging**
   - Per OOXML spec, runs concatenate without implicit spaces
   - Fixes fragmented text from Word editing

5. **Test Suite** (`docx2md/converter_test.go`)
   - Simple paragraph test
   - Bold/italic formatting tests
   - Heading detection test
   - Table conversion test
   - Invalid file handling

### Known Limitations

1. **Nested Tables**: May not render perfectly
2. **Advanced Formatting**: Text boxes, shapes ignored
3. **Track Changes**: Comments and revisions not included
4. **Headers/Footers**: Document headers/footers not extracted

### Future Improvements

- [x] Image extraction to separate files (implemented)
- [ ] Footnote/endnote support
- [ ] Header/footer extraction option
- [ ] Better nested list handling
- [ ] Support for document sections

### DOCX Test Cases

Test with sample files in `private/`:

| File | Features to Verify |
|------|-------------------|
| Pentest Report | Tables, headings, lists, text merging |
| Services Agreement | Nested lists, hyperlinks, bold/italic |
| MegaSuite Access | Multiple hyperlinks, formatting |

---

## Quality Assurance Checklist

When making changes, verify:
- [ ] `go build ./...` succeeds
- [ ] `go vet ./...` reports no issues
- [ ] `go test ./...` passes
- [ ] PDF conversion works correctly
- [ ] DOCX conversion works correctly
- [ ] Tables render with proper `|` separators
- [ ] Formatting (bold/italic) preserved
- [ ] Hyperlinks converted correctly

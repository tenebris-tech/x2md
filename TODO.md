# TODO.md

This file tracks pending improvements and known issues for x2md.

## Current Status

### PDF Conversion - Stable
All major features implemented and tested.

### DOCX Conversion - Stable
Core functionality implemented with footnote/endnote support.

### Image Extraction - Implemented
Both PDF and DOCX support image extraction:
- Images saved to `{output}_images/` subdirectory
- Markdown image links generated: `![alt](path)`
- JPEG images extracted directly
- PNG wrapping for raw pixel data
- Use `--no-images` flag to disable

---

## PDF Conversion

### Completed Work

#### Features
1. **Text Extraction** - Full text extraction from PDF content streams
2. **Heading Detection** - H1-H6 based on font size differences
3. **List Detection** - Bullet and numbered lists
4. **Table Support** - Multiple detection strategies:
   - Traditional header-based tables
   - Reference-style tables ([CC1], [CC2])
   - Cross-page table merging with header deduplication
5. **Formatting** - Bold/italic text detection
6. **Image Extraction** - JPEG and PNG images
7. **Footnotes** - Superscript and parenthesized footnote detection
8. **Nested Lists** - Proper indentation for nested lists

#### Issues Fixed
1. **Date Spacing** - Fixed extra space in "06-December -2023"
2. **False Header Detection** - Skip punctuation-ending lines
3. **Reference Table Grouping** - [CC1], [CC2] rendering
4. **Revision History Table** - Full cross-page table support

#### Code Quality
- Extracted magic numbers to constants
- Made known table headers configurable
- Added `Copy()` method to LineItem
- Added documentation to complex functions
- Refactored deeply nested code with helper methods
- Added validation for empty table rows
- Optimized string building performance

### Known Limitations

1. **Scanned PDFs**: Only text-based PDFs supported
2. **Complex Layouts**: Overlapping text regions may not convert perfectly
3. **Font Encodings**: Some non-standard fonts may have character issues
4. **Math Formulas**: Converted as plain text
5. **LZW Compression**: Not implemented (gracefully skipped)
6. **Header Over-detection**: Simple PDFs with limited font variation may have
   all lines detected as headers (see basic-text.pdf)

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

5. **Footnotes/Endnotes**
   - Footnote and endnote extraction
   - Proper markdown footnote syntax

6. **Image Extraction**
   - Images extracted to `_images/` subdirectory
   - Format detection from magic bytes

### Known Limitations

1. **Nested Tables**: May not render perfectly
2. **Advanced Formatting**: Text boxes, shapes ignored
3. **Track Changes**: Comments and revisions not included
4. **Headers/Footers**: Document headers/footers not extracted

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
- [ ] Image extraction works

---

## Next Steps for Robustness

### High Priority
1. **Header Detection Improvement** - Fix over-detection on simple PDFs
2. **Add Integration Tests** - Automated tests with real PDF/DOCX files
3. **Error Recovery** - Better handling of malformed documents

### Medium Priority
4. **PDF List Nesting** - Improve indentation detection
5. **Table Column Alignment** - Better heuristics for column detection
6. **DOCX Headers/Footers** - Optional extraction

### Low Priority
7. **LZW Compression** - Implement for image extraction
8. **OCR Integration** - Optional OCR for scanned PDFs
9. **Performance Optimization** - Large document handling

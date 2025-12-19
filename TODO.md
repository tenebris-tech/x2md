# TODO.md

This file tracks pending improvements and known issues for x2md.

## Current Status

### PDF Conversion - Production Ready
All major features implemented and tested with 10+ real-world PDFs.
Parser handles linearized PDFs, object streams, and encrypted file detection.

### DOCX Conversion - Production Ready
Core functionality implemented with footnote/endnote support.
Tested with 7 real-world DOCX files.

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

1. **Scanned PDFs**: Only text-based PDFs supported (OCR not available)
2. **Complex Layouts**: Overlapping text regions may not convert perfectly
3. **Font Encodings**: Some non-standard fonts may have character issues
4. **Math Formulas**: Converted as plain text
5. **LZW Compression**: Not implemented (gracefully skipped)
6. **Encrypted PDFs**: Not supported (graceful error message)

### Issues Discovered During Testing (2025-12-19)

1. **Simple Table Detection** (basic-text.pdf)
   - Tables without explicit borders not detected
   - Headers and rows combined into single lines
   - Affects: Simple PDFs with space-aligned columns

2. **Simple List Detection** (basic-text.pdf)
   - Unordered lists without explicit bullets not detected
   - Items like "Item 1", "Item 2" lack `-` prefix in output

3. **Bilingual PDF Layout** (SOR-2018-64.pdf)
   - English/French side-by-side text interleaved in output
   - Some hyphenated words broken across lines
   - This is inherent to complex multi-column layouts

### Test Results (2025-12-19)

**PDF Files Tested: 10**
| File | Status | Notes |
|------|--------|-------|
| basic-text.pdf | ✓ | Headers, lists work; simple tables need improvement |
| CPP_ND_V3.0E.pdf | ✓ | 245 pages, tables, footnotes all work |
| footnotes.pdf | ✓ | Footnotes detected correctly |
| 2021C25A.pdf | ✓ | Converted successfully |
| Common Sense Guide...pdf | ✓ | Complex layout, TOC as table |
| omNovos_Nessus...pdf | ✓ | Security report format |
| SOR-2018-64.pdf | ✓ | Bilingual layout challenges |
| The-Law-of-Expert...pdf | ✓ | Scanned PDF - images only (expected) |
| AFCEA IT Security...pdf | ✓ | Large file (152MB) - handles well |
| itsg33-ann3a-eng.pdf | ✗ | Encrypted - correctly rejected |

**DOCX Files Tested: 7**
| File | Status | Notes |
|------|--------|-------|
| 20080401 Pager Return.docx | ✓ | Simple letter format |
| 20110912 Manulife...docx | ✓ | Letter with numbered list |
| 20240626 Leasecake DR...docx | ✓ | Table format |
| 20240821 SOC Gap Letter.docx | ✓ | Letter with formatting |
| 20250930 Leasecake Policies...docx | ✓ | Multi-level headers, lists |
| 4_CSSP Project...docx | ✓ | Long report with footnotes |
| Cylance Install...docx | ✓ | Instructions with hyperlinks |

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

### Completed (2025-12-18/19)
1. ~~**Header Detection Improvement**~~ - Fixed (commit `dba1229`)
2. ~~**PDF Parser Robustness**~~ - Fixed (commit `2f1b606`):
   - Use last startxref (linearized PDF support)
   - PNG predictor support for xref streams
   - Object stream support (Type 2 xref entries)
   - Prev pointer following for incremental updates
   - Encryption detection with clear error message
3. ~~**Comprehensive Testing**~~ - Tested with 10 PDFs, 7 DOCX files
4. ~~**DOCX Paragraph Separation**~~ - Fixed (commit `0b7f0c8`):
   - Each paragraph now its own block in gather_blocks.go
   - Proper `\n\n` after paragraphs in to_markdown.go
5. ~~**DOCX XML Parsing Data Corruption**~~ - Fixed (commit `e469989`):
   - Critical bug: xml.Decoder.Token() CharData must be copied
   - Was causing footnotes to contain corrupted/random text
   - Fixed by copying CharData, Comment, ProcInst, Directive tokens

### High Priority
4. **Add Integration Tests** - Automated tests with real PDF/DOCX files
5. **Error Recovery** - Better handling of malformed documents

### Medium Priority
6. **PDF List Nesting** - Improve indentation detection
7. **Table Column Alignment** - Better heuristics for column detection
8. **DOCX Headers/Footers** - Optional extraction

### Low Priority
9. **LZW Compression** - Implement for image extraction
10. **OCR Integration** - Optional OCR for scanned PDFs
11. **Performance Optimization** - Large document handling

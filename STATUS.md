# x2md Project Status

**Last Updated:** 2025-12-18
**Branch:** `alpha`
**Status:** Pre-release - Production Ready

---

## For Claude: How to Resume This Project

When starting a new session, tell Claude:

> "Read STATUS.md, CLAUDE.md, and TODO.md in this repo. The project is x2md,
> a PDF/DOCX to Markdown converter. Continue with the next priority task."

Key context:
- All 71 tests pass, build is clean
- PDF parser is production-ready (tested with 16+ real documents)
- Branch `alpha` contains all current work
- Test files: 10 PDFs and 7 DOCX in `private/`

---

## Quick Start for New Session

```bash
# Build and verify
go build && go vet ./... && go test ./...

# Test conversions
./x2md private/CPP_ND_V3.0E.pdf   # Complex PDF (245 pages)
./x2md private/footnotes.pdf       # Footnotes test
./x2md private/basic-text.pdf      # Simple PDF (has known issues)

# View outputs
ls private/*.md private/*_images/
```

---

## Project Overview

x2md is a pure Go utility for converting PDF and DOCX files to Markdown. No CGO dependencies.

### Key Files
| File | Purpose |
|------|---------|
| `main.go` | CLI entry point, format detection |
| `pdf2md/converter.go` | PDF conversion API |
| `docx2md/converter.go` | DOCX conversion API |
| `pdf2md/models/` | Shared data structures |
| `CLAUDE.md` | Project architecture guide |
| `TODO.md` | Feature tracking and known issues |
| `NOTES.md` | Development notes |
| `test-plan.md` | Manual test plan for images |

---

## Build & Test Status (2025-12-17)

| Check | Status |
|-------|--------|
| `go build` | PASS |
| `go vet ./...` | PASS |
| `go test ./...` | PASS (71 tests) |
| PDF conversion | WORKING |
| DOCX conversion | WORKING |
| Image extraction | WORKING |

---

## Features Implemented

### PDF Conversion (`pdf2md/`)

| Feature | Status | Notes |
|---------|--------|-------|
| Text extraction | DONE | From content streams |
| Heading detection (H1-H6) | DONE | By font size |
| List detection | DONE | Bullets and numbers |
| Nested lists | DONE | Proper indentation |
| Table detection | DONE | Multiple strategies |
| Cross-page tables | DONE | Header deduplication |
| Bold/italic | DONE | From font info |
| Image extraction | DONE | JPEG and PNG |
| Footnotes | DONE | Superscript and parenthesized |
| Header/footer removal | DONE | Optional |
| TOC detection | DONE | |
| Blank page removal | DONE | Optional |
| Scan mode | DONE | Default on; extracts scanned pages as images |

### DOCX Conversion (`docx2md/`)

| Feature | Status | Notes |
|---------|--------|-------|
| Text extraction | DONE | From document.xml |
| Heading detection | DONE | From Word styles |
| List detection | DONE | Bullets, numbers, letters, roman |
| Nested lists | DONE | Proper indentation |
| Table extraction | DONE | |
| Bold/italic | DONE | From styles |
| Hyperlinks | DONE | Markdown syntax |
| Image extraction | DONE | From media/ |
| Footnotes/endnotes | DONE | |
| Text run merging | DONE | Per OOXML spec |

---

## Known Issues

### Critical
None

### High Priority
None

### Medium Priority
1. **DOCX Nested Tables**: May not render perfectly in all cases.
2. **Encrypted PDFs**: Not supported (graceful error message)

### Low Priority
3. **Scanned PDFs**: OCR not supported (but scan mode extracts pages as images)
4. **Math formulas**: Converted as plain text

---

## Test Files

Located in `private/`:

| File | Size | Purpose |
|------|------|---------|
| `CPP_ND_V3.0E.pdf` | 3.7 MB | Complex document (245 pages), tables, images |
| `footnotes.pdf` | 6.5 KB | Footnote testing |
| `basic-text.pdf` | 74 KB | Simple document (has header detection issues) |

Generated outputs:
- `private/*.md` - Converted markdown
- `private/CPP_ND_V3.0E_images/` - Extracted images (15+ files)

---

## Architecture Summary

### PDF Pipeline
```
PDF File
  → parser.go (xref, objects, streams)
  → extractor.go (text items with position)
  → CalculateGlobalStats (font sizes, distances)
  → CompactLines (group by Y, detect tables)
  → RemoveRepetitiveElements (headers/footers)
  → DetectTOC
  → DetectHeaders (by font size)
  → DetectListItems
  → GatherBlocks
  → RemoveBlankPages
  → ToTextBlocks
  → ToMarkdown
→ Markdown Output
```

### DOCX Pipeline
```
DOCX File (ZIP)
  → parser.go (extract XML)
  → styles.go (parse styles.xml)
  → numbering (parse numbering.xml)
  → extractor.go (document.xml → LineItems)
  → GatherBlocks
  → ToTextBlocks
  → ToMarkdown
→ Markdown Output
```

---

## Code Statistics

| Package | Files | Lines | Tests |
|---------|-------|-------|-------|
| pdf2md/pdf/ | 3 | ~2,663 | 0 |
| pdf2md/models/ | 5 | ~629 | 35 |
| pdf2md/transform/ | 11 | ~2,463 | 29 |
| docx2md/docx/ | 5 | ~1,533 | 0 |
| docx2md/transform/ | 4 | ~263 | 0 |
| docx2md/ | 2 | ~479 | 7 |
| imageutil/ | 1 | ~50 | 0 |
| main.go | 1 | 188 | 0 |
| **Total** | **32** | **~9,268** | **71** |

---

## Next Steps for 100% Robustness

### Completed (2025-12-18)
1. [x] **Fix Header Over-detection**: Added MinHeightRatio (1.5x) and MinHeightDifference (4pt) thresholds
2. [x] **PDF Parser Robustness**: Major improvements for real-world PDFs:
   - Use last startxref (linearized PDF support)
   - PNG predictor support for xref streams (predictors 10-15)
   - Object stream support (Type 2 xref entries)
   - Prev pointer following for incremental updates
   - Encryption detection with clear error message
3. [x] **Comprehensive Testing**: Tested with 10 PDFs, 7 DOCX files (16+ documents)

### Before Release
4. [ ] **Add Integration Tests**: Automated tests with real files
5. [ ] **Error Recovery**: Better handling of malformed DOCX files

### Before v1.0
6. [x] **PDF List Nesting**: Better indentation detection from X position
7. [x] **Table Column Heuristics**: Improve column boundary detection
8. [x] **DOCX Headers/Footers**: Optional extraction with flag
9. [ ] **Performance Testing**: Large document handling (1000+ pages)

### Future
10. [x] **LZW Compression**: For PDF image extraction
11. [ ] **OCR Integration**: Optional for scanned PDFs
12. [ ] **RTF Support**: New format

---

## Git History (Recent)

```
2f1b606 Improve PDF parser robustness for commercial use
e0a1ee2 Update docs: mark header over-detection as fixed
dba1229 Fix header over-detection in simple PDFs
f8b50a1 Add PROMPT.md for session resumption, update STATUS.md
02f3bdd Add Claude resumption instructions to STATUS.md
e186c13 Add STATUS.md, update documentation, enhance footnote detection
```

---

## CLI Reference

```bash
# Basic usage
./x2md input.pdf                    # Output: input.md
./x2md input.docx                   # Output: input.md
./x2md -output out.md input.pdf     # Custom output

# PDF-specific options
./x2md -strip-headers input.pdf     # Remove headers/footers
./x2md -strip-page-numbers input.pdf
./x2md -strip-toc input.pdf
./x2md -strip-footnotes input.pdf
./x2md -strip-blank-pages input.pdf
./x2md -no-lists input.pdf
./x2md -no-headings input.pdf
./x2md -no-scan-mode input.pdf      # Disable scanned page detection

# Common options
./x2md -no-formatting input.pdf     # No bold/italic
./x2md -no-images input.pdf         # Skip image extraction
./x2md -v input.pdf                 # Verbose mode
```

---

## Resuming Work

1. Read this STATUS.md
2. Read CLAUDE.md for architecture details
3. Read TODO.md for pending items
4. Run tests: `go test ./...`
5. Check uncommitted changes: `git diff`
6. Continue with next priority item

---

## Contact

Repository: github.com/tenebris-tech/x2md
License: Proprietary (contact for licensing)

# x2md Project Status

**Last Updated:** 2025-12-17
**Branch:** `nested-lists`
**Status:** Pre-release - Feature Complete

---

## For Claude: How to Resume This Project

When starting a new session, tell Claude:

> "Read STATUS.md, CLAUDE.md, and TODO.md in this repo. The project is x2md,
> a PDF/DOCX to Markdown converter. Continue with the next priority task."

Key context:
- All 71 tests pass, build is clean
- Main issue: header over-detection on simple PDFs (see Next Steps section)
- The `nested-lists` branch has all current work
- No DOCX test files exist in `private/` - only PDFs

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
1. **Header Over-detection (PDF)**: Simple PDFs with limited font variation
   (e.g., `basic-text.pdf`) may have all lines incorrectly detected as headers.
   The algorithm assumes font size variation indicates headings, which fails for
   documents with uniform formatting.

### Medium Priority
2. **LZW Compression**: Not implemented for PDF image extraction. Gracefully
   skipped with warning.
3. **DOCX Nested Tables**: May not render perfectly in all cases.

### Low Priority
4. **Scanned PDFs**: OCR not supported
5. **Math formulas**: Converted as plain text
6. **DOCX Headers/Footers**: Not extracted

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

### Immediate (Before Release)
1. [ ] **Fix Header Over-detection**: Improve algorithm for simple PDFs
   - **Problem**: `basic-text.pdf` outputs all lines as `## ` (H2)
   - **Root cause**: `pdf2md/transform/detect_headers.go` uses font height
     comparison to detect headings. When a PDF has minimal font variation,
     the algorithm incorrectly promotes most lines.
   - **Suggested fix**: Add minimum height difference threshold, or detect
     when document lacks clear heading hierarchy and disable detection.
   - **Test command**: `./x2md private/basic-text.pdf && head -30 private/basic-text.md`
   - **Expected**: Normal paragraphs, not all H2 headers

2. [ ] **Add Integration Tests**: Create automated tests with real files
   - PDF table detection verification
   - DOCX style detection verification
   - Image extraction verification

3. [ ] **Error Recovery**: Better handling of:
   - Malformed PDFs (corrupted xref, missing objects)
   - Malformed DOCX (missing XML files)

### Before v1.0
4. [ ] **PDF List Nesting**: Better indentation detection from X position
5. [ ] **Table Column Heuristics**: Improve column boundary detection
6. [ ] **DOCX Headers/Footers**: Optional extraction with flag
7. [ ] **Performance Testing**: Large document handling (1000+ pages)

### Future
8. [ ] **LZW Compression**: For PDF image extraction
9. [ ] **OCR Integration**: Optional for scanned PDFs
10. [ ] **RTF Support**: New format

---

## Git History (Recent)

```
02f3bdd Add Claude resumption instructions to STATUS.md
e186c13 Add STATUS.md, update documentation, enhance footnote detection
9649d07 Add footnote/endnote support for DOCX
0c02e99 Add nested list support for DOCX and PDF
d6aa240 Add image extraction test plan
c6e4912 Add image extraction for PDF and DOCX files
```

## Open Pull Request

**PR #1**: `nested-lists` → `alpha`
- URL: https://github.com/tenebris-tech/x2md/pull/1
- Status: Open (ready for review/merge)

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

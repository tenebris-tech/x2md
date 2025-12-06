# TODO.md

This file tracks pending improvements and known issues for x2md.

## Completed Work (Latest Session)

### Issues Fixed
All 4 issues from the original `issues1.txt` have been resolved:

1. **Date Spacing** - Fixed extra space in "06-December -2023"
   - Modified `needsSpaceBetween()` in `transform/compact_lines.go`
   - Checks for hyphens before considering gap size

2. **False Header Detection** - Fixed "academia." being detected as H4
   - Modified `transform/detect_headers.go`
   - Skips lines ending with punctuation (., ,, etc.)

3. **Reference Table Grouping** - Fixed [CC1], [CC2] table rendering
   - Added `detectReferenceTable()` in `transform/compact_lines.go`
   - Detects bracketed IDs and groups with descriptions

4. **Revision History Table** - Full table support implemented
   - Added table detection, rendering, header deduplication
   - Tables now render with `| col1 | col2 |` markdown syntax
   - Cross-page tables merged into single continuous table

### Files Modified
- `pdf2md/models/models.go` - Added IsTableRow, IsTableHeader, TableColumns to LineItem
- `pdf2md/models/blocktypes.go` - Added table rendering in LinesToText()
- `pdf2md/transform/compact_lines.go` - Table detection and column extraction
- `pdf2md/transform/gather_blocks.go` - Keep table rows together in blocks
- `pdf2md/transform/remove_blank_pages.go` - Count lines within blocks
- `pdf2md/transform/to_markdown.go` - Header deduplication, cross-page merging
- `pdf2md/transform/detect_headers.go` - Skip punctuation-ending lines

## High Priority Improvements

All high priority improvements have been completed:

1. ✅ **Extract Magic Numbers to Constants** - Added named constants at top of `pdf2md/transform/compact_lines.go`
2. ✅ **Make Known Table Headers Configurable** - Moved to `KnownTableHeaders` package-level variable
3. ✅ **Add Copy() Method to LineItem** - Added `Copy()` method and updated `AddItem()` to use it

## Medium Priority Improvements

All medium priority improvements have been completed:

4. ✅ **Add Documentation to Complex Functions** - Added godoc comments to `detectTableRegions()`, `groupAsTableWithMetadata()`, `detectMultiLineCells()`, `extractColumnTexts()`, and `groupAsTable()`
5. ✅ **Refactor Deeply Nested Code** - Extracted helper methods: `visualRow` struct with `addItemToRow()`, `isInYRange()`, plus `checkColumnOverlap()`, `tryAddItemToRow()`, and `sortRowItems()`
6. ✅ **Add Validation for Empty Table Rows** - Added check in `LinesToText()` to skip table rows where all columns are whitespace-only
7. ✅ **Performance: String Building Optimization** - Added `endsWithSpace` tracking variable to avoid repeated `text.String()` calls in `combineText()`

## Accuracy Improvements

### 8. Add Comprehensive Test Suite
Currently no test files exist. Priority test cases:
- Table detection with various column counts
- Cross-page table merging
- Header deduplication
- Date spacing edge cases
- False header detection prevention

### 9. Add Debug Logging Option
Add optional debug output for table detection decisions to help troubleshoot issues with new PDFs.

## Known Limitations

1. ✅ **Footer Threshold**: Now calculated dynamically as 88% of page height (was hardcoded Y=700).

2. **Known Headers List**: Uses `KnownTableHeaders` package variable. New document types may need additions.

3. **Two-Column Reference Tables**: Reference table detection uses exactly 2 columns (ID + description) intentionally to prevent date fragments from being split into separate columns. This is appropriate for most reference tables.

4. **Glossary/Definition Tables**: Tables with term-definition format may have content incorrectly split across columns if the layout uses complex multi-line cells.

## Quality Assurance Checklist

When making changes, verify:
- [ ] `go build ./...` succeeds
- [ ] `go vet ./...` reports no issues
- [ ] Test PDF converts without errors
- [ ] Tables render with proper `|` separators
- [ ] Cross-page tables merge correctly
- [ ] No false header detection on sentence fragments
- [ ] Date formatting preserved (no extra spaces)

## Reference Test Cases

Test with `CPP_ND_V3.0E.pdf` (245 pages):

| Page | Feature | Expected Result |
|------|---------|-----------------|
| 1 | Date | "06-December-2023" (no extra space) |
| 2 | Text | "academia." not a header |
| 3 | Reference table | [CC1], [CC2], etc. with \| separators |
| 3-5 | Revision History | Single merged table, 13 rows, one header |
| 5 | Last row | "0.1 \| 05-Sep-2014 \| Draft published..." |

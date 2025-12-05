# TODO.md

This file tracks pending improvements and known issues for the pdf2mdgo project.

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
- `models/models.go` - Added IsTableRow, IsTableHeader, TableColumns to LineItem
- `models/blocktypes.go` - Added table rendering in LinesToText()
- `transform/compact_lines.go` - Table detection and column extraction
- `transform/gather_blocks.go` - Keep table rows together in blocks
- `transform/remove_blank_pages.go` - Count lines within blocks
- `transform/to_markdown.go` - Header deduplication, cross-page merging
- `transform/detect_headers.go` - Skip punctuation-ending lines

## High Priority Improvements

### 1. Extract Magic Numbers to Constants
**Location**: `transform/compact_lines.go`

Many magic numbers should be named constants for readability:
```go
// Current (scattered throughout code)
if math.Abs(item.Y-minY) < 5
if item.Y >= headerY && item.Y < 700
if item.X-lastX >= 40

// Recommended
const (
    yPositionTolerance       = 5.0
    maxPageContentY          = 700.0
    minColumnSpacing         = 40.0
    columnAlignmentTolerance = 20.0
    columnBucketSize         = 15.0
    referenceColumnMinGap    = 30.0
)
```

**Affected lines**: 87, 232, 300, 342, 376, 419, 448, 518, 564

### 2. Make Known Table Headers Configurable
**Location**: `transform/compact_lines.go:309-313`

Currently hardcoded:
```go
knownHeaders := []string{
    "Version Date Description",
    "Element Evaluator Actions",
    "Requirement Dependency",
}
```

Should be moved to package-level variable or configuration for extensibility.

### 3. Add Copy() Method to LineItem
**Location**: `models/models.go`

The `AddItem()` function manually copies each field. If new fields are added, developers might forget to update the copy logic.

```go
func (l *LineItem) Copy() *LineItem {
    if l == nil {
        return nil
    }
    return &LineItem{
        X:              l.X,
        Y:              l.Y,
        Width:          l.Width,
        Height:         l.Height,
        Words:          l.Words,
        Type:           l.Type,
        Annotation:     l.Annotation,
        ParsedElements: l.ParsedElements,
        Font:           l.Font,
        IsTableRow:     l.IsTableRow,
        IsTableHeader:  l.IsTableHeader,
        TableColumns:   append([]string{}, l.TableColumns...),
    }
}
```

## Medium Priority Improvements

### 4. Add Documentation to Complex Functions
**Location**: `transform/compact_lines.go`

Functions needing godoc comments:
- `detectTableRegions()` - Explain the detection algorithms
- `groupAsTableWithMetadata()` - Explain row grouping logic
- `detectMultiLineCells()` - Explain multi-line cell detection
- `extractColumnTexts()` - Explain column text extraction

### 5. Refactor Deeply Nested Code
**Location**: `transform/compact_lines.go:732-838`

The `groupAsTable()` function has 4-5 levels of nesting. Extract helper functions:
```go
func (c *CompactLines) shouldAddToRow(item *models.TextItem, row *visualRow, columns []float64, rowThreshold float64) bool
```

### 6. Add Validation for Empty Table Rows
**Location**: `models/blocktypes.go:146-160`

Check if table rows have any non-empty content before rendering:
```go
if line.IsTableRow && len(line.TableColumns) > 0 {
    hasContent := false
    for _, col := range line.TableColumns {
        if strings.TrimSpace(col) != "" {
            hasContent = true
            break
        }
    }
    if !hasContent {
        continue // Skip empty table rows
    }
    // ... render table
}
```

### 7. Performance: String Building Optimization
**Location**: `transform/compact_lines.go:989-1041`

The `combineText()` function calls `text.String()` repeatedly, creating temporary copies. Cache the string when multiple checks are needed.

## Low Priority / Future Work

### 8. Add Comprehensive Test Suite
Currently no test files exist. Priority test cases:
- Table detection with various column counts
- Cross-page table merging
- Header deduplication
- Date spacing edge cases
- False header detection prevention

### 9. Add Debug Logging Option
Add optional debug output for table detection decisions to help troubleshoot issues with new PDFs.

### 10. Consider sync.Pool for High-Frequency Usage
If profiling shows GC pressure from temporary slices/maps in transformation pipeline, consider object pooling.

## Known Limitations

1. **Y=700 Footer Threshold**: Hardcoded value assumes standard page size. May need adjustment for non-standard PDFs.

2. **Known Headers List**: Only recognizes specific table headers for continuation detection. New document types may need additions.

3. **Two-Column Reference Tables**: Reference table detection assumes exactly 2 columns (ID + description).

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

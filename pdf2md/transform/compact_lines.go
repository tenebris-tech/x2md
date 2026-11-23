/******************************************************************************
 * Copyright (c) 2025-2026 Tenebris Technologies Inc.                         *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package transform

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/tenebris-tech/x2md/pdf2md/models"
)

// Constants for table detection and text layout analysis
const (
	// maxPageContentY is the maximum Y value for page content (footer threshold)
	maxPageContentY = 700.0

	// minColumnSpacing is the minimum horizontal gap between table columns
	minColumnSpacing = 40.0

	// columnAlignmentTolerance is the tolerance for aligning items to columns
	columnAlignmentTolerance = 20.0

	// referenceColumnMinGap is the minimum gap between reference ID and description
	referenceColumnMinGap = 30.0

	// maxHeaderItemLength is the maximum character length for header items
	maxHeaderItemLength = 30

	// largeGapThreshold is the gap size that definitely needs a space
	// (only used when characters are not both alphanumeric)
	largeGapThreshold = 30.0

	// minSpaceGapThreshold is the minimum gap that might need a space
	// (only used for non-alphanumeric boundaries)
	minSpaceGapThreshold = 10.0

	// referenceIDMinLen is the minimum length for a reference ID like [CC1]
	referenceIDMinLen = 3

	// referenceIDMaxLen is the maximum length for a reference ID
	referenceIDMaxLen = 10

	// minReferenceItems is the minimum reference items to detect a table
	minReferenceItems = 3

	// referenceAlignmentThreshold is the percentage of items that must align
	referenceAlignmentThreshold = 0.8

	// multiColumnMinGutter is the narrowest gap, in text heights, between the
	// two columns of a 2-column page; text on a line closer together than
	// that runs across the middle of the page
	multiColumnMinGutter = 1.0

	// ruledRowCoverage is the share of a table's width a horizontal rule must
	// run across to separate its rows
	ruledRowCoverage = 0.8

	// boundBlockLines is the number of lines a table is first bounded in,
	// and boundBlockMargin how many lines before the end of a block the table
	// must end for the block to be enough (see boundBelow)
	boundBlockLines  = 64
	boundBlockMargin = 8

	// exactAlignment is how far apart, in page units, the starts of cells
	// set at one tab stop or column edge are: the same position but for
	// rounding
	exactAlignment = 2.0

	// justifiedNeighbourLines is how many lines above or below a run of
	// two-column rows are looked at for the running text of a justified
	// paragraph around them
	justifiedNeighbourLines = 3

	// minAlignedRows is the fewest rows that align to the same columns for
	// text to be a table on alignment alone
	minAlignedRows = 3

	// maxTableRowGap is the number of consecutive non-tabular rows tolerated inside a
	// table region before the region is considered ended. Allows single-column wrapped
	// cells while preventing prose below a table from being absorbed.
	maxTableRowGap = 1

	// columnConsistencyThreshold is the minimum fraction of a table's data rows that must
	// place an item in a column for that column to count as a real, consistent column.
	// Justified prose only keeps the left margin consistent, so it fails the >=2 requirement.
	columnConsistencyThreshold = 0.5

	// minTableDataRows is the minimum number of aligned data rows required below a header
	// for a Method 1 table to be accepted.
	minTableDataRows = 2

	// rightMarginTolerance is how close to the right text edge a consistent column may sit
	// before it is treated as the right margin (where justified line-ending fragments cluster)
	// rather than a genuine interior table column.
	rightMarginTolerance = 40.0
)

// KnownTableHeaders contains patterns for recognizing table headers
// that span multiple pages. Can be extended for new document types.
var KnownTableHeaders = []string{
	"Version Date Description",
	"Element Evaluator Actions",
	"Requirement Dependency",
}

// CompactLines groups text items on the same Y into lines
type CompactLines struct{}

// NewCompactLines creates a new CompactLines transformation
func NewCompactLines() *CompactLines {
	return &CompactLines{}
}

// lineGroup holds text items for a line along with table metadata
type lineGroup struct {
	items      []*models.TextItem
	isTableRow bool
	isHeader   bool
	columns    []float64    // column X positions for table rows
	extents    [][2]float64 // where each column's text lies, when known
	cells      []string     // text of each column, when known (ruled tables)
}

// Transform groups text items into lines
func (c *CompactLines) Transform(result *models.ParseResult) *models.ParseResult {
	mostUsedDistance := result.Globals.MostUsedDistance
	fontToFormats := result.Globals.FontToFormats

	for _, page := range result.Pages {
		if len(page.Items) == 0 {
			continue
		}

		// Calculate footer threshold based on page height
		// Footer typically appears in the bottom 12% of the page
		footerThreshold := page.Height * 0.88
		if footerThreshold == 0 {
			// Fallback for pages without dimensions
			footerThreshold = maxPageContentY
		}

		// Detect table regions and group accordingly
		groupedLines := c.groupByLineWithTableDetection(visibleItems(page), page.Rules, mostUsedDistance, footerThreshold)

		// Convert grouped items to LineItems
		var lineItems []interface{}
		for _, group := range groupedLines {
			lineItem := c.compactLine(group.items, fontToFormats)
			if lineItem != nil {
				// Set table metadata
				if group.isTableRow {
					lineItem.IsTableRow = true
					lineItem.IsTableHeader = group.isHeader
					if group.cells != nil {
						lineItem.TableColumns = group.cells
					} else {
						lineItem.TableColumns = c.extractColumnTexts(group.items, group.columns, group.extents)
					}
				}
				lineItems = append(lineItems, lineItem)
			}
		}

		page.Items = lineItems
	}

	return result
}

// visibleItems returns the text items of a page that can be seen: their
// position and size are finite numbers, and they are on the page (within a
// page's width or height of its box, where the page size is known; off-page
// text is hidden). An item without a height gets the page's typical height,
// so that it groups into lines like the text around it.
func visibleItems(page *models.Page) []interface{} {
	var texts []*models.TextItem
	var result []interface{}
	for _, item := range page.Items {
		ti, ok := item.(*models.TextItem)
		if !ok {
			result = append(result, item)
			continue
		}
		if !finite(ti.X, ti.Y, ti.Width, ti.Height) {
			continue
		}
		if page.Width > 0 && page.Height > 0 &&
			(ti.X > 2*page.Width || ti.X+ti.Width < -page.Width || ti.Y > 2*page.Height || ti.Y < -page.Height) {
			continue
		}
		texts = append(texts, ti)
		result = append(result, ti)
	}
	typical := typicalHeight(texts)
	for _, ti := range texts {
		if ti.Height <= 0 {
			ti.Height = typical
		}
	}
	return result
}

// finite reports whether all values are finite numbers
func finite(values ...float64) bool {
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

// extractColumnTexts extracts and combines text content for each table column.
//
// Given a set of text items, the column X positions and (when known) the extent of each
// column, this function:
//  1. Assigns each line's runs of text to columns (see columnsOf); leader dots are dropped
//  2. Orders the items of each column by line, then X
//  3. Combines the text using combineText, which handles spacing and line wraps
//
// Returns a slice of strings where each element is the combined text for that column.
func (c *CompactLines) extractColumnTexts(items []*models.TextItem, columns []float64, extents [][2]float64) []string {
	if len(columns) == 0 {
		return nil
	}

	colOf := c.columnsOf(items, columns, extents)
	columnItems := make([][]*models.TextItem, len(columns))
	for _, item := range items {
		if col, ok := colOf[item]; ok && col < len(columnItems) {
			columnItems[col] = append(columnItems[col], item)
		}
	}

	// Combine text for each column
	result := make([]string, len(columns))
	for i, colItems := range columnItems {
		if len(colItems) > 0 {
			result[i] = trimLeader(c.combineText(readingOrder(colItems)))
		}
	}

	return result
}

// groupByLineWithTableDetection groups text items, detecting and handling tables.
// Tables drawn with rules come first; the text-based detection and the
// 2-column layout logic see only the text outside them.
func (c *CompactLines) groupByLineWithTableDetection(items []interface{}, rules []models.Rule, mostUsedDistance int, footerThreshold float64) []lineGroup {
	// Convert to TextItems and filter out invalid items
	var textItems []*models.TextItem
	for _, item := range items {
		if ti, ok := item.(*models.TextItem); ok {
			// Skip items with negative X coordinates (off-page content like hidden metadata)
			if ti.X < 0 {
				continue
			}
			// Skip items with excessive repetition (likely hidden metadata or watermarks)
			// Check if the text contains the same phrase repeated many times
			if isExcessivelyRepetitive(ti.Text) {
				continue
			}
			textItems = append(textItems, ti)
		}
	}

	if len(textItems) == 0 {
		return nil
	}

	tables, inTable := c.detectRuledTables(textItems, rules)
	if len(tables) > 0 {
		var rest []*models.TextItem
		for i, item := range textItems {
			if !inTable[i] {
				rest = append(rest, item)
			}
		}
		var lines []lineGroup
		splitX := 0.0
		if len(rest) > 0 {
			lines, splitX = c.groupTextLines(rest, rules, mostUsedDistance, footerThreshold)
		}
		return placeRuledTables(lines, tables, splitX)
	}

	lines, _ := c.groupTextLines(textItems, rules, mostUsedDistance, footerThreshold)
	return lines
}

// groupTextLines groups text items into lines and text-based tables. When
// the page is read as two columns, splitX is the left edge of the right
// column (0 otherwise).
func (c *CompactLines) groupTextLines(textItems []*models.TextItem, rules []models.Rule, mostUsedDistance int, footerThreshold float64) (lines []lineGroup, splitX float64) {
	// Identify table regions on the page
	tableRegions := c.detectTableRegions(textItems, mostUsedDistance, footerThreshold)

	if len(tableRegions) == 0 {
		// No tables detected - check for 2-column page layout
		leftItems, rightItems, isMultiColumn := c.detectAndSplitMultiColumnLayout(textItems)

		if isMultiColumn {
			// Process each column separately, then combine
			leftLines := visualLines(leftItems)
			rightLines := visualLines(rightItems)

			result := make([]lineGroup, 0, len(leftLines)+len(rightLines))
			for _, line := range leftLines {
				result = append(result, lineGroup{items: line, isTableRow: false})
			}
			for _, line := range rightLines {
				result = append(result, lineGroup{items: line, isTableRow: false})
			}
			splitX = rightItems[0].X
			for _, item := range rightItems {
				splitX = math.Min(splitX, item.X)
			}
			return result, splitX
		}

		// Standard Y-based grouping for single-column layouts
		lines := visualLines(textItems)
		result := make([]lineGroup, len(lines))
		for i, line := range lines {
			result[i] = lineGroup{items: line, isTableRow: false}
		}
		return result, 0
	}

	// Process items: table regions get table grouping, others get standard grouping
	var allLines []lineGroup

	// Sort items by Y for processing
	sortedItems := make([]*models.TextItem, len(textItems))
	copy(sortedItems, textItems)
	sort.Slice(sortedItems, func(i, j int) bool {
		return sortedItems[i].Y < sortedItems[j].Y
	})

	// Group items by whether they're in a table region or not
	var currentGroup []*models.TextItem
	var inTable bool
	var currentTableRegion *tableRegion

	for _, item := range sortedItems {
		region := c.findTableRegion(item.Y, tableRegions)
		itemInTable := region != nil

		if itemInTable != inTable || (itemInTable && region != currentTableRegion) {
			// Process previous group
			if len(currentGroup) > 0 {
				if inTable && currentTableRegion != nil {
					// Process as table
					tableLines := c.groupAsTableWithMetadata(currentGroup, currentTableRegion, rules)
					allLines = append(allLines, tableLines...)
				} else {
					// Process as regular text
					lines := visualLines(currentGroup)
					for _, line := range lines {
						allLines = append(allLines, lineGroup{items: line, isTableRow: false})
					}
				}
			}
			currentGroup = nil
			inTable = itemInTable
			currentTableRegion = region
		}
		currentGroup = append(currentGroup, item)
	}

	// Process final group
	if len(currentGroup) > 0 {
		if inTable && currentTableRegion != nil {
			tableLines := c.groupAsTableWithMetadata(currentGroup, currentTableRegion, rules)
			allLines = append(allLines, tableLines...)
		} else {
			lines := visualLines(currentGroup)
			for _, line := range lines {
				allLines = append(allLines, lineGroup{items: line, isTableRow: false})
			}
		}
	}

	// Sort all lines by Y
	sort.Slice(allLines, func(i, j int) bool {
		if len(allLines[i].items) == 0 || len(allLines[j].items) == 0 {
			return false
		}
		return allLines[i].items[0].Y < allLines[j].items[0].Y
	})

	return allLines, 0
}

// groupAsTableWithMetadata converts text items into table rows with associated metadata.
//
// It delegates to tableRows for the actual row grouping (or, where horizontal rules
// separate the rows, to the bands between the rules), then wraps each row
// in a lineGroup struct with table-specific metadata:
//   - isTableRow: always true for items in a table region
//   - isHeader: true only for the first row if region.hasHeader is set
//   - columns: the X positions defining column boundaries
//
// The first row is marked as header only if the table region was detected with
// a header row (e.g., via known header patterns or header-based detection).
func (c *CompactLines) groupAsTableWithMetadata(items []*models.TextItem, region *tableRegion, rules []models.Rule) []lineGroup {
	rows := c.tableRows(items, region.columns, region.extents)
	// Rules that cut the table into at least half as many rows as its text
	// lines up into are row separators; a rule under the header alone is not
	if ruled := c.rowsBetweenRules(items, region.columns, region.extents, rules); 2*len(ruled) >= len(rows) {
		rows = ruled
	}
	result := make([]lineGroup, len(rows))

	for i, row := range rows {
		isHeader := i == 0 && region.hasHeader
		result[i] = lineGroup{
			items:      row,
			isTableRow: true,
			isHeader:   isHeader,
			columns:    region.columns,
			extents:    region.extents,
		}
	}

	return result
}

// rowsBetweenRules groups the items of a table into the bands between the
// horizontal rules drawn across it, one row per band, or returns nil when no
// rule runs across the table between its lines. Where rows are ruled off,
// the lines between two rules are one row however they line up.
func (c *CompactLines) rowsBetweenRules(items []*models.TextItem, columns []float64, extents [][2]float64, rules []models.Rule) [][]*models.TextItem {
	if len(items) == 0 {
		return nil
	}
	left, right := items[0].X, items[0].X+items[0].Width
	top, bottom := math.Inf(1), math.Inf(-1)
	for _, item := range items {
		left, right = math.Min(left, item.X), math.Max(right, item.X+item.Width)
		h := textHeight(item)
		top, bottom = math.Min(top, item.Y-glyphAscent*h), math.Max(bottom, item.Y+glyphDescent*h)
	}

	// Rules across (nearly) the whole width of the table, inside it
	var breaks []float64
	for _, r := range rules {
		if r.Y1 != r.Y2 || r.Y1 <= top || r.Y1 >= bottom {
			continue
		}
		covered := math.Min(r.X2, right) - math.Max(r.X1, left)
		if covered >= ruledRowCoverage*(right-left) {
			breaks = append(breaks, r.Y1)
		}
	}
	if len(breaks) == 0 {
		return nil
	}
	sort.Float64s(breaks)

	bands := make([][]*models.TextItem, len(breaks)+1)
	for _, item := range items {
		_, middle := itemPoint(item)
		b := sort.SearchFloat64s(breaks, middle)
		bands[b] = append(bands[b], item)
	}

	var rows [][]*models.TextItem
	for _, band := range bands {
		if len(band) == 0 {
			continue
		}
		colOf := c.columnsOf(band, columns, extents)
		byCol := make([][]*models.TextItem, len(columns)+1)
		for _, item := range readingOrder(band) {
			if col, ok := colOf[item]; ok {
				byCol[col] = append(byCol[col], item)
			}
		}
		var row []*models.TextItem
		for _, cell := range byCol {
			row = append(row, cell...)
		}
		rows = append(rows, row)
	}
	return rows
}

// tableRegion represents a detected table area on the page
type tableRegion struct {
	minY, maxY float64
	columns    []float64
	extents    [][2]float64 // where each column's text lies, when known
	hasHeader  bool         // whether this table has a detected header row
}

// detectTableRegions identifies table regions on a page using multiple detection methods.
//
// Detection Methods:
//  1. Header-based detection: Looks for rows with well-spaced column headers
//     (e.g., "Version Date Description"). Validates by checking for aligned data rows.
//  2. Known header patterns: Matches against KnownTableHeaders for continuation tables
//     that span multiple pages.
//  3. Reference-style tables: Detects bracketed IDs like [CC1], [CC2] aligned in columns.
//
// The footerThreshold parameter specifies the Y coordinate below which content is
// considered footer (excluded from table detection). This is calculated dynamically
// based on page height rather than using a hardcoded value.
//
// Returns a slice of tableRegion structs containing Y boundaries, column X positions,
// and whether the region has a detected header row.
func (c *CompactLines) detectTableRegions(items []*models.TextItem, mostUsedDistance int, footerThreshold float64) []*tableRegion {
	var regions []*tableRegion

	// Method 1: Look for traditional header-based tables
	// (rows where multiple items start at distinct X positions like "Version Date Description")
	// findTableHeader only returns a header with genuine tabular structure beneath it
	// (see boundTableRegionByColumns). maxY is the end of the run of rows that align to
	// the header columns, so the region stops at the true end of the table instead of
	// extending to the page footer and absorbing the prose below it.
	for _, header := range c.findTableHeaders(items) {
		headerY, columns, maxY := header.y, header.columns, header.maxY
		// Find items that belong to this table (after header, within the bounded region)
		// Exclude items near bottom of page (likely footer) using dynamic threshold
		var tableItems []*models.TextItem
		for _, item := range items {
			if strings.TrimSpace(item.Text) == "" {
				continue
			}
			if item.Y >= headerY && item.Y <= maxY && item.Y < footerThreshold {
				tableItems = append(tableItems, item)
			}
		}

		// Validate items-to-columns ratio to avoid false positives on paragraph text
		if c.validateTableStructure(tableItems, columns) {
			// Accept as a table when a known continuation header pattern is present
			// (Method 1a, e.g. "Version Date Description"), the cells span multiple lines
			// (Method 1b, classic wrapped-cell table), or there are at least
			// minTableDataRows single-line rows under the header (Method 1c).
			knownHeader := len(tableItems) >= 3 && c.isKnownTableHeader(tableItems, columns)
			multiLine := len(tableItems) >= 6 && c.detectMultiLineCells(tableItems, columns, header.extents)
			if knownHeader || multiLine || header.rows >= minTableDataRows {
				regions = append(regions, &tableRegion{
					minY:      headerY,
					maxY:      maxY,
					columns:   columns,
					extents:   header.extents,
					hasHeader: true,
				})
			}
		}
	}

	// Method 2: Look for reference-style tables (bracketed IDs like [CC1], [CC2])
	refRegion := c.detectReferenceTable(items, mostUsedDistance)
	if refRegion != nil {
		// Check if it overlaps with existing regions
		overlaps := false
		for _, r := range regions {
			if refRegion.minY <= r.maxY && refRegion.maxY >= r.minY {
				overlaps = true
				break
			}
		}
		if !overlaps {
			regions = append(regions, refRegion)
		}
	}

	// Method 3: Look for space-aligned tables (consistent column positions across rows)
	if len(regions) == 0 {
		alignedRegions := c.detectAlignedTables(items, footerThreshold)
		regions = append(regions, alignedRegions...)
	}

	return regions
}

// boundTableRegionByColumns determines the true vertical extent of a header-based table and
// how many of its columns are consistently populated.
//
// below holds the text under the header. It is grouped into table rows (see tableRows),
// and each row's runs of text are matched to the columns: a run hits a column when it
// starts within columnAlignmentTolerance of the column's start, overlaps the column's
// header, or (past the first column) lies between the column's start and the next one's
// (right-aligned and centred cells). A row is part of the table when it hits at
// least two columns. A row with a run crossing a column boundary is running text (table
// cells keep to their columns) and ends the table. The scan extends maxY through table
// rows, tolerating up to maxTableRowGap consecutive rows that hit fewer columns, so prose below a table - or
// an entire prose page under a falsely detected header - is not pulled into the region.
//
// A row with text in interior columns only (an empty first cell) neither counts as a data
// row nor ends the table; the table extends over it only when a data row follows.
//
// Returns the Y of the lowest item in the table (maxY), the number of tabular data rows, the number of
// columns hit by at least columnConsistencyThreshold of those rows (consistentCols), how
// many of those consistent columns are "interior" - strictly right of the left margin and
// clear of the right text edge (interiorCols), and the Y of the row at which the scan stopped
// (stopY, 0 if it read every row).
//
// Justified prose keeps two columns consistent: the left margin and the right margin (where
// line-ending fragments and hyphens cluster). Neither is interior, so prose yields
// interiorCols == 0 and is rejected by the caller. Real tables have at least one genuine
// interior column.
func (c *CompactLines) boundTableRegionByColumns(below []*models.TextItem, headerY float64, columns []float64, extents [][2]float64, rightEdge float64) (maxY float64, dataRows int, consistentCols int, interiorCols int, stopY float64) {
	colHits := make([]int, len(columns))
	maxY = headerY
	gap := 0

	for _, row := range c.tableRows(below, columns, extents) {
		rowMaxY := 0.0
		for _, item := range row {
			rowMaxY = math.Max(rowMaxY, item.Y)
		}
		hit, crosses := c.rowHits(row, columns, extents)
		if crosses || c.isTitleRow(row, columns, extents) {
			// Text running across a column boundary is running text, not a
			// row, and a title over the next table is not a row: the table
			// ends above it
			stopY = rowTop(row)
			break
		}
		n := 0
		for _, h := range hit {
			if h {
				n++
			}
		}

		if n >= 2 {
			dataRows++
			for ci, h := range hit {
				if h {
					colHits[ci]++
				}
			}
			maxY = rowMaxY
			gap = 0
		} else if n == 1 && !hit[0] {
			// Interior cells only: inside the table if a data row follows
			continue
		} else {
			gap++
			if gap > maxTableRowGap {
				stopY = rowTop(row)
				break
			}
		}
	}

	if dataRows > 0 {
		leftMargin := columns[0] // columns are produced left-to-right
		for ci, h := range colHits {
			if float64(h)/float64(dataRows) < columnConsistencyThreshold {
				continue
			}
			consistentCols++
			if columns[ci] > leftMargin+columnAlignmentTolerance && columns[ci] < rightEdge-rightMarginTolerance {
				interiorCols++
			}
		}
	}

	return maxY, dataRows, consistentCols, interiorCols, stopY
}

// rowHits returns which columns a row's runs of text hit (see
// boundTableRegionByColumns), and whether any run crosses a column boundary
func (c *CompactLines) rowHits(row []*models.TextItem, columns []float64, extents [][2]float64) (hit []bool, crosses bool) {
	hit = make([]bool, len(columns))
	for _, line := range visualLines(row) {
		for _, r := range lineRuns(line) {
			if runCrosses(r, columns) {
				crosses = true
			}
			col := c.runColumn(r, columns, extents)
			inSlot := r.x >= columns[col]-columnAlignmentTolerance &&
				(col+1 == len(columns) || r.end <= columns[col+1]+columnAlignmentTolerance)
			if math.Abs(r.x-columns[col]) < columnAlignmentTolerance || (col > 0 && inSlot) ||
				(extents != nil && math.Min(r.end, extents[col][1]) > math.Max(r.x, extents[col][0])) {
				hit[col] = true
			}
		}
	}
	return hit, crosses
}

// isTitleRow reports whether a row is a title sitting alone between two
// groups of rows: one run of text on one line, in a column past the first,
// starting left of that column's start (centred over the table rather than
// set in the column, as a cell's text would be)
func (c *CompactLines) isTitleRow(row []*models.TextItem, columns []float64, extents [][2]float64) bool {
	lines := visualLines(row)
	if len(lines) != 1 {
		return false
	}
	runs := lineRuns(lines[0])
	if len(runs) != 1 {
		return false
	}
	col := c.runColumn(runs[0], columns, extents)
	return col > 0 && runs[0].x < columns[col]-exactAlignment
}

// rowTop returns the Y of the highest item of a row
func rowTop(row []*models.TextItem) float64 {
	top := math.Inf(1)
	for _, item := range row {
		top = math.Min(top, item.Y)
	}
	return top
}

// isKnownTableHeader checks if the first line of the items looks like a known table header
// This helps detect continuation tables that may have fewer items
func (c *CompactLines) isKnownTableHeader(items []*models.TextItem, columns []float64) bool {
	if len(items) == 0 || len(columns) < 2 {
		return false
	}

	var headerTexts []string
	for _, item := range visualLines(items)[0] {
		headerTexts = append(headerTexts, strings.TrimSpace(item.Text))
	}

	// Check for known header patterns
	headerLine := strings.Join(headerTexts, " ")

	for _, known := range KnownTableHeaders {
		if strings.Contains(headerLine, known) || headerLine == known {
			return true
		}
	}

	return false
}

// detectReferenceTable looks for tables with bracketed reference IDs like [CC1], [SD]
func (c *CompactLines) detectReferenceTable(items []*models.TextItem, mostUsedDistance int) *tableRegion {
	// Find items that look like reference IDs: [XXX] pattern
	var refItems []*models.TextItem
	for _, item := range items {
		text := strings.TrimSpace(item.Text)
		if strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]") && len(text) >= referenceIDMinLen && len(text) <= referenceIDMaxLen {
			refItems = append(refItems, item)
		}
	}

	// Need at least 3 reference IDs to consider it a table
	if len(refItems) < minReferenceItems {
		return nil
	}

	// Check if they're at consistent X positions (same column)
	refX := refItems[0].X
	alignedCount := 0
	for _, item := range refItems {
		if math.Abs(item.X-refX) < columnAlignmentTolerance {
			alignedCount++
		}
	}

	// At least 80% should be aligned
	if float64(alignedCount)/float64(len(refItems)) < referenceAlignmentThreshold {
		return nil
	}

	// Find the Y range of reference items
	minY := refItems[0].Y
	maxY := refItems[0].Y
	for _, item := range refItems {
		if item.Y < minY {
			minY = item.Y
		}
		if item.Y > maxY {
			maxY = item.Y
		}
	}

	// Find the second column position (first non-reference item to the right)
	var secondColX float64 = -1
	for _, item := range items {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		// Item to the right of reference column
		if item.X > refX+referenceColumnMinGap && item.Y >= minY-float64(mostUsedDistance)*2 && item.Y <= maxY+float64(mostUsedDistance) {
			if secondColX < 0 || item.X < secondColX {
				secondColX = item.X
			}
		}
	}

	if secondColX < 0 {
		return nil
	}

	// For reference tables, use exactly 2 columns: the reference column and the description column
	// This prevents date fragments (like "-", "2017", "04") from being detected as separate columns
	columns := []float64{refX, secondColX}

	// The table is the rows that hold the reference IDs, with all the lines
	// of their descriptions
	var below []*models.TextItem
	for _, item := range items {
		if strings.TrimSpace(item.Text) != "" && item.Y >= minY-float64(mostUsedDistance)*2 {
			below = append(below, item)
		}
	}
	isRef := make(map[*models.TextItem]bool, len(refItems))
	for _, item := range refItems {
		isRef[item] = true
	}
	for _, row := range c.tableRows(below, columns, nil) {
		hasRef := false
		for _, item := range row {
			hasRef = hasRef || isRef[item]
		}
		if !hasRef {
			continue
		}
		for _, item := range row {
			minY = math.Min(minY, item.Y)
			maxY = math.Max(maxY, item.Y)
		}
	}

	return &tableRegion{
		minY:    minY,
		maxY:    maxY,
		columns: columns,
	}
}

// detectAlignedTables detects tables by looking for consistent column alignment across rows.
//
// This method catches simple tables without explicit headers or reference IDs by:
//  1. Splitting each visual line into runs of text separated by column gaps (see lineRuns)
//  2. Finding lines with 2+ runs that start well apart (candidate rows)
//  3. Looking for sequences of 3+ rows whose runs line up: each run starts, ends or is
//     centred where the run of the same column in the next row does (left-, right- or
//     centre-aligned columns)
//  4. Requiring a whitespace corridor between neighbouring columns down the whole run
//     (no cell's text reaches the start of the next column in any row)
//  5. Validating that rows have a reasonable item-to-column ratio (to reject paragraph text)
//
// The line directly above the rows is their header when its runs line up with the columns.
// Returns table regions for any detected aligned tables.
func (c *CompactLines) detectAlignedTables(items []*models.TextItem, footerThreshold float64) []*tableRegion {
	var content []*models.TextItem
	for _, item := range items {
		if strings.TrimSpace(item.Text) == "" {
			continue
		}
		if item.Y >= footerThreshold {
			continue // Skip footer area
		}
		content = append(content, item)
	}
	lines := visualLines(content)

	// columnRuns returns the runs of a line that start columns
	columnRuns := func(line []*models.TextItem) []textRun {
		var runs []textRun
		lastX := math.Inf(-1)
		for _, r := range lineRuns(line) {
			if len(r.items) == 1 && isRaisedMark(r.items[0], line) {
				continue // a footnote mark does not start a column
			}
			if r.x-lastX >= minColumnSpacing {
				runs = append(runs, r)
				lastX = r.x
			}
		}
		return runs
	}

	// Each visual line with two or more columns is a candidate row
	var rows []alignedRow
	for li, line := range lines {
		if runs := columnRuns(line); len(runs) >= 2 {
			rows = append(rows, alignedRow{line: li, items: line, runs: runs})
		}
	}

	if len(rows) < minAlignedRows {
		return nil // Need at least 3 rows to be a table
	}

	// Look for sequences of rows with matching columns
	var regions []*tableRegion
	var layoutColumns [][]float64 // columns of runs rejected as 2-column page layout
	startIdx := 0

	for startIdx < len(rows) {
		// Find a run of consecutive rows with consistent columns
		endIdx := startIdx + 1
		for endIdx < len(rows) && runsAlign(rows[endIdx-1].runs, rows[endIdx].runs) &&
			!crossedBetween(lines, rows[endIdx-1], rows[endIdx]) {
			endIdx++
		}

		runLength := endIdx - startIdx
		if runLength < minAlignedRows {
			startIdx = endIdx
			continue
		}

		// Validate: check if this looks like paragraph text being falsely detected as a table
		// In real tables, items per row roughly matches column count
		// In paragraph text, many items are crammed into few "columns"
		totalItems := 0
		for i := startIdx; i < endIdx; i++ {
			totalItems += len(rows[i].items)
		}
		numColumns := len(rows[startIdx].runs)
		if float64(totalItems)/float64(runLength)/float64(numColumns) > 2.5 {
			startIdx = endIdx
			continue
		}

		// Columns: where each starts (its leftmost text) and the extent of its text
		columns := make([]float64, numColumns)
		extents := make([][2]float64, numColumns)
		for k := range columns {
			columns[k] = math.Inf(1)
			extents[k] = [2]float64{math.Inf(1), math.Inf(-1)}
			for i := startIdx; i < endIdx; i++ {
				r := rows[i].runs[k]
				columns[k] = math.Min(columns[k], r.x)
				extents[k] = [2]float64{math.Min(extents[k][0], r.x), math.Max(extents[k][1], r.end)}
			}
		}

		// A whitespace corridor must separate every two columns in every row
		corridor := true
		for k := 0; k+1 < numColumns; k++ {
			if extents[k][1] >= extents[k+1][0] {
				corridor = false
			}
		}
		if !corridor {
			startIdx = endIdx
			continue
		}

		// Two words spread to both margins of a justified paragraph line up
		// left and right; that is not a table
		if numColumns == 2 && justifiedLines(lines, rows[startIdx].line, rows[endIdx-1].line, extents, rows[startIdx:endIdx]) {
			startIdx = endIdx
			continue
		}

		// Collect all items in this region for multi-column layout check
		var regionItems []*models.TextItem
		for i := startIdx; i < endIdx; i++ {
			regionItems = append(regionItems, rows[i].items...)
		}

		// Check for 2-column page layout being misdetected as table
		if numColumns == 2 && c.looksLikeMultiColumnLayout(regionItems, columns) {
			layoutColumns = append(layoutColumns, columns)
			startIdx = endIdx
			continue
		}

		// The line directly above is the header when its text lines up with
		// the columns: each of its runs overlaps a column, none crosses one
		minY := lineTop(rows[startIdx].items)
		if above := rows[startIdx].line - 1; above >= 0 {
			if header := lineRuns(lines[above]); len(header) >= 2 && runsFitColumns(header, columns, extents) {
				minY = lineTop(lines[above])
				for _, r := range header {
					k := c.runColumn(r, columns, extents)
					extents[k] = [2]float64{math.Min(extents[k][0], r.x), math.Max(extents[k][1], r.end)}
				}
			}
		}

		maxY := minY
		for _, item := range regionItems {
			maxY = math.Max(maxY, item.Y)
		}

		regions = append(regions, &tableRegion{
			minY:      minY,
			maxY:      maxY,
			columns:   columns,
			extents:   extents,
			hasHeader: true, // the header line, or else the first row
		})

		startIdx = endIdx
	}

	// The columns of a 2-column page layout run the length of the page. A few
	// lines may show no sign of it on their own (every line in the right
	// column starts with a capital, say), so a region is also dropped when
	// its columns are those of a run already recognised as page layout.
	if len(layoutColumns) > 0 {
		kept := regions[:0]
		for _, region := range regions {
			isLayout := false
			for _, columns := range layoutColumns {
				if c.columnsMatch(region.columns, columns) {
					isLayout = true
					break
				}
			}
			if !isLayout {
				kept = append(kept, region)
			}
		}
		regions = kept
	}

	return regions
}

// alignedRow is a line that may be a row of an aligned table
type alignedRow struct {
	line  int // index in the page's visual lines
	items []*models.TextItem
	runs  []textRun // the runs that start its columns
}

// justifiedLines reports whether two-column rows (lines first to last) are
// lines of a justified paragraph: their only boundaries are the paragraph's
// margins. The first column starts at the left margin, the second is not
// left-aligned but all of it ends exactly at the right margin, and another line among or next
// to the rows (within justifiedNeighbourLines above or below) runs from the
// same left margin to the same right margin, as the lines of a justified
// paragraph do.
func justifiedLines(lines [][]*models.TextItem, first, last int, extents [][2]float64, rows []alignedRow) bool {
	starts := make([]float64, 0, len(rows))
	ends := make([]float64, 0, len(rows))
	for _, row := range rows {
		starts = append(starts, row.runs[1].x)
		ends = append(ends, row.runs[1].end)
	}
	sort.Float64s(starts)
	sort.Float64s(ends)
	if starts[len(starts)-1]-starts[0] <= exactAlignment {
		return false // a left-aligned second column: a real column
	}
	if ends[len(ends)-1]-ends[0] > exactAlignment {
		return false // the second column does not end at one right margin
	}
	left, right := extents[0][0], extents[1][1]
	// running reports whether a line is running text from margin to margin:
	// not a two-column row, starting at the left margin and ending at the
	// right one
	running := func(line []*models.TextItem) bool {
		runs := lineRuns(line)
		return len(runs) > 0 && len(runs) != 2 && math.Abs(runs[0].x-left) <= columnAlignmentTolerance &&
			math.Abs(runs[len(runs)-1].end-right) <= columnAlignmentTolerance
	}
	for k := max(0, first-justifiedNeighbourLines); k <= min(len(lines)-1, last+justifiedNeighbourLines); k++ {
		if running(lines[k]) {
			return true
		}
	}
	return false
}

// crossedBetween reports whether running text lies between two rows: a line
// between them whose text runs across the columns of the upper row. The
// lines of wrapped cells keep to the columns; a paragraph between two
// aligned lines (two numbered headings, say) ends the table.
func crossedBetween(lines [][]*models.TextItem, upper, lower alignedRow) bool {
	columns := make([]float64, len(upper.runs))
	for k, r := range upper.runs {
		columns[k] = r.x
	}
	for _, line := range lines[upper.line+1 : lower.line] {
		for _, r := range lineRuns(line) {
			if runCrosses(r, columns) {
				return true
			}
		}
	}
	return false
}

// runsAlign reports whether the runs of two rows line up column by column:
// the same number of columns, and each pair starting, ending or centred at
// the same place (left-, right- or centre-aligned columns)
func runsAlign(a, b []textRun) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		left := math.Abs(a[k].x-b[k].x) <= 2*columnAlignmentTolerance
		right := math.Abs(a[k].end-b[k].end) <= columnAlignmentTolerance
		centre := math.Abs((a[k].x+a[k].end)/2-(b[k].x+b[k].end)/2) <= columnAlignmentTolerance
		if !left && !right && !centre {
			return false
		}
	}
	return true
}

// runsFitColumns reports whether runs of text (a header line) each lie over
// one column without crossing into another
func runsFitColumns(runs []textRun, columns []float64, extents [][2]float64) bool {
	used := make(map[int]bool)
	for _, r := range runs {
		if runCrosses(r, columns) {
			return false
		}
		fits := false
		for k, e := range extents {
			if math.Min(r.end, e[1]) > math.Max(r.x, e[0]) || math.Abs(r.x-columns[k]) < columnAlignmentTolerance {
				if used[k] {
					return false
				}
				used[k] = true
				fits = true
				break
			}
		}
		if !fits {
			return false
		}
	}
	return true
}

// columnsMatch checks if two sets of column positions are similar
func (c *CompactLines) columnsMatch(cols1, cols2 []float64) bool {
	if len(cols1) != len(cols2) {
		return false
	}

	// Check if each column position is within tolerance
	for i := range cols1 {
		if math.Abs(cols1[i]-cols2[i]) > columnAlignmentTolerance*2 {
			return false
		}
	}

	return true
}

// headerTable is a header line found by findTableHeaders, with the extent of the table under it
type headerTable struct {
	y       float64      // Y of the top of the header line
	columns []float64    // where each column starts
	extents [][2]float64 // where each column's header text lies
	maxY    float64      // Y of the lowest item in the table under the header
	rows    int          // number of data rows under the header
}

// findTableHeaders looks for lines that are table headers: 2-6 runs of short text, each a
// column label, separated by column gaps (see lineRuns), with rows of data under them.
// Lines are tried from the top of the page down. Where the tables under several candidate
// headers overlap, the one covering the most data rows wins (the true header sits above
// the whole table, while artifact "headers" inside it only cover a locally consistent
// sub-run), then more consistent columns, then the upper one. A line inside the winner's
// table that is one of its data rows (it hits two or more of its columns) is not tried: it
// would find the rest of the same table, or join it to a table below. Tables that do not overlap are all returned, top to bottom.
func (c *CompactLines) findTableHeaders(items []*models.TextItem) []*headerTable {
	var content []*models.TextItem
	rightEdge := 0.0 // right text edge of the page, to identify the right-margin column
	for _, item := range items {
		if right := item.X + item.Width; right > rightEdge {
			rightEdge = right
		}
		if strings.TrimSpace(item.Text) != "" {
			content = append(content, item)
		}
	}
	lines := visualLines(content)

	var found []*headerTable
	var best *headerTable
	bestScore := 0
	for li, line := range lines {
		runs := lineRuns(line)
		if len(runs) < 2 || len(runs) > 6 {
			continue
		}

		var columns []float64
		var extents [][2]float64
		lastX := math.Inf(-1)
		for _, r := range runs {
			if len(r.items) == 1 && isRaisedMark(r.items[0], line) {
				continue // a footnote mark does not start a column
			}
			// Header labels are short
			if len(r.text()) > maxHeaderItemLength {
				continue
			}
			if r.x-lastX >= minColumnSpacing {
				columns = append(columns, r.x)
				extents = append(extents, [2]float64{r.x, r.end})
				lastX = r.x
			}
		}
		if len(columns) < 2 {
			continue
		}

		headerY := lineTop(line)
		if best != nil && headerY <= best.maxY {
			hit, crosses := c.rowHits(line, best.columns, best.extents)
			n := 0
			for _, h := range hit {
				if h {
					n++
				}
			}
			if !crosses && n >= 2 {
				continue // a row of the table already found
			}
		} else if best != nil {
			// Below the table found so far: that table is complete
			found = append(found, best)
			best = nil
		}

		maxY, dataRows, consistentCols, interiorCols := c.boundBelow(lines[li+1:], headerY, columns, extents, rightEdge)
		// A known header is evidence enough with a single row beneath it: the last
		// row of a table continued from the previous page.
		minDataRows := minTableDataRows
		if c.isKnownTableHeader(line, columns) {
			minDataRows = 1
		}
		if consistentCols >= 2 && interiorCols >= 1 && dataRows >= minDataRows &&
			!(len(columns) == 2 && c.justifiedUnder(lines, li, maxY)) {
			score := dataRows*100 + consistentCols*10 - int(headerY/10)
			if best == nil || score > bestScore {
				best = &headerTable{headerY, columns, extents, maxY, dataRows}
				bestScore = score
			}
		}
	}
	if best != nil {
		found = append(found, best)
	}
	return found
}

// justifiedUnder reports whether the two-column table from header line li
// down to maxY is the lines of a justified paragraph (see justifiedLines)
func (c *CompactLines) justifiedUnder(lines [][]*models.TextItem, li int, maxY float64) bool {
	var rows []alignedRow
	last := li
	for k := li; k < len(lines) && lineTop(lines[k]) <= maxY; k++ {
		if runs := lineRuns(lines[k]); len(runs) == 2 {
			rows = append(rows, alignedRow{line: k, items: lines[k], runs: runs})
		}
		last = k
	}
	if len(rows) < 2 {
		return false
	}
	extents := [][2]float64{{math.Inf(1), math.Inf(-1)}, {math.Inf(1), math.Inf(-1)}}
	for _, row := range rows {
		for k, r := range row.runs {
			extents[k] = [2]float64{math.Min(extents[k][0], r.x), math.Max(extents[k][1], r.end)}
		}
	}
	return justifiedLines(lines, li, last, extents, rows)
}

// boundBelow bounds the table under a header (see boundTableRegionByColumns) from the
// lines below it. It reads the lines a block at a time, doubling the block until the
// table ends well inside it, so a header near the top of a long page does not cost a
// pass over the whole page.
func (c *CompactLines) boundBelow(lines [][]*models.TextItem, headerY float64, columns []float64, extents [][2]float64, rightEdge float64) (maxY float64, dataRows, consistentCols, interiorCols int) {
	for n := boundBlockLines; ; n *= 2 {
		if n > len(lines) {
			n = len(lines)
		}
		var below []*models.TextItem
		for _, line := range lines[:n] {
			below = append(below, line...)
		}
		var stopY float64
		maxY, dataRows, consistentCols, interiorCols, stopY = c.boundTableRegionByColumns(below, headerY, columns, extents, rightEdge)
		// Done when every line was read, or the table ended before the last
		// few lines of the block (a row cut off by the block's end could
		// otherwise have ended it early)
		if n == len(lines) || (stopY > 0 && stopY < lineTop(lines[n-boundBlockMargin])) {
			return maxY, dataRows, consistentCols, interiorCols
		}
	}
}

// detectMultiLineCells determines if a potential table has cells that span multiple lines.
//
// This is a key indicator of table structure: regular paragraphs have consistent line
// spacing, while tables often have cells where text wraps to multiple lines within
// a single logical row. The items are grouped into table rows (see tableRows); a row has a
// multi-line cell when one of its columns holds text on two or more visual lines. It is a
// table if at least 2 rows have one.
//
// This helps distinguish tables from regular text that happens to have column-like
// alignment (e.g., lists with consistent indentation).
func (c *CompactLines) detectMultiLineCells(items []*models.TextItem, columns []float64, extents [][2]float64) bool {
	var content []*models.TextItem
	for _, item := range items {
		if strings.TrimSpace(item.Text) != "" {
			content = append(content, item)
		}
	}
	lineOf := make(map[*models.TextItem]int, len(content))
	for li, line := range visualLines(content) {
		for _, item := range line {
			lineOf[item] = li
		}
	}
	colOf := c.columnsOf(content, columns, extents)

	multiLineCount := 0
	for _, row := range c.tableRows(content, columns, extents) {
		firstLine := make(map[int]int)
		for _, item := range row {
			col := colOf[item]
			first, seen := firstLine[col]
			if !seen {
				firstLine[col] = lineOf[item]
			} else if lineOf[item] != first {
				multiLineCount++
				break
			}
		}
	}

	// Consider it a table if at least 2 rows have multi-line cells
	return multiLineCount >= 2
}

// validateTableStructure checks if detected table items have a reasonable structure.
// Returns false if the structure looks more like paragraph text than a table.
//
// The key insight is that in real tables, items per row roughly matches column count.
// In paragraph text incorrectly detected as a table, many items (words) get assigned
// to few "columns", resulting in a high items-to-columns ratio.
//
// Additionally, we check for multi-column page layouts where items in the "second column"
// are actually continuations of text from the first column (indicated by lowercase starts
// or word fragments).
func (c *CompactLines) validateTableStructure(items []*models.TextItem, columns []float64) bool {
	if len(items) == 0 || len(columns) < 2 {
		return false
	}

	var content []*models.TextItem
	for _, item := range items {
		if strings.TrimSpace(item.Text) != "" {
			content = append(content, item)
		}
	}
	lines := visualLines(content)
	if len(lines) == 0 {
		return false
	}

	// Calculate average items per line
	avgItemsPerRow := float64(len(content)) / float64(len(lines))

	// Calculate items-to-columns ratio
	// Real tables: ~1-2 items per column per row (maybe 3 for wrapped cells)
	// Paragraphs: many items (words) per column
	itemsToColumnsRatio := avgItemsPerRow / float64(len(columns))

	// If average items per row is much higher than column count, it's likely paragraph text
	if itemsToColumnsRatio > 2.5 {
		return false
	}

	// Check for multi-column page layout (text continuation between columns)
	// This detects PDFs with 2-column page layouts being misinterpreted as tables
	if len(columns) == 2 && c.looksLikeMultiColumnLayout(items, columns) {
		return false
	}

	return true
}

// looksLikeMultiColumnLayout checks if items look like a 2-column page layout
// rather than a table. Signs of page layout:
// - Second column items often start with lowercase (continuing a sentence)
// - Second column items are word fragments (start mid-word)
// - Items that are just punctuation like "-" (hyphenated words split across columns)
func (c *CompactLines) looksLikeMultiColumnLayout(items []*models.TextItem, columns []float64) bool {
	if len(columns) != 2 {
		return false
	}

	// Column boundary: midpoint between the two columns
	colBoundary := (columns[0] + columns[1]) / 2

	// Count items in the right column that look like text fragments
	fragmentCount := 0
	rightColItemCount := 0

	for _, item := range items {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}

		// Check if item is in the right column (second half of page)
		if item.X >= colBoundary {
			rightColItemCount++

			// Check for fragment indicators:
			// 1. Starts with lowercase letter (sentence continuation)
			// 2. Starts with punctuation that attaches to previous word
			// 3. Is just a hyphen (split hyphenated word)
			firstRune := []rune(text)[0]
			if firstRune >= 'a' && firstRune <= 'z' {
				fragmentCount++
			} else if firstRune == '-' || firstRune == '–' || firstRune == '.' ||
				firstRune == ',' || firstRune == ';' || firstRune == ':' {
				fragmentCount++
			}
		}
	}

	// If significant portion of right column items are fragments, it's a page layout
	if rightColItemCount > 0 {
		fragmentRatio := float64(fragmentCount) / float64(rightColItemCount)
		// More than 30% fragments suggests page layout, not table
		if fragmentRatio > 0.3 {
			return true
		}
	}

	return false
}

// detectAndSplitMultiColumnLayout detects 2-column page layouts and splits items
// into left and right columns for separate processing.
// Returns (leftItems, rightItems, isMultiColumn).
func (c *CompactLines) detectAndSplitMultiColumnLayout(items []*models.TextItem) ([]*models.TextItem, []*models.TextItem, bool) {
	if len(items) < 10 {
		return nil, nil, false // Too few items to be a multi-column layout
	}

	// Find the X range of all items
	minX, maxX := items[0].X, items[0].X
	for _, item := range items {
		if item.X < minX {
			minX = item.X
		}
		if item.X > maxX {
			maxX = item.X
		}
	}

	pageWidth := maxX - minX
	if pageWidth < 200 {
		return nil, nil, false // Page too narrow for multi-column
	}

	// Check if items cluster into two distinct X regions either side of a
	// gutter: the position in the middle third of the text area (from the
	// left edge of the text to where it ends on the right) that the least
	// text runs across, nearest the centre on a tie
	rightEdge := maxX
	for _, item := range items {
		rightEdge = math.Max(rightEdge, item.X+item.Width)
	}
	midPoint := findGutter(items, minX, rightEdge)
	leftCount, rightCount := 0, 0
	var leftItems, rightItems []*models.TextItem

	for _, item := range items {
		text := strings.TrimSpace(item.Text)
		// Include all items (including spaces) but only count non-empty for detection
		if item.X < midPoint {
			if text != "" {
				leftCount++
			}
			leftItems = append(leftItems, item)
		} else {
			if text != "" {
				rightCount++
			}
			rightItems = append(rightItems, item)
		}
	}

	// Both sides need significant content to be a 2-column layout
	if leftCount < 5 || rightCount < 5 {
		return nil, nil, false
	}

	// Check for centered or full-width text: a line whose text runs across the
	// midpoint spans both sides (like title pages), which a 2-column layout's
	// lines do not: its columns are separated by a gutter. A line runs across
	// when one item crosses the midpoint, or the text on either side of it is
	// no further apart than a word space.
	lines := visualLines(items)
	spanningLineCount := 0
	ownRightCount := 0 // right-side text on lines that do not span
	for _, line := range lines {
		spans := false
		var leftEnd, rightStart float64 = math.Inf(-1), math.Inf(1)
		var height float64
		for _, item := range line {
			height = math.Max(height, textHeight(item))
			if item.X < midPoint {
				if item.X+item.Width > midPoint {
					spans = true
				}
				leftEnd = math.Max(leftEnd, item.X+item.Width)
			} else {
				rightStart = math.Min(rightStart, item.X)
			}
		}
		if spans || rightStart-leftEnd < multiColumnMinGutter*height {
			spanningLineCount++
			continue
		}
		for _, item := range line {
			if item.X >= midPoint && strings.TrimSpace(item.Text) != "" {
				ownRightCount++
			}
		}
	}

	// The right column needs lines of its own, not only the ends of lines
	// that run across from the left
	if ownRightCount < 5 {
		return nil, nil, false
	}

	// If more than 30% of lines span both sides, it's not a true 2-column layout
	if len(lines) > 0 {
		spanningRatio := float64(spanningLineCount) / float64(len(lines))
		if spanningRatio > 0.3 {
			return nil, nil, false // Too many lines span both columns - probably centered text
		}
	}

	// Check if right column has fragment indicators (sentence continuations)
	fragmentCount := 0
	for _, item := range rightItems {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		firstRune := []rune(text)[0]
		if firstRune >= 'a' && firstRune <= 'z' {
			fragmentCount++
		} else if firstRune == '-' || firstRune == '–' || firstRune == '.' ||
			firstRune == ',' || firstRune == ';' || firstRune == ':' {
			fragmentCount++
		}
	}

	fragmentRatio := float64(fragmentCount) / float64(rightCount)
	if fragmentRatio < 0.3 {
		return nil, nil, false // Not enough fragments - probably not a 2-column layout
	}

	// This is a 2-column layout - return split items
	return leftItems, rightItems, true
}

// findGutter returns the X between left and right, within the middle third,
// that the fewest text items run across, nearest the centre on a tie. The
// number of items across X only changes at an item's edge, so it sweeps the
// edges in order rather than stepping across the page.
func findGutter(items []*models.TextItem, left, right float64) float64 {
	centre := (left + right) / 2
	third := (right - left) / 6
	lo, hi := centre-third, centre+third

	var starts, ends []float64
	points := []float64{lo, hi}
	for _, item := range items {
		if strings.TrimSpace(item.Text) == "" {
			continue
		}
		start, end := item.X, item.X+item.Width
		starts = append(starts, start)
		ends = append(ends, end)
		for _, p := range []float64{start, end} {
			if p > lo && p < hi {
				points = append(points, p)
			}
		}
	}
	sort.Float64s(starts)
	sort.Float64s(ends)
	sort.Float64s(points)

	// across returns how many items run across x (start <= x <= end)
	across := func(x float64) int {
		started := sort.Search(len(starts), func(i int) bool { return starts[i] > x })
		ended := sort.Search(len(ends), func(i int) bool { return ends[i] >= x })
		return started - ended
	}

	// Between two edges the count is constant: try each edge and, in each
	// gap between edges, the point nearest the centre
	best, bestCount := centre, -1
	try := func(x float64) {
		count := across(x)
		if bestCount < 0 || count < bestCount || (count == bestCount && math.Abs(x-centre) < math.Abs(best-centre)) {
			best, bestCount = x, count
		}
	}
	for i, p := range points {
		try(p)
		if i+1 < len(points) && points[i+1] > p {
			next := points[i+1]
			switch {
			case centre > p && centre < next:
				try(centre)
			case centre <= p:
				try(p + math.Min(0.5, (next-p)/2))
			default:
				try(next - math.Min(0.5, (next-p)/2))
			}
		}
	}
	return best
}

// findTableRegion returns the table region containing the given Y coordinate
func (c *CompactLines) findTableRegion(y float64, regions []*tableRegion) *tableRegion {
	for _, region := range regions {
		if y >= region.minY && y <= region.maxY {
			return region
		}
	}
	return nil
}

// getColumn returns which column index an X position belongs to
// Assigns to the rightmost column that starts at or before the X position
func (c *CompactLines) getColumn(x float64, columns []float64) int {
	if len(columns) == 0 {
		return 0
	}

	// Find the rightmost column that starts at or before X
	// (with some tolerance for items slightly before column start)
	tolerance := 15.0
	bestCol := 0

	for i := 0; i < len(columns); i++ {
		if x >= columns[i]-tolerance {
			bestCol = i
		} else {
			break
		}
	}

	return bestCol
}

func (c *CompactLines) compactLine(textItems []*models.TextItem, fontToFormats map[string]*models.WordFormat) *models.LineItem {
	if len(textItems) == 0 {
		return nil
	}

	// Note: Items should already be in correct order from caller (table rows
	// by column, then line, then X; other lines left to right). DO NOT
	// re-sort here as it would break the column ordering for table cells

	// Calculate average font size for this line based on height values
	// This helps with space detection when width calculations are inaccurate
	// Filter out obviously wrong height values (< 4pt is too small to be readable text)
	var avgHeight float64
	heightCount := 0
	for _, item := range textItems {
		if item.Height >= 4.0 { // Minimum readable font size
			avgHeight += item.Height
			heightCount++
		}
	}
	if heightCount > 0 {
		avgHeight /= float64(heightCount)
	}
	if avgHeight < 4.0 {
		avgHeight = 12.0 // Default font size
	}

	// Combine text and detect formatting
	words := c.itemsToWordsWithContext(textItems, fontToFormats, avgHeight)

	if len(words) == 0 {
		return nil
	}

	// Calculate dimensions
	// Skip whitespace-only items when determining height (they can have incorrect height values)
	var maxHeight float64
	var widthSum float64
	for _, item := range textItems {
		widthSum += item.Width
		// Only consider non-whitespace items for height calculation
		if strings.TrimSpace(item.Text) != "" && item.Height > maxHeight {
			maxHeight = item.Height
		}
	}

	// Detect footnotes and links
	parsedElements := c.detectElements(words, textItems)

	return &models.LineItem{
		X:              textItems[0].X,
		Y:              textItems[0].Y,
		Height:         maxHeight,
		Width:          widthSum,
		Words:          words,
		ParsedElements: parsedElements,
		Font:           textItems[0].Font,
	}
}

func (c *CompactLines) itemsToWordsWithContext(items []*models.TextItem, fontToFormats map[string]*models.WordFormat, avgFontSize float64) []*models.Word {
	// Combine text with spacing
	combinedText := c.combineTextWithContext(items, avgFontSize)

	// Split into words
	wordStrings := strings.Fields(combinedText)
	if len(wordStrings) == 0 {
		return nil
	}

	// Determine format based on first item's font
	var format *models.WordFormat
	if len(items) > 0 {
		format = fontToFormats[items[0].Font]
	}

	words := make([]*models.Word, len(wordStrings))
	for i, ws := range wordStrings {
		var wordType *models.WordType
		wordStr := ws

		// Detect links
		if strings.HasPrefix(wordStr, "http:") || strings.HasPrefix(wordStr, "https:") {
			wordType = models.WordTypeLink
		} else if strings.HasPrefix(wordStr, "www.") {
			wordStr = "http://" + wordStr
			wordType = models.WordTypeLink
		}

		words[i] = &models.Word{
			String: wordStr,
			Type:   wordType,
			Format: format,
		}
	}

	return words
}

func (c *CompactLines) combineText(items []*models.TextItem) string {
	return c.combineTextWithContext(items, 12.0) // Default font size
}

func (c *CompactLines) combineTextWithContext(items []*models.TextItem, avgFontSize float64) string {
	var text strings.Builder
	var lastItem *models.TextItem
	endsWithSpace := false // Track trailing space to avoid repeated String() calls

	// A word space is 0.25-0.33 em in common fonts; the gap between pieces
	// of one word (kerning, rounding) stays well below 0.2 em
	wordSpaceThreshold := avgFontSize * 0.2

	for _, item := range items {
		textToAdd := item.Text

		// If text starts with punctuation that attaches to previous word, trim trailing space
		nextFirst := firstRune(strings.TrimSpace(textToAdd))
		if nextFirst == ',' || nextFirst == '.' || nextFirst == ':' || nextFirst == ';' ||
			nextFirst == ')' || nextFirst == ']' || nextFirst == '}' {
			// Trim trailing space from accumulated text before adding punctuation
			if endsWithSpace {
				str := text.String()
				text.Reset()
				text.WriteString(strings.TrimRight(str, " "))
				endsWithSpace = false
			}
		}

		if !endsWithSpace && !strings.HasPrefix(textToAdd, " ") {
			if lastItem != nil {
				// Check if we're on a different line (cell wrap in table)
				if !onSameLine(lastItem, item, avgFontSize) {
					// Different line - add a space unless the word continues
					if c.lineWrapNeedsSpace(lastItem.Text, textToAdd) {
						text.WriteString(" ")
						endsWithSpace = true
					}
				} else {
					// Same line - check for word boundary using font-relative threshold
					effectiveWidth := c.getEffectiveWidth(lastItem, avgFontSize)
					xDistance := item.X - lastItem.X - effectiveWidth
					needsSpace := c.needsSpaceBetweenWithThreshold(lastItem.Text, textToAdd, xDistance, wordSpaceThreshold)
					if needsSpace {
						text.WriteString(" ")
						endsWithSpace = true
					}
				}
			} else {
				if isListItemCharacter(textToAdd) {
					textToAdd += " "
				}
			}
		}

		text.WriteString(textToAdd)
		endsWithSpace = strings.HasSuffix(textToAdd, " ")
		lastItem = item
	}

	return text.String()
}

// lineWrapNeedsSpace reports whether text that wraps from one line to the
// next is separated by a space: not after a hyphen at the end of the line,
// and not where the word continues (e.g., "AutoUpda" + "te")
func (c *CompactLines) lineWrapNeedsSpace(prevText, nextText string) bool {
	prevText = strings.TrimSpace(prevText)
	if strings.HasSuffix(prevText, "-") || strings.HasSuffix(prevText, "–") {
		return false
	}
	return !c.isWordContinuation(prevText, strings.TrimSpace(nextText))
}

// getEffectiveWidth returns the width of a text item on the page, estimated
// from its length when the item has none
func (c *CompactLines) getEffectiveWidth(item *models.TextItem, avgFontSize float64) float64 {
	if item.Width > 0 {
		return item.Width
	}
	return float64(len(item.Text)) * avgFontSize * 0.5
}

// needsSpaceBetweenWithThreshold determines if a space should be inserted, with a custom threshold
func (c *CompactLines) needsSpaceBetweenWithThreshold(prevText, nextText string, xDistance float64, wordThreshold float64) bool {
	// If negative distance (overlap), no space
	if xDistance < 0 {
		return false
	}

	// Check for punctuation that shouldn't have leading/trailing spaces
	prevLast := lastRune(prevText)
	nextFirst := firstRune(nextText)

	// Sentence-ending punctuation followed by a letter needs a space
	// This handles cases like "customers.and" -> "customers. and"
	if (prevLast == '.' || prevLast == '!' || prevLast == '?') &&
		((nextFirst >= 'a' && nextFirst <= 'z') || (nextFirst >= 'A' && nextFirst <= 'Z')) {
		return true
	}

	// Apostrophe/quote followed by a letter needs a space (e.g., "Inc.'sAsset" -> "Inc.'s Asset")
	if (prevLast == '\'' || prevLast == '\u2019' || prevLast == '"' || prevLast == '\u201d') &&
		(nextFirst >= 'A' && nextFirst <= 'Z') {
		return true
	}

	// Possessive "'s" followed by uppercase letter needs a space (e.g., "Inc.'s" + "Asset" -> "Inc.'s Asset")
	if (strings.HasSuffix(prevText, "'s") || strings.HasSuffix(prevText, "'s")) &&
		(nextFirst >= 'A' && nextFirst <= 'Z') {
		return true
	}

	// Hyphens/dashes connect without spaces (check BEFORE gap size)
	if prevLast == '-' || nextFirst == '-' || prevLast == '–' || nextFirst == '–' {
		return false
	}

	// Periods, commas, colons attached to previous text
	if nextFirst == '.' || nextFirst == ',' || nextFirst == ':' || nextFirst == ';' {
		return false
	}

	// Opening parens/brackets attach to next word
	if prevLast == '(' || prevLast == '[' || prevLast == '{' {
		return false
	}

	// Closing parens/brackets attach to previous word
	if nextFirst == ')' || nextFirst == ']' || nextFirst == '}' {
		return false
	}

	// For alphanumeric characters, use the font-relative threshold
	// This handles cases where width calculations are inaccurate due to font encoding issues
	if isAlphanumeric(prevLast) && isAlphanumeric(nextFirst) {
		// Check if prevText ends with a common short word - these always need space after
		// This handles cases like "and an" + "open" where the gap is small but it's a word boundary
		prevLower := strings.ToLower(strings.TrimSpace(prevText))
		shortWordEndings := []string{" a", " an", " the", " to", " of", " in", " on", " is", " be", " we", " or", " and", " for"}
		for _, ending := range shortWordEndings {
			if strings.HasSuffix(prevLower, ending) {
				return true // Always add space after common short words
			}
		}
		return xDistance > wordThreshold
	}

	// For mixed alphanumeric/punctuation (like "v3.0" or "FMT_MOF"), don't add space for small gaps
	if (isAlphanumeric(prevLast) && isPunctuation(nextFirst)) ||
		(isPunctuation(prevLast) && isAlphanumeric(nextFirst)) {
		return xDistance > largeGapThreshold
	}

	// If there's a large gap with non-alphanumeric chars, add space
	if xDistance > largeGapThreshold {
		return true
	}

	// Default: add space for gaps > threshold (for symbols, whitespace, etc.)
	return xDistance > minSpaceGapThreshold
}

func lastRune(s string) rune {
	if s == "" {
		return 0
	}
	runes := []rune(s)
	return runes[len(runes)-1]
}

func firstRune(s string) rune {
	if s == "" {
		return 0
	}
	return []rune(s)[0]
}

func isAlphanumeric(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

func isPunctuation(r rune) bool {
	return r == '-' || r == '–' || r == '.' || r == ',' || r == ':' || r == ';' ||
		r == '(' || r == ')' || r == '[' || r == ']' || r == '{' || r == '}' ||
		r == '_' || r == '/'
}

// isWordContinuation checks if nextText likely continues the word from prevText
// This handles cases where a word is split across lines in PDFs
func (c *CompactLines) isWordContinuation(prevText, nextText string) bool {
	if prevText == "" || nextText == "" {
		return false
	}

	prevLast := lastRune(prevText)
	nextFirst := firstRune(nextText)

	// If prev ends with alphanumeric and next starts with lowercase letter,
	// it's likely a word continuation (e.g., "AutoUpda" + "te")
	if isAlphanumeric(prevLast) && nextFirst >= 'a' && nextFirst <= 'z' {
		// Additional check: nextText should be short (1-3 chars) to be a fragment
		// 4+ chars is more likely to be a new word
		if len(nextText) > 3 {
			return false
		}

		// Check if prevText ends with a common standalone word
		// These are unlikely to be split mid-word
		prevLower := strings.ToLower(strings.TrimSpace(prevText))
		commonEndings := []string{" a", " an", " the", " to", " of", " in", " on", " is", " be", " we", " or"}
		for _, ending := range commonEndings {
			if strings.HasSuffix(prevLower, ending) {
				return false
			}
		}
		// Also check if the entire prevText is a short common word
		if len(prevLower) <= 3 {
			shortWords := []string{"a", "an", "to", "of", "in", "on", "is", "be", "we", "or", "as", "at", "by", "if", "so", "no", "up", "my"}
			for _, w := range shortWords {
				if prevLower == w {
					return false
				}
			}
		}

		return true
	}

	return false
}

func (c *CompactLines) detectElements(words []*models.Word, items []*models.TextItem) *models.ParsedElements {
	elements := &models.ParsedElements{}

	var firstY float64
	if len(items) > 0 {
		firstY = items[0].Y
	}

	for i, word := range words {
		// Count formatted words
		if word.Format != nil {
			elements.FormattedWords++
		}

		// Detect links
		if word.Type == models.WordTypeLink {
			elements.ContainLinks = true
		}

		// Detect footnotes (numbers that are superscript)
		if isNumber(word.String) && len(items) > i {
			item := items[i]
			if item.Y > firstY { // Superscript
				word.Type = models.WordTypeFootnoteLink
				num, _ := strconv.Atoi(word.String)
				elements.FootnoteLinks = append(elements.FootnoteLinks, num)
			}
		}

		// Detect parenthesized footnotes attached to words like "footnote(1)"
		// Standalone "(1)" patterns are not converted here - they rely on superscript detection above
		// because standalone (n) could be section references in technical docs (e.g., man pages)
		if num, prefix, ok := extractTrailingParenthesizedNumber(word.String); ok && prefix != "" {
			word.String = prefix + "[^" + strconv.Itoa(num) + "]"
			// Don't set Type - already formatted inline
			elements.FootnoteLinks = append(elements.FootnoteLinks, num)
		}
	}

	return elements
}

func isListItemCharacter(s string) bool {
	if len(s) == 0 {
		return false
	}
	// Use rune for proper Unicode comparison
	r := []rune(s)
	if len(r) != 1 {
		return false
	}
	c := r[0]
	// Common bullet characters used in PDFs
	switch c {
	case '-', '•', '–', '—', // hyphen, bullet, en-dash, em-dash
		'◦', '○', '●', // circles
		'▪', '■', '□', // squares
		'▸', '►', '▹', '▻', // triangles/arrows
		'★', '☆', // stars
		'·', '∙', // middle dots
		'⁃': // hyphen bullet
		return true
	}
	return false
}

func isNumber(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// extractTrailingParenthesizedNumber checks if string ends with a parenthesized number like (1), (2), etc.
// Returns the number, any prefix text, and true if matched.
// Examples: "(1)" -> 1, "", true; "footnote(1)" -> 1, "footnote", true
// Only matches small numbers (1-99) to avoid false positives with technical references.
func extractTrailingParenthesizedNumber(s string) (int, string, bool) {
	// Find the last '(' in the string
	openIdx := -1
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '(' {
			openIdx = i
			break
		}
	}
	if openIdx == -1 || s[len(s)-1] != ')' {
		return 0, "", false
	}

	inner := s[openIdx+1 : len(s)-1]
	if !isNumber(inner) {
		return 0, "", false
	}
	// Skip 3+ digit numbers - unlikely to be footnotes
	if len(inner) > 2 {
		return 0, "", false
	}
	num, err := strconv.Atoi(inner)
	if err != nil {
		return 0, "", false
	}

	prefix := s[:openIdx]

	// Skip common false positives - number words, Unix man page references
	prefixLower := strings.ToLower(prefix)
	falsePositives := []string{
		"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten",
		// Common Unix commands/functions that use (n) for man page sections
		"time", "date", "daytime", "printf", "scanf", "malloc", "free", "exit",
		"open", "close", "read", "write", "socket", "connect", "bind", "listen",
	}
	for _, fp := range falsePositives {
		if prefixLower == fp {
			return 0, "", false
		}
	}

	return num, prefix, true
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// isExcessivelyRepetitive detects text that contains the same phrase repeated many times.
// This is commonly found in PDF metadata, watermarks, or hidden form fields that shouldn't
// be extracted as regular content. For example: "Scott Chapman    Scott Chapman    Scott Chapman..."
func isExcessivelyRepetitive(text string) bool {
	// Skip short texts - they can't have meaningful repetition
	if len(text) < 100 {
		return false
	}

	// Clean and normalize the text
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}

	// Check for patterns like timestamps or names repeated
	// Split by common delimiters (whitespace sequences)
	words := strings.Fields(text)
	if len(words) < 6 {
		return false
	}

	// Look for repeated word sequences
	// Try different phrase lengths (1-4 words)
	for phraseLen := 1; phraseLen <= 4; phraseLen++ {
		if len(words) < phraseLen*4 {
			continue
		}

		// Build the first phrase
		firstPhrase := strings.Join(words[:phraseLen], " ")

		// Count how many times this phrase appears
		repeatCount := 0
		for i := 0; i+phraseLen <= len(words); i += phraseLen {
			phrase := strings.Join(words[i:i+phraseLen], " ")
			if phrase == firstPhrase {
				repeatCount++
			}
		}

		// If the same phrase appears more than 5 times, it's excessively repetitive
		if repeatCount > 5 {
			return true
		}
	}

	return false
}

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
	// yPositionTolerance is the tolerance for grouping items on the same line
	yPositionTolerance = 5.0

	// maxPageContentY is the maximum Y value for page content (footer threshold)
	maxPageContentY = 700.0

	// minColumnSpacing is the minimum horizontal gap between table columns
	minColumnSpacing = 40.0

	// columnAlignmentTolerance is the tolerance for aligning items to columns
	columnAlignmentTolerance = 20.0

	// columnBucketSize groups X positions for column detection
	columnBucketSize = 15.0

	// referenceColumnMinGap is the minimum gap between reference ID and description
	referenceColumnMinGap = 30.0

	// minColumnItemCount is the minimum items at an X position to be a column
	minColumnItemCount = 3

	// maxHeaderItemLength is the maximum character length for header items
	maxHeaderItemLength = 30

	// tableRegionTolerance extends table region boundaries
	tableRegionTolerance = 10.0

	// largeGapThreshold is the gap size that definitely needs a space
	// (only used when characters are not both alphanumeric)
	largeGapThreshold = 30.0

	// alphanumericGapThreshold is the gap size that adds space between alphanumeric chars
	// PDFs often have kerning/tracking that creates small gaps within words
	// Typical values: intra-word kerning gaps ~5-15px, word spacing ~20-40px
	// Set threshold between these ranges to avoid false breaks
	alphanumericGapThreshold = 25.0

	// smallGapThreshold is the gap size for word boundary detection
	smallGapThreshold = 15.0

	// minSpaceGapThreshold is the minimum gap that might need a space
	// (only used for non-alphanumeric boundaries)
	minSpaceGapThreshold = 10.0

	// minRowThreshold is the minimum row threshold for table grouping
	minRowThreshold = 35.0

	// minMultiLineCellThreshold is the minimum threshold for multi-line cell detection
	minMultiLineCellThreshold = 20.0

	// referenceIDMinLen is the minimum length for a reference ID like [CC1]
	referenceIDMinLen = 3

	// referenceIDMaxLen is the maximum length for a reference ID
	referenceIDMaxLen = 10

	// minReferenceItems is the minimum reference items to detect a table
	minReferenceItems = 3

	// referenceAlignmentThreshold is the percentage of items that must align
	referenceAlignmentThreshold = 0.8

	// yLineWrapThreshold is the Y distance that indicates a line wrap
	// Must be larger than the line grouping threshold (mostUsedDistance/2, typically 5-8px)
	// to avoid false word breaks from slight Y variations in same-line text
	yLineWrapThreshold = 10.0

	// yTolerance is the tolerance for considering items on the same line
	yTolerance = 2.0
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
	columns    []float64 // column X positions for table rows
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
		groupedLines := c.groupByLineWithTableDetection(page.Items, mostUsedDistance, footerThreshold)

		// Convert grouped items to LineItems
		var lineItems []interface{}
		for _, group := range groupedLines {
			lineItem := c.compactLine(group.items, fontToFormats)
			if lineItem != nil {
				// Set table metadata
				if group.isTableRow {
					lineItem.IsTableRow = true
					lineItem.IsTableHeader = group.isHeader
					lineItem.TableColumns = c.extractColumnTexts(group.items, group.columns)
				}
				lineItems = append(lineItems, lineItem)
			}
		}

		page.Items = lineItems
	}

	return result
}

// extractColumnTexts extracts and combines text content for each table column.
//
// Given a set of text items and column X positions, this function:
//  1. Assigns each item to a column based on its X position (using getColumn)
//  2. Sorts items within each column by Y position, then X position
//  3. Combines the text using combineText, which handles spacing and line wraps
//
// Returns a slice of strings where each element is the combined text for that column.
// Multi-line cell content is joined with appropriate spacing (handled by combineText).
func (c *CompactLines) extractColumnTexts(items []*models.TextItem, columns []float64) []string {
	if len(columns) == 0 {
		return nil
	}

	// Group items by column
	columnItems := make([][]*models.TextItem, len(columns))
	for i := range columnItems {
		columnItems[i] = []*models.TextItem{}
	}

	for _, item := range items {
		col := c.getColumn(item.X, columns)
		if col < len(columnItems) {
			columnItems[col] = append(columnItems[col], item)
		}
	}

	// Combine text for each column
	result := make([]string, len(columns))
	for i, colItems := range columnItems {
		if len(colItems) > 0 {
			// Sort by Y then X within column
			sort.Slice(colItems, func(a, b int) bool {
				if math.Abs(colItems[a].Y-colItems[b].Y) > yTolerance {
					return colItems[a].Y < colItems[b].Y
				}
				return colItems[a].X < colItems[b].X
			})
			result[i] = strings.TrimSpace(c.combineText(colItems))
		}
	}

	return result
}

// groupByLineWithTableDetection groups text items, detecting and handling tables
func (c *CompactLines) groupByLineWithTableDetection(items []interface{}, mostUsedDistance int, footerThreshold float64) []lineGroup {
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

	// Identify table regions on the page
	tableRegions := c.detectTableRegions(textItems, mostUsedDistance, footerThreshold)

	if len(tableRegions) == 0 {
		// No tables detected - check for 2-column page layout
		leftItems, rightItems, isMultiColumn := c.detectAndSplitMultiColumnLayout(textItems, mostUsedDistance)

		if isMultiColumn {
			// Process each column separately, then combine
			leftLines := c.groupTextItemsByLine(leftItems, mostUsedDistance)
			rightLines := c.groupTextItemsByLine(rightItems, mostUsedDistance)

			result := make([]lineGroup, 0, len(leftLines)+len(rightLines))
			for _, line := range leftLines {
				result = append(result, lineGroup{items: line, isTableRow: false})
			}
			for _, line := range rightLines {
				result = append(result, lineGroup{items: line, isTableRow: false})
			}
			return result
		}

		// Standard Y-based grouping for single-column layouts
		lines := c.groupTextItemsByLine(textItems, mostUsedDistance)
		result := make([]lineGroup, len(lines))
		for i, line := range lines {
			result[i] = lineGroup{items: line, isTableRow: false}
		}
		return result
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
					tableLines := c.groupAsTableWithMetadata(currentGroup, currentTableRegion, mostUsedDistance)
					allLines = append(allLines, tableLines...)
				} else {
					// Process as regular text
					lines := c.groupTextItemsByLine(currentGroup, mostUsedDistance)
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
			tableLines := c.groupAsTableWithMetadata(currentGroup, currentTableRegion, mostUsedDistance)
			allLines = append(allLines, tableLines...)
		} else {
			lines := c.groupTextItemsByLine(currentGroup, mostUsedDistance)
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

	return allLines
}

// groupAsTableWithMetadata converts text items into table rows with associated metadata.
//
// It delegates to groupAsTable for the actual row grouping, then wraps each row
// in a lineGroup struct with table-specific metadata:
//   - isTableRow: always true for items in a table region
//   - isHeader: true only for the first row if region.hasHeader is set
//   - columns: the X positions defining column boundaries
//
// The first row is marked as header only if the table region was detected with
// a header row (e.g., via known header patterns or header-based detection).
func (c *CompactLines) groupAsTableWithMetadata(items []*models.TextItem, region *tableRegion, mostUsedDistance int) []lineGroup {
	rows := c.groupAsTable(items, region.columns, mostUsedDistance)
	result := make([]lineGroup, len(rows))

	for i, row := range rows {
		isHeader := i == 0 && region.hasHeader
		result[i] = lineGroup{
			items:      row,
			isTableRow: true,
			isHeader:   isHeader,
			columns:    region.columns,
		}
	}

	return result
}

// tableRegion represents a detected table area on the page
type tableRegion struct {
	minY, maxY float64
	columns    []float64
	hasHeader  bool // whether this table has a detected header row
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
	headerY, columns := c.findTableHeader(items, mostUsedDistance)
	if headerY >= 0 && len(columns) >= 2 {
		// Find items that belong to this table (after header, with matching column structure)
		// Exclude items near bottom of page (likely footer) using dynamic threshold
		var tableItems []*models.TextItem
		for _, item := range items {
			if strings.TrimSpace(item.Text) == "" {
				continue
			}
			if item.Y >= headerY && item.Y < footerThreshold {
				tableItems = append(tableItems, item)
			}
		}

		var maxY float64 = headerY
		for _, item := range tableItems {
			if item.Y > maxY {
				maxY = item.Y
			}
		}

		// Validate items-to-columns ratio to avoid false positives on paragraph text
		if !c.validateTableStructure(tableItems, columns, mostUsedDistance) {
			// Skip - this looks like paragraph text, not a table
		} else if len(tableItems) >= 3 && c.isKnownTableHeader(tableItems, columns) {
			// Method 1a: Known table header pattern (like "Version Date Description")
			// This handles continuation tables that span multiple pages
			regions = append(regions, &tableRegion{
				minY:      headerY,
				maxY:      maxY,
				columns:   columns,
				hasHeader: true,
			})
		} else if len(tableItems) >= 6 && c.detectMultiLineCells(tableItems, columns, mostUsedDistance) {
			// Method 1b: Traditional multi-line cell detection
			regions = append(regions, &tableRegion{
				minY:      headerY,
				maxY:      maxY,
				columns:   columns,
				hasHeader: true,
			})
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
		alignedRegions := c.detectAlignedTables(items, mostUsedDistance, footerThreshold)
		regions = append(regions, alignedRegions...)
	}

	return regions
}

// isKnownTableHeader checks if the items at the header position look like a known table header
// This helps detect continuation tables that may have fewer items
func (c *CompactLines) isKnownTableHeader(items []*models.TextItem, columns []float64) bool {
	// Group items by Y to find the header row
	if len(items) == 0 || len(columns) < 2 {
		return false
	}

	// Find items at the header Y (first Y position)
	minY := items[0].Y
	for _, item := range items {
		if item.Y < minY {
			minY = item.Y
		}
	}

	var headerTexts []string
	for _, item := range items {
		if math.Abs(item.Y-minY) < yPositionTolerance {
			headerTexts = append(headerTexts, strings.TrimSpace(item.Text))
		}
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

	// Find all items in this Y range
	var tableItems []*models.TextItem
	for _, item := range items {
		if strings.TrimSpace(item.Text) == "" {
			continue
		}
		if item.Y >= minY-float64(mostUsedDistance)*2 && item.Y <= maxY+float64(mostUsedDistance) {
			tableItems = append(tableItems, item)
		}
	}

	// Update minY to include any items above the first reference but in the same row
	for _, item := range tableItems {
		if item.Y < minY {
			minY = item.Y
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
//  1. Grouping items into rows by Y position
//  2. Finding rows with 2+ distinct column positions (well-spaced items)
//  3. Looking for sequences of 3+ rows that share consistent column positions
//  4. Verifying significant column width (not just two items on a line)
//  5. Validating that rows have a reasonable item-to-column ratio (to reject paragraph text)
//
// Returns table regions for any detected aligned tables.
func (c *CompactLines) detectAlignedTables(items []*models.TextItem, mostUsedDistance int, footerThreshold float64) []*tableRegion {
	// Group items by Y position into rows
	yThreshold := float64(mostUsedDistance) / 2.0
	if yThreshold < 5 {
		yThreshold = 5
	}

	type rowInfo struct {
		y       float64
		items   []*models.TextItem
		columns []float64 // detected column X positions
	}

	yGroups := make(map[int][]*models.TextItem)
	for _, item := range items {
		if strings.TrimSpace(item.Text) == "" {
			continue
		}
		if item.Y >= footerThreshold {
			continue // Skip footer area
		}
		yBucket := int(item.Y / yThreshold)
		yGroups[yBucket] = append(yGroups[yBucket], item)
	}

	// Convert to sorted rows
	var rows []rowInfo
	var buckets []int
	for bucket := range yGroups {
		buckets = append(buckets, bucket)
	}
	sort.Ints(buckets)

	for _, bucket := range buckets {
		rowItems := yGroups[bucket]
		if len(rowItems) < 2 {
			continue // Need at least 2 items to be a table row
		}

		// Sort by X
		sort.Slice(rowItems, func(i, j int) bool {
			return rowItems[i].X < rowItems[j].X
		})

		// Detect column positions for this row
		var columns []float64
		lastX := float64(-1000)
		for _, item := range rowItems {
			if item.X-lastX >= minColumnSpacing {
				columns = append(columns, item.X)
				lastX = item.X
			}
		}

		if len(columns) >= 2 {
			rows = append(rows, rowInfo{
				y:       float64(bucket) * yThreshold,
				items:   rowItems,
				columns: columns,
			})
		}
	}

	if len(rows) < 3 {
		return nil // Need at least 3 rows to be a table
	}

	// Look for sequences of rows with matching column counts and aligned positions
	var regions []*tableRegion
	startIdx := 0

	for startIdx < len(rows) {
		// Find a run of consecutive rows with consistent columns
		endIdx := startIdx + 1
		baseColumns := rows[startIdx].columns

		for endIdx < len(rows) {
			nextColumns := rows[endIdx].columns
			if !c.columnsMatch(baseColumns, nextColumns) {
				break
			}
			endIdx++
		}

		runLength := endIdx - startIdx
		if runLength >= 3 {
			// Validate: check if this looks like paragraph text being falsely detected as a table
			// In real tables, items per row roughly matches column count
			// In paragraph text, many items are crammed into few "columns"
			totalItems := 0
			for i := startIdx; i < endIdx; i++ {
				totalItems += len(rows[i].items)
			}
			avgItemsPerRow := float64(totalItems) / float64(runLength)
			numColumns := len(rows[startIdx].columns)

			// If average items per row is much higher than column count, it's likely paragraph text
			// Real tables: ~1-2 items per column per row (maybe 3 for wrapped cells)
			// Paragraphs: many items (words) squeezed into few "columns"
			itemsToColumnsRatio := avgItemsPerRow / float64(numColumns)
			if itemsToColumnsRatio > 2.5 {
				// Too many items per column - this looks like paragraph text, not a table
				startIdx = endIdx
				continue
			}

			// Found a valid table sequence
			// Merge column positions from all rows for best accuracy
			var allColumns [][]float64
			for i := startIdx; i < endIdx; i++ {
				allColumns = append(allColumns, rows[i].columns)
			}
			mergedColumns := c.mergeColumnPositions(allColumns)

			// Collect all items in this region for multi-column layout check
			var regionItems []*models.TextItem
			for i := startIdx; i < endIdx; i++ {
				regionItems = append(regionItems, rows[i].items...)
			}

			// Check for 2-column page layout being misdetected as table
			yThreshold := float64(mostUsedDistance) / 2.0
			if yThreshold < 5 {
				yThreshold = 5
			}
			if len(mergedColumns) == 2 && c.looksLikeMultiColumnLayout(regionItems, mergedColumns, yThreshold) {
				startIdx = endIdx
				continue
			}

			// Calculate Y range
			minY := rows[startIdx].y
			maxY := rows[endIdx-1].y
			for i := startIdx; i < endIdx; i++ {
				for _, item := range rows[i].items {
					if item.Y > maxY {
						maxY = item.Y
					}
				}
			}

			regions = append(regions, &tableRegion{
				minY:      minY,
				maxY:      maxY,
				columns:   mergedColumns,
				hasHeader: true, // Treat first row as header
			})
		}

		startIdx = endIdx
	}

	return regions
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

// mergeColumnPositions averages column positions from multiple column sets
func (c *CompactLines) mergeColumnPositions(columnSets [][]float64) []float64 {
	if len(columnSets) == 0 {
		return nil
	}

	numCols := len(columnSets[0])
	result := make([]float64, numCols)

	for i := 0; i < numCols; i++ {
		sum := 0.0
		for _, cols := range columnSets {
			if i < len(cols) {
				sum += cols[i]
			}
		}
		result[i] = sum / float64(len(columnSets))
	}

	return result
}

// detectColumnsFromItems finds column X positions from a set of items
func (c *CompactLines) detectColumnsFromItems(items []*models.TextItem) []float64 {
	// Group items by X position (bucketed)
	xCounts := make(map[int]int)

	for _, item := range items {
		if strings.TrimSpace(item.Text) == "" {
			continue
		}
		bucket := int(item.X / columnBucketSize)
		xCounts[bucket]++
	}

	// Find X positions that have multiple items (potential columns)
	var rawColumns []float64

	var buckets []int
	for bucket := range xCounts {
		buckets = append(buckets, bucket)
	}
	sort.Ints(buckets)

	for _, bucket := range buckets {
		if xCounts[bucket] >= minColumnItemCount {
			rawColumns = append(rawColumns, float64(bucket)*columnBucketSize)
		}
	}

	// Merge columns that are too close together (likely wrapped text, not separate columns)
	// Real table columns should have significant horizontal spacing
	var columns []float64

	for i, col := range rawColumns {
		if i == 0 {
			columns = append(columns, col)
		} else {
			lastCol := columns[len(columns)-1]
			if col-lastCol >= minColumnSpacing {
				columns = append(columns, col)
			}
			// If gap is small, skip this column (it's part of the previous column's content)
		}
	}

	return columns
}

// findTableHeader looks for a row that appears to be a table header
// Returns the Y coordinate of the header and the column X positions
func (c *CompactLines) findTableHeader(items []*models.TextItem, mostUsedDistance int) (float64, []float64) {
	// Group items by Y to find rows
	yGroups := make(map[int][]*models.TextItem)
	yThreshold := float64(mostUsedDistance) / 2.0
	if yThreshold < 5 {
		yThreshold = 5
	}

	for _, item := range items {
		if strings.TrimSpace(item.Text) == "" {
			continue
		}
		// Round Y to nearest threshold
		yBucket := int(item.Y / yThreshold)
		yGroups[yBucket] = append(yGroups[yBucket], item)
	}

	// Look for rows that look like headers:
	// - 2-5 items on the same line
	// - Items are single words (short text, no spaces typically)
	// - Significant X spacing between items (>50 pixels)
	// - Prefer rows with 3+ columns (more likely to be table headers)

	type headerCandidate struct {
		y       float64
		columns []float64
		score   int // Higher is better
	}
	var candidates []headerCandidate

	for yBucket, rowItems := range yGroups {
		if len(rowItems) < 2 || len(rowItems) > 6 {
			continue
		}

		// Sort by X
		sort.Slice(rowItems, func(i, j int) bool {
			return rowItems[i].X < rowItems[j].X
		})

		// Check if items are well-spaced (like column headers)
		var columns []float64
		lastX := float64(-1000)

		for _, item := range rowItems {
			text := strings.TrimSpace(item.Text)
			// Header items should be relatively short (column labels)
			if len(text) > maxHeaderItemLength || len(text) == 0 {
				continue
			}
			// Check spacing from previous column
			if item.X-lastX >= minColumnSpacing {
				columns = append(columns, item.X)
				lastX = item.X
			}
		}

		// Need at least 2 columns with good spacing
		if len(columns) >= 2 {
			headerY := float64(yBucket) * yThreshold
			if c.hasAlignedDataRows(items, columns, headerY, mostUsedDistance) {
				// Score: prefer more columns, and earlier Y positions (tables typically at top of content)
				score := len(columns)*100 - int(headerY/10)
				candidates = append(candidates, headerCandidate{headerY, columns, score})
			}
		}
	}

	// Return the best candidate (highest score)
	if len(candidates) > 0 {
		best := candidates[0]
		for _, c := range candidates[1:] {
			if c.score > best.score {
				best = c
			}
		}
		return best.y, best.columns
	}

	return -1, nil
}

// hasAlignedDataRows checks if there are data rows after the header that align with columns
func (c *CompactLines) hasAlignedDataRows(items []*models.TextItem, columns []float64, headerY float64, mostUsedDistance int) bool {
	alignedRowCount := 0

	// Group items after header by approximate Y
	for _, item := range items {
		if item.Y <= headerY+yPositionTolerance { // Skip header row
			continue
		}
		if strings.TrimSpace(item.Text) == "" {
			continue
		}

		// Check if this item aligns with any column
		for _, col := range columns {
			if math.Abs(item.X-col) < columnAlignmentTolerance {
				alignedRowCount++
				break
			}
		}
	}

	// Need several aligned items to confirm table structure
	return alignedRowCount >= len(columns)*2
}

// detectMultiLineCells determines if a potential table has cells that span multiple lines.
//
// This is a key indicator of table structure: regular paragraphs have consistent line
// spacing, while tables often have cells where text wraps to multiple lines within
// a single logical row.
//
// Algorithm:
//  1. Group items into visual rows using a threshold based on mostUsedDistance
//  2. For each row, calculate the Y span (maxY - minY)
//  3. If the span exceeds 0.8 * mostUsedDistance, check if multiple items align
//     to the same column (indicating wrapped text within a cell)
//  4. Return true if at least 2 rows have multi-line cells
//
// This helps distinguish tables from regular text that happens to have column-like
// alignment (e.g., lists with consistent indentation).
func (c *CompactLines) detectMultiLineCells(items []*models.TextItem, columns []float64, mostUsedDistance int) bool {
	// Group items by approximate row (using a larger threshold)
	rowThreshold := float64(mostUsedDistance) * 2.5
	if rowThreshold < minMultiLineCellThreshold {
		rowThreshold = minMultiLineCellThreshold
	}

	// For each potential row, check if any column has items at multiple Y values
	type rowGroup struct {
		minY, maxY float64
		items      []*models.TextItem
	}

	var rows []rowGroup

	// Sort items by Y
	sorted := make([]*models.TextItem, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Y < sorted[j].Y
	})

	for _, item := range sorted {
		if strings.TrimSpace(item.Text) == "" {
			continue
		}

		added := false
		for i := range rows {
			if item.Y >= rows[i].minY-rowThreshold && item.Y <= rows[i].maxY+rowThreshold {
				rows[i].items = append(rows[i].items, item)
				if item.Y < rows[i].minY {
					rows[i].minY = item.Y
				}
				if item.Y > rows[i].maxY {
					rows[i].maxY = item.Y
				}
				added = true
				break
			}
		}
		if !added {
			rows = append(rows, rowGroup{
				minY:  item.Y,
				maxY:  item.Y,
				items: []*models.TextItem{item},
			})
		}
	}

	// Check if any row has items at significantly different Y values in the same column
	multiLineCount := 0
	for _, row := range rows {
		ySpan := row.maxY - row.minY
		if ySpan > float64(mostUsedDistance)*0.8 {
			// This row spans multiple text lines - check if it's column-aligned
			for _, col := range columns {
				colItems := 0
				for _, item := range row.items {
					if math.Abs(item.X-col) < referenceColumnMinGap {
						colItems++
					}
				}
				if colItems >= 2 {
					multiLineCount++
					break
				}
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
func (c *CompactLines) validateTableStructure(items []*models.TextItem, columns []float64, mostUsedDistance int) bool {
	if len(items) == 0 || len(columns) < 2 {
		return false
	}

	// Group items by Y position into rows
	yThreshold := float64(mostUsedDistance) / 2.0
	if yThreshold < 5 {
		yThreshold = 5
	}

	yGroups := make(map[int]int) // bucket -> item count
	for _, item := range items {
		if strings.TrimSpace(item.Text) == "" {
			continue
		}
		yBucket := int(item.Y / yThreshold)
		yGroups[yBucket]++
	}

	if len(yGroups) == 0 {
		return false
	}

	// Calculate average items per row
	totalItems := 0
	for _, count := range yGroups {
		totalItems += count
	}
	avgItemsPerRow := float64(totalItems) / float64(len(yGroups))

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
	if len(columns) == 2 && c.looksLikeMultiColumnLayout(items, columns, yThreshold) {
		return false
	}

	return true
}

// looksLikeMultiColumnLayout checks if items look like a 2-column page layout
// rather than a table. Signs of page layout:
// - Second column items often start with lowercase (continuing a sentence)
// - Second column items are word fragments (start mid-word)
// - Items that are just punctuation like "-" (hyphenated words split across columns)
func (c *CompactLines) looksLikeMultiColumnLayout(items []*models.TextItem, columns []float64, yThreshold float64) bool {
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
func (c *CompactLines) detectAndSplitMultiColumnLayout(items []*models.TextItem, mostUsedDistance int) ([]*models.TextItem, []*models.TextItem, bool) {
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

	// Check if items cluster into two distinct X regions
	midPoint := minX + pageWidth/2
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

	// Check for centered text: if many items at the same Y span across the midpoint,
	// it's likely centered text (like title pages), not a 2-column layout.
	// Group items by Y and check if any Y group has items on both sides of midpoint.
	yGroups := make(map[int]struct{ left, right int }) // key = Y * 10 (rounded)
	for _, item := range items {
		key := int(item.Y * 10)
		g := yGroups[key]
		if item.X < midPoint {
			g.left++
		} else {
			g.right++
		}
		yGroups[key] = g
	}

	// Count Y groups that span both sides (indicating centered/spanning lines)
	spanningLineCount := 0
	for _, g := range yGroups {
		if g.left > 0 && g.right > 0 {
			spanningLineCount++
		}
	}

	// If more than 30% of Y groups span both sides, it's not a true 2-column layout
	if len(yGroups) > 0 {
		spanningRatio := float64(spanningLineCount) / float64(len(yGroups))
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

// findTableRegion returns the table region containing the given Y coordinate
func (c *CompactLines) findTableRegion(y float64, regions []*tableRegion) *tableRegion {
	for _, region := range regions {
		if y >= region.minY-tableRegionTolerance && y <= region.maxY+tableRegionTolerance {
			return region
		}
	}
	return nil
}

// groupTextItemsByLine groups TextItems by Y coordinate (standard algorithm)
func (c *CompactLines) groupTextItemsByLine(items []*models.TextItem, mostUsedDistance int) [][]*models.TextItem {
	if len(items) == 0 {
		return nil
	}

	// Sort by Y
	sorted := make([]*models.TextItem, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Y < sorted[j].Y
	})

	var lines [][]*models.TextItem
	var currentLine []*models.TextItem

	threshold := float64(mostUsedDistance) / 2.0

	for _, item := range sorted {
		if len(currentLine) > 0 && math.Abs(currentLine[0].Y-item.Y) >= threshold {
			lines = append(lines, currentLine)
			currentLine = nil
		}
		currentLine = append(currentLine, item)
	}

	if len(currentLine) > 0 {
		lines = append(lines, currentLine)
	}

	// Sort items within each line by X coordinate
	for _, line := range lines {
		sort.Slice(line, func(i, j int) bool {
			return line[i].X < line[j].X
		})
	}

	return lines
}

// detectColumns finds consistent X positions that indicate table columns
func (c *CompactLines) detectColumns(items []*models.TextItem) []float64 {
	// Count how many items start at each X position (bucketed)
	xCounts := make(map[int]int)

	for _, item := range items {
		bucket := int(item.X / columnBucketSize)
		xCounts[bucket]++
	}

	// Find X positions that have multiple items (potential columns)
	var columns []float64

	// Get buckets sorted by X
	var buckets []int
	for bucket := range xCounts {
		buckets = append(buckets, bucket)
	}
	sort.Ints(buckets)

	for _, bucket := range buckets {
		if xCounts[bucket] >= minColumnItemCount {
			columns = append(columns, float64(bucket)*columnBucketSize)
		}
	}

	return columns
}

// visualRow represents a visual table row with Y boundaries and items
type visualRow struct {
	minY, maxY float64
	items      []*models.TextItem
}

// addItemToRow adds an item to a visual row and updates Y boundaries
func (row *visualRow) addItemToRow(item *models.TextItem) {
	row.items = append(row.items, item)
	if item.Y < row.minY {
		row.minY = item.Y
	}
	if item.Y > row.maxY {
		row.maxY = item.Y
	}
}

// isInYRange checks if an item's Y position is within the row's threshold range
func (row *visualRow) isInYRange(itemY, threshold float64) bool {
	return itemY >= row.minY-threshold && itemY <= row.maxY+threshold
}

// checkColumnOverlap determines if item's column exists in the row and if other columns exist
func (c *CompactLines) checkColumnOverlap(item *models.TextItem, row *visualRow, columns []float64) (sameColExists, diffColExists bool) {
	itemCol := c.getColumn(item.X, columns)
	for _, existing := range row.items {
		if c.getColumn(existing.X, columns) == itemCol {
			sameColExists = true
		} else {
			diffColExists = true
		}
	}
	return
}

// tryAddItemToRow attempts to add an item to an existing row based on Y proximity and column overlap.
// Returns true if item was added, false if it should start a new row.
func (c *CompactLines) tryAddItemToRow(item *models.TextItem, row *visualRow, columns []float64, rowThreshold float64) bool {
	if !row.isInYRange(item.Y, rowThreshold) {
		return false
	}

	sameColExists, diffColExists := c.checkColumnOverlap(item, row, columns)

	// Same column - this is likely a multi-line cell, be more permissive
	if sameColExists {
		row.addItemToRow(item)
		return true
	}

	// Different column - check if Y span is in a reasonable range
	if diffColExists {
		ySpan := row.maxY - row.minY
		if ySpan < rowThreshold*1.2 {
			row.addItemToRow(item)
			return true
		}
	}

	return false
}

// sortRowItems sorts items within a row by column, then Y, then X
func (c *CompactLines) sortRowItems(items []*models.TextItem, columns []float64) {
	sort.Slice(items, func(i, j int) bool {
		colI := c.getColumn(items[i].X, columns)
		colJ := c.getColumn(items[j].X, columns)
		if colI != colJ {
			return colI < colJ
		}
		// Within same column, sort by Y first
		if math.Abs(items[i].Y-items[j].Y) > yTolerance {
			return items[i].Y < items[j].Y
		}
		// Within same Y, sort by X
		return items[i].X < items[j].X
	})
}

// groupAsTable groups items as table rows, combining items in the same visual row.
//
// Algorithm:
//  1. Sort items by Y position
//  2. For each item, try to add it to an existing row based on Y proximity
//  3. Items in the same column can span wider Y ranges (multi-line cells)
//  4. Items in different columns must be in a tighter Y range (same visual row)
//  5. Sort items within each row by column, then Y, then X
//
// The rowThreshold determines how far apart items can be and still be considered
// part of the same row. It's based on mostUsedDistance (typical line spacing).
func (c *CompactLines) groupAsTable(items []*models.TextItem, columns []float64, mostUsedDistance int) [][]*models.TextItem {
	// Sort items by Y first
	sorted := make([]*models.TextItem, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Y < sorted[j].Y
	})

	// Calculate row threshold for grouping
	rowThreshold := float64(mostUsedDistance) * 3.0
	if rowThreshold < minRowThreshold {
		rowThreshold = minRowThreshold
	}

	var rows []visualRow

	for _, item := range sorted {
		added := false
		for i := range rows {
			if c.tryAddItemToRow(item, &rows[i], columns, rowThreshold) {
				added = true
				break
			}
		}

		if !added {
			rows = append(rows, visualRow{
				minY:  item.Y,
				maxY:  item.Y,
				items: []*models.TextItem{item},
			})
		}
	}

	// Convert to result format with sorted items
	result := make([][]*models.TextItem, len(rows))
	for i, row := range rows {
		c.sortRowItems(row.items, columns)
		result[i] = row.items
	}

	return result
}

// sharesColumn checks if an item shares a column with any items in the list
func (c *CompactLines) sharesColumn(item *models.TextItem, items []*models.TextItem, columns []float64) bool {
	itemCol := c.getColumn(item.X, columns)
	for _, other := range items {
		if c.getColumn(other.X, columns) == itemCol {
			return true
		}
	}
	return false
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

func (c *CompactLines) groupByLine(items []interface{}, mostUsedDistance int) [][]*models.TextItem {
	var lines [][]*models.TextItem
	var currentLine []*models.TextItem

	threshold := float64(mostUsedDistance) / 2.0

	for _, item := range items {
		textItem, ok := item.(*models.TextItem)
		if !ok {
			continue
		}

		if len(currentLine) > 0 && math.Abs(currentLine[0].Y-textItem.Y) >= threshold {
			lines = append(lines, currentLine)
			currentLine = nil
		}
		currentLine = append(currentLine, textItem)
	}

	if len(currentLine) > 0 {
		lines = append(lines, currentLine)
	}

	// Sort items within each line by X coordinate
	for _, line := range lines {
		sort.Slice(line, func(i, j int) bool {
			return line[i].X < line[j].X
		})
	}

	return lines
}

func (c *CompactLines) compactLine(textItems []*models.TextItem, fontToFormats map[string]*models.WordFormat) *models.LineItem {
	if len(textItems) == 0 {
		return nil
	}

	// Note: Items should already be in correct order from caller
	// - groupAsTable sorts by column, then Y, then X
	// - groupByLine and groupTextItemsByLine sort by X within each Y-group
	// DO NOT re-sort here as it would break the column ordering for table cells

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

func (c *CompactLines) itemsToWords(items []*models.TextItem, fontToFormats map[string]*models.WordFormat) []*models.Word {
	return c.itemsToWordsWithContext(items, fontToFormats, 12.0) // Default font size
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

	// Calculate dynamic thresholds based on font size
	// Word space is typically 0.2-0.33 em, so ~2.5-4 times font size for pixel gap
	// Use a more conservative threshold (3x font size) to avoid false breaks
	wordSpaceThreshold := avgFontSize * 3.0

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
				// Check if we're on a different Y line (cell wrap in table)
				yDistance := math.Abs(item.Y - lastItem.Y)
				if yDistance > yLineWrapThreshold {
					// Different line - check if this is a word continuation
					prevText := strings.TrimSpace(lastItem.Text)
					nextTextTrimmed := strings.TrimSpace(textToAdd)

					// Check for hyphen continuation
					if strings.HasSuffix(prevText, "-") || strings.HasSuffix(prevText, "–") {
						// Hyphen at end of line = continuation, no space needed
					} else if c.isWordContinuation(prevText, nextTextTrimmed) {
						// Likely a word continuation (e.g., "AutoUpda" + "te")
						// No space needed
					} else {
						// Normal line wrap - add space
						text.WriteString(" ")
						endsWithSpace = true
					}
				} else {
					// Same line - check for word boundary using font-relative threshold
					// Use corrected width if the stored width seems wrong
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

// getEffectiveWidth returns a reasonable width for the text item
// If the stored width seems wrong (too small for the text length), estimate based on font size
func (c *CompactLines) getEffectiveWidth(item *models.TextItem, avgFontSize float64) float64 {
	if item.Width <= 0 {
		// No width, estimate from text length
		return float64(len(item.Text)) * avgFontSize * 0.5
	}

	// Check if width is plausible
	// Average character width is roughly 0.5 * font size
	// Minimum expected width = text length * fontSize * 0.3 (for narrow fonts)
	textLen := len(strings.TrimSpace(item.Text))
	if textLen == 0 {
		return item.Width
	}

	minExpectedWidth := float64(textLen) * avgFontSize * 0.3
	if item.Width < minExpectedWidth {
		// Width seems too small, use estimated width instead
		return float64(textLen) * avgFontSize * 0.5
	}

	return item.Width
}

// needsSpaceBetween determines if a space should be inserted between two text items
func (c *CompactLines) needsSpaceBetween(prevText, nextText string, xDistance float64) bool {
	return c.needsSpaceBetweenWithThreshold(prevText, nextText, xDistance, alphanumericGapThreshold)
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

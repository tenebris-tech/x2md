package transform

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/tenebris-tech/x2md/pdf2md/models"
)

// CompactLines groups text items on the same Y into lines
type CompactLines struct{}

// NewCompactLines creates a new CompactLines transformation
func NewCompactLines() *CompactLines {
	return &CompactLines{}
}

// lineGroup holds text items for a line along with table metadata
type lineGroup struct {
	items       []*models.TextItem
	isTableRow  bool
	isHeader    bool
	columns     []float64 // column X positions for table rows
}

// Transform groups text items into lines
func (c *CompactLines) Transform(result *models.ParseResult) *models.ParseResult {
	mostUsedDistance := result.Globals.MostUsedDistance
	fontToFormats := result.Globals.FontToFormats

	for _, page := range result.Pages {
		if len(page.Items) == 0 {
			continue
		}

		// Detect table regions and group accordingly
		groupedLines := c.groupByLineWithTableDetection(page.Items, mostUsedDistance)

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

// extractColumnTexts extracts text for each column from the items
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
				if math.Abs(colItems[a].Y-colItems[b].Y) > 2 {
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
func (c *CompactLines) groupByLineWithTableDetection(items []interface{}, mostUsedDistance int) []lineGroup {
	// Convert to TextItems
	var textItems []*models.TextItem
	for _, item := range items {
		if ti, ok := item.(*models.TextItem); ok {
			textItems = append(textItems, ti)
		}
	}

	if len(textItems) == 0 {
		return nil
	}

	// Identify table regions on the page
	tableRegions := c.detectTableRegions(textItems, mostUsedDistance)

	if len(tableRegions) == 0 {
		// No tables detected, use standard Y-based grouping
		lines := c.groupByLine(items, mostUsedDistance)
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

// groupAsTableWithMetadata groups items as table rows and returns lineGroup with metadata
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

// detectTableRegions finds regions that appear to be tables
func (c *CompactLines) detectTableRegions(items []*models.TextItem, mostUsedDistance int) []*tableRegion {
	var regions []*tableRegion

	// Method 1: Look for traditional header-based tables
	// (rows where multiple items start at distinct X positions like "Version Date Description")
	headerY, columns := c.findTableHeader(items, mostUsedDistance)
	if headerY >= 0 && len(columns) >= 2 {
		// Find items that belong to this table (after header, with matching column structure)
		// Exclude items near bottom of page (likely footer) - use Y > 700 as threshold
		var tableItems []*models.TextItem
		for _, item := range items {
			if strings.TrimSpace(item.Text) == "" {
				continue
			}
			if item.Y >= headerY && item.Y < 700 {
				tableItems = append(tableItems, item)
			}
		}

		var maxY float64 = headerY
		for _, item := range tableItems {
			if item.Y > maxY {
				maxY = item.Y
			}
		}

		if len(tableItems) >= 3 && c.isKnownTableHeader(tableItems, columns) {
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
		if math.Abs(item.Y-minY) < 5 {
			headerTexts = append(headerTexts, strings.TrimSpace(item.Text))
		}
	}

	// Check for known header patterns
	headerLine := strings.Join(headerTexts, " ")

	// Known patterns for table headers that span pages
	knownHeaders := []string{
		"Version Date Description",
		"Element Evaluator Actions",
		"Requirement Dependency",
	}

	for _, known := range knownHeaders {
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
		if strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]") && len(text) >= 3 && len(text) <= 10 {
			refItems = append(refItems, item)
		}
	}

	// Need at least 3 reference IDs to consider it a table
	if len(refItems) < 3 {
		return nil
	}

	// Check if they're at consistent X positions (same column)
	refX := refItems[0].X
	tolerance := 20.0
	alignedCount := 0
	for _, item := range refItems {
		if math.Abs(item.X-refX) < tolerance {
			alignedCount++
		}
	}

	// At least 80% should be aligned
	if float64(alignedCount)/float64(len(refItems)) < 0.8 {
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
		if item.X > refX+30 && item.Y >= minY-float64(mostUsedDistance)*2 && item.Y <= maxY+float64(mostUsedDistance) {
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

// detectColumnsFromItems finds column X positions from a set of items
func (c *CompactLines) detectColumnsFromItems(items []*models.TextItem) []float64 {
	// Group items by X position (bucketed)
	xCounts := make(map[int]int)
	bucketSize := 15.0

	for _, item := range items {
		if strings.TrimSpace(item.Text) == "" {
			continue
		}
		bucket := int(item.X / bucketSize)
		xCounts[bucket]++
	}

	// Find X positions that have multiple items (potential columns)
	var rawColumns []float64
	minCount := 3

	var buckets []int
	for bucket := range xCounts {
		buckets = append(buckets, bucket)
	}
	sort.Ints(buckets)

	for _, bucket := range buckets {
		if xCounts[bucket] >= minCount {
			rawColumns = append(rawColumns, float64(bucket)*bucketSize)
		}
	}

	// Merge columns that are too close together (likely wrapped text, not separate columns)
	// Real table columns should have significant horizontal spacing (>50 pixels)
	var columns []float64
	minColumnGap := 40.0

	for i, col := range rawColumns {
		if i == 0 {
			columns = append(columns, col)
		} else {
			lastCol := columns[len(columns)-1]
			if col-lastCol >= minColumnGap {
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
			if len(text) > 30 || len(text) == 0 {
				continue
			}
			// Check spacing from previous column
			if item.X-lastX >= 40 { // At least 40 pixels between columns
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
		if item.Y <= headerY+5 { // Skip header row
			continue
		}
		if strings.TrimSpace(item.Text) == "" {
			continue
		}

		// Check if this item aligns with any column
		for _, col := range columns {
			if math.Abs(item.X-col) < 20 { // 20 pixel tolerance
				alignedRowCount++
				break
			}
		}
	}

	// Need several aligned items to confirm table structure
	return alignedRowCount >= len(columns)*2
}

// detectMultiLineCells checks if there are cells spanning multiple Y values
func (c *CompactLines) detectMultiLineCells(items []*models.TextItem, columns []float64, mostUsedDistance int) bool {
	// Group items by approximate row (using a larger threshold)
	rowThreshold := float64(mostUsedDistance) * 2.5
	if rowThreshold < 20 {
		rowThreshold = 20
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
					if math.Abs(item.X-col) < 30 {
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

// findTableRegion returns the table region containing the given Y coordinate
func (c *CompactLines) findTableRegion(y float64, regions []*tableRegion) *tableRegion {
	for _, region := range regions {
		if y >= region.minY-10 && y <= region.maxY+10 {
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
	bucketSize := 15.0 // Group X positions within 15 pixels

	for _, item := range items {
		bucket := int(item.X / bucketSize)
		xCounts[bucket]++
	}

	// Find X positions that have multiple items (potential columns)
	var columns []float64
	minCount := 3 // At least 3 items to consider it a column

	// Get buckets sorted by X
	var buckets []int
	for bucket := range xCounts {
		buckets = append(buckets, bucket)
	}
	sort.Ints(buckets)

	for _, bucket := range buckets {
		if xCounts[bucket] >= minCount {
			columns = append(columns, float64(bucket)*bucketSize)
		}
	}

	return columns
}

// groupAsTable groups items as table rows, combining items in the same visual row
func (c *CompactLines) groupAsTable(items []*models.TextItem, columns []float64, mostUsedDistance int) [][]*models.TextItem {
	// Sort items by Y first
	sorted := make([]*models.TextItem, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Y < sorted[j].Y
	})

	// Group into visual rows - items within a larger threshold that align with columns
	// Use a larger threshold for table rows (items can span multiple text lines within a cell)
	// For reference tables with vertically-centered text, rows can span 3+ lines
	rowThreshold := float64(mostUsedDistance) * 3.0
	if rowThreshold < 35 {
		rowThreshold = 35
	}

	type visualRow struct {
		minY, maxY float64
		items      []*models.TextItem
	}

	var rows []visualRow

	for _, item := range sorted {
		// Check if this item belongs to an existing row
		// It belongs if it's within threshold AND shares a column with existing items
		added := false

		for i := range rows {
			// Check Y proximity - use threshold from both min and max
			if item.Y >= rows[i].minY-rowThreshold && item.Y <= rows[i].maxY+rowThreshold {
				// For items in the same column, allow wider Y span (multi-line cells)
				// For items in different columns, be more strict (same visual row)
				itemCol := c.getColumn(item.X, columns)
				sameColExists := false
				diffColExists := false
				for _, existing := range rows[i].items {
					if c.getColumn(existing.X, columns) == itemCol {
						sameColExists = true
					} else {
						diffColExists = true
					}
				}

				// If this column already has items in this row, add if Y is close
				// If this is a new column in this row, add if we have items from other columns
				if sameColExists {
					// Same column - this is likely a multi-line cell, be more permissive
					rows[i].items = append(rows[i].items, item)
					if item.Y < rows[i].minY {
						rows[i].minY = item.Y
					}
					if item.Y > rows[i].maxY {
						rows[i].maxY = item.Y
					}
					added = true
					break
				} else if diffColExists {
					// Different column - check if Y is in a reasonable range
					ySpan := rows[i].maxY - rows[i].minY
					if ySpan < rowThreshold*1.2 {
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

	// Convert to result format, sorting each row by column, then Y, then X
	var result [][]*models.TextItem
	for _, row := range rows {
		// Sort by column first, then by Y within the same column, then by X within same Y
		sort.Slice(row.items, func(i, j int) bool {
			colI := c.getColumn(row.items[i].X, columns)
			colJ := c.getColumn(row.items[j].X, columns)
			if colI != colJ {
				return colI < colJ
			}
			// Within same column, sort by Y first
			if math.Abs(row.items[i].Y-row.items[j].Y) > 2 {
				return row.items[i].Y < row.items[j].Y
			}
			// Within same Y (±2 pixels), sort by X
			return row.items[i].X < row.items[j].X
		})
		result = append(result, row.items)
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

	// Combine text and detect formatting
	words := c.itemsToWords(textItems, fontToFormats)

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
	// Combine text with spacing
	combinedText := c.combineText(items)

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
	var text strings.Builder
	var lastItem *models.TextItem

	for _, item := range items {
		textToAdd := item.Text

		// If text starts with punctuation that attaches to previous word, trim trailing space
		nextFirst := firstRune(strings.TrimSpace(textToAdd))
		if nextFirst == ',' || nextFirst == '.' || nextFirst == ':' || nextFirst == ';' ||
			nextFirst == ')' || nextFirst == ']' || nextFirst == '}' {
			// Trim trailing space from accumulated text before adding punctuation
			str := text.String()
			if strings.HasSuffix(str, " ") {
				text.Reset()
				text.WriteString(strings.TrimRight(str, " "))
			}
		}

		if !strings.HasSuffix(text.String(), " ") && !strings.HasPrefix(textToAdd, " ") {
			if lastItem != nil {
				// Check if we're on a different Y line (cell wrap in table)
				yDistance := math.Abs(item.Y - lastItem.Y)
				if yDistance > 3 {
					// Different line - check if previous text ends with hyphen (continuation)
					prevText := strings.TrimSpace(lastItem.Text)
					if strings.HasSuffix(prevText, "-") || strings.HasSuffix(prevText, "–") {
						// Hyphen at end of line = continuation, no space needed
					} else {
						// Normal line wrap - add space
						text.WriteString(" ")
					}
				} else {
					// Same line - check for word boundary
					xDistance := item.X - lastItem.X - lastItem.Width
					needsSpace := c.needsSpaceBetween(lastItem.Text, textToAdd, xDistance)
					if needsSpace {
						text.WriteString(" ")
					}
				}
			} else {
				if isListItemCharacter(textToAdd) {
					textToAdd += " "
				}
			}
		}

		text.WriteString(textToAdd)
		lastItem = item
	}

	return text.String()
}

// needsSpaceBetween determines if a space should be inserted between two text items
func (c *CompactLines) needsSpaceBetween(prevText, nextText string, xDistance float64) bool {
	// If negative distance (overlap), no space
	if xDistance < 0 {
		return false
	}

	// Check for punctuation that shouldn't have leading/trailing spaces
	prevLast := lastRune(prevText)
	nextFirst := firstRune(nextText)

	// Hyphens/dashes connect without spaces (check BEFORE gap size)
	if prevLast == '-' || nextFirst == '-' || prevLast == '–' || nextFirst == '–' {
		return false
	}

	// If there's a large gap, add space (but only after checking punctuation)
	if xDistance > 30 {
		return true
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

	// For small gaps, use context: if both are alphanumeric, likely same word
	if xDistance < 15 {
		// Numbers and letters in sequence (like dates "23-Mar-2020")
		if (isAlphanumeric(prevLast) && isAlphanumeric(nextFirst)) ||
			(isAlphanumeric(prevLast) && isPunctuation(nextFirst)) ||
			(isPunctuation(prevLast) && isAlphanumeric(nextFirst)) {
			return false
		}
	}

	// Default: add space for gaps > 10
	return xDistance > 10
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
		r == '(' || r == ')' || r == '[' || r == ']' || r == '{' || r == '}'
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
	return c == '-' || c == '•' || c == '–'
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

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

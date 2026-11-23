/******************************************************************************
 * Copyright (c) 2026 Tenebris Technologies Inc.                              *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package transform

import (
	"math"
	"sort"
	"strings"

	"github.com/tenebris-tech/x2md/pdf2md/models"
)

const (
	// ruleSnap is how far apart line ends and crossings may be and still
	// meet: producers draw borders cell by cell, with line caps and widths
	// that leave ends a little short of, or past, the line they meet
	ruleSnap = 2.0

	// minRuleLength is the shortest rule that can bound a cell; shorter
	// strokes are tick marks or glyph-like decoration
	minRuleLength = 3.0

	// unruledLineAbove and unruledLineBelow give the vertical span of a
	// line of text around its baseline, in text heights, when telling rows
	// without rules between them apart (see splitUnruledRows)
	unruledLineAbove = 0.9
	unruledLineBelow = 0.55

	// textMiddleRatio is how far above the baseline (as a share of the text
	// height) the middle of a line of text is; text items carry the
	// baseline in Y
	textMiddleRatio = 0.35
)

// ruledGrid is a table drawn with rules: column boundaries xs and row
// boundaries ys, both ascending, and for each unit (row, column) of the
// grid the unit at the top left of the cell it belongs to
type ruledGrid struct {
	xs, ys []float64
	anchor [][][2]int
}

// ruledTable is a ruled grid with the text inside it, as table rows
type ruledTable struct {
	minX, minY, maxX, maxY float64
	rows                   []lineGroup
}

// findRuledGrids builds the ruled grids on a page from its rules: connected
// sets of horizontal and vertical rules with at least two rows and two
// columns of cells. Grids are returned in the order of their top edge.
func findRuledGrids(rules []models.Rule) []*ruledGrid {
	var horizontal, vertical []models.Rule
	for _, r := range rules {
		if r.Y1 == r.Y2 && r.X2-r.X1 >= minRuleLength {
			horizontal = append(horizontal, r)
		} else if r.X1 == r.X2 && r.Y2-r.Y1 >= minRuleLength {
			vertical = append(vertical, r)
		}
	}
	if len(horizontal) < 2 || len(vertical) < 2 {
		return nil
	}

	// Connect every horizontal rule with the vertical rules it meets
	n := len(horizontal) + len(vertical)
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra < rb {
			parent[rb] = ra
		} else if rb < ra {
			parent[ra] = rb
		}
	}
	for i, h := range horizontal {
		for j, v := range vertical {
			if v.X1 >= h.X1-ruleSnap && v.X1 <= h.X2+ruleSnap &&
				h.Y1 >= v.Y1-ruleSnap && h.Y1 <= v.Y2+ruleSnap {
				union(i, len(horizontal)+j)
			}
		}
	}

	// Collect the components in index order
	members := make(map[int][]int)
	var roots []int
	for i := 0; i < n; i++ {
		root := find(i)
		if _, seen := members[root]; !seen {
			roots = append(roots, root)
		}
		members[root] = append(members[root], i)
	}

	var grids []*ruledGrid
	for _, root := range roots {
		var hs, vs []models.Rule
		for _, i := range members[root] {
			if i < len(horizontal) {
				hs = append(hs, horizontal[i])
			} else {
				vs = append(vs, vertical[i-len(horizontal)])
			}
		}
		if grid := buildGrid(hs, vs); grid != nil {
			grids = append(grids, grid)
		}
	}
	sort.SliceStable(grids, func(i, j int) bool {
		return grids[i].ys[0] < grids[j].ys[0]
	})
	return grids
}

// buildGrid makes the cells of one connected set of rules, or returns nil if
// they do not form at least two rows and two columns
func buildGrid(hs, vs []models.Rule) *ruledGrid {
	if len(hs) < 2 || len(vs) < 2 {
		return nil
	}
	var xs, ys []float64
	for _, v := range vs {
		xs = append(xs, v.X1)
	}
	for _, h := range hs {
		ys = append(ys, h.Y1)
	}
	xs, ys = snapPositions(xs), snapPositions(ys)
	rows, cols := len(ys)-1, len(xs)-1
	if rows < 1 || cols < 1 {
		return nil
	}

	// A boundary between two units exists where a rule runs along all of
	// it (within the snapping tolerance)
	verticalBoundary := func(r, k int) bool {
		for _, v := range vs {
			if math.Abs(v.X1-xs[k]) <= ruleSnap && v.Y1 <= ys[r]+ruleSnap && v.Y2 >= ys[r+1]-ruleSnap {
				return true
			}
		}
		return false
	}
	horizontalBoundary := func(k, c int) bool {
		for _, h := range hs {
			if math.Abs(h.Y1-ys[k]) <= ruleSnap && h.X1 <= xs[c]+ruleSnap && h.X2 >= xs[c+1]-ruleSnap {
				return true
			}
		}
		return false
	}

	// Units with no boundary between them belong to one (merged) cell
	parent := make([]int, rows*cols)
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra < rb {
			parent[rb] = ra
		} else if rb < ra {
			parent[ra] = rb
		}
	}
	// Two units side by side are one cell only if neither is closed off from
	// the other and both have the same rules above and below (or the same
	// rules left and right, for units one above the other): an open area
	// next to a cell, such as the empty corner above a row of headers, is
	// not part of that cell.
	columnSplit, rowSplit := false, false
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			if c+1 < cols {
				if verticalBoundary(r, c+1) {
					columnSplit = true
				} else if horizontalBoundary(r, c) == horizontalBoundary(r, c+1) &&
					horizontalBoundary(r+1, c) == horizontalBoundary(r+1, c+1) {
					union(r*cols+c, r*cols+c+1)
				}
			}
			if r+1 < rows {
				if horizontalBoundary(r+1, c) {
					rowSplit = true
				} else if verticalBoundary(r, c) == verticalBoundary(r+1, c) &&
					verticalBoundary(r, c+1) == verticalBoundary(r+1, c+1) {
					union(r*cols+c, (r+1)*cols+c)
				}
			}
		}
	}
	// A box, a box split only one way, and lone rules are not tables
	if !columnSplit || !rowSplit {
		return nil
	}

	// The root of each set is its lowest index, the top left unit
	grid := &ruledGrid{xs: xs, ys: ys, anchor: make([][][2]int, rows)}
	for r := 0; r < rows; r++ {
		grid.anchor[r] = make([][2]int, cols)
		for c := 0; c < cols; c++ {
			root := find(r*cols + c)
			grid.anchor[r][c] = [2]int{root / cols, root % cols}
		}
	}
	return grid
}

// snapPositions sorts positions and replaces those within the snapping
// tolerance of each other by their mean
func snapPositions(values []float64) []float64 {
	sort.Float64s(values)
	var result []float64
	for start := 0; start < len(values); {
		end := start + 1
		sum := values[start]
		for end < len(values) && values[end]-values[end-1] <= ruleSnap {
			sum += values[end]
			end++
		}
		result = append(result, sum/float64(end-start))
		start = end
	}
	return result
}

// unitAt returns the grid unit containing a point, or false if the point is
// outside the grid
func (g *ruledGrid) unitAt(x, y float64) (int, int, bool) {
	r := sort.SearchFloat64s(g.ys, y) - 1
	c := sort.SearchFloat64s(g.xs, x) - 1
	if r < 0 || c < 0 || r >= len(g.ys)-1 || c >= len(g.xs)-1 {
		return 0, 0, false
	}
	return r, c, true
}

// itemPoint is the point that places a text item in a cell: where it starts,
// at the middle of its glyphs
func itemPoint(item *models.TextItem) (float64, float64) {
	return item.X + 0.5, item.Y - textMiddleRatio*item.Height
}

// detectRuledTables finds the ruled tables on a page and the text inside
// them. inTable reports, for each item, whether a table took it.
func (c *CompactLines) detectRuledTables(items []*models.TextItem, rules []models.Rule) (tables []*ruledTable, inTable []bool) {
	inTable = make([]bool, len(items))
	for _, grid := range findRuledGrids(rules) {
		rows, cols := len(grid.ys)-1, len(grid.xs)-1

		// Text of each cell, by the cell's top left unit, with the grid
		// column each item starts in
		type placed struct {
			item *models.TextItem
			col  int
		}
		inCell := make([][][]placed, rows)
		for r := range inCell {
			inCell[r] = make([][]placed, cols)
		}
		var taken []int
		for i, item := range items {
			if inTable[i] || strings.TrimSpace(item.Text) == "" {
				continue
			}
			r, col, ok := grid.unitAt(itemPoint(item))
			if !ok {
				continue
			}
			a := grid.anchor[r][col]
			inCell[a[0]][a[1]] = append(inCell[a[0]][a[1]], placed{item, col})
			taken = append(taken, i)
		}

		// A cell's text goes in its first column. When a cell that spans
		// several columns holds text starting in more than one of them, the
		// columns are there without their rules (a table body drawn as one
		// box): each item then goes in the column it starts in.
		cells := make([][][]*models.TextItem, rows)
		for r := range cells {
			cells[r] = make([][]*models.TextItem, cols)
		}
		for r := range inCell {
			for col, cell := range inCell[r] {
				spread := false
				for _, p := range cell {
					if p.col != cell[0].col {
						spread = true
						break
					}
				}
				for _, p := range cell {
					if spread {
						cells[r][p.col] = append(cells[r][p.col], p.item)
					} else {
						cells[r][col] = append(cells[r][col], p.item)
					}
				}
			}
		}

		// Rows of the table: each grid row, split where it holds several
		// rows of text with no rule between them
		var tableRows [][][]*models.TextItem
		for r := 0; r < rows; r++ {
			tableRows = append(tableRows, splitUnruledRows(cells[r])...)
		}

		// Keep the rows and columns that have text
		var usedRows, usedCols []int
		for r, row := range tableRows {
			for _, cell := range row {
				if len(cell) > 0 {
					usedRows = append(usedRows, r)
					break
				}
			}
		}
		for col := 0; col < cols; col++ {
			for _, row := range tableRows {
				if len(row[col]) > 0 {
					usedCols = append(usedCols, col)
					break
				}
			}
		}
		if len(usedRows) < 2 || len(usedCols) < 2 {
			continue
		}

		table := &ruledTable{
			minX: grid.xs[0], maxX: grid.xs[cols],
			minY: grid.ys[0], maxY: grid.ys[rows],
		}
		for n, r := range usedRows {
			group := lineGroup{isTableRow: true, isHeader: n == 0}
			for _, col := range usedCols {
				cellItems := tableRows[r][col]
				group.cells = append(group.cells, c.cellText(cellItems))
				group.items = append(group.items, sortReadingOrder(cellItems)...)
			}
			table.rows = append(table.rows, group)
		}
		for _, i := range taken {
			inTable[i] = true
		}
		tables = append(tables, table)
	}
	return tables, inTable
}

// sortReadingOrder returns items sorted by line, then left to right
func sortReadingOrder(items []*models.TextItem) []*models.TextItem {
	sorted := append([]*models.TextItem(nil), items...)
	sort.SliceStable(sorted, func(a, b int) bool {
		if math.Abs(sorted[a].Y-sorted[b].Y) > yTolerance {
			return sorted[a].Y < sorted[b].Y
		}
		return sorted[a].X < sorted[b].X
	})
	return sorted
}

// cellLines splits the text of a cell into lines, top to bottom, each left
// to right. A line is the items whose baselines are within half a line
// height of its first item.
func cellLines(items []*models.TextItem) [][]*models.TextItem {
	sorted := sortReadingOrder(items)
	var lines [][]*models.TextItem
	for start := 0; start < len(sorted); {
		end := start + 1
		for end < len(sorted) && math.Abs(sorted[end].Y-sorted[start].Y) < 0.5*math.Max(sorted[start].Height, 1) {
			end++
		}
		line := append([]*models.TextItem(nil), sorted[start:end]...)
		sort.SliceStable(line, func(a, b int) bool { return line[a].X < line[b].X })
		lines = append(lines, line)
		start = end
	}
	return lines
}

// cellText joins the text of one table cell: each line left to right, and
// the lines top to bottom
func (c *CompactLines) cellText(items []*models.TextItem) string {
	var text string
	for _, line := range cellLines(items) {
		lineText := strings.TrimSpace(c.combineText(line))
		switch {
		case lineText == "":
		case text == "":
			text = lineText
		case c.lineWrapNeedsSpace(text, lineText):
			text += " " + lineText
		default:
			text += lineText
		}
	}
	return text
}

// splitUnruledRows splits the cells of one grid row into the rows of text
// they hold when the table has no rules between those rows. Each line of
// text spans from unruledLineAbove to unruledLineBelow times its height
// around its baseline. The lines of one paragraph (1.15-1.25 heights apart)
// overlap; rows set further apart leave a gap across all columns. A band of
// lines found in only one column continues the row above it (a second
// paragraph in one cell), so a new row needs text in two columns.
func splitUnruledRows(row [][]*models.TextItem) [][][]*models.TextItem {
	type line struct {
		col        int
		items      []*models.TextItem
		start, end float64
		band       int
	}
	var lines []*line
	for col, cell := range row {
		for _, items := range cellLines(cell) {
			y, h := items[0].Y, 0.0
			for _, item := range items {
				h = math.Max(h, item.Height)
			}
			lines = append(lines, &line{col: col, items: items,
				start: y - unruledLineAbove*h, end: y + unruledLineBelow*h})
		}
	}
	if len(lines) < 2 {
		return [][][]*models.TextItem{row}
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].start < lines[j].start })

	// Bands of overlapping lines, then merged until each has two columns
	type band struct {
		end  float64
		cols map[int]bool
	}
	var bands []*band
	for _, l := range lines {
		last := len(bands) - 1
		if last >= 0 && (l.start <= bands[last].end || len(bands[last].cols) < 2) {
			bands[last].end = math.Max(bands[last].end, l.end)
			bands[last].cols[l.col] = true
			l.band = last
			continue
		}
		bands = append(bands, &band{end: l.end, cols: map[int]bool{l.col: true}})
		l.band = len(bands) - 1
	}
	// A band in one column continues the row above it
	target := make([]int, len(bands))
	for b := range bands {
		target[b] = b
		if b > 0 && len(bands[b].cols) < 2 {
			target[b] = target[b-1]
		}
	}
	count := 0
	index := make([]int, len(bands))
	for b := range bands {
		if target[b] == b {
			index[b] = count
			count++
		} else {
			index[b] = index[target[b]]
		}
	}
	if count < 2 {
		return [][][]*models.TextItem{row}
	}

	result := make([][][]*models.TextItem, count)
	for i := range result {
		result[i] = make([][]*models.TextItem, len(row))
	}
	for _, l := range lines {
		r := index[l.band]
		result[r][l.col] = append(result[r][l.col], l.items...)
	}
	return result
}

// placeRuledTables adds the rows of ruled tables to the lines of the rest of
// the page. Each table stays together, in the order of its rows, and goes
// where its top edge is. splitX is the left edge of the right column when
// the rest of the page was read as two columns (0 otherwise): each table
// then joins the column it starts in.
func placeRuledTables(lines []lineGroup, tables []*ruledTable, splitX float64) []lineGroup {
	if len(tables) == 0 {
		return lines
	}

	type block struct {
		top    float64
		groups []lineGroup
	}
	order := func(blocks []block) []lineGroup {
		sort.SliceStable(blocks, func(i, j int) bool { return blocks[i].top < blocks[j].top })
		var result []lineGroup
		for _, b := range blocks {
			result = append(result, b.groups...)
		}
		return result
	}

	var left, right []block
	for _, line := range lines {
		b := block{top: groupTop(line), groups: []lineGroup{line}}
		if splitX > 0 && len(line.items) > 0 && line.items[0].X >= splitX {
			right = append(right, b)
		} else {
			left = append(left, b)
		}
	}
	for _, t := range tables {
		b := block{top: t.minY, groups: t.rows}
		if splitX > 0 && t.minX >= splitX-ruleSnap {
			right = append(right, b)
		} else {
			left = append(left, b)
		}
	}
	return append(order(left), order(right)...)
}

// groupTop is the Y of a line group's first item
func groupTop(g lineGroup) float64 {
	if len(g.items) == 0 {
		return 0
	}
	return g.items[0].Y
}

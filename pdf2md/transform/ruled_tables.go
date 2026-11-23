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

	// maxRulesPerPage is the most rules a page may have for ruled tables to
	// be looked for: the corpus has at most 237 on a page (99.9% of pages
	// with rules have under 180), so a page with thousands is a drawing
	maxRulesPerPage = 2000

	// maxCellLines is the most lines of text a ruled table cell holds; a
	// "cell" with more is a page column framed by layout rules. The ruled
	// tables of the corpus have at most 27 lines in a cell (median 6); a
	// page column has 45 or more.
	maxCellLines = 30

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

// placedItem is a text item in a ruled grid with the grid column it starts in
type placedItem struct {
	item *models.TextItem
	col  int
}

// ruledTable is a ruled grid with the text inside it, as table rows
type ruledTable struct {
	minX, minY, maxX, maxY float64
	rows                   []lineGroup
}

// findRuledGrids builds the ruled grids on a page from its rules: connected
// sets of horizontal and vertical rules with at least two rows and two
// columns of cells. Grids are returned in the order of their top edge. A
// page with more than maxRulesPerPage rules is a drawing, not tables.
func findRuledGrids(rules []models.Rule) []*ruledGrid {
	if len(rules) > maxRulesPerPage {
		return nil
	}
	var horizontal, vertical []models.Rule
	for _, r := range rules {
		if r.Y1 == r.Y2 && r.X2-r.X1 >= minRuleLength {
			horizontal = append(horizontal, r)
		} else if r.X1 == r.X2 && r.Y2-r.Y1 >= minRuleLength {
			vertical = append(vertical, r)
		}
	}
	if len(horizontal) < 2 || len(vertical) < 1 {
		return nil
	}
	sort.SliceStable(vertical, func(i, j int) bool { return vertical[i].X1 < vertical[j].X1 })

	// Connect every horizontal rule with the vertical rules it meets,
	// searching only the verticals within its horizontal reach
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
		first := sort.Search(len(vertical), func(j int) bool { return vertical[j].X1 >= h.X1-ruleSnap })
		for j := first; j < len(vertical) && vertical[j].X1 <= h.X2+ruleSnap; j++ {
			v := vertical[j]
			if h.Y1 >= v.Y1-ruleSnap && h.Y1 <= v.Y2+ruleSnap {
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
		if len(hs) < 2 || len(vs) < 1 {
			continue
		}
		for _, part := range splitStackedTables(hs, vs) {
			if grid := buildGrid(part.hs, part.vs); grid != nil {
				grids = append(grids, grid)
			}
		}
	}
	sort.SliceStable(grids, func(i, j int) bool {
		return grids[i].ys[0] < grids[j].ys[0]
	})
	return grids
}

// ruleSet is the horizontal and vertical rules of one table
type ruleSet struct {
	hs, vs []models.Rule
}

// splitStackedTables splits connected rules into the tables stacked in them.
// Two tables drawn one above the other share the rule between them; no
// interior column rule crosses that rule, and the columns change there. A
// horizontal rule that no column rule crosses but with the same columns
// above and below (a row spanning the table), or with no column rules on
// one side (a body drawn as one box under ruled headers), does not split it.
func splitStackedTables(hs, vs []models.Rule) []ruleSet {
	var ys []float64
	left, right := hs[0].X1, hs[0].X2
	for _, h := range hs {
		ys = append(ys, h.Y1)
		left, right = math.Min(left, h.X1), math.Max(right, h.X2)
	}
	for _, v := range vs {
		left, right = math.Min(left, v.X1), math.Max(right, v.X1)
	}
	ys = snapPositions(ys)
	interior := func(v models.Rule) bool {
		return v.X1 > left+ruleSnap && v.X1 < right-ruleSnap
	}

	// Candidate split positions: interior row rules no column rule crosses
	cuts := []float64{math.Inf(-1)}
	for _, y := range ys[1 : len(ys)-1] {
		crossed := false
		for _, v := range vs {
			if interior(v) && v.Y1 < y-ruleSnap && v.Y2 > y+ruleSnap {
				crossed = true
				break
			}
		}
		if !crossed {
			cuts = append(cuts, y)
		}
	}
	cuts = append(cuts, math.Inf(1))

	// The interior columns of each band between cuts
	columnsIn := func(top, bottom float64) []float64 {
		var xs []float64
		for _, v := range vs {
			if mid := (v.Y1 + v.Y2) / 2; interior(v) && mid > top && mid < bottom {
				xs = append(xs, v.X1)
			}
		}
		return snapPositions(xs)
	}
	sameColumns := func(a, b []float64) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if math.Abs(a[i]-b[i]) > ruleSnap {
				return false
			}
		}
		return true
	}

	// Keep only the cuts where the columns change
	kept := []float64{cuts[0]}
	for k := 1; k+1 < len(cuts); k++ {
		above := columnsIn(kept[len(kept)-1], cuts[k])
		below := columnsIn(cuts[k], cuts[k+1])
		// A band without column rules is the body of the table above it,
		// drawn as one box
		if len(above) > 0 && len(below) > 0 && !sameColumns(above, below) {
			kept = append(kept, cuts[k])
		}
	}
	kept = append(kept, cuts[len(cuts)-1])
	if len(kept) == 2 {
		return []ruleSet{{hs, vs}}
	}

	// The rule at a cut belongs to the tables on both sides of it
	var parts []ruleSet
	for k := 0; k+1 < len(kept); k++ {
		top, bottom := kept[k], kept[k+1]
		var part ruleSet
		for _, h := range hs {
			if h.Y1 >= top-ruleSnap && h.Y1 <= bottom+ruleSnap {
				part.hs = append(part.hs, h)
			}
		}
		for _, v := range vs {
			if mid := (v.Y1 + v.Y2) / 2; mid > top && mid < bottom {
				part.vs = append(part.vs, v)
			}
		}
		if len(part.hs) >= 2 && len(part.vs) >= 1 {
			parts = append(parts, part)
		}
	}
	return parts
}

// buildGrid makes the cells of one connected set of rules, or returns nil if
// they do not form at least two rows and two columns
func buildGrid(hs, vs []models.Rule) *ruledGrid {
	if len(hs) < 2 || len(vs) < 1 {
		return nil
	}
	// Column boundaries are the vertical rules and, for a table drawn
	// without side borders, the ends of the horizontal rules; row
	// boundaries likewise
	var xs, ys []float64
	minX, maxX := hs[0].X1, hs[0].X2
	for _, h := range hs {
		ys = append(ys, h.Y1)
		minX, maxX = math.Min(minX, h.X1), math.Max(maxX, h.X2)
	}
	minY, maxY := vs[0].Y1, vs[0].Y2
	for _, v := range vs {
		xs = append(xs, v.X1)
		minY, maxY = math.Min(minY, v.Y1), math.Max(maxY, v.Y2)
	}
	xs = appendIfApart(xs, minX, maxX)
	ys = appendIfApart(ys, minY, maxY)
	xs, ys = snapPositions(xs), snapPositions(ys)
	rows, cols := len(ys)-1, len(xs)-1
	if rows < 1 || cols < 1 {
		return nil
	}

	// A boundary between two units exists where a rule runs along all of
	// it (within the snapping tolerance). The rules are indexed by the
	// boundary position they lie on.
	vAt := make([][]models.Rule, len(xs))
	for _, v := range vs {
		if k := nearestPosition(xs, v.X1); k >= 0 {
			vAt[k] = append(vAt[k], v)
		}
	}
	hAt := make([][]models.Rule, len(ys))
	for _, h := range hs {
		if k := nearestPosition(ys, h.Y1); k >= 0 {
			hAt[k] = append(hAt[k], h)
		}
	}
	verticalBoundary := func(r, k int) bool {
		for _, v := range vAt[k] {
			if v.Y1 <= ys[r]+ruleSnap && v.Y2 >= ys[r+1]-ruleSnap {
				return true
			}
		}
		return false
	}
	horizontalBoundary := func(k, c int) bool {
		for _, h := range hAt[k] {
			if h.X1 <= xs[c]+ruleSnap && h.X2 >= xs[c+1]-ruleSnap {
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

// nearestPosition returns the index of the position within the snapping
// tolerance of v, or -1
func nearestPosition(positions []float64, v float64) int {
	k := sort.SearchFloat64s(positions, v)
	best := -1
	for _, i := range []int{k - 1, k} {
		if i >= 0 && i < len(positions) && math.Abs(positions[i]-v) <= ruleSnap &&
			(best < 0 || math.Abs(positions[i]-v) < math.Abs(positions[best]-v)) {
			best = i
		}
	}
	return best
}

// appendIfApart adds the values that are not within the snapping tolerance
// of a position already in the list
func appendIfApart(positions []float64, values ...float64) []float64 {
	n := len(positions)
	for _, v := range values {
		near := false
		for _, p := range positions[:n] {
			if math.Abs(p-v) <= ruleSnap {
				near = true
				break
			}
		}
		if !near {
			positions = append(positions, v)
		}
	}
	return positions
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
		inCell := make([][][]placedItem, rows)
		for r := range inCell {
			inCell[r] = make([][]placedItem, cols)
		}
		var taken []int
		for i, item := range items {
			// Rotated text (a diagonal watermark) is drawn over the table,
			// not in its cells
			if inTable[i] || item.Rotated || strings.TrimSpace(item.Text) == "" {
				continue
			}
			r, col, ok := grid.unitAt(itemPoint(item))
			if !ok {
				continue
			}
			a := grid.anchor[r][col]
			inCell[a[0]][a[1]] = append(inCell[a[0]][a[1]], placedItem{item, col})
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
				spread := spreadAcrossColumns(cell)
				for _, p := range cell {
					if spread {
						cells[r][p.col] = append(cells[r][p.col], p.item)
					} else {
						cells[r][col] = append(cells[r][col], p.item)
					}
				}
			}
		}

		// A first row that is one cell across the whole table is its title,
		// not its header: it stays text above the table
		firstRow, top := 0, grid.ys[0]
		if isTitleRow(grid, inCell[0]) {
			firstRow, top = 1, grid.ys[1]
			titled := make(map[*models.TextItem]bool)
			for _, p := range inCell[0][0] {
				titled[p.item] = true
			}
			kept := taken[:0]
			for _, i := range taken {
				if !titled[items[i]] {
					kept = append(kept, i)
				}
			}
			taken = kept
		}

		// Rows of the table: each grid row, split where it holds several
		// rows of text with no rule between them
		var tableRows [][][]*models.TextItem
		for r := firstRow; r < rows; r++ {
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
		if len(usedRows) < 2 || len(usedCols) < 2 || !holdsTableContent(cells[firstRow:], tableRows) {
			continue
		}

		table := &ruledTable{
			minX: grid.xs[0], maxX: grid.xs[cols],
			minY: top, maxY: grid.ys[rows],
		}
		for n, r := range usedRows {
			group := lineGroup{isTableRow: true, isHeader: n == 0}
			for _, col := range usedCols {
				cellItems := tableRows[r][col]
				group.cells = append(group.cells, c.cellText(cellItems))
				group.items = append(group.items, readingOrder(cellItems)...)
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

// cellText joins the text of one table cell: each line left to right, and
// the lines top to bottom
func (c *CompactLines) cellText(items []*models.TextItem) string {
	var text string
	for _, line := range visualLines(items) {
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
		for _, items := range visualLines(cell) {
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

// spreadAcrossColumns reports whether the text of a cell is laid out in the
// grid's columns: some line of it has items starting in two columns. Lines
// that only start at different columns one below the other (centred text)
// are not.
func spreadAcrossColumns(cell []placedItem) bool {
	var items []*models.TextItem
	colOf := make(map[*models.TextItem]int, len(cell))
	for _, p := range cell {
		items = append(items, p.item)
		colOf[p.item] = p.col
	}
	for _, line := range visualLines(items) {
		for _, item := range line[1:] {
			if colOf[item] != colOf[line[0]] {
				return true
			}
		}
	}
	return false
}

// isTitleRow reports whether the first row of a grid is one cell spanning
// all its columns, holding text that is not laid out in columns
func isTitleRow(grid *ruledGrid, firstRow [][]placedItem) bool {
	if len(grid.ys) < 3 {
		return false
	}
	for _, a := range grid.anchor[0] {
		if a != [2]int{0, 0} {
			return false
		}
	}
	return len(firstRow[0]) > 0 && !spreadAcrossColumns(firstRow[0])
}

// holdsTableContent reports whether the text in a grid is a table's, not a
// page layout framed by rules: no grid cell holds more lines than a table
// cell does (a page column of running text), and at least two rows have text
// in two or more cells (or one, in a grid of three or more columns, such as
// the last row of a table continued from the page before; a framed page
// layout has two)
func holdsTableContent(gridCells [][][]*models.TextItem, rows [][][]*models.TextItem) bool {
	for _, row := range gridCells {
		for _, cell := range row {
			if len(cell) > 0 && len(visualLines(cell)) > maxCellLines {
				return false
			}
		}
	}
	fullRows := 0
	for _, row := range rows {
		filled := 0
		for _, cell := range row {
			if len(cell) > 0 {
				filled++
			}
		}
		if filled >= 2 {
			fullRows++
		}
	}
	return fullRows >= 2 || (fullRows == 1 && len(rows[0]) >= 3)
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

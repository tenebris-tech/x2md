/******************************************************************************
 * Copyright (c) 2026 Tenebris Technologies Inc.                              *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package transform

import (
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/tenebris-tech/x2md/pdf2md/models"
)

const (
	// sameLineTolerance is how far two baselines may be apart, as a share of
	// the smaller of the two text heights, and still be one line. Measured
	// on the corpus (pairs of items, distance over the smaller height):
	// items side by side on one line are 0-0.1 apart, superscripts and
	// footnote marks against their line 0.5-0.8; consecutive lines are at
	// least 1.04 apart in 99% of cases (median 1.15).
	sameLineTolerance = 0.85

	// sameSizeTolerance is the same for two items of about the same height
	// (sameSizeRatio): text of one size on one line shares its baseline (the
	// corpus has such pairs 0-0.2 apart), and a tighter limit keeps the
	// lines of two page columns whose baselines are offset from chaining
	// into one line
	sameSizeTolerance = 0.25
	sameSizeRatio     = 0.85

	// defaultTextHeight stands in for the height of text when no item says
	// what it is
	defaultTextHeight = 12.0

	// raisedMarkOffset is how far above its line's baseline, in the line's
	// text height, a smaller item must sit to be a superscript
	raisedMarkOffset = 0.15

	// glyphAscent and glyphDescent give the vertical extent of a line of
	// text around its baseline (Y), in text heights
	glyphAscent  = 0.8
	glyphDescent = 0.2

	// columnGap is the narrowest gap between two table cells on a line, in
	// text heights. Measured on the corpus: gaps between the items of a line
	// on pages without tables are 0-0.5 heights (word spaces and font
	// changes; 90% under 0.81), between the cells of ruled tables 1.44 or
	// more for 90% of them (median 3.8).
	columnGap = 1.0

	// cellContinuationGap is the largest gap, in text heights, between the
	// extents of two lines in one column that are one cell's text
	cellContinuationGap = 1.0
)

// textHeight returns an item's height, or 1 for an item without one. Code
// that groups items by their heights uses heightOr with the typical height
// of the items instead.
func textHeight(item *models.TextItem) float64 {
	return math.Max(item.Height, 1)
}

// typicalHeight returns the median height of the items that have one, or
// defaultTextHeight when none has
func typicalHeight(items []*models.TextItem) float64 {
	var heights []float64
	for _, item := range items {
		if item.Height > 0 {
			heights = append(heights, item.Height)
		}
	}
	if len(heights) == 0 {
		return defaultTextHeight
	}
	sort.Float64s(heights)
	return heights[len(heights)/2]
}

// heightOr returns an item's height, or typical for an item without one
func heightOr(item *models.TextItem, typical float64) float64 {
	if item.Height > 0 {
		return item.Height
	}
	return typical
}

// sameLine reports whether two baselines belong to one visual line
func sameLine(y1, h1, y2, h2 float64) bool {
	return math.Abs(y1-y2) <= sameLineTolerance*math.Min(h1, h2)
}

// onSameLine reports whether two text items are on one visual line, an item
// without a height taking the other's (or typical, when neither has one)
func onSameLine(a, b *models.TextItem, typical float64) bool {
	ha, hb := heightOr(a, heightOr(b, typical)), heightOr(b, heightOr(a, typical))
	return sameLine(a.Y, ha, b.Y, hb)
}

// visualLines groups text items into the lines they are drawn on, top to
// bottom, each left to right. An item joins the line above it when its
// baseline is within sameLineTolerance of the baseline of the line's tallest
// item, so a superscript joins its line and the next line does not.
func visualLines(items []*models.TextItem) [][]*models.TextItem {
	sorted := append([]*models.TextItem(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Y != sorted[j].Y {
			return sorted[i].Y < sorted[j].Y
		}
		return sorted[i].X < sorted[j].X
	})

	typical := typicalHeight(items)
	var lines [][]*models.TextItem
	var ref *models.TextItem // tallest item of the current line
	for _, item := range sorted {
		if ref != nil && onSameLine(ref, item, typical) && fitsSizes(lines[len(lines)-1], item, typical) {
			last := len(lines) - 1
			lines[last] = append(lines[last], item)
			if heightOr(item, typical) > heightOr(ref, typical) {
				ref = item
			}
			continue
		}
		lines = append(lines, []*models.TextItem{item})
		ref = item
	}

	for _, line := range lines {
		sort.SliceStable(line, func(i, j int) bool { return line[i].X < line[j].X })
	}
	return lines
}

// fitsSizes reports whether an item's baseline agrees with every item of
// about its size already on a line
func fitsSizes(line []*models.TextItem, item *models.TextItem, typical float64) bool {
	h := heightOr(item, typical)
	for _, other := range line {
		oh := heightOr(other, typical)
		small, large := math.Min(h, oh), math.Max(h, oh)
		if small/large > sameSizeRatio && math.Abs(other.Y-item.Y) > sameSizeTolerance*small {
			return false
		}
	}
	return true
}

// readingOrder returns items line by line, each line left to right
func readingOrder(items []*models.TextItem) []*models.TextItem {
	var result []*models.TextItem
	for _, line := range visualLines(items) {
		result = append(result, line...)
	}
	return result
}

// lineTop returns the Y of the highest item in a line
func lineTop(line []*models.TextItem) float64 {
	top := line[0].Y
	for _, item := range line[1:] {
		top = math.Min(top, item.Y)
	}
	return top
}

// tableRows groups the items of a table into its rows. Each visual line is
// split into fragments by column, by runs of text (see columnsOf). Fragments in different columns are in the
// same row when their vertical extents overlap, so a cell centred against a
// neighbour of more or fewer lines, half a line off, still pairs up. A line
// that overlaps nothing in another column is more text of the cell above or
// below it in its own column (whichever is closer, within
// cellContinuationGap), or a row of its own. Rows are returned top to
// bottom, their items ordered by column, then line, then X.
func (c *CompactLines) tableRows(items []*models.TextItem, columns []float64, extents [][2]float64) [][]*models.TextItem {
	colOf := c.columnsOf(items, columns, extents)
	typical := typicalHeight(items)
	type fragment struct {
		col         int
		line        int
		items       []*models.TextItem
		top, bottom float64
		height      float64
	}
	var frags []*fragment
	for li, line := range visualLines(items) {
		byCol := make(map[int]*fragment)
		var order []int
		for _, item := range line {
			col, ok := colOf[item]
			if !ok {
				continue // leader dots and blanks are not cell text
			}
			f, ok := byCol[col]
			if !ok {
				f = &fragment{col: col, line: li, top: math.Inf(1), bottom: math.Inf(-1)}
				byCol[col] = f
				order = append(order, col)
			}
			h := heightOr(item, typical)
			f.items = append(f.items, item)
			f.top = math.Min(f.top, item.Y-glyphAscent*h)
			f.bottom = math.Max(f.bottom, item.Y+glyphDescent*h)
			f.height = math.Max(f.height, h)
		}
		sort.Ints(order)
		for _, col := range order {
			frags = append(frags, byCol[col])
		}
	}
	if len(frags) == 0 {
		return nil
	}

	parent := make([]int, len(frags))
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

	// Pair fragments in different columns whose extents overlap: a sweep
	// in order of top edge, keeping the fragments still open (whose bottom
	// is below the current top)
	order := make([]int, len(frags))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return frags[order[a]].top < frags[order[b]].top })
	paired := make([]bool, len(frags))
	var open []int
	for _, i := range order {
		f := frags[i]
		kept := open[:0]
		for _, j := range open {
			if frags[j].bottom > f.top {
				kept = append(kept, j)
			}
		}
		open = kept
		for _, j := range open {
			if frags[j].col != f.col && frags[j].top < f.bottom {
				union(i, j)
				paired[i], paired[j] = true, true
			}
		}
		open = append(open, i)
	}

	// A fragment paired with nothing continues the nearest cell in its column
	previous := make([]int, len(frags)) // the fragment above in the same column
	next := make([]int, len(frags))     // the fragment below in the same column
	lastInColumn := make(map[int]int)
	for i, f := range frags {
		previous[i], next[i] = -1, -1
		if j, ok := lastInColumn[f.col]; ok {
			previous[i], next[j] = j, i
		}
		lastInColumn[f.col] = i
	}
	for i, f := range frags {
		// A line in a column past the first that starts left of the column
		// is a title over the next rows, not more text of a cell
		if paired[i] || (f.col > 0 && f.col < len(columns) && f.items[0].X < columns[f.col]-exactAlignment) {
			continue
		}
		above, below := previous[i], next[i]
		limit := cellContinuationGap * f.height
		gapAbove, gapBelow := math.Inf(1), math.Inf(1)
		if above >= 0 {
			gapAbove = f.top - frags[above].bottom
		}
		if below >= 0 {
			gapBelow = frags[below].top - f.bottom
		}
		switch {
		case gapAbove <= gapBelow && gapAbove <= limit:
			union(i, above)
		case gapBelow < gapAbove && gapBelow <= limit:
			union(i, below)
		}
	}

	// Collect rows in order of their first fragment
	rowOf := make(map[int]int)
	var rows [][]*fragment
	for i, f := range frags {
		root := find(i)
		r, ok := rowOf[root]
		if !ok {
			r = len(rows)
			rowOf[root] = r
			rows = append(rows, nil)
		}
		rows[r] = append(rows[r], f)
	}

	result := make([][]*models.TextItem, len(rows))
	for r, row := range rows {
		sort.SliceStable(row, func(i, j int) bool {
			if row[i].col != row[j].col {
				return row[i].col < row[j].col
			}
			return row[i].line < row[j].line
		})
		for _, f := range row {
			result[r] = append(result[r], f.items...)
		}
	}
	return result
}

// isRaisedMark reports whether an item is a superscript on its line, such as
// a footnote mark: smaller than the line's tallest text and raised above its
// baseline
func isRaisedMark(item *models.TextItem, line []*models.TextItem) bool {
	tallest := line[0]
	for _, other := range line[1:] {
		if textHeight(other) > textHeight(tallest) {
			tallest = other
		}
	}
	h := textHeight(tallest)
	return textHeight(item) < sameSizeRatio*h && item.Y < tallest.Y-raisedMarkOffset*h
}

// textRun is text on one line drawn without a column gap: one table cell's
// text on that line, or a whole line of running text
type textRun struct {
	items  []*models.TextItem
	x, end float64
}

// text returns the run's text, its items joined
func (r textRun) text() string {
	var parts []string
	for _, item := range r.items {
		parts = append(parts, strings.TrimSpace(item.Text))
	}
	return strings.Join(parts, " ")
}

// isLeader reports whether an item is only leader dots (as between a title
// and its page number in a table of contents)
func isLeader(item *models.TextItem) bool {
	return leaderPattern.MatchString(strings.TrimSpace(item.Text))
}

var (
	leaderPattern         = regexp.MustCompile(`^(?:[.·…‥]\s*){3,}$|^…+$`)
	trailingLeaderPattern = regexp.MustCompile(`\s*(?:[.·…‥]\s*){3,}$`)
)

// trimLeader removes leader dots from the end of a cell's text, unless they
// are all of it (a cell whose value is "…")
func trimLeader(text string) string {
	if trimmed := strings.TrimSpace(trailingLeaderPattern.ReplaceAllString(text, "")); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(text)
}

// lineRuns splits a visual line into runs of text: a new run starts where
// the gap from the end of the text before it is at least columnGap text
// heights. Blank items, and leader dots between a title and its page
// number, are not text: they are part of the gap.
func lineRuns(line []*models.TextItem) []textRun {
	typical := typicalHeight(line)
	isText := func(item *models.TextItem) bool { return strings.TrimSpace(item.Text) != "" }

	// Leader dots are filler only between text on both sides of them; dots
	// that are a line's only text, or that end it, are a value
	firstText, lastText := -1, -1
	for i, item := range line {
		if isText(item) && !isLeader(item) {
			if firstText < 0 {
				firstText = i
			}
			lastText = i
		}
	}

	var runs []textRun
	var runHeight float64
	for i, item := range line {
		if !isText(item) || (isLeader(item) && i > firstText && i < lastText) {
			continue
		}
		end := item.X + item.Width
		h := heightOr(item, typical)
		if n := len(runs); n > 0 {
			last := &runs[n-1]
			if item.X-last.end < columnGap*math.Max(runHeight, h) {
				last.items = append(last.items, item)
				last.end = math.Max(last.end, end)
				runHeight = math.Max(runHeight, h)
				continue
			}
		}
		runs = append(runs, textRun{items: []*models.TextItem{item}, x: item.X, end: end})
		runHeight = h
	}
	return runs
}

// runColumn returns the column a run of text belongs to: with column
// extents (where each column's text lies), the one it overlaps most, so
// that right-aligned and centred text find their column; otherwise the
// column it starts in
func (c *CompactLines) runColumn(r textRun, columns []float64, extents [][2]float64) int {
	best, bestOverlap := -1, 0.0
	for k, e := range extents {
		if overlap := math.Min(r.end, e[1]) - math.Max(r.x, e[0]); overlap > bestOverlap {
			best, bestOverlap = k, overlap
		}
	}
	if best >= 0 {
		return best
	}
	return c.getColumn(r.x, columns)
}

// runCrosses reports whether a run of text runs across a column boundary:
// the start of a column lies well inside it. Table cells keep to their
// column; running text crosses.
func runCrosses(r textRun, columns []float64) bool {
	for _, start := range columns[1:] {
		if start > r.x+columnAlignmentTolerance && start < r.end-columnAlignmentTolerance {
			return true
		}
	}
	return false
}

// columnsOf assigns the items of a table to columns run by run, so the
// items of one cell's text on a line stay in one column. Leader dots and
// blank items get no column.
func (c *CompactLines) columnsOf(items []*models.TextItem, columns []float64, extents [][2]float64) map[*models.TextItem]int {
	colOf := make(map[*models.TextItem]int, len(items))
	for _, line := range visualLines(items) {
		for _, r := range lineRuns(line) {
			col := c.runColumn(r, columns, extents)
			for _, item := range r.items {
				colOf[item] = col
			}
		}
	}
	return colOf
}

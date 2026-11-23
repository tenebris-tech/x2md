/******************************************************************************
 * Copyright (c) 2026 Tenebris Technologies Inc.                              *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package pdf

import (
	"math"
	"sort"
)

const (
	// ruleAxisTolerance is how far the two ends of a painted segment may be
	// apart across its direction and still count as horizontal or vertical
	ruleAxisTolerance = 0.5

	// ruleMaxThickness is the thickest filled rectangle that is drawn as a
	// line rather than as an area
	ruleMaxThickness = 1.5

	// ruleMergeGap is the largest gap between two collinear rules that are
	// joined into one (a border drawn cell by cell leaves small gaps)
	ruleMergeGap = 1.0
)

// Rule is a straight horizontal or vertical line painted on a page, in the
// coordinates of text items (Y grows downward). X1 <= X2 and Y1 <= Y2; a
// horizontal rule has Y1 == Y2 and a vertical rule X1 == X2.
type Rule struct {
	X1, Y1, X2, Y2 float64
}

// Horizontal reports whether the rule is horizontal
func (r Rule) Horizontal() bool {
	return r.Y1 == r.Y2
}

// pathPoint is a point of the current path in page coordinates
type pathPoint struct {
	x, y float64
}

// subpath is one subpath of the current path. straight[i] tells whether the
// segment ending at points[i] is a straight line (false for curves; the
// first entry belongs to the starting point and is unused).
type subpath struct {
	points   []pathPoint
	straight []bool
	closed   bool
}

// pathBuilder holds the path under construction in a content stream
type pathBuilder struct {
	subpaths []subpath
}

// pathPoint maps a user space point to page coordinates
func (e *TextExtractor) pagePoint(x, y float64, gs *GraphicsState, mediaBox [4]float64) pathPoint {
	ctm := gs.CTM
	px := x*ctm[0] + y*ctm[2] + ctm[4]
	py := x*ctm[1] + y*ctm[3] + ctm[5]
	return pathPoint{x: px, y: (mediaBox[3] - mediaBox[1]) - py}
}

// lastFloats returns the last n operands as numbers, or false if there are
// fewer than n or they are not all numbers
func (e *TextExtractor) lastFloats(operands []interface{}, n int) ([]float64, bool) {
	if len(operands) < n {
		return nil, false
	}
	values := make([]float64, n)
	for i, op := range operands[len(operands)-n:] {
		v, ok := op.(float64)
		if !ok {
			return nil, false
		}
		values[i] = v
	}
	return values, true
}

// pathOperator applies a path construction or painting operator. It returns
// false for any other operator.
func (e *TextExtractor) pathOperator(op string, operands []interface{}, gs *GraphicsState, mediaBox [4]float64) bool {
	p := &e.path
	current := func() *subpath {
		if len(p.subpaths) == 0 {
			return nil
		}
		return &p.subpaths[len(p.subpaths)-1]
	}

	switch op {
	case "m":
		if v, ok := e.lastFloats(operands, 2); ok {
			p.subpaths = append(p.subpaths, subpath{
				points:   []pathPoint{e.pagePoint(v[0], v[1], gs, mediaBox)},
				straight: []bool{false},
			})
		}
	case "l", "c", "v", "y":
		n := map[string]int{"l": 2, "c": 6, "v": 4, "y": 4}[op]
		v, ok := e.lastFloats(operands, n)
		if !ok {
			return true
		}
		end := e.pagePoint(v[n-2], v[n-1], gs, mediaBox)
		sp := current()
		if sp == nil || sp.closed {
			// A segment with no current point starts a new subpath
			p.subpaths = append(p.subpaths, subpath{points: []pathPoint{end}, straight: []bool{false}})
			return true
		}
		sp.points = append(sp.points, end)
		sp.straight = append(sp.straight, op == "l")
	case "h":
		if sp := current(); sp != nil {
			sp.closed = true
		}
	case "re":
		if v, ok := e.lastFloats(operands, 4); ok {
			x, y, w, h := v[0], v[1], v[2], v[3]
			p.subpaths = append(p.subpaths, subpath{
				points: []pathPoint{
					e.pagePoint(x, y, gs, mediaBox),
					e.pagePoint(x+w, y, gs, mediaBox),
					e.pagePoint(x+w, y+h, gs, mediaBox),
					e.pagePoint(x, y+h, gs, mediaBox),
				},
				straight: []bool{false, true, true, true},
				closed:   true,
			})
		}
	case "S", "s":
		e.strokePath(op == "s")
		p.subpaths = nil
	case "f", "F", "f*":
		e.fillPath()
		p.subpaths = nil
	case "B", "B*", "b", "b*":
		close := op == "b" || op == "b*"
		e.strokePath(close)
		e.fillPath()
		p.subpaths = nil
	case "n":
		// Ends the path without painting it (after W or W* it only clips)
		p.subpaths = nil
	case "W", "W*":
		// Clipping does not paint; the operator that follows decides
	default:
		return false
	}
	return true
}

// strokePath records the straight horizontal and vertical segments of the
// current path as rules
func (e *TextExtractor) strokePath(closeAll bool) {
	for _, sp := range e.path.subpaths {
		for i := 1; i < len(sp.points); i++ {
			if sp.straight[i] {
				e.addRule(sp.points[i-1], sp.points[i])
			}
		}
		if (sp.closed || closeAll) && len(sp.points) > 2 {
			e.addRule(sp.points[len(sp.points)-1], sp.points[0])
		}
	}
}

// fillPath records each filled rectangle thin enough to be a line as a rule
// along its length. Wider filled areas are shading, not rules.
func (e *TextExtractor) fillPath() {
	for _, sp := range e.path.subpaths {
		minX, minY, maxX, maxY, ok := axisRectangle(sp)
		if !ok {
			continue
		}
		w, h := maxX-minX, maxY-minY
		switch {
		case h <= ruleMaxThickness && w > h:
			y := (minY + maxY) / 2
			e.addRule(pathPoint{minX, y}, pathPoint{maxX, y})
		case w <= ruleMaxThickness && h > w:
			x := (minX + maxX) / 2
			e.addRule(pathPoint{x, minY}, pathPoint{x, maxY})
		}
	}
}

// axisRectangle returns the bounds of a subpath that is a rectangle with
// horizontal and vertical sides
func axisRectangle(sp subpath) (minX, minY, maxX, maxY float64, ok bool) {
	points := sp.points
	// A closing point equal to the start is not a corner
	if n := len(points); n == 5 && math.Abs(points[4].x-points[0].x) <= ruleAxisTolerance &&
		math.Abs(points[4].y-points[0].y) <= ruleAxisTolerance {
		points = points[:4]
	}
	if len(points) != 4 {
		return 0, 0, 0, 0, false
	}
	for i := 1; i < len(sp.straight) && i < 4; i++ {
		if !sp.straight[i] {
			return 0, 0, 0, 0, false
		}
	}
	for i := range points {
		a, b := points[i], points[(i+1)%4]
		if math.Abs(a.x-b.x) > ruleAxisTolerance && math.Abs(a.y-b.y) > ruleAxisTolerance {
			return 0, 0, 0, 0, false
		}
	}
	minX, minY = points[0].x, points[0].y
	maxX, maxY = minX, minY
	for _, pt := range points[1:] {
		minX, maxX = math.Min(minX, pt.x), math.Max(maxX, pt.x)
		minY, maxY = math.Min(minY, pt.y), math.Max(maxY, pt.y)
	}
	return minX, minY, maxX, maxY, true
}

// addRule records the segment a-b if it is horizontal or vertical
func (e *TextExtractor) addRule(a, b pathPoint) {
	dx, dy := math.Abs(a.x-b.x), math.Abs(a.y-b.y)
	switch {
	case dy <= ruleAxisTolerance && dx > ruleAxisTolerance:
		y := (a.y + b.y) / 2
		e.rules = append(e.rules, Rule{X1: math.Min(a.x, b.x), Y1: y, X2: math.Max(a.x, b.x), Y2: y})
	case dx <= ruleAxisTolerance && dy > ruleAxisTolerance:
		x := (a.x + b.x) / 2
		e.rules = append(e.rules, Rule{X1: x, Y1: math.Min(a.y, b.y), X2: x, Y2: math.Max(a.y, b.y)})
	}
}

// Rules returns the horizontal and vertical lines painted on the page last
// extracted, with collinear rules that touch or overlap joined into one.
// Horizontal rules come first, each group sorted by position.
func (e *TextExtractor) Rules() []Rule {
	var horizontal, vertical []Rule
	for _, r := range e.rules {
		if r.Horizontal() {
			horizontal = append(horizontal, r)
		} else {
			// Merge vertical rules as horizontal ones with X and Y swapped
			vertical = append(vertical, Rule{X1: r.Y1, Y1: r.X1, X2: r.Y2, Y2: r.X2})
		}
	}
	merged := mergeRules(horizontal)
	for _, r := range mergeRules(vertical) {
		merged = append(merged, Rule{X1: r.Y1, Y1: r.X1, X2: r.Y2, Y2: r.X2})
	}
	return merged
}

// mergeRules joins horizontal rules that lie on the same line and touch or
// overlap
func mergeRules(rules []Rule) []Rule {
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Y1 != rules[j].Y1 {
			return rules[i].Y1 < rules[j].Y1
		}
		if rules[i].X1 != rules[j].X1 {
			return rules[i].X1 < rules[j].X1
		}
		return rules[i].X2 < rules[j].X2
	})

	var result []Rule
	for start := 0; start < len(rules); {
		// Rules within the axis tolerance of the first one form a band
		end := start + 1
		for end < len(rules) && rules[end].Y1-rules[start].Y1 <= ruleAxisTolerance {
			end++
		}
		band := append([]Rule(nil), rules[start:end]...)
		y := 0.0
		for _, r := range band {
			y += r.Y1
		}
		y /= float64(len(band))
		sort.Slice(band, func(i, j int) bool {
			if band[i].X1 != band[j].X1 {
				return band[i].X1 < band[j].X1
			}
			return band[i].X2 < band[j].X2
		})

		cur := Rule{X1: band[0].X1, Y1: y, X2: band[0].X2, Y2: y}
		for _, r := range band[1:] {
			if r.X1 <= cur.X2+ruleMergeGap {
				cur.X2 = math.Max(cur.X2, r.X2)
				continue
			}
			result = append(result, cur)
			cur = Rule{X1: r.X1, Y1: y, X2: r.X2, Y2: y}
		}
		result = append(result, cur)
		start = end
	}
	return result
}

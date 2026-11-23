/******************************************************************************
 * Copyright (c) 2026 Tenebris Technologies Inc.                              *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package docx

import (
	"regexp"
	"strconv"
	"strings"
)

// numberLabel is the rendered number of a numbered paragraph
type numberLabel struct {
	text    string // rendered w:lvlText, e.g. "1.", "1.1", "(a)", "Article 1"
	bullet  bool   // the level's numFmt is bullet
	defined bool   // the numbering definition has this level
}

// plainListLabel matches a decimal number followed by "." or ")", the only
// labels CommonMark takes as an ordered list marker
var plainListLabel = regexp.MustCompile(`^[0-9]+[.)]$`)

// lvlTextPlaceholder matches %1 to %9 in a w:lvlText template
var lvlTextPlaceholder = regexp.MustCompile(`%[1-9]`)

// levelStart returns the number a level starts and restarts at: its
// w:start (from a w:lvl in the num's w:lvlOverride when there is one),
// else 0 (the OOXML default)
func (n *Numbering) levelStart(numID, level int) int {
	if lvl := n.GetLevel(numID, level); lvl != nil && lvl.Start != nil {
		return lvl.Start.Val
	}
	return 0
}

// startOverride returns the num's w:startOverride for a level, if any
func (n *Numbering) startOverride(numID, level int) (int, bool) {
	if override := n.levelOverride(numID, level); override != nil && override.StartOverride != nil {
		return override.StartOverride.Val, true
	}
	return 0, false
}

// restartsAfter reports whether an item at level used restarts deeper level.
// By default a level restarts after any shallower level; w:lvlRestart N
// restarts it only after levels 1..N (1-based), and 0 never restarts it.
func (n *Numbering) restartsAfter(numID, deeper, used int) bool {
	lvl := n.GetLevel(numID, deeper)
	if lvl == nil || lvl.LvlRestart == nil {
		return true
	}
	return used < lvl.LvlRestart.Val
}

// listKey identifies a list: all nums that reference one abstractNum (after
// following w:numStyleLink) form one list with one sequence of numbers, as
// Word numbers them. A num without a definition is a list of its own.
type listKey struct {
	abstractNumID int
	numID         int // only set when the num has no abstractNum
}

// listKeyFor returns the list a num belongs to
func (n *Numbering) listKeyFor(numID int) listKey {
	if abstractNum := n.GetAbstractNum(numID); abstractNum != nil {
		return listKey{abstractNumID: abstractNum.AbstractNumID}
	}
	return listKey{abstractNumID: -1, numID: numID}
}

// numLevel is a level of one num, for tracking its w:startOverride
type numLevel struct{ numID, level int }

// nextNumber advances the list counter at level, restarts the deeper levels
// it governs, and returns the paragraph's rendered label. Headings and list
// items share these counters in document order, across every num of the
// same abstractNum. A num's w:startOverride sets the counter the first time
// that num is used at that level; later items continue from there.
func (e *Extractor) nextNumber(numID, level int) numberLabel {
	key := e.numbering.listKeyFor(numID)
	counters := e.listCounters[key]
	if counters == nil {
		counters = make(map[int]int)
		e.listCounters[key] = counters
	}
	start, overridden := e.numbering.startOverride(numID, level)
	switch v, ok := counters[level]; {
	case overridden && !e.startOverridesApplied[numLevel{numID, level}]:
		e.startOverridesApplied[numLevel{numID, level}] = true
		counters[level] = start
	case ok:
		counters[level] = v + 1
	default:
		counters[level] = e.numbering.levelStart(numID, level)
	}
	for deeper := range counters {
		if deeper > level && e.numbering.restartsAfter(numID, deeper, level) {
			delete(counters, deeper)
		}
	}
	return e.renderNumber(numID, level)
}

// renderNumber fills in the level's w:lvlText template from the current
// counters. Each %n is the number of level n-1 in that level's numFmt, or in
// decimal when the level being rendered has w:isLgl. A level not used yet
// shows its start value.
func (e *Extractor) renderNumber(numID, level int) numberLabel {
	lvl := e.numbering.GetLevel(numID, level)
	if lvl == nil {
		return numberLabel{}
	}
	label := numberLabel{defined: true, bullet: lvl.NumFmt != nil && lvl.NumFmt.Val == "bullet"}
	if label.bullet || lvl.LvlText == nil {
		return label
	}

	legal := lvl.IsLgl.IsTrue()
	label.text = lvlTextPlaceholder.ReplaceAllStringFunc(lvl.LvlText.Val, func(ph string) string {
		ref, _ := strconv.Atoi(ph[1:])
		ref--
		value, ok := e.listCounters[e.numbering.listKeyFor(numID)][ref]
		if !ok {
			value = e.numbering.levelStart(numID, ref)
		}
		format := "decimal"
		if refLvl := e.numbering.GetLevel(numID, ref); refLvl != nil && refLvl.NumFmt != nil {
			format = refLvl.NumFmt.Val
		}
		if legal && format != "none" && format != "bullet" {
			format = "decimal"
		}
		return formatNumber(value, format)
	})
	label.text = strings.TrimSpace(label.text)
	return label
}

// listMarker returns the Markdown that starts a numbered list item. A plain
// "N." / "N)" label is already an ordered-list marker; any other label (such
// as "a)", "iv.", "1.1", "(a)" or "Article 1") is kept verbatim after a bullet
// marker, so the line is still a list item. Bullets, and levels the numbering does not
// define, become "-"; an empty label (numFmt none) gives no marker.
func listMarker(label numberLabel) string {
	switch {
	case label.bullet || !label.defined:
		return "-"
	case label.text == "":
		return ""
	case plainListLabel.MatchString(label.text):
		return label.text
	default:
		return "- " + label.text
	}
}

// headingNumber returns the number shown before a heading's text; bullets
// and empty labels give none
func headingNumber(label numberLabel) string {
	if label.bullet {
		return ""
	}
	return label.text
}

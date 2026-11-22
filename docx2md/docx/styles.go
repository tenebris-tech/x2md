/******************************************************************************
 * Copyright (c) 2025-2026 Tenebris Technologies Inc.                         *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package docx

import (
	"encoding/xml"
	"strconv"
	"strings"
)

// Styles represents the styles definition file (word/styles.xml)
type Styles struct {
	XMLName xml.Name   `xml:"styles"`
	Styles  []StyleDef `xml:"style"`
}

// StyleDef defines a single style
type StyleDef struct {
	XMLName             xml.Name             `xml:"style"`
	Type                string               `xml:"type,attr"`    // paragraph, character, table, numbering
	StyleID             string               `xml:"styleId,attr"` // style identifier
	Default             string               `xml:"default,attr"` // 1 if default style
	Name                *StyleName           `xml:"name"`
	BasedOn             *BasedOn             `xml:"basedOn"`
	Next                *Next                `xml:"next"`
	ParagraphProperties *ParagraphProperties `xml:"pPr"`
	RunProperties       *RunProperties       `xml:"rPr"`
}

// StyleName contains the style's display name
type StyleName struct {
	Val string `xml:"val,attr"`
}

// BasedOn references the parent style
type BasedOn struct {
	Val string `xml:"val,attr"`
}

// Next specifies the next style to use
type Next struct {
	Val string `xml:"val,attr"`
}

// GetStyle returns a style by its ID
func (s *Styles) GetStyle(id string) *StyleDef {
	for i := range s.Styles {
		if s.Styles[i].StyleID == id {
			return &s.Styles[i]
		}
	}
	return nil
}

// styleChain returns the style and its basedOn ancestors, nearest first.
// A cycle in the basedOn chain ends the walk.
func (s *Styles) styleChain(id string) []*StyleDef {
	var chain []*StyleDef
	seen := make(map[string]bool)
	for id != "" && !seen[id] {
		seen[id] = true
		style := s.GetStyle(id)
		if style == nil {
			break
		}
		chain = append(chain, style)
		if style.BasedOn == nil {
			break
		}
		id = style.BasedOn.Val
	}
	return chain
}

// StyleNumbering returns the numbering a paragraph style gives its
// paragraphs, following the basedOn chain. numID comes from the nearest
// style that sets it; level only from that style or a nearer one, so it
// never comes from an ancestor that belongs to another numbering. Both are
// nil when not set. A numID of 0 means the style turns numbering off.
func (s *Styles) StyleNumbering(styleID string) (numID *NumID, level *ILevel) {
	for _, style := range s.styleChain(styleID) {
		if style.ParagraphProperties == nil || style.ParagraphProperties.NumPr == nil {
			continue
		}
		numPr := style.ParagraphProperties.NumPr
		if level == nil && numPr.ILevel != nil {
			level = numPr.ILevel
		}
		if numPr.NumID != nil {
			return numPr.NumID, level
		}
	}
	return nil, nil
}

// ListStyleDepth returns the nesting depth implied by Word's built-in list
// style names "List N", "List Bullet N" and "List Number N" (depth N-1),
// taken from the nearest style in the basedOn chain with such a name.
// These styles are usually bound to separate single-level lists whose only
// sign of nesting is their indentation. Other styles return 0.
func (s *Styles) ListStyleDepth(styleID string) int {
	for _, style := range s.styleChain(styleID) {
		if style.Name == nil {
			continue
		}
		if depth, ok := builtInListDepth(style.Name.Val); ok {
			return depth
		}
	}
	return 0
}

// builtInListDepth reports whether name is one of Word's built-in list
// style names ("List", "List Bullet", "List Number", optionally followed
// by 2-9) and the nesting depth it implies.
func builtInListDepth(name string) (int, bool) {
	name = strings.ToLower(name)
	for _, base := range []string{"list bullet", "list number", "list"} {
		rest, ok := strings.CutPrefix(name, base)
		if !ok {
			continue
		}
		if rest == "" {
			return 0, true
		}
		n, err := strconv.Atoi(strings.TrimPrefix(rest, " "))
		if strings.HasPrefix(rest, " ") && err == nil && n >= 2 && n <= 9 {
			return n - 1, true
		}
	}
	return 0, false
}

// IsHeading checks if a style is a heading and returns the level (1-6)
func (s *Styles) IsHeading(styleID string) (bool, int) {
	style := s.GetStyle(styleID)
	if style == nil {
		return false, 0
	}

	// Check style name for heading patterns
	if style.Name != nil {
		name := strings.ToLower(style.Name.Val)

		// Check for "Heading 1", "Heading 2", etc.
		if strings.HasPrefix(name, "heading ") {
			level := int(name[8] - '0')
			if level >= 1 && level <= 6 {
				return true, level
			}
		}

		// Check for "Title" (treat as H1)
		if name == "title" {
			return true, 1
		}

		// Check for "Subtitle" (treat as H2)
		if name == "subtitle" {
			return true, 2
		}
	}

	// Check style ID for heading patterns
	idLower := strings.ToLower(styleID)
	if strings.HasPrefix(idLower, "heading") {
		if len(idLower) > 7 {
			level := int(idLower[7] - '0')
			if level >= 1 && level <= 6 {
				return true, level
			}
		}
	}

	// Check outline level in paragraph properties
	if style.ParagraphProperties != nil && style.ParagraphProperties.OutlineLevel != nil {
		level := style.ParagraphProperties.OutlineLevel.Val + 1 // 0-based to 1-based
		if level >= 1 && level <= 6 {
			return true, level
		}
	}

	return false, 0
}

// IsBold checks if a style includes bold formatting
func (s *Styles) IsBold(styleID string) bool {
	style := s.GetStyle(styleID)
	if style == nil {
		return false
	}
	if style.RunProperties != nil && style.RunProperties.Bold != nil {
		return style.RunProperties.Bold.IsTrue()
	}
	return false
}

// IsItalic checks if a style includes italic formatting
func (s *Styles) IsItalic(styleID string) bool {
	style := s.GetStyle(styleID)
	if style == nil {
		return false
	}
	if style.RunProperties != nil && style.RunProperties.Italic != nil {
		return style.RunProperties.Italic.IsTrue()
	}
	return false
}

// Numbering represents the numbering definitions file (word/numbering.xml)
type Numbering struct {
	XMLName      xml.Name      `xml:"numbering"`
	AbstractNums []AbstractNum `xml:"abstractNum"`
	Nums         []Num         `xml:"num"`
}

// AbstractNum defines an abstract numbering definition
type AbstractNum struct {
	XMLName       xml.Name      `xml:"abstractNum"`
	AbstractNumID int           `xml:"abstractNumId,attr"`
	Levels        []NumberLevel `xml:"lvl"`
	NumStyleLink  *StyleRef     `xml:"numStyleLink"` // levels are defined by the abstractNum with this styleLink
	StyleLink     *StyleRef     `xml:"styleLink"`    // this abstractNum defines the numbering style
}

// NumberLevel defines a numbering level
type NumberLevel struct {
	XMLName     xml.Name     `xml:"lvl"`
	ILevel      int          `xml:"ilvl,attr"`
	Start       *NumStart    `xml:"start"`
	NumFmt      *NumFmt      `xml:"numFmt"`
	LvlText     *LvlText     `xml:"lvlText"`
	LvlJc       *LvlJc       `xml:"lvlJc"`
	PStyle      *StyleRef    `xml:"pStyle"` // paragraph style bound to this level
	LvlRestart  *NumStart    `xml:"lvlRestart"`
	IsLgl       *BoolProp    `xml:"isLgl"` // show every level's number as decimal
	Indentation *Indentation `xml:"pPr>ind"`
}

// NumStart holds an integer w:val (w:start, w:lvlRestart, w:startOverride)
type NumStart struct {
	Val int `xml:"val,attr"`
}

// NumFmt specifies the number format
type NumFmt struct {
	Val string `xml:"val,attr"` // decimal, bullet, lowerLetter, upperLetter, lowerRoman, upperRoman, none
}

// LvlText specifies the level text template
type LvlText struct {
	Val string `xml:"val,attr"` // e.g., "%1.", "•", "%1.%2."
}

// LvlJc specifies level justification
type LvlJc struct {
	Val string `xml:"val,attr"` // left, center, right
}

// Num maps a numId to an abstractNumId
type Num struct {
	XMLName       xml.Name          `xml:"num"`
	NumID         int               `xml:"numId,attr"`
	AbstractNumID *AbstractNumIDRef `xml:"abstractNumId"`
	LvlOverrides  []LvlOverride     `xml:"lvlOverride"`
}

// LvlOverride overrides one level of a num's abstract definition
type LvlOverride struct {
	ILevel        int          `xml:"ilvl,attr"`
	StartOverride *NumStart    `xml:"startOverride"`
	Level         *NumberLevel `xml:"lvl"`
}

// AbstractNumIDRef references an abstract numbering definition
type AbstractNumIDRef struct {
	Val int `xml:"val,attr"`
}

// GetAbstractNum returns the abstract numbering definition for a numId
func (n *Numbering) GetAbstractNum(numID int) *AbstractNum {
	// Find the num element
	var abstractNumID int
	found := false
	for _, num := range n.Nums {
		if num.NumID == numID {
			if num.AbstractNumID != nil {
				abstractNumID = num.AbstractNumID.Val
				found = true
				break
			}
		}
	}
	if !found {
		return nil
	}

	// Find the abstract num
	var abstractNum *AbstractNum
	for i := range n.AbstractNums {
		if n.AbstractNums[i].AbstractNumID == abstractNumID {
			abstractNum = &n.AbstractNums[i]
			break
		}
	}

	// An abstractNum that only links to a numbering style takes its levels
	// from the abstractNum that defines that style. The hop limit guards
	// against link cycles.
	for hops := 0; abstractNum != nil && abstractNum.NumStyleLink != nil && hops < len(n.AbstractNums); hops++ {
		abstractNum = n.styleLinkTarget(abstractNum.NumStyleLink.Val)
	}
	return abstractNum
}

// styleLinkTarget returns the abstractNum that defines numbering style styleID
func (n *Numbering) styleLinkTarget(styleID string) *AbstractNum {
	for i := range n.AbstractNums {
		if link := n.AbstractNums[i].StyleLink; link != nil && link.Val == styleID {
			return &n.AbstractNums[i]
		}
	}
	return nil
}

// getNum returns the num element for a numId
func (n *Numbering) getNum(numID int) *Num {
	for i := range n.Nums {
		if n.Nums[i].NumID == numID {
			return &n.Nums[i]
		}
	}
	return nil
}

// levelOverride returns numID's override of a level, or nil
func (n *Numbering) levelOverride(numID, level int) *LvlOverride {
	num := n.getNum(numID)
	if num == nil {
		return nil
	}
	for i := range num.LvlOverrides {
		if num.LvlOverrides[i].ILevel == level {
			return &num.LvlOverrides[i]
		}
	}
	return nil
}

// GetLevel returns the level definition for a numId and level. A w:lvl in
// the num's w:lvlOverride replaces the abstract definition's level.
func (n *Numbering) GetLevel(numID, level int) *NumberLevel {
	if override := n.levelOverride(numID, level); override != nil && override.Level != nil {
		return override.Level
	}

	abstractNum := n.GetAbstractNum(numID)
	if abstractNum == nil {
		return nil
	}

	for i := range abstractNum.Levels {
		if abstractNum.Levels[i].ILevel == level {
			return &abstractNum.Levels[i]
		}
	}
	return nil
}

// LevelForStyle returns the level of numID's definition that is bound to
// the paragraph style styleID through w:lvl/w:pStyle, taking a w:lvl in the
// num's w:lvlOverride in place of the abstract definition's level.
func (n *Numbering) LevelForStyle(numID int, styleID string) (int, bool) {
	if styleID == "" {
		return 0, false
	}
	for level := 0; level <= maxNumberingLevel; level++ {
		if lvl := n.GetLevel(numID, level); lvl != nil && lvl.PStyle != nil && lvl.PStyle.Val == styleID {
			return level, true
		}
	}
	return 0, false
}

// maxNumberingLevel is the deepest w:ilvl WordprocessingML allows
const maxNumberingLevel = 8

// formatNumber renders a number in a w:numFmt format. "none" and "bullet"
// render as nothing; unknown formats render as decimal.
func formatNumber(n int, format string) string {
	switch format {
	case "none", "bullet":
		return ""
	case "decimalZero":
		if n >= 0 && n < 10 {
			return "0" + strconv.Itoa(n)
		}
	case "lowerLetter":
		if n >= 1 {
			return letters(n, 'a')
		}
	case "upperLetter":
		if n >= 1 {
			return letters(n, 'A')
		}
	case "lowerRoman":
		if r := toRoman(n, false); r != "" {
			return r
		}
	case "upperRoman":
		if r := toRoman(n, true); r != "" {
			return r
		}
	}
	return strconv.Itoa(n)
}

// letters renders n as Word does: a-z, then aa-zz, then aaa-zzz, ...
func letters(n int, first rune) string {
	letter := string(first + rune((n-1)%26))
	return strings.Repeat(letter, (n-1)/26+1)
}

// toRoman converts a number to Roman numerals
func toRoman(n int, upper bool) string {
	if n <= 0 || n > 3999 {
		return ""
	}

	values := []int{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
	symbols := []string{"m", "cm", "d", "cd", "c", "xc", "l", "xl", "x", "ix", "v", "iv", "i"}

	var result strings.Builder
	for i, v := range values {
		for n >= v {
			result.WriteString(symbols[i])
			n -= v
		}
	}

	if upper {
		return strings.ToUpper(result.String())
	}
	return result.String()
}

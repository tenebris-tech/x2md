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
}

// NumberLevel defines a numbering level
type NumberLevel struct {
	XMLName     xml.Name     `xml:"lvl"`
	ILevel      int          `xml:"ilvl,attr"`
	Start       *NumStart    `xml:"start"`
	NumFmt      *NumFmt      `xml:"numFmt"`
	LvlText     *LvlText     `xml:"lvlText"`
	LvlJc       *LvlJc       `xml:"lvlJc"`
	Indentation *Indentation `xml:"pPr>ind"`
}

// NumStart specifies the starting number
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
	for i := range n.AbstractNums {
		if n.AbstractNums[i].AbstractNumID == abstractNumID {
			return &n.AbstractNums[i]
		}
	}
	return nil
}

// GetLevel returns the level definition for a numId and level
func (n *Numbering) GetLevel(numID, level int) *NumberLevel {
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

// IsBullet checks if a numbering level is a bullet list
func (n *Numbering) IsBullet(numID, level int) bool {
	lvl := n.GetLevel(numID, level)
	if lvl == nil {
		return false
	}
	if lvl.NumFmt != nil && lvl.NumFmt.Val == "bullet" {
		return true
	}
	return false
}

// GetListPrefix returns the list prefix for a numbered item
func (n *Numbering) GetListPrefix(numID, level, itemNum int) string {
	return n.GetListPrefixFromCounters(numID, level, map[int]int{level: itemNum})
}

// GetLevelStart returns the configured starting value for a level.
func (n *Numbering) GetLevelStart(numID, level int) int {
	lvl := n.GetLevel(numID, level)
	if lvl == nil {
		return 1
	}
	if lvl.Start != nil && lvl.Start.Val > 0 {
		return lvl.Start.Val
	}
	return 1
}

// GetListPrefixFromCounters returns the prefix for a numbered item using the
// current counter state for all levels referenced by the OOXML lvlText pattern.
func (n *Numbering) GetListPrefixFromCounters(numID, level int, counters map[int]int) string {
	lvl := n.GetLevel(numID, level)
	if lvl == nil {
		return "- "
	}

	if lvl.NumFmt != nil {
		switch lvl.NumFmt.Val {
		case "bullet":
			return "- "
		case "none":
			return ""
		}
	}

	format := "decimal"
	if lvl.NumFmt != nil {
		format = lvl.NumFmt.Val
	}

	if lvl.LvlText != nil && lvl.LvlText.Val != "" {
		prefix := lvl.LvlText.Val
		for i := 0; i <= level; i++ {
			value := counters[i]
			if value == 0 {
				value = n.GetLevelStart(numID, i)
			}
			levelFormat := format
			if levelDef := n.GetLevel(numID, i); levelDef != nil && levelDef.NumFmt != nil {
				levelFormat = levelDef.NumFmt.Val
			}
			prefix = strings.ReplaceAll(prefix, "%"+strconv.Itoa(i+1), formatNumber(value, levelFormat))
		}
		if prefix != "" && !strings.HasSuffix(prefix, " ") {
			prefix += " "
		}
		return prefix
	}

	value := counters[level]
	if value == 0 {
		value = n.GetLevelStart(numID, level)
	}
	return formatNumber(value, format) + ". "
}

// formatNumber converts a number to the specified format
func formatNumber(n int, format string) string {
	switch format {
	case "decimal":
		return strconv.Itoa(n)
	case "lowerLetter":
		return toLetters(n, false)
	case "upperLetter":
		return toLetters(n, true)
	case "lowerRoman":
		return toRoman(n, false)
	case "upperRoman":
		return toRoman(n, true)
	default:
		return strconv.Itoa(n)
	}
}

func toLetters(n int, upper bool) string {
	if n <= 0 {
		return ""
	}

	var chars []byte
	for n > 0 {
		n--
		base := byte('a')
		if upper {
			base = 'A'
		}
		chars = append([]byte{base + byte(n%26)}, chars...)
		n /= 26
	}
	return string(chars)
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

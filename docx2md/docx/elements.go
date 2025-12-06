// Package docx provides DOCX file parsing functionality
package docx

import "encoding/xml"

// Document represents the main document structure (word/document.xml)
type Document struct {
	XMLName xml.Name `xml:"document"`
	Body    Body     `xml:"body"`
}

// Body contains the document content
type Body struct {
	Content []interface{} `xml:",any"` // Mixed paragraphs and tables
}

// Paragraph represents a document paragraph (w:p)
type Paragraph struct {
	XMLName    xml.Name              `xml:"p"`
	Properties *ParagraphProperties  `xml:"pPr"`
	Runs       []Run                 `xml:"r"`
	Hyperlinks []ParagraphHyperlink  `xml:"hyperlink"`
	BookmarkStart []BookmarkStart    `xml:"bookmarkStart"`
}

// ParagraphProperties contains paragraph-level properties (w:pPr)
type ParagraphProperties struct {
	Style       *StyleRef     `xml:"pStyle"`
	NumPr       *NumberingPr  `xml:"numPr"`
	Indentation *Indentation  `xml:"ind"`
	Justification *Justification `xml:"jc"`
	OutlineLevel *OutlineLevel `xml:"outlineLvl"`
	RunProperties *RunProperties `xml:"rPr"` // Paragraph-level run properties
}

// StyleRef references a style by ID (w:pStyle or w:rStyle)
type StyleRef struct {
	Val string `xml:"val,attr"`
}

// NumberingPr contains list numbering properties (w:numPr)
type NumberingPr struct {
	ILevel *ILevel `xml:"ilvl"`
	NumID  *NumID  `xml:"numId"`
}

// ILevel specifies the list indentation level
type ILevel struct {
	Val int `xml:"val,attr"`
}

// NumID references the numbering definition
type NumID struct {
	Val int `xml:"val,attr"`
}

// Indentation specifies paragraph indentation (w:ind)
type Indentation struct {
	Left      string `xml:"left,attr"`
	Right     string `xml:"right,attr"`
	Hanging   string `xml:"hanging,attr"`
	FirstLine string `xml:"firstLine,attr"`
}

// Justification specifies paragraph alignment (w:jc)
type Justification struct {
	Val string `xml:"val,attr"` // left, center, right, both
}

// OutlineLevel specifies heading outline level
type OutlineLevel struct {
	Val int `xml:"val,attr"`
}

// Run represents a run of text with formatting (w:r)
type Run struct {
	XMLName    xml.Name       `xml:"r"`
	Properties *RunProperties `xml:"rPr"`
	Text       []Text         `xml:"t"`
	Tab        []Tab          `xml:"tab"`
	Break      []Break        `xml:"br"`
	Drawing    []Drawing      `xml:"drawing"`
}

// RunProperties contains character-level formatting (w:rPr)
type RunProperties struct {
	Bold           *BoolProp  `xml:"b"`
	BoldCS         *BoolProp  `xml:"bCs"`
	Italic         *BoolProp  `xml:"i"`
	ItalicCS       *BoolProp  `xml:"iCs"`
	Underline      *Underline `xml:"u"`
	Strike         *BoolProp  `xml:"strike"`
	DoubleStrike   *BoolProp  `xml:"dstrike"`
	FontSize       *FontSize  `xml:"sz"`
	FontSizeCS     *FontSize  `xml:"szCs"`
	Color          *Color     `xml:"color"`
	Highlight      *Highlight `xml:"highlight"`
	VertAlign      *VertAlign `xml:"vertAlign"`
	Style          *StyleRef  `xml:"rStyle"`
	Font           *Font      `xml:"rFonts"`
}

// BoolProp represents a boolean property with optional val attribute
type BoolProp struct {
	Val *bool `xml:"val,attr"`
}

// IsTrue returns whether the property is enabled
func (b *BoolProp) IsTrue() bool {
	if b == nil {
		return false
	}
	// If val attribute is not present, the property is true
	if b.Val == nil {
		return true
	}
	return *b.Val
}

// Underline specifies underline formatting
type Underline struct {
	Val string `xml:"val,attr"` // single, double, wave, etc.
}

// FontSize specifies font size in half-points
type FontSize struct {
	Val int `xml:"val,attr"`
}

// Color specifies text color
type Color struct {
	Val string `xml:"val,attr"`
}

// Highlight specifies text highlight color
type Highlight struct {
	Val string `xml:"val,attr"`
}

// VertAlign specifies vertical alignment (superscript/subscript)
type VertAlign struct {
	Val string `xml:"val,attr"` // superscript, subscript
}

// Font specifies font family
type Font struct {
	ASCII   string `xml:"ascii,attr"`
	HAnsi   string `xml:"hAnsi,attr"`
	EastAsia string `xml:"eastAsia,attr"`
	CS      string `xml:"cs,attr"`
}

// Text contains the actual text content (w:t)
type Text struct {
	XMLName xml.Name `xml:"t"`
	Space   string   `xml:"space,attr"` // preserve
	Value   string   `xml:",chardata"`
}

// Tab represents a tab character (w:tab)
type Tab struct {
	XMLName xml.Name `xml:"tab"`
}

// Break represents a line or page break (w:br)
type Break struct {
	XMLName xml.Name `xml:"br"`
	Type    string   `xml:"type,attr"` // page, column, textWrapping
}

// Drawing represents an embedded image or shape
type Drawing struct {
	XMLName xml.Name `xml:"drawing"`
	Inline  *Inline  `xml:"inline"`
	Anchor  *Anchor  `xml:"anchor"`
}

// Inline represents an inline image
type Inline struct {
	DocPr  *DocPr  `xml:"docPr"`
	Blip   *Blip   `xml:"blip"`
	Extent *Extent `xml:"extent"`
}

// Anchor represents an anchored image
type Anchor struct {
	DocPr  *DocPr  `xml:"docPr"`
	Blip   *Blip   `xml:"blip"`
	Extent *Extent `xml:"extent"`
}

// DocPr contains document properties for drawings
type DocPr struct {
	ID    int    `xml:"id,attr"`
	Name  string `xml:"name,attr"`
	Descr string `xml:"descr,attr"`
}

// Blip references an image resource
type Blip struct {
	Embed string `xml:"embed,attr"`
}

// Extent specifies image dimensions
type Extent struct {
	CX int64 `xml:"cx,attr"`
	CY int64 `xml:"cy,attr"`
}

// ParagraphHyperlink represents a hyperlink in a paragraph
type ParagraphHyperlink struct {
	XMLName xml.Name `xml:"hyperlink"`
	ID      string   `xml:"id,attr"`
	Anchor  string   `xml:"anchor,attr"`
	Runs    []Run    `xml:"r"`
}

// BookmarkStart marks the beginning of a bookmark
type BookmarkStart struct {
	XMLName xml.Name `xml:"bookmarkStart"`
	ID      string   `xml:"id,attr"`
	Name    string   `xml:"name,attr"`
}

// Table represents a document table (w:tbl)
type Table struct {
	XMLName    xml.Name         `xml:"tbl"`
	Properties *TableProperties `xml:"tblPr"`
	Grid       *TableGrid       `xml:"tblGrid"`
	Rows       []TableRow       `xml:"tr"`
}

// TableProperties contains table-level properties
type TableProperties struct {
	Style   *StyleRef   `xml:"tblStyle"`
	Width   *TableWidth `xml:"tblW"`
	Borders *TableBorders `xml:"tblBorders"`
}

// TableWidth specifies table width
type TableWidth struct {
	W    int    `xml:"w,attr"`
	Type string `xml:"type,attr"` // auto, dxa, pct
}

// TableBorders specifies table border styles
type TableBorders struct {
	Top     *Border `xml:"top"`
	Left    *Border `xml:"left"`
	Bottom  *Border `xml:"bottom"`
	Right   *Border `xml:"right"`
	InsideH *Border `xml:"insideH"`
	InsideV *Border `xml:"insideV"`
}

// Border specifies a single border
type Border struct {
	Val   string `xml:"val,attr"`
	Sz    int    `xml:"sz,attr"`
	Space int    `xml:"space,attr"`
	Color string `xml:"color,attr"`
}

// TableGrid defines column widths
type TableGrid struct {
	Columns []GridCol `xml:"gridCol"`
}

// GridCol specifies a column width
type GridCol struct {
	W int `xml:"w,attr"`
}

// TableRow represents a table row (w:tr)
type TableRow struct {
	XMLName    xml.Name           `xml:"tr"`
	Properties *TableRowProperties `xml:"trPr"`
	Cells      []TableCell        `xml:"tc"`
}

// TableRowProperties contains row-level properties
type TableRowProperties struct {
	Height   *RowHeight `xml:"trHeight"`
	IsHeader *BoolProp  `xml:"tblHeader"`
}

// RowHeight specifies row height
type RowHeight struct {
	Val  int    `xml:"val,attr"`
	Rule string `xml:"hRule,attr"` // exact, atLeast, auto
}

// TableCell represents a table cell (w:tc)
type TableCell struct {
	XMLName    xml.Name            `xml:"tc"`
	Properties *TableCellProperties `xml:"tcPr"`
	Paragraphs []Paragraph         `xml:"p"`
}

// TableCellProperties contains cell-level properties
type TableCellProperties struct {
	Width     *TableWidth    `xml:"tcW"`
	GridSpan  *GridSpan      `xml:"gridSpan"`
	VMerge    *VMerge        `xml:"vMerge"`
	Borders   *TableBorders  `xml:"tcBorders"`
	Shading   *Shading       `xml:"shd"`
}

// GridSpan specifies horizontal cell merge
type GridSpan struct {
	Val int `xml:"val,attr"`
}

// VMerge specifies vertical cell merge
type VMerge struct {
	Val string `xml:"val,attr"` // restart, continue, or empty
}

// Shading specifies cell background
type Shading struct {
	Val   string `xml:"val,attr"`
	Color string `xml:"color,attr"`
	Fill  string `xml:"fill,attr"`
}

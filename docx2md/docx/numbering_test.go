/******************************************************************************
 * Copyright (c) 2026 Tenebris Technologies Inc.                              *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package docx

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/tenebris-tech/x2md/pdf2md/models"
)

// Styles for the numbering tests. ListNumber and ListBullet2 are Word's
// built-in list styles; Clause is numbered through a derived style and
// Clause2 is bound to level 1 of the same definition; NoClause turns the
// inherited numbering off; LoopA and LoopB form a basedOn cycle.
// CustomClause2 and MyBullet derive from bound and built-in list styles;
// NumberOverDeep sets numId 1 over an ancestor with level 1 of numId 3;
// LevelOverBase sets only a level over ClauseBase.
const testStylesXML = `<w:styles ` + wordNS + `>
<w:style w:type="paragraph" w:styleId="Normal"><w:name w:val="Normal"/></w:style>
<w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/><w:pPr><w:numPr><w:numId w:val="3"/></w:numPr></w:pPr></w:style>
<w:style w:type="paragraph" w:styleId="Heading2"><w:name w:val="heading 2"/></w:style>
<w:style w:type="paragraph" w:styleId="ListNumber"><w:name w:val="List Number"/><w:basedOn w:val="Normal"/><w:pPr><w:numPr><w:numId w:val="1"/></w:numPr></w:pPr></w:style>
<w:style w:type="paragraph" w:styleId="ListBullet2"><w:name w:val="List Bullet 2"/><w:basedOn w:val="Normal"/><w:pPr><w:numPr><w:numId w:val="2"/></w:numPr></w:pPr></w:style>
<w:style w:type="paragraph" w:styleId="ClauseBase"><w:name w:val="Clause Base"/><w:pPr><w:numPr><w:numId w:val="3"/></w:numPr></w:pPr></w:style>
<w:style w:type="paragraph" w:styleId="Clause"><w:name w:val="Clause"/><w:basedOn w:val="ClauseBase"/></w:style>
<w:style w:type="paragraph" w:styleId="Clause2"><w:name w:val="Clause 2"/><w:basedOn w:val="ClauseBase"/></w:style>
<w:style w:type="paragraph" w:styleId="NoClause"><w:name w:val="No Clause"/><w:basedOn w:val="Clause"/><w:pPr><w:numPr><w:numId w:val="0"/></w:numPr></w:pPr></w:style>
<w:style w:type="paragraph" w:styleId="NoClauseChild"><w:name w:val="No Clause Child"/><w:basedOn w:val="NoClause"/></w:style>
<w:style w:type="paragraph" w:styleId="CustomClause2"><w:name w:val="Custom Clause 2"/><w:basedOn w:val="Clause2"/></w:style>
<w:style w:type="paragraph" w:styleId="DeepClause"><w:name w:val="Deep Clause"/><w:pPr><w:numPr><w:ilvl w:val="1"/><w:numId w:val="3"/></w:numPr></w:pPr></w:style>
<w:style w:type="paragraph" w:styleId="NumberOverDeep"><w:name w:val="Number Over Deep"/><w:basedOn w:val="DeepClause"/><w:pPr><w:numPr><w:numId w:val="1"/></w:numPr></w:pPr></w:style>
<w:style w:type="paragraph" w:styleId="LevelOverBase"><w:name w:val="Level Over Base"/><w:basedOn w:val="ClauseBase"/><w:pPr><w:numPr><w:ilvl w:val="1"/></w:numPr></w:pPr></w:style>
<w:style w:type="paragraph" w:styleId="MyBullet"><w:name w:val="My Bullet"/><w:basedOn w:val="ListBullet2"/></w:style>
<w:style w:type="paragraph" w:styleId="LoopA"><w:name w:val="Loop A"/><w:basedOn w:val="LoopB"/></w:style>
<w:style w:type="paragraph" w:styleId="LoopB"><w:name w:val="Loop B"/><w:basedOn w:val="LoopA"/></w:style>
</w:styles>`

const testNumberingXML = `<w:numbering ` + wordNS + `>
<w:abstractNum w:abstractNumId="10"><w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1."/><w:pStyle w:val="ListNumber"/></w:lvl></w:abstractNum>
<w:abstractNum w:abstractNumId="11"><w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="bullet"/><w:lvlText w:val="&#xF0B7;"/><w:pStyle w:val="ListBullet2"/></w:lvl></w:abstractNum>
<w:abstractNum w:abstractNumId="12">
<w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1."/><w:pStyle w:val="Clause"/></w:lvl>
<w:lvl w:ilvl="1"><w:start w:val="1"/><w:numFmt w:val="lowerLetter"/><w:lvlText w:val="%2."/><w:pStyle w:val="Clause2"/></w:lvl>
</w:abstractNum>
<w:num w:numId="1"><w:abstractNumId w:val="10"/></w:num>
<w:num w:numId="2"><w:abstractNumId w:val="11"/></w:num>
<w:num w:numId="3"><w:abstractNumId w:val="12"/></w:num>
</w:numbering>`

func newTestExtractor(t *testing.T) *Extractor {
	t.Helper()
	styles := &Styles{}
	if err := xml.Unmarshal([]byte(testStylesXML), styles); err != nil {
		t.Fatalf("parsing styles: %v", err)
	}
	numbering := &Numbering{}
	if err := xml.Unmarshal([]byte(testNumberingXML), numbering); err != nil {
		t.Fatalf("parsing numbering: %v", err)
	}
	return &Extractor{styles: styles, numbering: numbering, listCounters: make(map[int]map[int]int)}
}

// directNumPr builds a direct numPr; a negative level omits w:ilvl
func directNumPr(numID, level int) *NumberingPr {
	numPr := &NumberingPr{NumID: &NumID{Val: numID}}
	if level >= 0 {
		numPr.ILevel = &ILevel{Val: level}
	}
	return numPr
}

// paragraph is one input paragraph and the line it must produce
type paragraph struct {
	style     string
	numPr     *NumberingPr
	wantText  string
	wantType  *models.BlockType
	wantLevel int
}

func TestCreateLineItem_Numbering(t *testing.T) {
	tests := []struct {
		name       string
		paragraphs []paragraph
	}{
		{
			name: "numbering resolved from the paragraph style",
			paragraphs: []paragraph{
				{style: "ListNumber", wantText: "1. a", wantType: models.BlockTypeList},
				{style: "ListNumber", wantText: "2. a", wantType: models.BlockTypeList},
			},
		},
		{
			name: "built-in List Bullet 2 is a nested bullet",
			paragraphs: []paragraph{
				{style: "ListNumber", wantText: "1. a", wantType: models.BlockTypeList},
				{style: "ListBullet2", wantText: "- a", wantType: models.BlockTypeList, wantLevel: 1},
			},
		},
		{
			name: "numbering resolved through basedOn and level from w:lvl/w:pStyle",
			paragraphs: []paragraph{
				{style: "Clause", wantText: "1. a", wantType: models.BlockTypeList},
				{style: "Clause2", wantText: "- a. a", wantType: models.BlockTypeList, wantLevel: 1},
				{style: "Clause2", wantText: "- b. a", wantType: models.BlockTypeList, wantLevel: 1},
				{style: "Clause", wantText: "2. a", wantType: models.BlockTypeList},
			},
		},
		{
			name: "deeper levels restart when a higher level advances",
			paragraphs: []paragraph{
				{style: "Clause", wantText: "1. a", wantType: models.BlockTypeList},
				{style: "Clause2", wantText: "- a. a", wantType: models.BlockTypeList, wantLevel: 1},
				{style: "Clause", wantText: "2. a", wantType: models.BlockTypeList},
				{style: "Clause2", wantText: "- a. a", wantType: models.BlockTypeList, wantLevel: 1},
			},
		},
		{
			name: "level from the w:lvl/w:pStyle binding of an ancestor style",
			paragraphs: []paragraph{
				{style: "CustomClause2", wantText: "- a. a", wantType: models.BlockTypeList, wantLevel: 1},
			},
		},
		{
			name: "level is not taken from an ancestor beyond the style that sets numId",
			paragraphs: []paragraph{
				{style: "NumberOverDeep", wantText: "1. a", wantType: models.BlockTypeList},
			},
		},
		{
			name: "level from a style nearer than the one that sets numId",
			paragraphs: []paragraph{
				{style: "LevelOverBase", wantText: "- a. a", wantType: models.BlockTypeList, wantLevel: 1},
			},
		},
		{
			name: "style derived from List Bullet 2 is a nested bullet",
			paragraphs: []paragraph{
				{style: "MyBullet", wantText: "- a", wantType: models.BlockTypeList, wantLevel: 1},
			},
		},
		{
			name: "direct numId 0 switches style numbering off",
			paragraphs: []paragraph{
				{style: "ListNumber", numPr: directNumPr(0, -1), wantText: "a"},
			},
		},
		{
			name: "derived style with numId 0 switches numbering off for itself and descendants",
			paragraphs: []paragraph{
				{style: "NoClause", wantText: "a"},
				{style: "NoClauseChild", wantText: "a"},
			},
		},
		{
			name: "direct numPr overrides the style",
			paragraphs: []paragraph{
				{style: "ListNumber", numPr: directNumPr(2, 0), wantText: "- a", wantType: models.BlockTypeList},
			},
		},
		{
			name: "direct level with the style's numId",
			paragraphs: []paragraph{
				{style: "Clause", numPr: &NumberingPr{ILevel: &ILevel{Val: 1}}, wantText: "- a. a", wantType: models.BlockTypeList, wantLevel: 1},
			},
		},
		{
			name: "style and direct numbering share counters",
			paragraphs: []paragraph{
				{style: "ListNumber", wantText: "1. a", wantType: models.BlockTypeList},
				{numPr: directNumPr(1, 0), wantText: "2. a", wantType: models.BlockTypeList},
				{style: "ListNumber", wantText: "3. a", wantType: models.BlockTypeList},
			},
		},
		{
			name: "heading numbered through its style stays a heading",
			paragraphs: []paragraph{
				{style: "Heading1", wantText: "1. a", wantType: models.BlockTypeH1},
			},
		},
		{
			name: "basedOn cycle ends the walk",
			paragraphs: []paragraph{
				{style: "LoopA", wantText: "a"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestExtractor(t)
			for i, p := range tt.paragraphs {
				item := e.createLineItem([]*models.Word{{String: "a"}}, p.style, p.numPr)
				if item == nil {
					t.Fatalf("paragraph %d: no line item", i)
				}
				if got := strings.TrimSpace(models.LinesToText([]*models.LineItem{item}, false)); got != p.wantText {
					t.Errorf("paragraph %d: text %q, want %q", i, got, p.wantText)
				}
				if item.Type != p.wantType {
					t.Errorf("paragraph %d: type %v, want %v", i, item.Type, p.wantType)
				}
				if item.ListLevel != p.wantLevel {
					t.Errorf("paragraph %d: list level %d, want %d", i, item.ListLevel, p.wantLevel)
				}
			}
		})
	}
}

func TestCreateLineItem_DecimalPastNine(t *testing.T) {
	e := newTestExtractor(t)
	var last string
	for range 12 {
		item := e.createLineItem([]*models.Word{{String: "a"}}, "ListNumber", nil)
		last = models.LinesToText([]*models.LineItem{item}, false)
	}
	if last != "12. a\n" {
		t.Errorf("12th item = %q, want \"12. a\\n\"", last)
	}
}

func TestListStyleDepth(t *testing.T) {
	styles := &Styles{Styles: []StyleDef{
		{StyleID: "a", Name: &StyleName{Val: "List Bullet 3"}},
		{StyleID: "b", Name: &StyleName{Val: "List Number 2"}},
		{StyleID: "c", Name: &StyleName{Val: "List 4"}},
		{StyleID: "d", Name: &StyleName{Val: "List Bullet"}},
		{StyleID: "e", Name: &StyleName{Val: "List Paragraph"}},
		{StyleID: "f", Name: &StyleName{Val: "Custom"}, BasedOn: &BasedOn{Val: "a"}},
		{StyleID: "g", Name: &StyleName{Val: "List Bullet"}, BasedOn: &BasedOn{Val: "a"}},
	}}
	for id, want := range map[string]int{"a": 2, "b": 1, "c": 3, "d": 0, "e": 0, "f": 2, "g": 0, "missing": 0} {
		if got := styles.ListStyleDepth(id); got != want {
			t.Errorf("ListStyleDepth(%q) = %d, want %d", id, got, want)
		}
	}
}

// Numbering for the label tests. abstractNum 20 is a multi-level heading
// scheme; num 21 restarts it at 5 and num 22 replaces its first level.
// abstractNum 30 only links to the numbering style that abstractNum 31
// defines; 40 links to itself. In abstractNum 50, level 1 never restarts.
const labelNumberingXML = `<w:numbering ` + wordNS + `>
<w:abstractNum w:abstractNumId="20">
<w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1."/></w:lvl>
<w:lvl w:ilvl="1"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1.%2"/></w:lvl>
<w:lvl w:ilvl="2"><w:start w:val="1"/><w:numFmt w:val="lowerLetter"/><w:lvlText w:val="(%3)"/></w:lvl>
<w:lvl w:ilvl="3"><w:start w:val="1"/><w:numFmt w:val="upperRoman"/><w:lvlText w:val="Article %4"/></w:lvl>
<w:lvl w:ilvl="4"><w:start w:val="1"/><w:numFmt w:val="lowerLetter"/><w:isLgl/><w:lvlText w:val="%4.%5"/></w:lvl>
<w:lvl w:ilvl="5"><w:start w:val="1"/><w:numFmt w:val="decimalZero"/><w:lvlText w:val="%6"/></w:lvl>
<w:lvl w:ilvl="6"><w:start w:val="1"/><w:numFmt w:val="bullet"/><w:lvlText w:val="&#xF0B7;"/></w:lvl>
<w:lvl w:ilvl="7"><w:start w:val="1"/><w:numFmt w:val="none"/><w:lvlText w:val=""/></w:lvl>
<w:lvl w:ilvl="8"><w:start w:val="1"/><w:numFmt w:val="ordinalText"/><w:lvlText w:val="%9."/></w:lvl>
</w:abstractNum>
<w:abstractNum w:abstractNumId="30"><w:numStyleLink w:val="OutlineList"/></w:abstractNum>
<w:abstractNum w:abstractNumId="31"><w:styleLink w:val="OutlineList"/>
<w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1)"/></w:lvl>
</w:abstractNum>
<w:abstractNum w:abstractNumId="40"><w:numStyleLink w:val="Loop"/><w:styleLink w:val="Loop"/></w:abstractNum>
<w:abstractNum w:abstractNumId="50">
<w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1."/></w:lvl>
<w:lvl w:ilvl="1"><w:start w:val="1"/><w:lvlRestart w:val="0"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1.%2"/></w:lvl>
</w:abstractNum>
<w:num w:numId="20"><w:abstractNumId w:val="20"/></w:num>
<w:num w:numId="21"><w:abstractNumId w:val="20"/><w:lvlOverride w:ilvl="0"><w:startOverride w:val="5"/></w:lvlOverride></w:num>
<w:num w:numId="22"><w:abstractNumId w:val="20"/><w:lvlOverride w:ilvl="0"><w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="upperLetter"/><w:lvlText w:val="Part %1"/></w:lvl></w:lvlOverride></w:num>
<w:num w:numId="30"><w:abstractNumId w:val="30"/></w:num>
<w:num w:numId="40"><w:abstractNumId w:val="40"/></w:num>
<w:num w:numId="50"><w:abstractNumId w:val="50"/></w:num>
</w:numbering>`

// renderNumbered converts paragraphs of the given style and direct numbering
// through createLineItem and returns each as Markdown
func renderNumbered(t *testing.T, paragraphs []struct {
	style string
	numPr *NumberingPr
}) []string {
	t.Helper()
	e := newTestExtractor(t)
	e.numbering = &Numbering{}
	if err := xml.Unmarshal([]byte(labelNumberingXML), e.numbering); err != nil {
		t.Fatalf("parsing numbering: %v", err)
	}
	var out []string
	for _, p := range paragraphs {
		item := e.createLineItem([]*models.Word{{String: "Text"}}, p.style, p.numPr)
		block := &models.LineItemBlock{Type: item.Type}
		block.AddItem(item)
		out = append(out, strings.TrimSuffix(models.BlockToText(block), "\n"))
	}
	return out
}

func TestNumberLabels(t *testing.T) {
	type para = struct {
		style string
		numPr *NumberingPr
	}
	tests := []struct {
		name       string
		paragraphs []para
		want       []string
	}{
		{
			name: "numbered headings share counters with list items",
			paragraphs: []para{
				{"Heading1", directNumPr(20, 0)},
				{"Heading2", directNumPr(20, 1)},
				{"", directNumPr(20, 1)},
				{"Heading2", directNumPr(20, 1)},
				{"Heading1", directNumPr(20, 0)},
				{"Heading2", directNumPr(20, 1)},
			},
			want: []string{"# 1. Text", "## 1.1 Text", "    - 1.2 Text", "## 1.3 Text", "# 2. Text", "## 2.1 Text"},
		},
		{
			name: "decorated labels and formats",
			paragraphs: []para{
				{"", directNumPr(20, 2)},
				{"", directNumPr(20, 2)},
				{"", directNumPr(20, 3)},
				{"", directNumPr(20, 3)},
				{"", directNumPr(20, 4)},
				{"", directNumPr(20, 5)},
				{"", directNumPr(20, 8)},
			},
			want: []string{
				"        - (a) Text", "        - (b) Text",
				"            - Article I Text", "            - Article II Text",
				"                - 2.1 Text",
				"                    - 01 Text",
				"                                1. Text",
			},
		},
		{
			name:       "bullet and none levels",
			paragraphs: []para{{"", directNumPr(20, 6)}, {"", directNumPr(20, 7)}, {"Heading1", directNumPr(20, 6)}, {"Heading1", directNumPr(20, 7)}},
			want:       []string{"                        - Text", "                            Text", "# Text", "# Text"},
		},
		{
			name:       "startOverride",
			paragraphs: []para{{"", directNumPr(21, 0)}, {"", directNumPr(21, 0)}},
			want:       []string{"5. Text", "6. Text"},
		},
		{
			name:       "level replaced by lvlOverride",
			paragraphs: []para{{"Heading1", directNumPr(22, 0)}, {"", directNumPr(22, 0)}, {"", directNumPr(22, 1)}},
			want:       []string{"# Part A Text", "- Part B Text", "    - B.1 Text"},
		},
		{
			name:       "numStyleLink resolves to the abstractNum defining the style",
			paragraphs: []para{{"", directNumPr(30, 0)}, {"", directNumPr(30, 0)}},
			want:       []string{"1) Text", "2) Text"},
		},
		{
			name:       "numStyleLink cycle",
			paragraphs: []para{{"", directNumPr(40, 0)}},
			want:       []string{"- Text"},
		},
		{
			name: "lvlRestart 0 never restarts",
			paragraphs: []para{
				{"", directNumPr(50, 0)}, {"", directNumPr(50, 1)}, {"", directNumPr(50, 0)}, {"", directNumPr(50, 1)},
			},
			want: []string{"1. Text", "    - 1.1 Text", "2. Text", "    - 2.2 Text"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderNumbered(t, tt.paragraphs)
			if strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Errorf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

func TestFormatNumber_Formats(t *testing.T) {
	tests := []struct {
		n      int
		format string
		want   string
	}{
		{27, "lowerLetter", "aa"},
		{53, "upperLetter", "AAA"},
		{7, "decimalZero", "07"},
		{12, "decimalZero", "12"},
		{3, "none", ""},
		{3, "bullet", ""},
		{0, "lowerRoman", "0"},
		{4, "cardinalText", "4"},
	}
	for _, tt := range tests {
		if got := formatNumber(tt.n, tt.format); got != tt.want {
			t.Errorf("formatNumber(%d, %q) = %q, want %q", tt.n, tt.format, got, tt.want)
		}
	}
}

func TestLevelForStyle_LvlOverride(t *testing.T) {
	numbering := &Numbering{}
	err := xml.Unmarshal([]byte(`<w:numbering `+wordNS+`>
<w:abstractNum w:abstractNumId="60">
<w:lvl w:ilvl="0"><w:numFmt w:val="decimal"/><w:lvlText w:val="%1."/></w:lvl>
<w:lvl w:ilvl="1"><w:numFmt w:val="lowerLetter"/><w:lvlText w:val="%2)"/><w:pStyle w:val="Replaced"/></w:lvl>
</w:abstractNum>
<w:num w:numId="60"><w:abstractNumId w:val="60"/>
<w:lvlOverride w:ilvl="1"><w:lvl w:ilvl="1"><w:numFmt w:val="lowerLetter"/><w:lvlText w:val="%2)"/><w:pStyle w:val="Override"/></w:lvl></w:lvlOverride>
</w:num>
</w:numbering>`), numbering)
	if err != nil {
		t.Fatal(err)
	}
	if level, ok := numbering.LevelForStyle(60, "Override"); !ok || level != 1 {
		t.Errorf("LevelForStyle(Override) = %d, %v; want 1, true", level, ok)
	}
	if _, ok := numbering.LevelForStyle(60, "Replaced"); ok {
		t.Error("LevelForStyle(Replaced) found a level the override replaces")
	}
}

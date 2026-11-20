/******************************************************************************
 * Copyright (c) 2026 Tenebris Technologies Inc.                              *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package docx2md

import (
	"strings"
	"testing"
)

// Styles that carry numbering themselves (ListNumber, ListBullet), inherit it
// through basedOn (StepChild, StepGrandchild, SubStep), switch it off again
// (NoList), or combine it with a heading (Heading1). Loop1/Loop2 form a
// basedOn cycle.
const styleNumberingStyles = `<?xml version="1.0" encoding="UTF-8"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style>
  <w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/><w:basedOn w:val="Normal"/>
    <w:pPr><w:numPr><w:numId w:val="1"/></w:numPr></w:pPr></w:style>
  <w:style w:type="paragraph" w:styleId="ListNumber"><w:name w:val="List Number"/><w:basedOn w:val="Normal"/>
    <w:pPr><w:numPr><w:numId w:val="1"/></w:numPr></w:pPr></w:style>
  <w:style w:type="paragraph" w:styleId="ListBullet"><w:name w:val="List Bullet"/><w:basedOn w:val="Normal"/>
    <w:pPr><w:numPr><w:numId w:val="2"/></w:numPr></w:pPr></w:style>
  <w:style w:type="paragraph" w:styleId="StepChild"><w:name w:val="Step Child"/><w:basedOn w:val="ListNumber"/></w:style>
  <w:style w:type="paragraph" w:styleId="StepGrandchild"><w:name w:val="Step Grandchild"/><w:basedOn w:val="StepChild"/>
    <w:pPr><w:ind w:left="720"/></w:pPr></w:style>
  <w:style w:type="paragraph" w:styleId="SubStep"><w:name w:val="Sub Step"/><w:basedOn w:val="ListNumber"/>
    <w:pPr><w:numPr><w:ilvl w:val="1"/></w:numPr></w:pPr></w:style>
  <w:style w:type="paragraph" w:styleId="NoList"><w:name w:val="No List Text"/><w:basedOn w:val="ListNumber"/>
    <w:pPr><w:numPr><w:numId w:val="0"/></w:numPr></w:pPr></w:style>
  <w:style w:type="paragraph" w:styleId="Loop1"><w:name w:val="Loop 1"/><w:basedOn w:val="Loop2"/></w:style>
  <w:style w:type="paragraph" w:styleId="Loop2"><w:name w:val="Loop 2"/><w:basedOn w:val="Loop1"/></w:style>
</w:styles>`

const styleNumberingNumbering = `<?xml version="1.0" encoding="UTF-8"?>
<w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:abstractNum w:abstractNumId="0">
    <w:lvl w:ilvl="0"><w:numFmt w:val="decimal"/></w:lvl>
    <w:lvl w:ilvl="1"><w:numFmt w:val="lowerLetter"/></w:lvl>
  </w:abstractNum>
  <w:abstractNum w:abstractNumId="1"><w:lvl w:ilvl="0"><w:numFmt w:val="bullet"/></w:lvl></w:abstractNum>
  <w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num>
  <w:num w:numId="2"><w:abstractNumId w:val="1"/></w:num>
</w:numbering>`

// styled returns a paragraph in the given style; extraPPr is placed inside
// w:pPr after the style reference
func styled(style, extraPPr, text string) string {
	return `<w:p><w:pPr><w:pStyle w:val="` + style + `"/>` + extraPPr + `</w:pPr><w:r><w:t>` + text + `</w:t></w:r></w:p>`
}

func convertStyleNumbering(t *testing.T, body string) string {
	t.Helper()
	md, err := New(WithParagraphMarkers(false)).Convert(createTestDocxWithParts(body, map[string]string{
		"word/styles.xml":    styleNumberingStyles,
		"word/numbering.xml": styleNumberingNumbering,
	}))
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	return md
}

// expectLines fails unless every wanted string is a complete line of md
func expectLines(t *testing.T, md string, want ...string) {
	t.Helper()
	lines := make(map[string]bool)
	for _, line := range strings.Split(md, "\n") {
		lines[line] = true
	}
	for _, w := range want {
		if !lines[w] {
			t.Errorf("expected line %q in:\n%s", w, md)
		}
	}
}

func TestStyleNumbering_DirectOnStyle(t *testing.T) {
	md := convertStyleNumbering(t,
		styled("ListNumber", "", "alpha")+
			styled("ListNumber", "", "beta")+
			styled("ListNumber", "", "gamma")+
			styled("ListBullet", "", "delta"))
	expectLines(t, md, "1. alpha", "2. beta", "3. gamma", "- delta")
}

func TestStyleNumbering_InheritedThroughBasedOn(t *testing.T) {
	md := convertStyleNumbering(t,
		styled("ListNumber", "", "alpha")+
			styled("StepChild", "", "beta")+
			styled("StepGrandchild", "", "gamma"))
	// All three resolve to the same numId, so they share one counter
	expectLines(t, md, "1. alpha", "2. beta", "3. gamma")
}

func TestStyleNumbering_LevelFromChildNumIDFromParent(t *testing.T) {
	md := convertStyleNumbering(t,
		styled("ListNumber", "", "alpha")+
			styled("SubStep", "", "beta")+
			styled("SubStep", "", "gamma"))
	expectLines(t, md, "1. alpha", "  a. beta", "  b. gamma")
}

func TestStyleNumbering_ParagraphNumPrWins(t *testing.T) {
	md := convertStyleNumbering(t,
		styled("ListNumber", `<w:numPr><w:ilvl w:val="0"/><w:numId w:val="2"/></w:numPr>`, "alpha"))
	expectLines(t, md, "- alpha")
}

func TestStyleNumbering_ParagraphLevelWithStyleNumID(t *testing.T) {
	md := convertStyleNumbering(t,
		styled("ListNumber", `<w:numPr><w:ilvl w:val="1"/></w:numPr>`, "alpha"))
	expectLines(t, md, "  a. alpha")
}

func TestStyleNumbering_NumIDZeroSwitchesOff(t *testing.T) {
	md := convertStyleNumbering(t,
		styled("ListNumber", `<w:numPr><w:numId w:val="0"/></w:numPr>`, "alpha")+
			styled("NoList", "", "beta")+
			styled("ListNumber", "", "gamma"))
	// Neither unnumbered paragraph consumes a number
	expectLines(t, md, "alpha", "beta", "1. gamma")
}

func TestStyleNumbering_NumberedHeadingStaysHeading(t *testing.T) {
	md := convertStyleNumbering(t, styled("Heading1", "", "Title")+styled("ListNumber", "", "alpha"))
	expectLines(t, md, "# Title", "1. alpha")
}

func TestStyleNumbering_UnnumberedAndUnknownStyles(t *testing.T) {
	md := convertStyleNumbering(t,
		styled("Normal", "", "alpha")+
			styled("Missing", "", "beta")+
			styled("Loop1", "", "gamma"))
	expectLines(t, md, "alpha", "beta", "gamma")
}

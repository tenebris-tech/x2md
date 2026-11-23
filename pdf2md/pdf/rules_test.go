/******************************************************************************
 * Copyright (c) 2026 Tenebris Technologies Inc.                              *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package pdf

import (
	"math"
	"testing"
)

// pageRules runs a page content stream on a 612x792 page and returns the
// rules it paints
func pageRules(t *testing.T, content string) []Rule {
	t.Helper()
	e := NewTextExtractor(NewParser(nil))
	e.rules = nil
	if _, err := e.parseContentStream([]byte(content), [4]float64{0, 0, 612, 792}); err != nil {
		t.Fatalf("parsing content: %v", err)
	}
	return e.Rules()
}

func sameRules(got, want []Rule) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if math.Abs(got[i].X1-want[i].X1) > 0.01 || math.Abs(got[i].Y1-want[i].Y1) > 0.01 ||
			math.Abs(got[i].X2-want[i].X2) > 0.01 || math.Abs(got[i].Y2-want[i].Y2) > 0.01 {
			return false
		}
	}
	return true
}

func TestRules(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []Rule
	}{
		{
			name:    "stroked horizontal line",
			content: "100 692 m 300 692 l S",
			want:    []Rule{{X1: 100, Y1: 100, X2: 300, Y2: 100}},
		},
		{
			name:    "stroked vertical line",
			content: "100 692 m 100 592 l S",
			want:    []Rule{{X1: 100, Y1: 100, X2: 100, Y2: 200}},
		},
		{
			name:    "diagonal and curve ignored",
			content: "100 692 m 200 592 l 250 600 260 610 270 692 c S",
			want:    nil,
		},
		{
			name:    "thin filled rectangle is a line",
			content: "100 691.5 200 1 re f",
			want:    []Rule{{X1: 100, Y1: 100, X2: 300, Y2: 100}},
		},
		{
			name:    "thin filled rectangle drawn as a closed path",
			content: "100 691.5 m 100 492 l 101 492 l 101 691.5 l h f",
			want:    []Rule{{X1: 100.5, Y1: 100.5, X2: 100.5, Y2: 300}},
		},
		{
			name:    "wide filled rectangle is shading",
			content: "100 600 200 50 re f",
			want:    nil,
		},
		{
			name:    "stroked rectangle has four edges",
			content: "100 592 200 100 re S",
			want: []Rule{
				{X1: 100, Y1: 100, X2: 300, Y2: 100},
				{X1: 100, Y1: 200, X2: 300, Y2: 200},
				{X1: 100, Y1: 100, X2: 100, Y2: 200},
				{X1: 300, Y1: 100, X2: 300, Y2: 200},
			},
		},
		{
			name:    "stroked and filled cell has its edges",
			content: "100 592 200 100 re B",
			want: []Rule{
				{X1: 100, Y1: 100, X2: 300, Y2: 100},
				{X1: 100, Y1: 200, X2: 300, Y2: 200},
				{X1: 100, Y1: 100, X2: 100, Y2: 200},
				{X1: 300, Y1: 100, X2: 300, Y2: 200},
			},
		},
		{
			name:    "clipping path paints nothing",
			content: "100 592 200 100 re W n 100 692 m 300 692 l n",
			want:    nil,
		},
		{
			name:    "cm scales and q/Q restores",
			content: "q 2 0 0 2 0 0 cm 50 346 m 150 346 l S Q 100 592 m 300 592 l S",
			want: []Rule{
				{X1: 100, Y1: 100, X2: 300, Y2: 100},
				{X1: 100, Y1: 200, X2: 300, Y2: 200},
			},
		},
		{
			name:    "collinear touching segments are one rule",
			content: "100 692 m 200 692 l S 200.5 692 m 300 692 l S 150 692 m 250 692 l S",
			want:    []Rule{{X1: 100, Y1: 100, X2: 300, Y2: 100}},
		},
		{
			// Operators the parser did not know (scn) must not leave their
			// operands in front of the next operator's
			name:    "unknown operator operands do not leak",
			content: "0 0 1 scn 2 0 0 2 0 0 cm 50 346 m 150 346 l S",
			want:    []Rule{{X1: 100, Y1: 100, X2: 300, Y2: 100}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pageRules(t, tt.content); !sameRules(got, tt.want) {
				t.Errorf("rules = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// Form XObjects paint rules through their Matrix
func TestRules_FormXObject(t *testing.T) {
	e := NewTextExtractor(NewParser(nil))
	e.xobjects["Fm1"] = &Object{
		Dict: map[string]interface{}{
			"Subtype": "/Form",
			"Matrix":  []interface{}{2.0, 0.0, 0.0, 2.0, 0.0, 0.0},
		},
		Stream: []byte("50 346 m 150 346 l S"),
	}
	if _, err := e.parseContentStream([]byte("q /Fm1 Do Q"), [4]float64{0, 0, 612, 792}); err != nil {
		t.Fatalf("parsing content: %v", err)
	}
	want := []Rule{{X1: 100, Y1: 100, X2: 300, Y2: 100}}
	if got := e.Rules(); !sameRules(got, want) {
		t.Errorf("rules = %+v, want %+v", got, want)
	}
}

// A font selected after an operator the parser did not know (scn) is still
// the font of the text that follows
func TestUnknownOperatorBeforeTf(t *testing.T) {
	items := extractWith(t, map[string]map[string]interface{}{"F1": testFont()},
		"BT 0 0 1 scn /F1 10 Tf 100 700 Td (ABC) Tj ET")
	item := oneItem(t, items)
	if item.Font != "F1" || !near(item.Height, 10) {
		t.Errorf("font = %q, height = %v; want F1, 10", item.Font, item.Height)
	}
}

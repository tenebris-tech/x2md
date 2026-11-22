/******************************************************************************
 * Copyright (c) 2026 Tenebris Technologies Inc.                              *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package pdf

import (
	"math"
	"testing"
)

// Many producers select a font at size 1 and set the real size with the text
// matrix. Text shown with TJ must report that size, as text shown with Tj does.
func TestShowTextArray_FontSizeIncludesTextMatrix(t *testing.T) {
	e := NewTextExtractor(NewParser(nil))
	mediaBox := [4]float64{0, 0, 612, 792}

	newState := func() *GraphicsState {
		return &GraphicsState{
			CTM:          [6]float64{1, 0, 0, 1, 0, 0},
			TextMatrix:   [6]float64{12, 0, 0, 12, 72, 700},
			FontName:     "F1",
			FontSize:     1,
			HorizScaling: 100,
		}
	}

	array := e.showTextArray([]interface{}{"[", "Hello", "]"}, newState(), nil, mediaBox)
	if len(array) != 1 {
		t.Fatalf("got %d items, want 1", len(array))
	}
	if math.Abs(array[0].Height-12) > 0.001 {
		t.Errorf("TJ height = %v, want 12", array[0].Height)
	}

	single := e.showText("Hello", newState(), mediaBox)
	if math.Abs(single.Height-array[0].Height) > 0.001 {
		t.Errorf("Tj height = %v, TJ height = %v, want the same", single.Height, array[0].Height)
	}
}

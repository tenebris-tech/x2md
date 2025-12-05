package transform

import (
	"github.com/tenebris-tech/x2md/pdf2md/models"
)

// DetectTOC detects table of contents pages
type DetectTOC struct{}

// NewDetectTOC creates a new DetectTOC transformation
func NewDetectTOC() *DetectTOC {
	return &DetectTOC{}
}

// Transform detects TOC pages
func (d *DetectTOC) Transform(result *models.ParseResult) *models.ParseResult {
	// Initialize globals if needed
	if result.Globals.TOCPages == nil {
		result.Globals.TOCPages = []int{}
	}
	if result.Globals.HeadlineTypeToHeightRange == nil {
		result.Globals.HeadlineTypeToHeightRange = make(map[string]*models.HeightRange)
	}

	// TOC detection is complex - for now, we'll skip it and rely on header detection
	// This is a simplified version that just passes through
	return result
}

// Package transform provides the transformation pipeline for PDF to Markdown conversion
package transform

import (
	"github.com/tenebris-tech/x2md/pdf2md/models"
	"github.com/tenebris-tech/x2md/pdf2md/pdf"
)

// PipelineOptions configures which transformations to apply
type PipelineOptions struct {
	StripHeadersFooters bool
	StripPageNumbers    bool
	StripTOC            bool
	StripFootnotes      bool
	StripBlankPages     bool
}

// Transformation is the interface for all transformations
type Transformation interface {
	Transform(result *models.ParseResult) *models.ParseResult
}

// Pipeline runs all transformations in sequence
type Pipeline struct {
	transformations []Transformation
	options         *PipelineOptions
}

// NewPipeline creates a new transformation pipeline
func NewPipeline(fontMap map[string]*pdf.Font, opts *PipelineOptions) *Pipeline {
	if opts == nil {
		opts = &PipelineOptions{}
	}

	transformations := []Transformation{
		NewCalculateGlobalStats(fontMap),
		NewCompactLines(),
	}

	// Conditionally add stripping transformations
	if opts.StripHeadersFooters {
		transformations = append(transformations, NewRemoveRepetitiveElements())
	}

	transformations = append(transformations,
		NewDetectTOC(),
		NewDetectHeaders(),
		NewDetectListItems(),
		NewGatherBlocks(),
	)

	// Add blank page removal if enabled
	if opts.StripBlankPages {
		transformations = append(transformations, NewRemoveBlankPages())
	}

	transformations = append(transformations,
		NewToTextBlocks(),
		NewToMarkdown(),
	)

	return &Pipeline{
		transformations: transformations,
		options:         opts,
	}
}

// Transform runs all transformations
func (p *Pipeline) Transform(pages []*models.Page) *models.ParseResult {
	result := &models.ParseResult{
		Pages:   pages,
		Globals: &models.Globals{},
	}

	for _, t := range p.transformations {
		result = t.Transform(result)
	}

	return result
}

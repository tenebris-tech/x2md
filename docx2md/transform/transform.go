// Package transform provides the transformation pipeline for DOCX to Markdown conversion
package transform

import (
	"github.com/tenebris-tech/x2md/pdf2md/models"
)

// PipelineOptions configures the transformation pipeline
type PipelineOptions struct {
	// PreserveFormatting keeps bold/italic formatting
	PreserveFormatting bool
}

// Transformation is the interface for pipeline steps
type Transformation interface {
	Transform(result *models.ParseResult) *models.ParseResult
}

// Pipeline orchestrates the transformation steps
type Pipeline struct {
	transformations []Transformation
	options         *PipelineOptions
}

// NewPipeline creates a new transformation pipeline
func NewPipeline(opts *PipelineOptions) *Pipeline {
	if opts == nil {
		opts = &PipelineOptions{
			PreserveFormatting: true,
		}
	}

	return &Pipeline{
		options: opts,
		transformations: []Transformation{
			NewGatherBlocks(),
			NewToTextBlocks(),
			NewToMarkdown(),
		},
	}
}

// Transform runs the pipeline on a page
func (p *Pipeline) Transform(page *models.Page) *models.ParseResult {
	result := &models.ParseResult{
		Pages: []*models.Page{page},
		Globals: &models.Globals{
			FontToFormats: make(map[string]*models.WordFormat),
		},
	}

	for _, t := range p.transformations {
		result = t.Transform(result)
	}

	return result
}

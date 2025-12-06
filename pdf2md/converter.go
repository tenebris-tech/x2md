// Package pdf2md provides a pure Go library to convert PDF files to Markdown
package pdf2md

import (
	"fmt"
	"os"
	"strings"

	"github.com/tenebris-tech/x2md/pdf2md/models"
	"github.com/tenebris-tech/x2md/pdf2md/pdf"
	"github.com/tenebris-tech/x2md/pdf2md/transform"
)

// StripOption specifies content to strip from the output
type StripOption int

const (
	// HeadersFooters strips repetitive page headers and footers
	HeadersFooters StripOption = iota
	// PageNumbers strips page numbers
	PageNumbers
	// TOC strips table of contents pages
	TOC
	// Footnotes strips footnote references and content
	Footnotes
	// BlankPages strips empty or near-empty pages
	BlankPages
)

// DefaultStrip defines what gets stripped by default.
// Change this to modify default behavior without breaking API.
var DefaultStrip = []StripOption{HeadersFooters, BlankPages}

// Converter is the main PDF to Markdown converter
type Converter struct {
	options *Options
}

// Options holds configuration for the converter
type Options struct {
	// StripOptions specifies what content to strip
	StripOptions []StripOption
	// stripExplicitlySet tracks if WithStrip was called
	stripExplicitlySet bool

	// DetectLists enables list detection
	DetectLists bool

	// DetectHeadings enables heading detection
	DetectHeadings bool

	// PreserveFormatting preserves bold/italic formatting
	PreserveFormatting bool

	// PageSeparator is the separator between pages
	PageSeparator string

	// Callbacks for conversion progress
	OnPageParsed         func(pageNum, totalPages int)
	OnFontParsed         func(fontName string)
	OnConversionComplete func()
}

// ShouldStrip checks if a given StripOption is enabled
func (o *Options) ShouldStrip(opt StripOption) bool {
	for _, s := range o.StripOptions {
		if s == opt {
			return true
		}
	}
	return false
}

// Option is a functional option for configuring the converter
type Option func(*Options)

// DefaultOptions returns the default options
func DefaultOptions() *Options {
	return &Options{
		StripOptions:       nil, // Will be set to DefaultStrip if not explicitly set
		stripExplicitlySet: false,
		DetectLists:        true,
		DetectHeadings:     true,
		PreserveFormatting: true,
		PageSeparator:      "\n",
	}
}

// WithStrip sets which content to strip. Pass no arguments to strip nothing.
// If WithStrip is not called, DefaultStrip is used.
func WithStrip(opts ...StripOption) Option {
	return func(o *Options) {
		o.StripOptions = opts
		o.stripExplicitlySet = true
	}
}

// WithDetectLists sets whether to detect lists
func WithDetectLists(detect bool) Option {
	return func(o *Options) {
		o.DetectLists = detect
	}
}

// WithDetectHeadings sets whether to detect headings
func WithDetectHeadings(detect bool) Option {
	return func(o *Options) {
		o.DetectHeadings = detect
	}
}

// WithPreserveFormatting sets whether to preserve bold/italic
func WithPreserveFormatting(preserve bool) Option {
	return func(o *Options) {
		o.PreserveFormatting = preserve
	}
}

// WithPageSeparator sets the page separator
func WithPageSeparator(sep string) Option {
	return func(o *Options) {
		o.PageSeparator = sep
	}
}

// WithOnPageParsed sets the callback for page parsing
func WithOnPageParsed(callback func(pageNum, totalPages int)) Option {
	return func(o *Options) {
		o.OnPageParsed = callback
	}
}

// WithOnFontParsed sets the callback for font parsing
func WithOnFontParsed(callback func(fontName string)) Option {
	return func(o *Options) {
		o.OnFontParsed = callback
	}
}

// WithOnConversionComplete sets the callback for completion
func WithOnConversionComplete(callback func()) Option {
	return func(o *Options) {
		o.OnConversionComplete = callback
	}
}

// New creates a new Converter with the given options
func New(opts ...Option) *Converter {
	options := DefaultOptions()
	for _, opt := range opts {
		opt(options)
	}
	// Apply DefaultStrip if WithStrip was not called
	if !options.stripExplicitlySet {
		options.StripOptions = DefaultStrip
	}
	return &Converter{options: options}
}

// ConvertFile converts a PDF file to Markdown
func (c *Converter) ConvertFile(inputPath string) (string, error) {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return "", fmt.Errorf("reading file: %w", err)
	}
	return c.Convert(data)
}

// ConvertFileToFile converts a PDF file and writes the result to a file
func (c *Converter) ConvertFileToFile(inputPath, outputPath string) error {
	markdown, err := c.ConvertFile(inputPath)
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, []byte(markdown), 0644)
}

// Convert converts PDF data to Markdown
func (c *Converter) Convert(data []byte) (string, error) {
	// Parse PDF
	parser := pdf.NewParser(data)
	if err := parser.Parse(); err != nil {
		return "", fmt.Errorf("parsing PDF: %w", err)
	}

	// Get page count
	pageCount, err := parser.GetPageCount()
	if err != nil {
		return "", fmt.Errorf("getting page count: %w", err)
	}

	// Extract text from each page
	extractor := pdf.NewTextExtractor(parser)
	var pages []*models.Page

	for i := 0; i < pageCount; i++ {
		textItems, err := extractor.ExtractPage(i)
		if err != nil {
			// Skip pages that fail to extract
			continue
		}

		// Convert pdf.TextItem to models.TextItem
		var items []interface{}
		for _, ti := range textItems {
			items = append(items, &models.TextItem{
				X:      ti.X,
				Y:      ti.Y,
				Width:  ti.Width,
				Height: ti.Height,
				Text:   ti.Text,
				Font:   ti.Font,
			})
		}

		// Get page dimensions
		pageWidth, pageHeight, _ := extractor.GetPageDimensions(i)

		pages = append(pages, &models.Page{
			Index:  i,
			Items:  items,
			Width:  pageWidth,
			Height: pageHeight,
		})

		if c.options.OnPageParsed != nil {
			c.options.OnPageParsed(i+1, pageCount)
		}
	}

	// Get fonts for formatting detection
	fonts := extractor.GetFonts()
	if c.options.OnFontParsed != nil {
		for name := range fonts {
			c.options.OnFontParsed(name)
		}
	}

	// Run transformation pipeline
	pipelineOpts := &transform.PipelineOptions{
		StripHeadersFooters: c.options.ShouldStrip(HeadersFooters),
		StripPageNumbers:    c.options.ShouldStrip(PageNumbers),
		StripTOC:            c.options.ShouldStrip(TOC),
		StripFootnotes:      c.options.ShouldStrip(Footnotes),
		StripBlankPages:     c.options.ShouldStrip(BlankPages),
	}
	pipeline := transform.NewPipeline(fonts, pipelineOpts)
	result := pipeline.Transform(pages)

	// Combine page outputs
	var output strings.Builder
	for i, page := range result.Pages {
		for _, item := range page.Items {
			if text, ok := item.(string); ok {
				output.WriteString(text)
			}
		}
		if i < len(result.Pages)-1 {
			output.WriteString(c.options.PageSeparator)
		}
	}

	if c.options.OnConversionComplete != nil {
		c.options.OnConversionComplete()
	}

	return output.String(), nil
}

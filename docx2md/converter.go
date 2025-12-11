// Package docx2md provides a pure Go library to convert DOCX files to Markdown
package docx2md

import (
	"fmt"
	"os"
	"strings"

	"github.com/tenebris-tech/x2md/docx2md/docx"
	"github.com/tenebris-tech/x2md/docx2md/transform"
	"github.com/tenebris-tech/x2md/imageutil"
	"github.com/tenebris-tech/x2md/pdf2md/models"
)

// Converter is the main DOCX to Markdown converter
type Converter struct {
	options *Options
}

// Options holds configuration for the converter
type Options struct {
	// PreserveFormatting preserves bold/italic formatting
	PreserveFormatting bool

	// PreserveImages includes image references in output
	PreserveImages bool

	// ImageLinkFormat is the template for image links (default: "![{alt}]({path})")
	ImageLinkFormat string

	// PageSeparator is the separator between sections (currently unused as DOCX has no pages)
	PageSeparator string

	// Callbacks for conversion progress
	OnDocumentParsed func()
	OnStylesParsed   func(styleCount int)
}

// Option is a functional option for configuring the converter
type Option func(*Options)

// DefaultOptions returns the default options
func DefaultOptions() *Options {
	return &Options{
		PreserveFormatting: true,
		PreserveImages:     true,
		ImageLinkFormat:    "![%s](%s)",
		PageSeparator:      "\n",
	}
}

// WithPreserveFormatting sets whether to preserve bold/italic
func WithPreserveFormatting(preserve bool) Option {
	return func(o *Options) {
		o.PreserveFormatting = preserve
	}
}

// WithPreserveImages sets whether to include image references
func WithPreserveImages(preserve bool) Option {
	return func(o *Options) {
		o.PreserveImages = preserve
	}
}

// WithImageLinkFormat sets the template for image links
func WithImageLinkFormat(format string) Option {
	return func(o *Options) {
		o.ImageLinkFormat = format
	}
}

// WithPageSeparator sets the separator between sections
func WithPageSeparator(sep string) Option {
	return func(o *Options) {
		o.PageSeparator = sep
	}
}

// WithOnDocumentParsed sets the callback for document parsing
func WithOnDocumentParsed(callback func()) Option {
	return func(o *Options) {
		o.OnDocumentParsed = callback
	}
}

// WithOnStylesParsed sets the callback for styles parsing
func WithOnStylesParsed(callback func(styleCount int)) Option {
	return func(o *Options) {
		o.OnStylesParsed = callback
	}
}

// New creates a new Converter with the given options
func New(opts ...Option) *Converter {
	options := DefaultOptions()
	for _, opt := range opts {
		opt(options)
	}
	return &Converter{options: options}
}

// ConvertFile converts a DOCX file to Markdown
func (c *Converter) ConvertFile(inputPath string) (string, error) {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return "", fmt.Errorf("reading file: %w", err)
	}
	return c.Convert(data)
}

// ConvertFileToFile converts a DOCX file and writes the result to a file
func (c *Converter) ConvertFileToFile(inputPath, outputPath string) error {
	// Read input file
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	// Convert to markdown and get images
	markdown, images, err := c.ConvertWithImages(data)
	if err != nil {
		return err
	}

	// Write images if enabled and there are images
	if c.options.PreserveImages && len(images) > 0 {
		markdown, err = c.writeImages(outputPath, markdown, images)
		if err != nil {
			return fmt.Errorf("writing images: %w", err)
		}
	}

	return os.WriteFile(outputPath, []byte(markdown), 0644)
}

// writeImages writes images to disk and updates markdown with correct paths
func (c *Converter) writeImages(outputPath, markdown string, images []*models.ImageItem) (string, error) {
	writer, err := imageutil.NewImageWriter(outputPath)
	if err != nil {
		return markdown, err
	}

	// Build map of image IDs to output paths
	imageMap := make(map[string]string)

	for _, img := range images {
		relativePath, err := writer.WriteImage(img)
		if err != nil {
			// Log warning but continue
			continue
		}
		imageMap[img.ID] = relativePath
	}

	// Replace image placeholders in markdown
	// The placeholder format is ![image_001] (from WordTypeImage.ToText)
	for id, path := range imageMap {
		// Find the image in the list to get alt text
		altText := "image"
		for _, img := range images {
			if img.ID == id {
				if img.AltText != "" {
					altText = img.AltText
				}
				break
			}
		}
		placeholder := fmt.Sprintf("![%s]", id)
		replacement := fmt.Sprintf("![%s](%s)", altText, path)
		markdown = strings.ReplaceAll(markdown, placeholder, replacement)
	}

	return markdown, nil
}

// Convert converts DOCX data to Markdown
func (c *Converter) Convert(data []byte) (string, error) {
	markdown, _, err := c.ConvertWithImages(data)
	return markdown, err
}

// ConvertWithImages converts DOCX data to Markdown and returns extracted images
func (c *Converter) ConvertWithImages(data []byte) (string, []*models.ImageItem, error) {
	// Parse DOCX
	parser, err := docx.NewParser(data)
	if err != nil {
		return "", nil, fmt.Errorf("parsing DOCX: %w", err)
	}

	if err := parser.Parse(); err != nil {
		return "", nil, fmt.Errorf("validating DOCX: %w", err)
	}

	if c.options.OnDocumentParsed != nil {
		c.options.OnDocumentParsed()
	}

	// Create extractor
	extractor, err := docx.NewExtractor(parser)
	if err != nil {
		return "", nil, fmt.Errorf("creating extractor: %w", err)
	}

	// Report styles if callback is set
	if c.options.OnStylesParsed != nil {
		styles := extractor.GetStyles()
		c.options.OnStylesParsed(len(styles.Styles))
	}

	// Extract content to Page format
	page, images, err := extractor.Extract()
	if err != nil {
		return "", nil, fmt.Errorf("extracting content: %w", err)
	}

	// Collect footnotes/endnotes
	footnotes := extractor.GetCollectedFootnotes()

	// Run transformation pipeline
	pipelineOpts := &transform.PipelineOptions{
		PreserveFormatting: c.options.PreserveFormatting,
	}
	pipeline := transform.NewPipeline(pipelineOpts)
	result := pipeline.Transform(page)

	// Combine output
	var output strings.Builder
	for _, page := range result.Pages {
		for _, item := range page.Items {
			if text, ok := item.(string); ok {
				output.WriteString(text)
			}
		}
	}

	// Append footnotes section if any
	if len(footnotes) > 0 {
		output.WriteString("\n\n## Footnotes\n\n")
		for _, fn := range footnotes {
			output.WriteString(fmt.Sprintf("[^%s]: %s\n\n", fn.ID, fn.Content))
		}
	}

	return output.String(), images, nil
}

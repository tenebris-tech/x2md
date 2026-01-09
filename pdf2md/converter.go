// Package pdf2md provides a pure Go library to convert PDF files to Markdown
package pdf2md

import (
	"fmt"
	"os"
	"strings"

	"github.com/tenebris-tech/x2md/imageutil"
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

	// ExtractImages enables image extraction
	ExtractImages bool

	// ScanMode enables automatic detection of scanned pages.
	// When enabled, pages with little/no text but large images are treated as scans
	// and the page image is extracted instead of attempting text extraction.
	ScanMode bool

	// PageSeparator is the separator between pages
	PageSeparator string

	// Callbacks for conversion progress
	OnPageParsed         func(pageNum, totalPages int)
	OnFontParsed         func(fontName string)
	OnConversionComplete func()
	OnPageSkipped        func(pageNum int, reason string)
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
		ExtractImages:      true,
		ScanMode:           true, // Auto-detect scanned pages by default
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

// WithOnPageSkipped sets the callback for skipped pages
func WithOnPageSkipped(callback func(pageNum int, reason string)) Option {
	return func(o *Options) {
		o.OnPageSkipped = callback
	}
}

// WithExtractImages sets whether to extract images
func WithExtractImages(extract bool) Option {
	return func(o *Options) {
		o.ExtractImages = extract
	}
}

// WithScanMode enables automatic detection of scanned pages.
// When enabled, pages with little/no text but containing images are treated as scans.
// The page image is extracted as page_NNN.png instead of attempting text extraction.
func WithScanMode(enabled bool) Option {
	return func(o *Options) {
		o.ScanMode = enabled
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
	if c.options.ExtractImages && len(images) > 0 {
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
		// For PNG format images that are raw data, wrap in PNG
		if img.Format == "png" && len(img.Data) > 0 {
			// Check if it's already PNG (has PNG magic bytes)
			if len(img.Data) < 8 || img.Data[0] != 0x89 || img.Data[1] != 0x50 {
				// Wrap raw data in PNG format
				pngData, err := imageutil.CreatePNG(img.Data, img.Width, img.Height, 8, "DeviceRGB")
				if err == nil {
					img.Data = pngData
				}
			}
		}

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

// Convert converts PDF data to Markdown
func (c *Converter) Convert(data []byte) (string, error) {
	markdown, _, err := c.ConvertWithImages(data)
	return markdown, err
}

// ConvertWithImages converts PDF data to Markdown and returns extracted images
func (c *Converter) ConvertWithImages(data []byte) (string, []*models.ImageItem, error) {
	// Parse PDF
	parser := pdf.NewParser(data)
	if err := parser.Parse(); err != nil {
		return "", nil, fmt.Errorf("parsing PDF: %w", err)
	}

	// Check for encryption
	if parser.IsEncrypted() {
		msg := "# Conversion Failed\n\n" +
			"This PDF document is encrypted and requires a password to access its contents.\n\n" +
			"The document could not be converted to Markdown.\n"
		return msg, nil, nil
	}

	// Get page count
	pageCount, err := parser.GetPageCount()
	if err != nil {
		return "", nil, fmt.Errorf("getting page count: %w", err)
	}

	// Extract text from each page
	extractor := pdf.NewTextExtractor(parser)
	var pages []*models.Page
	var allImages []*models.ImageItem
	var scannedPageImages []*models.ImageItem // Page images for scanned pages
	imageCounter := 0

	for i := 0; i < pageCount; i++ {
		textItems, err := extractor.ExtractPage(i)
		if err != nil {
			// Skip pages that fail to extract, notify via callback
			if c.options.OnPageSkipped != nil {
				c.options.OnPageSkipped(i+1, err.Error())
			}
			continue
		}

		// Get page dimensions
		pageWidth, pageHeight, _ := extractor.GetPageDimensions(i)

		// Get page images
		var pageImages []*pdf.ImageData
		var imageNames []string
		if c.options.ExtractImages || c.options.ScanMode {
			pageImages, imageNames, _ = parser.GetAllPageImages(i)
		}

		// Check if this page is a scan (ScanMode enabled)
		if c.options.ScanMode && c.isScannedPage(textItems, pageImages, pageWidth, pageHeight) {
			// This is a scanned page - extract the largest image as the page image
			if len(pageImages) > 0 {
				// Find the largest image (likely the full page scan)
				largestImg := c.findLargestImage(pageImages)
				if largestImg != nil {
					pageImageID := fmt.Sprintf("page_%03d", i+1)
					ext := ".png"
					if largestImg.Format == "jpeg" || largestImg.Format == "jpg" {
						ext = ".jpg"
					}
					img := &models.ImageItem{
						ID:         pageImageID,
						SourcePath: pageImageID,
						OutputPath: pageImageID + ext, // Use page_XXX naming for file
						Format:     largestImg.Format,
						Data:       largestImg.Data,
						AltText:    fmt.Sprintf("Page %d", i+1),
						PageIndex:  i,
						Width:      largestImg.Width,
						Height:     largestImg.Height,
					}
					scannedPageImages = append(scannedPageImages, img)
				}
			}

			// Add empty page to maintain page structure
			pages = append(pages, &models.Page{
				Index:    i,
				Items:    nil, // No text items for scanned pages
				Width:    pageWidth,
				Height:   pageHeight,
				IsScanned: true,
			})

			if c.options.OnPageParsed != nil {
				c.options.OnPageParsed(i+1, pageCount)
			}
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

		// Extract images from this page if enabled (non-scanned pages)
		if c.options.ExtractImages && len(pageImages) > 0 {
			for j, imgData := range pageImages {
				imageCounter++
				imgName := ""
				if j < len(imageNames) {
					imgName = imageNames[j]
				}

				img := &models.ImageItem{
					ID:         fmt.Sprintf("image_%03d", imageCounter),
					SourcePath: imgName,
					Format:     imgData.Format,
					Data:       imgData.Data,
					AltText:    imgName,
					PageIndex:  i,
					Width:      imgData.Width,
					Height:     imgData.Height,
				}
				allImages = append(allImages, img)
			}
		}

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

	// Handle scanned vs non-scanned pages
	if len(scannedPageImages) > 0 && len(scannedPageImages) == pageCount {
		// All pages are scanned - just output image references
		for i, img := range scannedPageImages {
			output.WriteString(fmt.Sprintf("![%s]\n\n", img.ID))
			if i < len(scannedPageImages)-1 {
				output.WriteString(c.options.PageSeparator)
			}
		}
	} else if len(scannedPageImages) > 0 {
		// Mixed document - some scanned, some text
		// Output scanned page images in order, interleaved with text content
		scannedPageIdx := 0
		for pageIdx := 0; pageIdx < pageCount; pageIdx++ {
			// Check if this page was scanned
			if scannedPageIdx < len(scannedPageImages) &&
				scannedPageImages[scannedPageIdx].PageIndex == pageIdx {
				// This is a scanned page
				img := scannedPageImages[scannedPageIdx]
				output.WriteString(fmt.Sprintf("![%s]\n\n", img.ID))
				scannedPageIdx++
			}
			// Text content from pipeline will be at the end
		}
		// Add pipeline text output (for non-scanned pages)
		for _, page := range result.Pages {
			for _, item := range page.Items {
				if text, ok := item.(string); ok {
					output.WriteString(text)
				}
			}
		}
	} else {
		// No scanned pages - normal text output
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
	}

	// Combine all images (scanned pages + regular images)
	allImages = append(scannedPageImages, allImages...)

	// Add image references at the end if there are regular images
	// (scanned page images are already referenced inline)
	if len(allImages) > len(scannedPageImages) {
		output.WriteString("\n\n## Images\n\n")
		for _, img := range allImages[len(scannedPageImages):] {
			// Write placeholder that will be replaced with actual path
			output.WriteString(fmt.Sprintf("![%s]\n\n", img.ID))
		}
	}

	if c.options.OnConversionComplete != nil {
		c.options.OnConversionComplete()
	}

	// Check for empty output
	markdown := output.String()
	if strings.TrimSpace(markdown) == "" && len(allImages) == 0 {
		msg := "# Conversion Failed\n\n" +
			"No text content could be extracted from this PDF document.\n\n" +
			"Possible reasons:\n" +
			"- The PDF contains only scanned images without a text layer (OCR required)\n" +
			"- The PDF uses an unsupported text encoding or font structure\n" +
			"- The PDF content streams could not be parsed\n\n" +
			fmt.Sprintf("Document info: %d pages\n", pageCount)
		return msg, nil, nil
	}

	return markdown, allImages, nil
}

// isScannedPage determines if a page is likely a scanned image.
// A page is considered scanned if it has very little text and contains images.
func (c *Converter) isScannedPage(textItems []pdf.TextItem, images []*pdf.ImageData, pageWidth, pageHeight float64) bool {
	// No images = not a scan
	if len(images) == 0 {
		return false
	}

	// Calculate total text length
	totalTextLen := 0
	for _, item := range textItems {
		totalTextLen += len(strings.TrimSpace(item.Text))
	}

	// If very little text (less than 100 characters), likely a scan
	if totalTextLen < 100 {
		// Check if any image is large enough to be a page scan
		// (at least 50% of page dimensions)
		for _, img := range images {
			imgWidth := float64(img.Width)
			imgHeight := float64(img.Height)

			// Image should cover significant portion of page
			if imgWidth > pageWidth*0.5 || imgHeight > pageHeight*0.5 {
				return true
			}

			// Or if it's a reasonably sized image (at least 500x500)
			if imgWidth >= 500 && imgHeight >= 500 {
				return true
			}
		}
	}

	return false
}

// findLargestImage returns the largest image by pixel count
func (c *Converter) findLargestImage(images []*pdf.ImageData) *pdf.ImageData {
	if len(images) == 0 {
		return nil
	}

	largest := images[0]
	largestPixels := largest.Width * largest.Height

	for _, img := range images[1:] {
		pixels := img.Width * img.Height
		if pixels > largestPixels {
			largest = img
			largestPixels = pixels
		}
	}

	return largest
}

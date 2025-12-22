// Package convert provides a unified API for converting documents to Markdown.
// It wraps the pdf2md and docx2md packages and adds support for batch processing,
// recursive directory traversal, and output directory management.
package convert

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tenebris-tech/x2md/docx2md"
	"github.com/tenebris-tech/x2md/pdf2md"
)

// DefaultExtensions lists the file extensions supported by default
var DefaultExtensions = []string{".pdf", ".docx"}

// Converter handles document to Markdown conversion
type Converter struct {
	options *Options
	// Track visited directories and files to avoid loops and duplicates
	visitedDirs    map[string]bool
	processedFiles map[string]bool
}

// Options holds configuration for the converter
type Options struct {
	// Recursion enables recursive directory traversal
	Recursion bool

	// Extensions lists file extensions to convert (default: .pdf, .docx)
	Extensions []string

	// SkipExisting skips files where .md already exists (default: true)
	SkipExisting bool

	// OutputDirectory writes all output files to this directory (flat structure)
	// If empty, output files are placed next to source files
	OutputDirectory string

	// Verbose enables verbose output
	Verbose bool

	// PDFOptions are passed to the PDF converter
	PDFOptions []pdf2md.Option

	// DOCXOptions are passed to the DOCX converter
	DOCXOptions []docx2md.Option

	// OnFileStart is called when starting to convert a file
	OnFileStart func(path string)

	// OnFileComplete is called when a file conversion completes
	OnFileComplete func(path, outputPath string, err error)

	// OnFileSkipped is called when a file is skipped (e.g., .md already exists)
	OnFileSkipped func(path, outputPath, reason string)
}

// Result contains the results of a conversion operation
type Result struct {
	Converted int
	Skipped   int
	Failed    int
	Errors    []error
}

// Option is a functional option for configuring the converter
type Option func(*Options)

// DefaultOptions returns the default options
func DefaultOptions() *Options {
	return &Options{
		Recursion:    false,
		Extensions:   DefaultExtensions,
		SkipExisting: true,
	}
}

// WithRecursion enables or disables recursive directory traversal
func WithRecursion(recursive bool) Option {
	return func(o *Options) {
		o.Recursion = recursive
	}
}

// WithExtensions sets the file extensions to convert
func WithExtensions(exts []string) Option {
	return func(o *Options) {
		// Normalize extensions to lowercase with leading dot
		normalized := make([]string, len(exts))
		for i, ext := range exts {
			ext = strings.ToLower(ext)
			if !strings.HasPrefix(ext, ".") {
				ext = "." + ext
			}
			normalized[i] = ext
		}
		o.Extensions = normalized
	}
}

// WithSkipExisting sets whether to skip files where .md already exists
func WithSkipExisting(skip bool) Option {
	return func(o *Options) {
		o.SkipExisting = skip
	}
}

// WithOutputDirectory sets the output directory for converted files
func WithOutputDirectory(dir string) Option {
	return func(o *Options) {
		o.OutputDirectory = dir
	}
}

// WithVerbose enables verbose output
func WithVerbose(verbose bool) Option {
	return func(o *Options) {
		o.Verbose = verbose
	}
}

// WithPDFOptions sets options to pass to the PDF converter
func WithPDFOptions(opts ...pdf2md.Option) Option {
	return func(o *Options) {
		o.PDFOptions = opts
	}
}

// WithDOCXOptions sets options to pass to the DOCX converter
func WithDOCXOptions(opts ...docx2md.Option) Option {
	return func(o *Options) {
		o.DOCXOptions = opts
	}
}

// WithOnFileStart sets the callback for when file conversion starts
func WithOnFileStart(callback func(path string)) Option {
	return func(o *Options) {
		o.OnFileStart = callback
	}
}

// WithOnFileComplete sets the callback for when file conversion completes
func WithOnFileComplete(callback func(path, outputPath string, err error)) Option {
	return func(o *Options) {
		o.OnFileComplete = callback
	}
}

// WithOnFileSkipped sets the callback for when a file is skipped
func WithOnFileSkipped(callback func(path, outputPath, reason string)) Option {
	return func(o *Options) {
		o.OnFileSkipped = callback
	}
}

// New creates a new Converter with the given options
func New(opts ...Option) *Converter {
	options := DefaultOptions()
	for _, opt := range opts {
		opt(options)
	}
	return &Converter{
		options:        options,
		visitedDirs:    make(map[string]bool),
		processedFiles: make(map[string]bool),
	}
}

// Convert converts a file or directory to Markdown.
// If path is a file, it converts that file.
// If path is a directory and Recursion is enabled, it recursively converts all matching files.
// Returns an error if path is a directory and Recursion is disabled.
func (c *Converter) Convert(path string) (*Result, error) {
	// Reset tracking maps for each Convert call
	c.visitedDirs = make(map[string]bool)
	c.processedFiles = make(map[string]bool)

	result := &Result{}

	// Get file info, following symlinks
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("cannot access %s: %w", path, err)
	}

	// Create output directory if specified
	if c.options.OutputDirectory != "" {
		if err := os.MkdirAll(c.options.OutputDirectory, 0755); err != nil {
			return nil, fmt.Errorf("cannot create output directory: %w", err)
		}
	}

	if info.IsDir() {
		if !c.options.Recursion {
			return nil, fmt.Errorf("%s is a directory; use WithRecursion(true) to process directories", path)
		}
		c.walkDir(path, result)
	} else {
		c.processFile(path, result)
	}

	return result, nil
}

// walkDir recursively walks a directory, following symlinks
func (c *Converter) walkDir(dir string, result *Result) {
	// Resolve to real path to detect loops
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		result.Failed++
		result.Errors = append(result.Errors, fmt.Errorf("cannot resolve %s: %w", dir, err))
		return
	}

	// Check if we've already visited this real directory
	if c.visitedDirs[realDir] {
		return
	}
	c.visitedDirs[realDir] = true

	// Read directory contents
	entries, err := os.ReadDir(dir)
	if err != nil {
		result.Failed++
		result.Errors = append(result.Errors, fmt.Errorf("cannot read directory %s: %w", dir, err))
		return
	}

	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())

		// Get file info, following symlinks
		info, err := os.Stat(path)
		if err != nil {
			// Only count as failure if it looks like a convertible file
			ext := strings.ToLower(filepath.Ext(path))
			if c.hasExtension(ext) {
				result.Failed++
				result.Errors = append(result.Errors, fmt.Errorf("cannot access %s: %w", path, err))
			}
			// Silently skip broken symlinks to directories or non-convertible files
			continue
		}

		if info.IsDir() {
			c.walkDir(path, result)
		} else {
			c.processFile(path, result)
		}
	}
}

// processFile converts a single file if it matches the configured extensions
func (c *Converter) processFile(path string, result *Result) {
	// Check file extension
	ext := strings.ToLower(filepath.Ext(path))
	if !c.hasExtension(ext) {
		return
	}

	// Resolve symlinks to get real path
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		result.Failed++
		result.Errors = append(result.Errors, fmt.Errorf("cannot resolve %s: %w", path, err))
		return
	}

	// Skip if we've already processed this real file
	if c.processedFiles[realPath] {
		return
	}
	c.processedFiles[realPath] = true

	// Determine output path
	outputPath, skip, reason := c.getOutputPath(realPath, ext)
	if skip {
		if c.options.OnFileSkipped != nil {
			c.options.OnFileSkipped(realPath, outputPath, reason)
		}
		result.Skipped++
		return
	}

	// Notify start
	if c.options.OnFileStart != nil {
		c.options.OnFileStart(realPath)
	}

	// Convert the file
	var convErr error
	switch ext {
	case ".pdf":
		convErr = c.convertPDF(realPath, outputPath)
	case ".docx":
		convErr = c.convertDOCX(realPath, outputPath)
	}

	// Notify completion
	if c.options.OnFileComplete != nil {
		c.options.OnFileComplete(realPath, outputPath, convErr)
	}

	if convErr != nil {
		result.Failed++
		result.Errors = append(result.Errors, fmt.Errorf("%s: %w", realPath, convErr))
	} else {
		result.Converted++
	}
}

// hasExtension checks if the given extension is in the configured list
func (c *Converter) hasExtension(ext string) bool {
	for _, e := range c.options.Extensions {
		if e == ext {
			return true
		}
	}
	return false
}

// getOutputPath determines the output path for a given input file.
// Returns the output path, whether to skip the file, and the skip reason.
func (c *Converter) getOutputPath(inputPath, ext string) (string, bool, string) {
	// Append .md to full filename (e.g., file.pdf -> file.pdf.md)
	baseName := filepath.Base(inputPath)

	var outputPath string
	if c.options.OutputDirectory != "" {
		// Output to specified directory
		outputPath = filepath.Join(c.options.OutputDirectory, baseName+".md")
	} else {
		// Output next to source file
		outputPath = inputPath + ".md"
	}

	// Check if output file already exists
	if _, err := os.Stat(outputPath); err == nil {
		if c.options.SkipExisting {
			return outputPath, true, "output file exists"
		}
		// Find a unique name
		outputPath = c.findUniquePath(outputPath)
	}

	return outputPath, false, ""
}

// findUniquePath finds a unique output path by appending a number
func (c *Converter) findUniquePath(basePath string) string {
	ext := filepath.Ext(basePath)
	nameWithoutExt := strings.TrimSuffix(basePath, ext)

	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s-%d%s", nameWithoutExt, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

// convertPDF converts a PDF file to Markdown
func (c *Converter) convertPDF(inputPath, outputPath string) error {
	converter := pdf2md.New(c.options.PDFOptions...)
	return converter.ConvertFileToFile(inputPath, outputPath)
}

// convertDOCX converts a DOCX file to Markdown
func (c *Converter) convertDOCX(inputPath, outputPath string) error {
	converter := docx2md.New(c.options.DOCXOptions...)
	return converter.ConvertFileToFile(inputPath, outputPath)
}

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tenebris-tech/x2md/docx2md"
	"github.com/tenebris-tech/x2md/pdf2md"
)

func main() {
	// Parse command line flags
	inputFile := flag.String("input", "", "Input file path (PDF or DOCX)")
	outputFile := flag.String("output", "", "Output Markdown file path (default: input file with .md extension)")
	stripNone := flag.Bool("strip-none", false, "Don't strip anything (overrides default) [PDF only]")
	stripHeaders := flag.Bool("strip-headers", false, "Strip repetitive headers/footers [PDF only]")
	stripPageNumbers := flag.Bool("strip-page-numbers", false, "Strip page numbers [PDF only]")
	stripTOC := flag.Bool("strip-toc", false, "Strip table of contents [PDF only]")
	stripFootnotes := flag.Bool("strip-footnotes", false, "Strip footnotes [PDF only]")
	stripBlankPages := flag.Bool("strip-blank-pages", false, "Strip blank pages [PDF only]")
	noLists := flag.Bool("no-lists", false, "Don't detect lists [PDF only]")
	noHeadings := flag.Bool("no-headings", false, "Don't detect headings [PDF only]")
	noFormatting := flag.Bool("no-formatting", false, "Don't preserve bold/italic formatting")
	verbose := flag.Bool("v", false, "Verbose output")

	flag.Parse()

	// Check for positional argument
	if *inputFile == "" && flag.NArg() > 0 {
		*inputFile = flag.Arg(0)
	}

	if *inputFile == "" {
		fmt.Println("Usage: x2md [options] <input.pdf|input.docx>")
		fmt.Println()
		fmt.Println("Converts PDF or DOCX files to Markdown.")
		fmt.Println()
		fmt.Println("Options:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Determine file type from extension
	ext := strings.ToLower(filepath.Ext(*inputFile))

	// Set default output file
	if *outputFile == "" {
		*outputFile = strings.TrimSuffix(*inputFile, filepath.Ext(*inputFile)) + ".md"
	}

	fmt.Printf("Converting %s to %s...\n", *inputFile, *outputFile)

	var err error

	switch ext {
	case ".pdf":
		err = convertPDF(*inputFile, *outputFile, &pdfOptions{
			stripNone:       *stripNone,
			stripHeaders:    *stripHeaders,
			stripPageNumbers: *stripPageNumbers,
			stripTOC:        *stripTOC,
			stripFootnotes:  *stripFootnotes,
			stripBlankPages: *stripBlankPages,
			noLists:         *noLists,
			noHeadings:      *noHeadings,
			noFormatting:    *noFormatting,
			verbose:         *verbose,
		})

	case ".docx":
		err = convertDOCX(*inputFile, *outputFile, &docxOptions{
			noFormatting: *noFormatting,
			verbose:      *verbose,
		})

	default:
		fmt.Fprintf(os.Stderr, "Error: unsupported file type: %s\n", ext)
		fmt.Fprintf(os.Stderr, "Supported types: .pdf, .docx\n")
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Conversion complete!")
}

type pdfOptions struct {
	stripNone        bool
	stripHeaders     bool
	stripPageNumbers bool
	stripTOC         bool
	stripFootnotes   bool
	stripBlankPages  bool
	noLists          bool
	noHeadings       bool
	noFormatting     bool
	verbose          bool
}

func convertPDF(inputFile, outputFile string, opts *pdfOptions) error {
	var converterOpts []pdf2md.Option

	// Handle strip options
	if opts.stripNone || opts.stripHeaders || opts.stripPageNumbers || opts.stripTOC || opts.stripFootnotes || opts.stripBlankPages {
		var stripOpts []pdf2md.StripOption
		if opts.stripHeaders {
			stripOpts = append(stripOpts, pdf2md.HeadersFooters)
		}
		if opts.stripPageNumbers {
			stripOpts = append(stripOpts, pdf2md.PageNumbers)
		}
		if opts.stripTOC {
			stripOpts = append(stripOpts, pdf2md.TOC)
		}
		if opts.stripFootnotes {
			stripOpts = append(stripOpts, pdf2md.Footnotes)
		}
		if opts.stripBlankPages {
			stripOpts = append(stripOpts, pdf2md.BlankPages)
		}
		converterOpts = append(converterOpts, pdf2md.WithStrip(stripOpts...))
	}

	if opts.noLists {
		converterOpts = append(converterOpts, pdf2md.WithDetectLists(false))
	}
	if opts.noHeadings {
		converterOpts = append(converterOpts, pdf2md.WithDetectHeadings(false))
	}
	if opts.noFormatting {
		converterOpts = append(converterOpts, pdf2md.WithPreserveFormatting(false))
	}

	if opts.verbose {
		converterOpts = append(converterOpts, pdf2md.WithOnPageParsed(func(pageNum, totalPages int) {
			fmt.Printf("Processing page %d/%d\n", pageNum, totalPages)
		}))
		converterOpts = append(converterOpts, pdf2md.WithOnFontParsed(func(fontName string) {
			fmt.Printf("Found font: %s\n", fontName)
		}))
	}

	converter := pdf2md.New(converterOpts...)
	return converter.ConvertFileToFile(inputFile, outputFile)
}

type docxOptions struct {
	noFormatting bool
	verbose      bool
}

func convertDOCX(inputFile, outputFile string, opts *docxOptions) error {
	var converterOpts []docx2md.Option

	if opts.noFormatting {
		converterOpts = append(converterOpts, docx2md.WithPreserveFormatting(false))
	}

	if opts.verbose {
		converterOpts = append(converterOpts, docx2md.WithOnDocumentParsed(func() {
			fmt.Println("Document parsed")
		}))
		converterOpts = append(converterOpts, docx2md.WithOnStylesParsed(func(count int) {
			fmt.Printf("Found %d styles\n", count)
		}))
	}

	converter := docx2md.New(converterOpts...)
	return converter.ConvertFileToFile(inputFile, outputFile)
}

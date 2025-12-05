package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tenebris-tech/x2md/pdf2md"
)

func main() {

	// Parse command line flags
	inputFile := flag.String("input", "", "Input PDF file path")
	outputFile := flag.String("output", "", "Output Markdown file path (default: input file with .md extension)")
	stripNone := flag.Bool("strip-none", false, "Don't strip anything (overrides default)")
	stripHeaders := flag.Bool("strip-headers", false, "Strip repetitive headers/footers")
	stripPageNumbers := flag.Bool("strip-page-numbers", false, "Strip page numbers")
	stripTOC := flag.Bool("strip-toc", false, "Strip table of contents")
	stripFootnotes := flag.Bool("strip-footnotes", false, "Strip footnotes")
	stripBlankPages := flag.Bool("strip-blank-pages", false, "Strip blank pages")
	noLists := flag.Bool("no-lists", false, "Don't detect lists")
	noHeadings := flag.Bool("no-headings", false, "Don't detect headings")
	noFormatting := flag.Bool("no-formatting", false, "Don't preserve bold/italic formatting")
	verbose := flag.Bool("v", false, "Verbose output")

	flag.Parse()

	// Check for positional argument
	if *inputFile == "" && flag.NArg() > 0 {
		*inputFile = flag.Arg(0)
	}

	if *inputFile == "" {
		fmt.Println("Usage: pdf2md [options] <input.pdf>")
		fmt.Println()
		fmt.Println("Options:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Set default output file
	if *outputFile == "" {
		ext := filepath.Ext(*inputFile)
		*outputFile = strings.TrimSuffix(*inputFile, ext) + ".md"
	}

	// Build options
	var opts []pdf2md.Option

	// Handle strip options
	// If any strip flag is explicitly set, use explicit mode
	if *stripNone || *stripHeaders || *stripPageNumbers || *stripTOC || *stripFootnotes || *stripBlankPages {
		var stripOpts []pdf2md.StripOption
		if *stripHeaders {
			stripOpts = append(stripOpts, pdf2md.HeadersFooters)
		}
		if *stripPageNumbers {
			stripOpts = append(stripOpts, pdf2md.PageNumbers)
		}
		if *stripTOC {
			stripOpts = append(stripOpts, pdf2md.TOC)
		}
		if *stripFootnotes {
			stripOpts = append(stripOpts, pdf2md.Footnotes)
		}
		if *stripBlankPages {
			stripOpts = append(stripOpts, pdf2md.BlankPages)
		}
		opts = append(opts, pdf2md.WithStrip(stripOpts...))
	}
	// Otherwise, DefaultStrip is used automatically

	if *noLists {
		opts = append(opts, pdf2md.WithDetectLists(false))
	}
	if *noHeadings {
		opts = append(opts, pdf2md.WithDetectHeadings(false))
	}
	if *noFormatting {
		opts = append(opts, pdf2md.WithPreserveFormatting(false))
	}

	if *verbose {
		opts = append(opts, pdf2md.WithOnPageParsed(func(pageNum, totalPages int) {
			fmt.Printf("Processing page %d/%d\n", pageNum, totalPages)
		}))
		opts = append(opts, pdf2md.WithOnFontParsed(func(fontName string) {
			fmt.Printf("Found font: %s\n", fontName)
		}))
	}

	// Create converter
	converter := pdf2md.New(opts...)

	// Convert
	fmt.Printf("Converting %s to %s...\n", *inputFile, *outputFile)

	err := converter.ConvertFileToFile(*inputFile, *outputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Conversion complete!")
}

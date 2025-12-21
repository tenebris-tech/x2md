package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tenebris-tech/x2md/convert"
	"github.com/tenebris-tech/x2md/docx2md"
	"github.com/tenebris-tech/x2md/pdf2md"
)

func main() {
	// Parse command line flags
	recursive := flag.Bool("r", false, "Recursively process directories")
	outputDir := flag.String("output-dir", "", "Output directory for converted files (flat structure)")
	outputFile := flag.String("output", "", "Output file path (single file mode only)")
	skipExisting := flag.Bool("skip-existing", true, "Skip files where .md already exists")

	// PDF-specific options
	stripNone := flag.Bool("strip-none", false, "Don't strip anything (overrides default) [PDF only]")
	stripHeaders := flag.Bool("strip-headers", false, "Strip repetitive headers/footers [PDF only]")
	stripPageNumbers := flag.Bool("strip-page-numbers", false, "Strip page numbers [PDF only]")
	stripTOC := flag.Bool("strip-toc", false, "Strip table of contents [PDF only]")
	stripFootnotes := flag.Bool("strip-footnotes", false, "Strip footnotes [PDF only]")
	stripBlankPages := flag.Bool("strip-blank-pages", false, "Strip blank pages [PDF only]")
	noLists := flag.Bool("no-lists", false, "Don't detect lists [PDF only]")
	noHeadings := flag.Bool("no-headings", false, "Don't detect headings [PDF only]")
	noScanMode := flag.Bool("no-scan-mode", false, "Disable automatic scanned page detection [PDF only]")

	// Common options
	noFormatting := flag.Bool("no-formatting", false, "Don't preserve bold/italic formatting")
	noImages := flag.Bool("no-images", false, "Don't extract images")
	verbose := flag.Bool("v", false, "Verbose output")

	flag.Parse()

	// Check for positional argument
	inputPath := ""
	if flag.NArg() > 0 {
		inputPath = flag.Arg(0)
	}

	if inputPath == "" {
		printUsage()
		os.Exit(1)
	}

	// Build PDF options
	var pdfOpts []pdf2md.Option
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
		pdfOpts = append(pdfOpts, pdf2md.WithStrip(stripOpts...))
	}
	if *noLists {
		pdfOpts = append(pdfOpts, pdf2md.WithDetectLists(false))
	}
	if *noHeadings {
		pdfOpts = append(pdfOpts, pdf2md.WithDetectHeadings(false))
	}
	if *noFormatting {
		pdfOpts = append(pdfOpts, pdf2md.WithPreserveFormatting(false))
	}
	if *noImages {
		pdfOpts = append(pdfOpts, pdf2md.WithExtractImages(false))
	}
	if *noScanMode {
		pdfOpts = append(pdfOpts, pdf2md.WithScanMode(false))
	}

	// Build DOCX options
	var docxOpts []docx2md.Option
	if *noFormatting {
		docxOpts = append(docxOpts, docx2md.WithPreserveFormatting(false))
	}
	if *noImages {
		docxOpts = append(docxOpts, docx2md.WithPreserveImages(false))
	}

	// Build converter options
	var converterOpts []convert.Option
	converterOpts = append(converterOpts, convert.WithRecursion(*recursive))
	converterOpts = append(converterOpts, convert.WithSkipExisting(*skipExisting))

	if *outputDir != "" {
		converterOpts = append(converterOpts, convert.WithOutputDirectory(*outputDir))
	}

	if len(pdfOpts) > 0 {
		converterOpts = append(converterOpts, convert.WithPDFOptions(pdfOpts...))
	}
	if len(docxOpts) > 0 {
		converterOpts = append(converterOpts, convert.WithDOCXOptions(docxOpts...))
	}

	// Add verbose callbacks
	if *verbose {
		converterOpts = append(converterOpts, convert.WithOnFileStart(func(path string) {
			fmt.Printf("Converting: %s\n", path)
		}))
		converterOpts = append(converterOpts, convert.WithOnFileComplete(func(path, outputPath string, err error) {
			if err != nil {
				fmt.Printf("  Error: %v\n", err)
			} else {
				fmt.Printf("  Created: %s\n", outputPath)
			}
		}))

		// Add PDF-specific verbose callbacks
		pdfOpts = append(pdfOpts,
			pdf2md.WithOnPageParsed(func(pageNum, totalPages int) {
				fmt.Printf("  Processing page %d/%d\n", pageNum, totalPages)
			}),
			pdf2md.WithOnFontParsed(func(fontName string) {
				fmt.Printf("  Found font: %s\n", fontName)
			}),
		)
		converterOpts = append(converterOpts, convert.WithPDFOptions(pdfOpts...))

		// Add DOCX-specific verbose callbacks
		docxOpts = append(docxOpts,
			docx2md.WithOnDocumentParsed(func() {
				fmt.Println("  Document parsed")
			}),
			docx2md.WithOnStylesParsed(func(count int) {
				fmt.Printf("  Found %d styles\n", count)
			}),
		)
		converterOpts = append(converterOpts, convert.WithDOCXOptions(docxOpts...))
	}

	// Handle single file with explicit output path
	if *outputFile != "" && !*recursive {
		// Use the underlying converters directly for explicit output path
		info, err := os.Stat(inputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if info.IsDir() {
			fmt.Fprintf(os.Stderr, "Error: -output cannot be used with directories\n")
			os.Exit(1)
		}

		if *verbose {
			fmt.Printf("Converting %s to %s...\n", inputPath, *outputFile)
		}

		err = convertSingleFile(inputPath, *outputFile, pdfOpts, docxOpts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Conversion complete!")
		return
	}

	// Use convert package for all other cases
	converter := convert.New(converterOpts...)
	result, err := converter.Convert(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Print summary
	if *recursive || *verbose {
		fmt.Printf("\nComplete: %d converted, %d skipped, %d failed\n",
			result.Converted, result.Skipped, result.Failed)
	} else if result.Converted > 0 {
		fmt.Println("Conversion complete!")
	}

	if result.Failed > 0 {
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: x2md [options] <input.pdf|input.docx|directory>")
	fmt.Println()
	fmt.Println("Converts PDF or DOCX files to Markdown.")
	fmt.Println()
	fmt.Println("Options:")
	flag.PrintDefaults()
}

func convertSingleFile(inputPath, outputPath string, pdfOpts []pdf2md.Option, docxOpts []docx2md.Option) error {
	ext := getExtension(inputPath)

	switch ext {
	case ".pdf":
		converter := pdf2md.New(pdfOpts...)
		return converter.ConvertFileToFile(inputPath, outputPath)
	case ".docx":
		converter := docx2md.New(docxOpts...)
		return converter.ConvertFileToFile(inputPath, outputPath)
	default:
		return fmt.Errorf("unsupported file type: %s (supported: .pdf, .docx)", ext)
	}
}

func getExtension(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			ext := path[i:]
			// Convert to lowercase
			result := make([]byte, len(ext))
			for j := 0; j < len(ext); j++ {
				c := ext[j]
				if c >= 'A' && c <= 'Z' {
					c += 'a' - 'A'
				}
				result[j] = c
			}
			return string(result)
		}
		if path[i] == '/' || path[i] == '\\' {
			break
		}
	}
	return ""
}

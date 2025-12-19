package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tenebris-tech/x2md/docx2md"
	"github.com/tenebris-tech/x2md/pdf2md"
)

// Track visited directories and files to avoid loops and duplicates
var visitedDirs = make(map[string]bool)
var processedFiles = make(map[string]bool)

var converted, skipped, failed int

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: x2md-batch <directory>")
		fmt.Println()
		fmt.Println("Recursively converts all PDF and DOCX files to Markdown.")
		fmt.Println("Follows symlinks to directories.")
		fmt.Println("Skips files that already have a .md version.")
		fmt.Println("Tracks real paths to avoid duplicate work and loops.")
		os.Exit(1)
	}

	startDir := os.Args[1]

	// Verify directory exists
	info, err := os.Stat(startDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if !info.IsDir() {
		fmt.Fprintf(os.Stderr, "Error: %s is not a directory\n", startDir)
		os.Exit(1)
	}

	// Walk the directory tree, following symlinks
	walkDir(startDir)

	fmt.Println()
	fmt.Printf("Complete: %d converted, %d skipped (already exist), %d failed\n", converted, skipped, failed)

	if failed > 0 {
		os.Exit(1)
	}
}

// walkDir recursively walks a directory, following symlinks
func walkDir(dir string) {
	// Resolve to real path to detect loops
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: cannot resolve %s: %v\n", dir, err)
		return
	}

	// Check if we've already visited this real directory
	if visitedDirs[realDir] {
		return
	}
	visitedDirs[realDir] = true

	// Read directory contents
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: cannot read directory %s: %v\n", dir, err)
		return
	}

	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())

		// Get file info, following symlinks
		info, err := os.Stat(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: cannot stat %s: %v\n", path, err)
			continue
		}

		if info.IsDir() {
			// Recurse into directory (including symlinked directories)
			walkDir(path)
		} else {
			// Process file
			processFile(path)
		}
	}
}

// processFile converts a PDF or DOCX file to markdown if needed
func processFile(path string) {
	// Check file extension
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".pdf" && ext != ".docx" {
		return
	}

	// Resolve symlinks to get real path
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: cannot resolve %s: %v\n", path, err)
		return
	}

	// Skip if we've already processed this real file
	if processedFiles[realPath] {
		return
	}
	processedFiles[realPath] = true

	// Check if .md version already exists
	mdPath := strings.TrimSuffix(realPath, ext) + ".md"
	if _, err := os.Stat(mdPath); err == nil {
		skipped++
		return
	}

	// Convert the file
	fmt.Printf("Converting: %s\n", realPath)

	var convErr error
	switch ext {
	case ".pdf":
		convErr = convertPDF(realPath, mdPath)
	case ".docx":
		convErr = convertDOCX(realPath, mdPath)
	}

	if convErr != nil {
		fmt.Fprintf(os.Stderr, "  Error: %v\n", convErr)
		failed++
	} else {
		fmt.Printf("  Created: %s\n", mdPath)
		converted++
	}
}

func convertPDF(inputFile, outputFile string) error {
	converter := pdf2md.New(
		pdf2md.WithExtractImages(false), // Don't extract images for audit evidence
	)
	return converter.ConvertFileToFile(inputFile, outputFile)
}

func convertDOCX(inputFile, outputFile string) error {
	converter := docx2md.New(
		docx2md.WithPreserveImages(false), // Don't extract images for audit evidence
	)
	return converter.ConvertFileToFile(inputFile, outputFile)
}

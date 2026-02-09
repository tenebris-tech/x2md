// Package imageutil provides utilities for image extraction and writing
package imageutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tenebris-tech/x2md/pdf2md/models"
)

// ImageWriter handles writing extracted images to disk
type ImageWriter struct {
	OutputDir   string // Directory to write images to
	RelativeDir string // Relative path for markdown links
	counter     int    // Counter for generating unique filenames
}

// NewImageWriter creates a new ImageWriter for the given markdown output path.
// For output.md, it creates and uses output_images/ directory.
func NewImageWriter(mdOutputPath string) (*ImageWriter, error) {
	// Generate directory name from markdown output path
	baseName := strings.TrimSuffix(filepath.Base(mdOutputPath), filepath.Ext(mdOutputPath))
	dir := filepath.Dir(mdOutputPath)
	imageDir := filepath.Join(dir, baseName+"_images")

	// Create the directory
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create image directory %s: %w", imageDir, err)
	}

	return &ImageWriter{
		OutputDir:   imageDir,
		RelativeDir: baseName + "_images",
		counter:     0,
	}, nil
}

// WriteImage writes an image to disk and returns the relative path for markdown.
// If the image already has an OutputPath set, it uses that; otherwise generates one.
func (w *ImageWriter) WriteImage(img *models.ImageItem) (string, error) {
	if img == nil || len(img.Data) == 0 {
		return "", fmt.Errorf("invalid image: nil or empty data")
	}

	// Generate filename if not already set
	filename := img.OutputPath
	if filename == "" {
		filename = w.GenerateFilename(img.Format)
	} else {
		// Extract just the filename from OutputPath if it includes directory
		filename = filepath.Base(img.OutputPath)
	}
	// Always update OutputPath to include the relative directory
	img.OutputPath = filepath.Join(w.RelativeDir, filename)

	// Write file
	fullPath := filepath.Join(w.OutputDir, filename)
	if err := os.WriteFile(fullPath, img.Data, 0644); err != nil {
		return "", fmt.Errorf("failed to write image %s: %w", fullPath, err)
	}

	return img.OutputPath, nil
}

// GenerateFilename creates a unique filename like "image_001.png"
func (w *ImageWriter) GenerateFilename(format string) string {
	w.counter++
	ext := formatToExtension(format)
	return fmt.Sprintf("image_%03d%s", w.counter, ext)
}

// formatToExtension converts format name to file extension
func formatToExtension(format string) string {
	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		return ".jpg"
	case "png":
		return ".png"
	case "gif":
		return ".gif"
	case "bmp":
		return ".bmp"
	case "tiff", "tif":
		return ".tiff"
	case "webp":
		return ".webp"
	case "jp2", "jpeg2000":
		return ".jp2"
	case "emf":
		return ".emf"
	case "wmf":
		return ".wmf"
	default:
		return ".bin"
	}
}


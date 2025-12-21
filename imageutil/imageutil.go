// Package imageutil provides utilities for image extraction and writing
package imageutil

import (
	"bytes"
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

// DetectFormat detects image format from magic bytes
func DetectFormat(data []byte) string {
	if len(data) < 8 {
		return "bin"
	}

	// JPEG: FF D8 FF
	if bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}) {
		return "jpeg"
	}

	// PNG: 89 50 4E 47 0D 0A 1A 0A
	if bytes.HasPrefix(data, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}) {
		return "png"
	}

	// GIF: 47 49 46 38
	if bytes.HasPrefix(data, []byte{0x47, 0x49, 0x46, 0x38}) {
		return "gif"
	}

	// BMP: 42 4D
	if bytes.HasPrefix(data, []byte{0x42, 0x4D}) {
		return "bmp"
	}

	// TIFF: 49 49 2A 00 (little-endian) or 4D 4D 00 2A (big-endian)
	if bytes.HasPrefix(data, []byte{0x49, 0x49, 0x2A, 0x00}) ||
		bytes.HasPrefix(data, []byte{0x4D, 0x4D, 0x00, 0x2A}) {
		return "tiff"
	}

	// WebP: 52 49 46 46 ... 57 45 42 50
	if len(data) >= 12 && bytes.HasPrefix(data, []byte{0x52, 0x49, 0x46, 0x46}) &&
		bytes.Equal(data[8:12], []byte{0x57, 0x45, 0x42, 0x50}) {
		return "webp"
	}

	// JPEG 2000: 00 00 00 0C 6A 50 20 20
	if bytes.HasPrefix(data, []byte{0x00, 0x00, 0x00, 0x0C, 0x6A, 0x50, 0x20, 0x20}) {
		return "jp2"
	}

	// EMF: 01 00 00 00
	if bytes.HasPrefix(data, []byte{0x01, 0x00, 0x00, 0x00}) && len(data) > 40 {
		// Check for EMF signature at offset 40
		if len(data) > 44 && bytes.Equal(data[40:44], []byte{0x20, 0x45, 0x4D, 0x46}) {
			return "emf"
		}
	}

	// WMF: D7 CD C6 9A (placeable) or 01 00 09 00 (standard)
	if bytes.HasPrefix(data, []byte{0xD7, 0xCD, 0xC6, 0x9A}) ||
		bytes.HasPrefix(data, []byte{0x01, 0x00, 0x09, 0x00}) {
		return "wmf"
	}

	return "bin"
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

// FormatMarkdownImage generates a markdown image reference
func FormatMarkdownImage(altText, path string) string {
	if altText == "" {
		altText = "image"
	}
	return fmt.Sprintf("![%s](%s)", altText, path)
}

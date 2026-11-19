/******************************************************************************
 * Copyright (c) 2026 Tenebris Technologies Inc.                              *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package imageutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tenebris-tech/x2md/pdf2md/models"
)

func TestNewImageWriter_CreatesDirectoryNextToOutput(t *testing.T) {
	dir := t.TempDir()
	w, err := NewImageWriter(filepath.Join(dir, "report.md"))
	if err != nil {
		t.Fatalf("NewImageWriter: %v", err)
	}
	if w.RelativeDir != "report_images" {
		t.Errorf("RelativeDir = %q, want report_images", w.RelativeDir)
	}
	if info, err := os.Stat(filepath.Join(dir, "report_images")); err != nil || !info.IsDir() {
		t.Errorf("image directory was not created: %v", err)
	}
}

func TestWriteImage(t *testing.T) {
	dir := t.TempDir()
	w, err := NewImageWriter(filepath.Join(dir, "report.md"))
	if err != nil {
		t.Fatalf("NewImageWriter: %v", err)
	}

	tests := []struct {
		name string
		img  *models.ImageItem
		want string
	}{
		{"generated name", &models.ImageItem{Format: "png", Data: []byte("a")}, filepath.Join("report_images", "image_001.png")},
		{"counter advances", &models.ImageItem{Format: "jpeg", Data: []byte("b")}, filepath.Join("report_images", "image_002.jpg")},
		{"preset name keeps only its base", &models.ImageItem{OutputPath: "some/dir/page_003.png", Data: []byte("c")}, filepath.Join("report_images", "page_003.png")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := w.WriteImage(tt.img)
			if err != nil {
				t.Fatalf("WriteImage: %v", err)
			}
			if got != tt.want || tt.img.OutputPath != tt.want {
				t.Errorf("path = %q (OutputPath %q), want %q", got, tt.img.OutputPath, tt.want)
			}
			data, err := os.ReadFile(filepath.Join(dir, got))
			if err != nil || string(data) != string(tt.img.Data) {
				t.Errorf("file content = %q, %v; want %q", data, err, tt.img.Data)
			}
		})
	}
}

func TestWriteImage_RejectsMissingData(t *testing.T) {
	w, err := NewImageWriter(filepath.Join(t.TempDir(), "report.md"))
	if err != nil {
		t.Fatalf("NewImageWriter: %v", err)
	}
	for name, img := range map[string]*models.ImageItem{"nil image": nil, "empty data": {Format: "png"}} {
		if _, err := w.WriteImage(img); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestFormatToExtension(t *testing.T) {
	tests := map[string]string{
		"jpeg": ".jpg", "JPG": ".jpg", "png": ".png", "gif": ".gif", "bmp": ".bmp",
		"tif": ".tiff", "webp": ".webp", "jpeg2000": ".jp2",
	}
	for format, want := range tests {
		if got := formatToExtension(format); got != want {
			t.Errorf("formatToExtension(%q) = %q, want %q", format, got, want)
		}
	}
}

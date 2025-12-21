package pdf2md

import (
	"testing"

	"github.com/tenebris-tech/x2md/pdf2md/pdf"
)

func TestScanModeEnabledByDefault(t *testing.T) {
	opts := DefaultOptions()
	if !opts.ScanMode {
		t.Error("ScanMode should be enabled by default")
	}
}

func TestWithScanModeDisables(t *testing.T) {
	converter := New(WithScanMode(false))
	if converter.options.ScanMode {
		t.Error("WithScanMode(false) should disable scan mode")
	}
}

func TestWithScanModeEnables(t *testing.T) {
	converter := New(WithScanMode(true))
	if !converter.options.ScanMode {
		t.Error("WithScanMode(true) should enable scan mode")
	}
}

func TestIsScannedPage_NoImages(t *testing.T) {
	converter := New()
	// No images = not a scan
	result := converter.isScannedPage(nil, nil, 612, 792)
	if result {
		t.Error("Page with no images should not be detected as scanned")
	}
}

func TestIsScannedPage_TooMuchText(t *testing.T) {
	converter := New()
	// Create text items with more than 100 characters
	textItems := []pdf.TextItem{
		{Text: "This is a line of text that contains more than one hundred characters when combined with other lines on the page."},
	}
	images := []*pdf.ImageData{
		{Width: 2550, Height: 3300, Format: "jpeg"},
	}
	result := converter.isScannedPage(textItems, images, 612, 792)
	if result {
		t.Error("Page with >100 chars of text should not be detected as scanned")
	}
}

func TestIsScannedPage_LittleTextLargeImage(t *testing.T) {
	converter := New()
	// Less than 100 characters
	textItems := []pdf.TextItem{
		{Text: "Short text"},
	}
	// Large image (>50% of page dimensions)
	images := []*pdf.ImageData{
		{Width: 2550, Height: 3300, Format: "jpeg"}, // Much larger than 612*0.5=306
	}
	result := converter.isScannedPage(textItems, images, 612, 792)
	if !result {
		t.Error("Page with little text and large image should be detected as scanned")
	}
}

func TestIsScannedPage_LittleTextSmallImage(t *testing.T) {
	converter := New()
	// Less than 100 characters
	textItems := []pdf.TextItem{
		{Text: "Short"},
	}
	// Small image (less than 50% of page and less than 500x500)
	images := []*pdf.ImageData{
		{Width: 100, Height: 100, Format: "jpeg"},
	}
	result := converter.isScannedPage(textItems, images, 612, 792)
	if result {
		t.Error("Page with little text but small image should not be detected as scanned")
	}
}

func TestIsScannedPage_NoTextMediumImage(t *testing.T) {
	converter := New()
	// No text at all
	var textItems []pdf.TextItem
	// Image >= 500x500
	images := []*pdf.ImageData{
		{Width: 500, Height: 500, Format: "png"},
	}
	result := converter.isScannedPage(textItems, images, 612, 792)
	if !result {
		t.Error("Page with no text and 500x500 image should be detected as scanned")
	}
}

func TestFindLargestImage_Empty(t *testing.T) {
	converter := New()
	result := converter.findLargestImage(nil)
	if result != nil {
		t.Error("findLargestImage with nil should return nil")
	}

	result = converter.findLargestImage([]*pdf.ImageData{})
	if result != nil {
		t.Error("findLargestImage with empty slice should return nil")
	}
}

func TestFindLargestImage_Single(t *testing.T) {
	converter := New()
	img := &pdf.ImageData{Width: 100, Height: 100}
	result := converter.findLargestImage([]*pdf.ImageData{img})
	if result != img {
		t.Error("findLargestImage with single image should return that image")
	}
}

func TestFindLargestImage_Multiple(t *testing.T) {
	converter := New()
	small := &pdf.ImageData{Width: 100, Height: 100}   // 10,000 pixels
	medium := &pdf.ImageData{Width: 200, Height: 200}  // 40,000 pixels
	large := &pdf.ImageData{Width: 1000, Height: 1000} // 1,000,000 pixels

	// Test with different orderings
	result := converter.findLargestImage([]*pdf.ImageData{small, large, medium})
	if result != large {
		t.Errorf("findLargestImage should return largest image, got %dx%d", result.Width, result.Height)
	}

	result = converter.findLargestImage([]*pdf.ImageData{large, small, medium})
	if result != large {
		t.Errorf("findLargestImage should return largest image regardless of order, got %dx%d", result.Width, result.Height)
	}
}

func TestFindLargestImage_SameSize(t *testing.T) {
	converter := New()
	img1 := &pdf.ImageData{Width: 100, Height: 100}
	img2 := &pdf.ImageData{Width: 100, Height: 100}

	// Should return first one when sizes are equal
	result := converter.findLargestImage([]*pdf.ImageData{img1, img2})
	if result != img1 {
		t.Error("findLargestImage should return first image when sizes are equal")
	}
}

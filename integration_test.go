package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tenebris-tech/x2md/docx2md"
	"github.com/tenebris-tech/x2md/pdf2md"
)

// Integration tests for x2md converter
// These tests use files in private/ directory if available

func TestPDFConversion(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		contains []string // Expected substrings in output
		notEmpty bool
	}{
		{
			name:     "basic-text",
			file:     "private/basic-text.pdf",
			contains: []string{"Sample Document", "Introduction", "Text Formatting"},
			notEmpty: true,
		},
		{
			name:     "CPP_ND technical document",
			file:     "private/CPP_ND_V3.0E.pdf",
			contains: []string{"Protection Profile", "Network Devices", "Version"},
			notEmpty: true,
		},
		{
			name:     "footnotes",
			file:     "private/footnotes.pdf",
			contains: []string{"Footnotes", "footnote"},
			notEmpty: true,
		},
	}

	converter := pdf2md.New()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := os.Stat(tt.file); os.IsNotExist(err) {
				t.Skipf("Test file not available: %s", tt.file)
			}

			data, err := os.ReadFile(tt.file)
			if err != nil {
				t.Fatalf("Failed to read file: %v", err)
			}

			result, _, err := converter.ConvertWithImages(data)
			if err != nil {
				t.Fatalf("Conversion failed: %v", err)
			}

			if tt.notEmpty && len(strings.TrimSpace(result)) == 0 {
				t.Error("Expected non-empty output")
			}

			for _, expected := range tt.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("Output missing expected content: %q", expected)
				}
			}
		})
	}
}

func TestDOCXConversion(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		contains []string
		notEmpty bool
	}{
		{
			name:     "simple letter",
			file:     "private/20080401 Pager Return.docx",
			contains: []string{"Eric Jacksch", "Sincerely"},
			notEmpty: true,
		},
		{
			name:     "letter with list",
			file:     "private/20110912 Manulife Financial re CPAP Claim.docx",
			contains: []string{"Dear Sir or Madam", "1.", "2.", "Sincerely"},
			notEmpty: true,
		},
		{
			name:     "table document",
			file:     "private/20240626 Leasecake DR Exercise.docx",
			contains: []string{"|", "Disaster Recovery"},
			notEmpty: true,
		},
		{
			name:     "formatted letter",
			file:     "private/20240821 SOC Gap Letter.docx",
			contains: []string{"SOC", "Leasecake"},
			notEmpty: true,
		},
		{
			name:     "policy document",
			file:     "private/20250930 Leasecake - Policies and Controls.docx",
			contains: []string{"#", "Policy", "##"},
			notEmpty: true,
		},
		{
			name:     "report with footnotes",
			file:     "private/4_CSSP Project W7714-196635_001_SL_NG9-1-1 PSAP SOC Proof of Concept Report_26Jun2023(Final)-EJ.docx",
			contains: []string{"NG9-1-1", "[^", "Footnotes"},
			notEmpty: true,
		},
		{
			name:     "instructions with links",
			file:     "private/Cylance Install Instructions.docx",
			contains: []string{"Cylance", "STEP"},
			notEmpty: true,
		},
	}

	converter := docx2md.New()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := os.Stat(tt.file); os.IsNotExist(err) {
				t.Skipf("Test file not available: %s", tt.file)
			}

			data, err := os.ReadFile(tt.file)
			if err != nil {
				t.Fatalf("Failed to read file: %v", err)
			}

			result, _, err := converter.ConvertWithImages(data)
			if err != nil {
				t.Fatalf("Conversion failed: %v", err)
			}

			if tt.notEmpty && len(strings.TrimSpace(result)) == 0 {
				t.Error("Expected non-empty output")
			}

			for _, expected := range tt.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("Output missing expected content: %q", expected)
				}
			}
		})
	}
}

func TestDOCXParagraphSeparation(t *testing.T) {
	// Regression test for paragraph separation fix (commit 0b7f0c8)
	file := "private/20240821 SOC Gap Letter.docx"
	if _, err := os.Stat(file); os.IsNotExist(err) {
		t.Skipf("Test file not available: %s", file)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	converter := docx2md.New()
	result, _, err := converter.ConvertWithImages(data)
	if err != nil {
		t.Fatalf("Conversion failed: %v", err)
	}

	// Check for blank lines between paragraphs (double newlines)
	if !strings.Contains(result, "\n\n") {
		t.Error("Expected blank lines between paragraphs (double newlines)")
	}

	// Count paragraphs - should have multiple separated blocks
	blocks := strings.Split(result, "\n\n")
	if len(blocks) < 5 {
		t.Errorf("Expected at least 5 paragraph blocks, got %d", len(blocks))
	}
}

func TestDOCXFootnoteContent(t *testing.T) {
	// Regression test for footnote content corruption fix (commit e469989)
	file := "private/4_CSSP Project W7714-196635_001_SL_NG9-1-1 PSAP SOC Proof of Concept Report_26Jun2023(Final)-EJ.docx"
	if _, err := os.Stat(file); os.IsNotExist(err) {
		t.Skipf("Test file not available: %s", file)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	converter := docx2md.New()
	result, _, err := converter.ConvertWithImages(data)
	if err != nil {
		t.Fatalf("Conversion failed: %v", err)
	}

	// Check that footnotes contain valid URLs (not corrupted text)
	if strings.Contains(result, "footnotesbalwa") {
		t.Error("Footnote content is corrupted (contains 'footnotesbalwa')")
	}

	// Check for expected footnote content
	expectedURLs := []string{
		"opnsense.org",
		"aws.amazon.com",
		"github.com",
	}

	for _, url := range expectedURLs {
		if !strings.Contains(result, url) {
			t.Errorf("Expected footnote URL not found: %s", url)
		}
	}
}

func TestEncryptedPDFHandling(t *testing.T) {
	// This test verifies that encrypted PDFs are handled correctly.
	// With AES decryption support, permission-restricted PDFs (no user password)
	// can now be decrypted. Only PDFs with unknown passwords should fail.
	file := "private/itsg33-ann3a-eng.pdf"
	if _, err := os.Stat(file); os.IsNotExist(err) {
		t.Skipf("Test file not available: %s", file)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	converter := pdf2md.New()
	result, _, err := converter.ConvertWithImages(data)

	// If error occurs, it should mention encryption
	if err != nil {
		if !strings.Contains(err.Error(), "encrypt") {
			t.Errorf("Expected encryption-related error, got: %v", err)
		}
		return
	}

	// If no error, we successfully decrypted - verify we got content
	if len(result) == 0 {
		t.Error("Expected non-empty result after decryption")
	}
}

func TestPDFTableDetection(t *testing.T) {
	file := "private/CPP_ND_V3.0E.pdf"
	if _, err := os.Stat(file); os.IsNotExist(err) {
		t.Skipf("Test file not available: %s", file)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	converter := pdf2md.New()
	result, _, err := converter.ConvertWithImages(data)
	if err != nil {
		t.Fatalf("Conversion failed: %v", err)
	}

	// Check for markdown table syntax
	if !strings.Contains(result, "| ") {
		t.Error("Expected markdown table syntax (| ) in output")
	}

	// Check for table separator
	if !strings.Contains(result, "| ---") {
		t.Error("Expected table separator (| ---) in output")
	}
}

func TestPDFHeaderDetection(t *testing.T) {
	file := "private/basic-text.pdf"
	if _, err := os.Stat(file); os.IsNotExist(err) {
		t.Skipf("Test file not available: %s", file)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	converter := pdf2md.New()
	result, _, err := converter.ConvertWithImages(data)
	if err != nil {
		t.Fatalf("Conversion failed: %v", err)
	}

	// Check for H1 and H2 headers
	if !strings.Contains(result, "# ") {
		t.Error("Expected H1 header (# ) in output")
	}

	if !strings.Contains(result, "## ") {
		t.Error("Expected H2 header (## ) in output")
	}
}

func TestImageExtraction(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{"PDF with images", "private/CPP_ND_V3.0E.pdf"},
		{"DOCX with images", "private/20240821 SOC Gap Letter.docx"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := os.Stat(tt.file); os.IsNotExist(err) {
				t.Skipf("Test file not available: %s", tt.file)
			}

			data, err := os.ReadFile(tt.file)
			if err != nil {
				t.Fatalf("Failed to read file: %v", err)
			}

			var result string
			var images interface{}

			ext := strings.ToLower(filepath.Ext(tt.file))
			if ext == ".pdf" {
				converter := pdf2md.New()
				result, images, err = converter.ConvertWithImages(data)
			} else {
				converter := docx2md.New()
				result, images, err = converter.ConvertWithImages(data)
			}

			if err != nil {
				t.Fatalf("Conversion failed: %v", err)
			}

			// Check for image references in output
			if strings.Contains(result, "![") {
				t.Logf("Found image references in output")
			}

			// Images slice should not be nil (even if empty)
			if images == nil {
				t.Error("Images result should not be nil")
			}
		})
	}
}

func TestLargePDFPerformance(t *testing.T) {
	file := "private/AFCEA IT Security Course - Security Assurance - Jacksch.pdf"
	if _, err := os.Stat(file); os.IsNotExist(err) {
		t.Skipf("Test file not available: %s", file)
	}

	// Skip in short mode
	if testing.Short() {
		t.Skip("Skipping large file test in short mode")
	}

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	t.Logf("Testing large PDF: %d bytes", len(data))

	converter := pdf2md.New()
	result, _, err := converter.ConvertWithImages(data)
	if err != nil {
		t.Fatalf("Conversion failed: %v", err)
	}

	if len(result) == 0 {
		t.Error("Expected non-empty output from large PDF")
	}

	t.Logf("Output size: %d bytes", len(result))
}

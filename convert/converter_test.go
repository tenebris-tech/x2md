package convert

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/tenebris-tech/x2md/docx2md"
	"github.com/tenebris-tech/x2md/pdf2md"
)

// createTestDocx creates a minimal valid DOCX for testing
func createTestDocx(content string) []byte {
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	contentTypes := `<?xml version="1.0" encoding="UTF-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`
	f, _ := w.Create("[Content_Types].xml")
	f.Write([]byte(contentTypes))

	rels := `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`
	f, _ = w.Create("_rels/.rels")
	f.Write([]byte(rels))

	document := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>` + content + `</w:body>
</w:document>`
	f, _ = w.Create("word/document.xml")
	f.Write([]byte(document))

	w.Close()
	return buf.Bytes()
}

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()

	if opts.Recursion {
		t.Error("Expected Recursion to be false by default")
	}
	if !opts.SkipExisting {
		t.Error("Expected SkipExisting to be true by default")
	}
	if len(opts.Extensions) != 2 {
		t.Errorf("Expected 2 default extensions, got %d", len(opts.Extensions))
	}
	if opts.OutputDirectory != "" {
		t.Error("Expected OutputDirectory to be empty by default")
	}
}

func TestWithRecursion(t *testing.T) {
	c := New(WithRecursion(true))
	if !c.options.Recursion {
		t.Error("Expected Recursion to be true")
	}
}

func TestWithExtensions(t *testing.T) {
	// Test normalization
	c := New(WithExtensions([]string{"pdf", ".DOCX", "PDF"}))

	if len(c.options.Extensions) != 3 {
		t.Errorf("Expected 3 extensions, got %d", len(c.options.Extensions))
	}

	// All should be lowercase with leading dot
	for _, ext := range c.options.Extensions {
		if ext[0] != '.' {
			t.Errorf("Expected extension to start with dot: %s", ext)
		}
		if ext != ".pdf" && ext != ".docx" {
			t.Errorf("Unexpected extension: %s", ext)
		}
	}
}

func TestWithSkipExisting(t *testing.T) {
	c := New(WithSkipExisting(false))
	if c.options.SkipExisting {
		t.Error("Expected SkipExisting to be false")
	}
}

func TestWithOutputDirectory(t *testing.T) {
	c := New(WithOutputDirectory("/tmp/output"))
	if c.options.OutputDirectory != "/tmp/output" {
		t.Errorf("Expected /tmp/output, got %s", c.options.OutputDirectory)
	}
}

func TestWithPDFOptions(t *testing.T) {
	c := New(WithPDFOptions(pdf2md.WithExtractImages(false)))
	if len(c.options.PDFOptions) != 1 {
		t.Errorf("Expected 1 PDF option, got %d", len(c.options.PDFOptions))
	}
}

func TestWithDOCXOptions(t *testing.T) {
	c := New(WithDOCXOptions(docx2md.WithPreserveImages(false)))
	if len(c.options.DOCXOptions) != 1 {
		t.Errorf("Expected 1 DOCX option, got %d", len(c.options.DOCXOptions))
	}
}

func TestConvertSingleDocxFile(t *testing.T) {
	// Create a temp directory
	tmpDir, err := os.MkdirTemp("", "convert_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Write test DOCX
	docxPath := filepath.Join(tmpDir, "test.docx")
	docxData := createTestDocx(`<w:p><w:r><w:t>Hello World</w:t></w:r></w:p>`)
	if err := os.WriteFile(docxPath, docxData, 0644); err != nil {
		t.Fatal(err)
	}

	// Convert
	c := New()
	result, err := c.Convert(docxPath)
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	if result.Converted != 1 {
		t.Errorf("Expected 1 converted, got %d", result.Converted)
	}

	// Check output file exists
	mdPath := filepath.Join(tmpDir, "test.md")
	if _, err := os.Stat(mdPath); os.IsNotExist(err) {
		t.Error("Expected output file to exist")
	}

	// Check content
	content, _ := os.ReadFile(mdPath)
	if !bytes.Contains(content, []byte("Hello World")) {
		t.Errorf("Expected output to contain 'Hello World', got: %s", content)
	}
}

func TestConvertDirectoryWithoutRecursion(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "convert_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	c := New() // Recursion is false by default
	_, err = c.Convert(tmpDir)
	if err == nil {
		t.Error("Expected error when converting directory without recursion")
	}
}

func TestConvertDirectoryWithRecursion(t *testing.T) {
	// Create temp directory structure
	tmpDir, err := os.MkdirTemp("", "convert_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create subdirectory
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write test DOCX files
	docxData := createTestDocx(`<w:p><w:r><w:t>Test</w:t></w:r></w:p>`)
	if err := os.WriteFile(filepath.Join(tmpDir, "file1.docx"), docxData, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "file2.docx"), docxData, 0644); err != nil {
		t.Fatal(err)
	}

	// Convert with recursion
	c := New(WithRecursion(true))
	result, err := c.Convert(tmpDir)
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	if result.Converted != 2 {
		t.Errorf("Expected 2 converted, got %d", result.Converted)
	}

	// Check output files exist
	if _, err := os.Stat(filepath.Join(tmpDir, "file1.md")); os.IsNotExist(err) {
		t.Error("Expected file1.md to exist")
	}
	if _, err := os.Stat(filepath.Join(subDir, "file2.md")); os.IsNotExist(err) {
		t.Error("Expected file2.md to exist")
	}
}

func TestSkipExisting(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "convert_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Write test DOCX
	docxPath := filepath.Join(tmpDir, "test.docx")
	docxData := createTestDocx(`<w:p><w:r><w:t>Hello</w:t></w:r></w:p>`)
	if err := os.WriteFile(docxPath, docxData, 0644); err != nil {
		t.Fatal(err)
	}

	// Create existing .md file
	mdPath := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(mdPath, []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}

	// Convert with SkipExisting=true (default)
	c := New()
	result, err := c.Convert(docxPath)
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	if result.Skipped != 1 {
		t.Errorf("Expected 1 skipped, got %d", result.Skipped)
	}

	// Verify original content unchanged
	content, _ := os.ReadFile(mdPath)
	if string(content) != "existing" {
		t.Error("Expected existing file to be unchanged")
	}
}

func TestNoSkipExistingCreatesNumberedFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "convert_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Write test DOCX
	docxPath := filepath.Join(tmpDir, "test.docx")
	docxData := createTestDocx(`<w:p><w:r><w:t>New Content</w:t></w:r></w:p>`)
	if err := os.WriteFile(docxPath, docxData, 0644); err != nil {
		t.Fatal(err)
	}

	// Create existing .md file
	mdPath := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(mdPath, []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}

	// Convert with SkipExisting=false
	c := New(WithSkipExisting(false))
	result, err := c.Convert(docxPath)
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	if result.Converted != 1 {
		t.Errorf("Expected 1 converted, got %d", result.Converted)
	}

	// Check numbered file was created
	numberedPath := filepath.Join(tmpDir, "test-1.md")
	if _, err := os.Stat(numberedPath); os.IsNotExist(err) {
		t.Error("Expected test-1.md to exist")
	}

	// Verify original still exists
	if _, err := os.Stat(mdPath); os.IsNotExist(err) {
		t.Error("Expected test.md to still exist")
	}
}

func TestOutputDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "convert_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	srcDir := filepath.Join(tmpDir, "src")
	outDir := filepath.Join(tmpDir, "out")
	if err := os.Mkdir(srcDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write test DOCX
	docxPath := filepath.Join(srcDir, "test.docx")
	docxData := createTestDocx(`<w:p><w:r><w:t>Hello</w:t></w:r></w:p>`)
	if err := os.WriteFile(docxPath, docxData, 0644); err != nil {
		t.Fatal(err)
	}

	// Convert with output directory
	c := New(WithOutputDirectory(outDir))
	result, err := c.Convert(docxPath)
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	if result.Converted != 1 {
		t.Errorf("Expected 1 converted, got %d", result.Converted)
	}

	// Check output file in output directory
	mdPath := filepath.Join(outDir, "test.md")
	if _, err := os.Stat(mdPath); os.IsNotExist(err) {
		t.Error("Expected test.md in output directory")
	}

	// Verify no file created next to source
	srcMdPath := filepath.Join(srcDir, "test.md")
	if _, err := os.Stat(srcMdPath); !os.IsNotExist(err) {
		t.Error("Expected no test.md next to source file")
	}
}

func TestOutputDirectoryFlatStructure(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "convert_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	srcDir := filepath.Join(tmpDir, "src")
	subDir := filepath.Join(srcDir, "sub")
	outDir := filepath.Join(tmpDir, "out")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write test DOCX files in different directories
	docxData := createTestDocx(`<w:p><w:r><w:t>Hello</w:t></w:r></w:p>`)
	if err := os.WriteFile(filepath.Join(srcDir, "file1.docx"), docxData, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "file2.docx"), docxData, 0644); err != nil {
		t.Fatal(err)
	}

	// Convert with recursion and output directory
	c := New(WithRecursion(true), WithOutputDirectory(outDir))
	result, err := c.Convert(srcDir)
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	if result.Converted != 2 {
		t.Errorf("Expected 2 converted, got %d", result.Converted)
	}

	// Check both files in output directory (flat)
	if _, err := os.Stat(filepath.Join(outDir, "file1.md")); os.IsNotExist(err) {
		t.Error("Expected file1.md in output directory")
	}
	if _, err := os.Stat(filepath.Join(outDir, "file2.md")); os.IsNotExist(err) {
		t.Error("Expected file2.md in output directory")
	}
}

func TestOutputDirectoryConflictingNames(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "convert_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	srcDir := filepath.Join(tmpDir, "src")
	subDir := filepath.Join(srcDir, "sub")
	outDir := filepath.Join(tmpDir, "out")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write test DOCX files with same name in different directories
	docxData := createTestDocx(`<w:p><w:r><w:t>Hello</w:t></w:r></w:p>`)
	if err := os.WriteFile(filepath.Join(srcDir, "test.docx"), docxData, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "test.docx"), docxData, 0644); err != nil {
		t.Fatal(err)
	}

	// Convert with recursion and output directory, SkipExisting=false
	c := New(WithRecursion(true), WithOutputDirectory(outDir), WithSkipExisting(false))
	result, err := c.Convert(srcDir)
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	if result.Converted != 2 {
		t.Errorf("Expected 2 converted, got %d", result.Converted)
	}

	// Check both files exist (one numbered)
	if _, err := os.Stat(filepath.Join(outDir, "test.md")); os.IsNotExist(err) {
		t.Error("Expected test.md in output directory")
	}
	if _, err := os.Stat(filepath.Join(outDir, "test-1.md")); os.IsNotExist(err) {
		t.Error("Expected test-1.md in output directory")
	}
}

func TestExtensionFilter(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "convert_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Write test files
	docxData := createTestDocx(`<w:p><w:r><w:t>Hello</w:t></w:r></w:p>`)
	if err := os.WriteFile(filepath.Join(tmpDir, "test.docx"), docxData, 0644); err != nil {
		t.Fatal(err)
	}
	// Also create a .txt file (should be ignored)
	if err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("text"), 0644); err != nil {
		t.Fatal(err)
	}

	// Convert only .pdf files (none exist)
	c := New(WithRecursion(true), WithExtensions([]string{".pdf"}))
	result, err := c.Convert(tmpDir)
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	if result.Converted != 0 {
		t.Errorf("Expected 0 converted (only .pdf filtered), got %d", result.Converted)
	}
}

func TestCallbacks(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "convert_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Write test DOCX
	docxPath := filepath.Join(tmpDir, "test.docx")
	docxData := createTestDocx(`<w:p><w:r><w:t>Hello</w:t></w:r></w:p>`)
	if err := os.WriteFile(docxPath, docxData, 0644); err != nil {
		t.Fatal(err)
	}

	var startCalled, completeCalled bool
	var completePath, completeOutput string
	var completeErr error

	c := New(
		WithOnFileStart(func(path string) {
			startCalled = true
		}),
		WithOnFileComplete(func(path, outputPath string, err error) {
			completeCalled = true
			completePath = path
			completeOutput = outputPath
			completeErr = err
		}),
	)

	_, err = c.Convert(docxPath)
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	if !startCalled {
		t.Error("Expected OnFileStart to be called")
	}
	if !completeCalled {
		t.Error("Expected OnFileComplete to be called")
	}
	if completePath == "" {
		t.Error("Expected completePath to be set")
	}
	if completeOutput == "" {
		t.Error("Expected completeOutput to be set")
	}
	if completeErr != nil {
		t.Errorf("Expected no error, got: %v", completeErr)
	}
}

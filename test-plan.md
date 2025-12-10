# Image Extraction Test Plan

Manual test plan for verifying image extraction functionality in x2md.

---

## Prerequisites

- Built CLI: `go build -o x2md .`
- Test files available in `private/` directory
- PDF with images (e.g., `CPP_ND_V3.0E.pdf`)
- DOCX with images

---

## Test Cases

### 1. PDF Image Extraction

#### 1.1 Basic PDF Image Extraction
```bash
rm -rf private/CPP_ND_V3.0E_images private/CPP_ND_V3.0E.md
./x2md private/CPP_ND_V3.0E.pdf
```

**Expected:**
- `private/CPP_ND_V3.0E.md` created
- `private/CPP_ND_V3.0E_images/` directory created
- Multiple image files (PNG/JPG) in images directory

**Verify:**
```bash
ls -la private/CPP_ND_V3.0E_images/
grep '!\[' private/CPP_ND_V3.0E.md | head -5
```

#### 1.2 PDF Image Format Validity
```bash
file private/CPP_ND_V3.0E_images/*.jpg
file private/CPP_ND_V3.0E_images/*.png
```

**Expected:**
- JPG files: "JPEG image data, JFIF standard..."
- PNG files: "PNG image data, NxN, 8-bit/color RGB, non-interlaced"

#### 1.3 PDF --no-images Flag
```bash
rm -rf private/CPP_ND_V3.0E_images private/CPP_ND_V3.0E.md
./x2md --no-images private/CPP_ND_V3.0E.pdf
```

**Expected:**
- `private/CPP_ND_V3.0E.md` created
- NO `private/CPP_ND_V3.0E_images/` directory
- No image references in markdown

**Verify:**
```bash
ls private/CPP_ND_V3.0E_images 2>&1  # Should fail
grep '!\[' private/CPP_ND_V3.0E.md   # Should return nothing
```

#### 1.4 PDF Custom Output Path
```bash
rm -rf /tmp/test-output_images /tmp/test-output.md
./x2md -output /tmp/test-output.md private/CPP_ND_V3.0E.pdf
```

**Expected:**
- `/tmp/test-output.md` created
- `/tmp/test-output_images/` directory created with images

---

### 2. DOCX Image Extraction

#### 2.1 Basic DOCX Image Extraction
```bash
rm -rf private/test_images private/test.md
./x2md private/test.docx  # Replace with actual test file
```

**Expected:**
- Markdown file created
- `{name}_images/` directory created if DOCX contains images
- Image files extracted

**Verify:**
```bash
ls -la private/*_images/
grep '!\[' private/*.md
```

#### 2.2 DOCX --no-images Flag
```bash
./x2md --no-images private/test.docx
```

**Expected:**
- Markdown file created
- NO images directory created

#### 2.3 DOCX Without Images
```bash
./x2md private/text-only.docx  # DOCX with no images
```

**Expected:**
- Markdown file created
- NO images directory created (nothing to extract)

---

### 3. Image Link Format

#### 3.1 Markdown Link Syntax
```bash
grep '!\[' private/CPP_ND_V3.0E.md | head -3
```

**Expected format:**
```
![AltText](CPP_ND_V3.0E_images/image_001.png)
![AltText](CPP_ND_V3.0E_images/image_002.jpg)
```

#### 3.2 Relative Paths
**Expected:**
- Image paths are relative to markdown file location
- No absolute paths in markdown output

---

### 4. Edge Cases

#### 4.1 PDF with No Images
```bash
./x2md private/text-only.pdf
```

**Expected:**
- Markdown file created
- NO images directory created

#### 4.2 Existing Images Directory
```bash
mkdir -p private/CPP_ND_V3.0E_images
echo "existing" > private/CPP_ND_V3.0E_images/old-file.txt
./x2md private/CPP_ND_V3.0E.pdf
```

**Expected:**
- Old files may be overwritten or coexist (document actual behavior)
- New images extracted successfully

#### 4.3 Output to Different Directory
```bash
./x2md -output /tmp/subdir/output.md private/CPP_ND_V3.0E.pdf
```

**Expected:**
- Creates `/tmp/subdir/` if needed
- Creates `/tmp/subdir/output_images/`
- Images extracted correctly

---

### 5. Image Types

#### 5.1 JPEG Images (DCTDecode)
- Source: PDF with JPEG-compressed images
- **Expected:** `.jpg` files, valid JFIF format

#### 5.2 Raw/FlateDecode Images
- Source: PDF with raw pixel data
- **Expected:** `.png` files, valid PNG format with correct dimensions

#### 5.3 Common DOCX Formats
- PNG, JPEG, GIF, BMP, TIFF, EMF, WMF
- **Expected:** Format detected from magic bytes, correct extension

---

### 6. Build Verification

```bash
go build ./...
go test ./...
go vet ./...
```

**Expected:** All pass with no errors

---

## Test Results

| Test | Pass/Fail | Notes |
|------|-----------|-------|
| 1.1 Basic PDF extraction | | |
| 1.2 PDF image validity | | |
| 1.3 PDF --no-images | | |
| 1.4 PDF custom output | | |
| 2.1 Basic DOCX extraction | | |
| 2.2 DOCX --no-images | | |
| 2.3 DOCX without images | | |
| 3.1 Markdown link syntax | | |
| 3.2 Relative paths | | |
| 4.1 PDF with no images | | |
| 4.2 Existing images dir | | |
| 4.3 Output to different dir | | |
| 5.1 JPEG images | | |
| 5.2 Raw/PNG images | | |
| 5.3 DOCX formats | | |
| 6 Build verification | | |

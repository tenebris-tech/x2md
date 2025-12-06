# x2md Development Notes

## Overview
x2md is a pure Go utility for converting various file formats to Markdown. Currently supports PDF and DOCX formats.

---

## PDF Conversion Architecture

### PDF Parsing Layer (`pdf2md/pdf/`)
- `parser.go` - Low-level PDF parsing (xref, objects, streams)
- `extractor.go` - Text extraction from content streams

### Models (`pdf2md/models/`)
- `TextItem` - Individual text element with position (x, y, width, height, text, font)
- `Page` - Collection of items on a page
- `LineItem` - Multiple text items grouped into a line
- `LineItemBlock` - Multiple lines grouped into a block
- `Word` - Individual word with type and format
- `ParseResult` - Result of parsing with globals and messages

### PDF Transformations Pipeline
1. `CalculateGlobalStats` - Find most used height, font, line distance
2. `CompactLines` - Group text items on same Y into lines
3. `RemoveRepetitiveElements` - Remove headers/footers
4. `DetectTOC` - Find table of contents
5. `DetectHeaders` - Identify headings by height/font
6. `DetectListItems` - Find bullet/numbered lists
7. `GatherBlocks` - Group lines into blocks
8. `RemoveBlankPages` - Filter empty pages
9. `ToTextBlocks` - Convert to text blocks
10. `ToMarkdown` - Final markdown output

### PDF Content Stream Operators
Key operators for text extraction:
- `BT` - Begin text object
- `ET` - End text object
- `Tf` - Set font and size
- `Tm` - Set text matrix (position)
- `Td`, `TD` - Move text position
- `Tj` - Show text
- `TJ` - Show text with positioning
- `'` - Move to next line and show text
- `"` - Set spacing, move to next line, show text

---

## DOCX Conversion Architecture

### DOCX File Structure
DOCX files are ZIP archives (Office Open XML format) containing:
```
document.docx
├── [Content_Types].xml
├── _rels/
│   └── .rels
├── word/
│   ├── document.xml      # Main content
│   ├── styles.xml        # Style definitions
│   ├── numbering.xml     # List numbering
│   ├── _rels/
│   │   └── document.xml.rels  # Relationships
│   └── media/            # Images
└── docProps/
    └── core.xml          # Metadata
```

### DOCX Parsing Layer (`docx2md/docx/`)
- `parser.go` - ZIP extraction, XML parsing
- `extractor.go` - Content extraction to LineItems
- `elements.go` - XML element structures
- `styles.go` - Style and numbering definitions
- `relationships.go` - Hyperlink/image relationships

### Key DOCX XML Elements

#### Paragraphs (`<w:p>`)
```xml
<w:p>
  <w:pPr>
    <w:pStyle w:val="Heading1"/>
    <w:numPr>...</w:numPr>
  </w:pPr>
  <w:r>
    <w:rPr><w:b/></w:rPr>
    <w:t>Bold text</w:t>
  </w:r>
</w:p>
```

#### Text Runs (`<w:r>`)
- `<w:rPr>` - Run properties (bold, italic, etc.)
- `<w:t>` - Text content
- `xml:space="preserve"` - Preserve whitespace

#### Tables (`<w:tbl>`)
```xml
<w:tbl>
  <w:tr>
    <w:tc>
      <w:p>...</w:p>
    </w:tc>
  </w:tr>
</w:tbl>
```

#### Hyperlinks
```xml
<w:hyperlink r:id="rId1">
  <w:r><w:t>Link text</w:t></w:r>
</w:hyperlink>
```
Resolved via `word/_rels/document.xml.rels`

### DOCX Text Run Handling

Per OOXML specification (ECMA-376):
- Text runs concatenate directly without implicit spaces
- `xml:space="preserve"` preserves leading/trailing whitespace
- Word fragments text during editing, creating multiple runs

Example of fragmented text:
```xml
<w:t xml:space="preserve">constrained by time an</w:t>
<w:t xml:space="preserve">d restrictions</w:t>
```
These concatenate to "constrained by time and restrictions"

### DOCX Style Resolution

Styles are defined in `word/styles.xml`:
```xml
<w:style w:type="paragraph" w:styleId="Heading1">
  <w:name w:val="Heading 1"/>
  <w:pPr><w:outlineLvl w:val="0"/></w:pPr>
</w:style>
```

Heading detection:
1. Check style name (e.g., "Heading 1", "Title")
2. Check outline level in paragraph properties
3. Check styleId pattern (e.g., "Heading1")

### DOCX List Numbering

Lists use `word/numbering.xml`:
```xml
<w:abstractNum w:abstractNumId="0">
  <w:lvl w:ilvl="0">
    <w:numFmt w:val="bullet"/>
    <w:lvlText w:val="•"/>
  </w:lvl>
</w:abstractNum>
```

Number formats:
- `bullet` - Bullet point (-)
- `decimal` - 1, 2, 3
- `lowerLetter` - a, b, c
- `upperLetter` - A, B, C
- `lowerRoman` - i, ii, iii
- `upperRoman` - I, II, III

---

## Block Types (Shared)
- H1-H6 (headings)
- PARAGRAPH
- LIST
- CODE
- TOC
- FOOTNOTES

## Word Formats (Shared)
- BOLD (`**text**`)
- OBLIQUE/ITALIC (`_text_`)
- BOLD_OBLIQUE (`**_text_**`)

---

## Key Insights

### PDF
1. Text extraction relies on position-based grouping
2. Line detection groups items by similar Y coordinate
3. Header detection uses height comparison and font differences
4. Table detection uses column alignment patterns

### DOCX
1. Structure is explicit in XML (paragraphs, runs, tables)
2. Formatting is inherited from styles
3. Text runs must be concatenated directly (no implicit spaces)
4. Relationships resolve hyperlinks and images

---

## Acknowledgments

PDF conversion inspired by:
- https://github.com/opendocsg/pdf2md (MIT License)
- https://github.com/mozilla/pdf.js (Apache 2.0 License)

DOCX format reference:
- ECMA-376 Office Open XML specification

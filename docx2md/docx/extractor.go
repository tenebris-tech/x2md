package docx

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/tenebris-tech/x2md/pdf2md/models"
)

// Extractor converts DOCX content to intermediate format
type Extractor struct {
	parser        *Parser
	styles        *Styles
	numbering     *Numbering
	relationships *Relationships
	footnotes     *Footnotes
	endnotes      *Endnotes

	// List tracking
	listCounters map[int]map[int]int // numId -> level -> counter

	// Image tracking
	images       []*models.ImageItem
	imageCounter int

	// Footnote/endnote tracking (IDs in order of appearance)
	footnoteRefs []string
	endnoteRefs  []string
}

// NewExtractor creates a new document extractor
func NewExtractor(parser *Parser) (*Extractor, error) {
	styles, err := parser.GetStyles()
	if err != nil {
		return nil, fmt.Errorf("getting styles: %w", err)
	}

	numbering, err := parser.GetNumbering()
	if err != nil {
		return nil, fmt.Errorf("getting numbering: %w", err)
	}

	rels, err := parser.GetRelationships()
	if err != nil {
		return nil, fmt.Errorf("getting relationships: %w", err)
	}

	footnotes, err := parser.GetFootnotes()
	if err != nil {
		return nil, fmt.Errorf("getting footnotes: %w", err)
	}

	endnotes, err := parser.GetEndnotes()
	if err != nil {
		return nil, fmt.Errorf("getting endnotes: %w", err)
	}

	return &Extractor{
		parser:        parser,
		styles:        styles,
		numbering:     numbering,
		relationships: rels,
		footnotes:     footnotes,
		endnotes:      endnotes,
		listCounters:  make(map[int]map[int]int),
	}, nil
}

// Extract converts the DOCX document to Page format
func (e *Extractor) Extract() (*models.Page, []*models.ImageItem, error) {
	// Read raw document XML for custom parsing
	docData, err := e.parser.ReadFile("word/document.xml")
	if err != nil {
		return nil, nil, err
	}

	// Parse document body content
	items, err := e.parseDocumentBody(docData)
	if err != nil {
		return nil, nil, err
	}

	return &models.Page{
		Index: 0,
		Items: items,
	}, e.images, nil
}

// parseDocumentBody parses the document body into LineItems
func (e *Extractor) parseDocumentBody(data []byte) ([]interface{}, error) {
	var items []interface{}

	// Use streaming parser to handle mixed content
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = false

	var inBody bool
	var depth int

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			localName := stripNamespacePrefix(t.Name.Local)

			if localName == "body" {
				inBody = true
				depth = 0
				continue
			}

			if !inBody {
				continue
			}

			if depth == 0 {
				switch localName {
				case "p":
					// Parse paragraph
					para, err := e.parseParagraphElement(decoder)
					if err != nil {
						return nil, err
					}
					if para != nil {
						items = append(items, para)
					}
				case "tbl":
					// Parse table
					tableItems, err := e.parseTableElement(decoder)
					if err != nil {
						return nil, err
					}
					items = append(items, tableItems...)
				default:
					depth++
				}
			} else {
				depth++
			}

		case xml.EndElement:
			localName := stripNamespacePrefix(t.Name.Local)
			if localName == "body" {
				inBody = false
			} else if inBody && depth > 0 {
				depth--
			}
		}
	}

	return items, nil
}

// parseParagraphElement parses a paragraph element into a LineItem
func (e *Extractor) parseParagraphElement(decoder *xml.Decoder) (*models.LineItem, error) {
	var words []*models.Word
	var styleID string
	var numPr *NumberingPr
	var depth int

	var currentBold, currentItalic bool
	var inHyperlink bool
	var hyperlinkID string

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			localName := stripNamespacePrefix(t.Name.Local)

			switch localName {
			case "pStyle":
				for _, attr := range t.Attr {
					if stripNamespacePrefix(attr.Name.Local) == "val" {
						styleID = attr.Value
					}
				}
			case "numPr":
				numPr = &NumberingPr{}
			case "ilvl":
				if numPr != nil {
					for _, attr := range t.Attr {
						if stripNamespacePrefix(attr.Name.Local) == "val" {
							var lvl ILevel
							fmt.Sscanf(attr.Value, "%d", &lvl.Val)
							numPr.ILevel = &lvl
						}
					}
				}
			case "numId":
				if numPr != nil {
					for _, attr := range t.Attr {
						if stripNamespacePrefix(attr.Name.Local) == "val" {
							var id NumID
							fmt.Sscanf(attr.Value, "%d", &id.Val)
							numPr.NumID = &id
						}
					}
				}
			case "b", "bCs":
				currentBold = true
			case "i", "iCs":
				currentItalic = true
			case "hyperlink":
				inHyperlink = true
				for _, attr := range t.Attr {
					name := stripNamespacePrefix(attr.Name.Local)
					if name == "id" {
						hyperlinkID = attr.Value
					}
				}
			case "t":
				// Read text content
				text, err := e.readTextContent(decoder)
				if err != nil {
					return nil, err
				}
				if text != "" {
					word := &models.Word{
						String: text,
						Format: e.getWordFormat(currentBold, currentItalic),
					}
					if inHyperlink && hyperlinkID != "" {
						target := e.relationships.GetTarget(hyperlinkID)
						if target != "" {
							word.Type = &models.WordType{
								Name: "LINK",
							}
							// Store the URL in a custom field or handle during rendering
							word.String = fmt.Sprintf("[%s](%s)", text, target)
							word.Format = nil // Links don't need additional formatting
						}
					}
					words = append(words, word)
				}
			case "tab":
				words = append(words, &models.Word{String: "\t"})
			case "br":
				// Check break type
				breakType := ""
				for _, attr := range t.Attr {
					if stripNamespacePrefix(attr.Name.Local) == "type" {
						breakType = attr.Value
					}
				}
				if breakType == "page" {
					// Page break - could be handled specially
				}
			case "r":
				// Reset run-level formatting
				currentBold = false
				currentItalic = false
			case "drawing":
				// Parse drawing element for images
				imgWord, err := e.parseDrawingElement(decoder)
				if err != nil {
					return nil, err
				}
				if imgWord != nil {
					words = append(words, imgWord)
				}
			case "footnoteReference":
				// Parse footnote reference
				for _, attr := range t.Attr {
					if stripNamespacePrefix(attr.Name.Local) == "id" {
						// Skip separators (id="0" and id="-1")
						if attr.Value != "0" && attr.Value != "-1" {
							e.footnoteRefs = append(e.footnoteRefs, attr.Value)
							words = append(words, &models.Word{
								String: attr.Value,
								Type:   models.WordTypeFootnoteLink,
							})
						}
					}
				}
			case "endnoteReference":
				// Parse endnote reference
				for _, attr := range t.Attr {
					if stripNamespacePrefix(attr.Name.Local) == "id" {
						// Skip separators (id="0" and id="-1")
						if attr.Value != "0" && attr.Value != "-1" {
							e.endnoteRefs = append(e.endnoteRefs, attr.Value)
							words = append(words, &models.Word{
								String: attr.Value,
								Type:   models.WordTypeFootnoteLink,
							})
						}
					}
				}
			default:
				depth++
			}

		case xml.EndElement:
			localName := stripNamespacePrefix(t.Name.Local)

			switch localName {
			case "p":
				// End of paragraph
				return e.createLineItem(words, styleID, numPr), nil
			case "hyperlink":
				inHyperlink = false
				hyperlinkID = ""
			case "r":
				// Reset run formatting at end of run
				currentBold = false
				currentItalic = false
			default:
				if depth > 0 {
					depth--
				}
			}
		}
	}

	return e.createLineItem(words, styleID, numPr), nil
}

// readTextContent reads text content until the closing tag
func (e *Extractor) readTextContent(decoder *xml.Decoder) (string, error) {
	var text strings.Builder
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		switch t := tok.(type) {
		case xml.CharData:
			text.Write(t)
		case xml.EndElement:
			if stripNamespacePrefix(t.Name.Local) == "t" {
				return text.String(), nil
			}
		}
	}
	return text.String(), nil
}

// parseTableElement parses a table into LineItems
func (e *Extractor) parseTableElement(decoder *xml.Decoder) ([]interface{}, error) {
	var items []interface{}
	var currentRow []string
	var isFirstRow bool = true
	var depth int

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			localName := stripNamespacePrefix(t.Name.Local)

			switch localName {
			case "tr":
				currentRow = []string{}
			case "tc":
				// Parse cell content
				cellText, err := e.parseTableCell(decoder)
				if err != nil {
					return nil, err
				}
				currentRow = append(currentRow, cellText)
			default:
				depth++
			}

		case xml.EndElement:
			localName := stripNamespacePrefix(t.Name.Local)

			switch localName {
			case "tbl":
				return items, nil
			case "tr":
				if len(currentRow) > 0 {
					lineItem := &models.LineItem{
						IsTableRow:    true,
						IsTableHeader: isFirstRow,
						TableColumns:  currentRow,
						Words:         []*models.Word{{String: strings.Join(currentRow, " | ")}},
					}
					items = append(items, lineItem)
					isFirstRow = false
				}
			default:
				if depth > 0 {
					depth--
				}
			}
		}
	}

	return items, nil
}

// parseTableCell parses a table cell and returns its text content
func (e *Extractor) parseTableCell(decoder *xml.Decoder) (string, error) {
	var text strings.Builder
	var depth int

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			localName := stripNamespacePrefix(t.Name.Local)
			if localName == "t" {
				content, err := e.readTextContent(decoder)
				if err != nil {
					return "", err
				}
				if text.Len() > 0 && content != "" {
					text.WriteString(" ")
				}
				text.WriteString(content)
			} else {
				depth++
			}

		case xml.EndElement:
			localName := stripNamespacePrefix(t.Name.Local)
			if localName == "tc" {
				return strings.TrimSpace(text.String()), nil
			}
			if depth > 0 {
				depth--
			}
		}
	}

	return strings.TrimSpace(text.String()), nil
}

// createLineItem creates a LineItem from parsed content
func (e *Extractor) createLineItem(words []*models.Word, styleID string, numPr *NumberingPr) *models.LineItem {
	if len(words) == 0 {
		return nil
	}

	// Merge consecutive words with the same format to fix broken formatting
	// Per OOXML spec, runs concatenate directly without implicit spaces
	words = mergeConsecutiveFormattedWords(words)

	// Clean up whitespace in words
	words = cleanupWordWhitespace(words)

	if len(words) == 0 {
		return nil
	}

	lineItem := &models.LineItem{
		Words: words,
	}

	// Determine block type from style
	if styleID != "" {
		isHeading, level := e.styles.IsHeading(styleID)
		if isHeading {
			lineItem.Type = models.HeadlineByLevel(level)
		}
	}

	// Handle list items
	if numPr != nil && numPr.NumID != nil {
		lineItem.Type = models.BlockTypeList

		// Get list prefix
		numID := numPr.NumID.Val
		level := 0
		if numPr.ILevel != nil {
			level = numPr.ILevel.Val
		}

		// Track counters
		if e.listCounters[numID] == nil {
			e.listCounters[numID] = make(map[int]int)
		}
		e.listCounters[numID][level]++
		counter := e.listCounters[numID][level]

		// Get prefix
		prefix := e.numbering.GetListPrefix(numID, level, counter)

		// Store list level on LineItem (indentation handled at render time)
		lineItem.ListLevel = level

		// Prepend prefix to first word
		if len(lineItem.Words) > 0 {
			lineItem.Words[0].String = prefix + lineItem.Words[0].String
		}
	}

	return lineItem
}

// getWordFormat returns the word format for given formatting flags
func (e *Extractor) getWordFormat(bold, italic bool) *models.WordFormat {
	if bold && italic {
		return models.WordFormatBoldOblique
	}
	if bold {
		return models.WordFormatBold
	}
	if italic {
		return models.WordFormatOblique
	}
	return nil
}

// GetStyles returns the parsed styles
func (e *Extractor) GetStyles() *Styles {
	return e.styles
}

// GetRelationships returns the parsed relationships
func (e *Extractor) GetRelationships() *Relationships {
	return e.relationships
}

// mergeConsecutiveFormattedWords merges adjacent words that have the same formatting
// This fixes issues where DOCX splits formatted text across multiple runs.
// Per OOXML spec, runs are concatenated directly - spaces only exist if explicitly
// present in the <w:t> content with xml:space="preserve".
func mergeConsecutiveFormattedWords(words []*models.Word) []*models.Word {
	if len(words) <= 1 {
		return words
	}

	var result []*models.Word
	current := words[0]

	for i := 1; i < len(words); i++ {
		next := words[i]

		// Check if formats match and neither is a special type (like links)
		if sameFormat(current.Format, next.Format) && current.Type == nil && next.Type == nil {
			// Merge: concatenate directly per OOXML spec (no implicit space)
			current.String += next.String
		} else {
			result = append(result, current)
			current = next
		}
	}
	result = append(result, current)

	return result
}

// sameFormat checks if two word formats are equivalent
func sameFormat(a, b *models.WordFormat) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Name == b.Name
}

// cleanupWordWhitespace cleans up whitespace issues in words
func cleanupWordWhitespace(words []*models.Word) []*models.Word {
	var result []*models.Word
	for _, w := range words {
		// Skip empty words and pure whitespace words (except tabs)
		trimmed := strings.TrimSpace(w.String)
		if trimmed == "" && w.String != "\t" {
			continue
		}

		// Normalize multiple spaces to single space
		w.String = normalizeSpaces(w.String)

		result = append(result, w)
	}
	return result
}

// normalizeSpaces replaces multiple consecutive spaces with a single space
func normalizeSpaces(s string) string {
	var result strings.Builder
	prevSpace := false
	for _, r := range s {
		if r == ' ' {
			if !prevSpace {
				result.WriteRune(r)
				prevSpace = true
			}
		} else {
			result.WriteRune(r)
			prevSpace = false
		}
	}
	return result.String()
}

// parseDrawingElement parses a drawing element and extracts image information.
// Returns a Word with type IMAGE containing the image ID reference.
func (e *Extractor) parseDrawingElement(decoder *xml.Decoder) (*models.Word, error) {
	var embedID string
	var altText string
	var depth int

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			localName := stripNamespacePrefix(t.Name.Local)

			switch localName {
			case "docPr":
				// Get alt text from description attribute
				for _, attr := range t.Attr {
					name := stripNamespacePrefix(attr.Name.Local)
					if name == "descr" {
						altText = attr.Value
					} else if name == "name" && altText == "" {
						// Use name as fallback for alt text
						altText = attr.Value
					}
				}
			case "blip":
				// Get the embed relationship ID
				for _, attr := range t.Attr {
					name := stripNamespacePrefix(attr.Name.Local)
					if name == "embed" {
						embedID = attr.Value
					}
				}
			default:
				depth++
			}

		case xml.EndElement:
			localName := stripNamespacePrefix(t.Name.Local)
			if localName == "drawing" {
				// End of drawing element
				if embedID != "" {
					return e.extractAndRegisterImage(embedID, altText)
				}
				return nil, nil
			}
			if depth > 0 {
				depth--
			}
		}
	}

	return nil, nil
}

// extractAndRegisterImage extracts image data from the DOCX archive and registers it.
// Returns a Word with type IMAGE containing the image ID reference.
func (e *Extractor) extractAndRegisterImage(relID, altText string) (*models.Word, error) {
	// Check if this is actually an image relationship
	if !e.relationships.IsImage(relID) {
		return nil, nil
	}

	// Get the target path from relationships
	target := e.relationships.GetTarget(relID)
	if target == "" {
		return nil, nil
	}

	// Normalize path - relationships use relative paths from word/ directory
	imagePath := target
	if !strings.HasPrefix(target, "word/") {
		imagePath = "word/" + target
	}

	// Read the image data from the DOCX archive
	data, err := e.parser.ReadFile(imagePath)
	if err != nil {
		// Image not found, skip silently
		return nil, nil
	}

	// Generate unique ID
	e.imageCounter++
	imageID := fmt.Sprintf("image_%03d", e.imageCounter)

	// Detect format from magic bytes
	format := detectImageFormat(data)

	// Create ImageItem
	img := &models.ImageItem{
		ID:         imageID,
		SourcePath: imagePath,
		Format:     format,
		Data:       data,
		AltText:    altText,
		PageIndex:  0, // DOCX doesn't have pages in the same way
	}

	// Register the image
	e.images = append(e.images, img)

	// Return a word referencing this image
	return &models.Word{
		String: imageID,
		Type:   models.WordTypeImage,
	}, nil
}

// detectImageFormat detects image format from magic bytes
func detectImageFormat(data []byte) string {
	if len(data) < 8 {
		return "bin"
	}

	// JPEG: FF D8 FF
	if len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "jpeg"
	}

	// PNG: 89 50 4E 47 0D 0A 1A 0A
	if len(data) >= 8 && data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return "png"
	}

	// GIF: 47 49 46 38
	if len(data) >= 4 && data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x38 {
		return "gif"
	}

	// BMP: 42 4D
	if len(data) >= 2 && data[0] == 0x42 && data[1] == 0x4D {
		return "bmp"
	}

	// TIFF: 49 49 2A 00 (little-endian) or 4D 4D 00 2A (big-endian)
	if len(data) >= 4 {
		if (data[0] == 0x49 && data[1] == 0x49 && data[2] == 0x2A && data[3] == 0x00) ||
			(data[0] == 0x4D && data[1] == 0x4D && data[2] == 0x00 && data[3] == 0x2A) {
			return "tiff"
		}
	}

	// EMF/WMF are common in DOCX
	if len(data) >= 4 {
		// EMF signature check
		if data[0] == 0x01 && data[1] == 0x00 && data[2] == 0x00 && data[3] == 0x00 {
			return "emf"
		}
		// WMF placeable header
		if data[0] == 0xD7 && data[1] == 0xCD && data[2] == 0xC6 && data[3] == 0x9A {
			return "wmf"
		}
	}

	return "bin"
}

// GetImages returns all extracted images
func (e *Extractor) GetImages() []*models.ImageItem {
	return e.images
}

// FootnoteItem represents a collected footnote/endnote for output
type FootnoteItem struct {
	ID        string
	Content   string
	IsEndnote bool
}

// GetCollectedFootnotes returns all footnotes and endnotes in order of appearance
func (e *Extractor) GetCollectedFootnotes() []FootnoteItem {
	var result []FootnoteItem

	// Collect footnotes
	for _, id := range e.footnoteRefs {
		content := e.getFootnoteContent(id)
		if content != "" {
			result = append(result, FootnoteItem{ID: id, Content: content})
		}
	}

	// Collect endnotes
	for _, id := range e.endnoteRefs {
		content := e.getEndnoteContent(id)
		if content != "" {
			result = append(result, FootnoteItem{ID: id, Content: content, IsEndnote: true})
		}
	}

	return result
}

// getFootnoteContent extracts text content from a footnote by ID
func (e *Extractor) getFootnoteContent(id string) string {
	if e.footnotes == nil {
		return ""
	}
	for _, fn := range e.footnotes.Footnotes {
		if fn.ID == id && fn.Type == "" { // Skip separators
			return e.extractParagraphsText(fn.Paragraphs)
		}
	}
	return ""
}

// getEndnoteContent extracts text content from an endnote by ID
func (e *Extractor) getEndnoteContent(id string) string {
	if e.endnotes == nil {
		return ""
	}
	for _, en := range e.endnotes.Endnotes {
		if en.ID == id && en.Type == "" { // Skip separators
			return e.extractParagraphsText(en.Paragraphs)
		}
	}
	return ""
}

// extractParagraphsText extracts plain text from a slice of paragraphs
func (e *Extractor) extractParagraphsText(paras []Paragraph) string {
	var parts []string
	for _, para := range paras {
		text := e.extractParagraphText(para)
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " ")
}

// extractParagraphText extracts plain text from a single paragraph
func (e *Extractor) extractParagraphText(para Paragraph) string {
	var text strings.Builder

	// Extract text from direct runs
	for _, run := range para.Runs {
		for _, t := range run.Text {
			text.WriteString(t.Value)
		}
	}

	// Extract text from hyperlinks (hyperlinks contain their own runs)
	for _, hl := range para.Hyperlinks {
		for _, run := range hl.Runs {
			for _, t := range run.Text {
				text.WriteString(t.Value)
			}
		}
	}

	return strings.TrimSpace(text.String())
}

// HeaderFooterContent represents extracted header or footer content
type HeaderFooterContent struct {
	ID      string // e.g., "header1", "footer2"
	Content string // Plain text content
}

// ExtractHeaders extracts text from all document headers
func (e *Extractor) ExtractHeaders() ([]HeaderFooterContent, error) {
	headers, err := e.parser.GetHeaders()
	if err != nil {
		return nil, err
	}

	var result []HeaderFooterContent
	for id, header := range headers {
		content := e.extractHeaderFooterContent(header.Paragraphs, header.Tables)
		if content != "" {
			result = append(result, HeaderFooterContent{ID: id, Content: content})
		}
	}

	return result, nil
}

// ExtractFooters extracts text from all document footers
func (e *Extractor) ExtractFooters() ([]HeaderFooterContent, error) {
	footers, err := e.parser.GetFooters()
	if err != nil {
		return nil, err
	}

	var result []HeaderFooterContent
	for id, footer := range footers {
		content := e.extractHeaderFooterContent(footer.Paragraphs, footer.Tables)
		if content != "" {
			result = append(result, HeaderFooterContent{ID: id, Content: content})
		}
	}

	return result, nil
}

// extractHeaderFooterContent extracts text from paragraphs and tables
func (e *Extractor) extractHeaderFooterContent(paras []Paragraph, tables []Table) string {
	var parts []string

	// Extract text from paragraphs
	for _, para := range paras {
		text := e.extractParagraphText(para)
		if text != "" {
			parts = append(parts, text)
		}
	}

	// Extract text from tables
	for _, table := range tables {
		for _, row := range table.Rows {
			var rowParts []string
			for _, cell := range row.Cells {
				cellText := e.extractParagraphsText(cell.Paragraphs)
				if cellText != "" {
					rowParts = append(rowParts, cellText)
				}
			}
			if len(rowParts) > 0 {
				parts = append(parts, strings.Join(rowParts, " | "))
			}
		}
	}

	return strings.Join(parts, " ")
}

/******************************************************************************
 * Copyright (c) 2025-2026 Tenebris Technologies Inc.                         *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package docx

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
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

// bodyReader reads document.xml tokens and counts the paragraphs it passes, so
// that content can be traced back to its position in the original document.
//
// The count is the 1-based position of each w:p element in document order,
// including paragraphs inside table cells. Paragraphs inside mc:Fallback are
// not counted: they duplicate the content of the preceding mc:Choice, and a
// viewer renders only one of the two.
type bodyReader struct {
	*xml.Decoder
	paragraphs    int
	fallbackDepth int
}

// Token returns the next token, updating the paragraph count
func (r *bodyReader) Token() (xml.Token, error) {
	tok, err := r.Decoder.Token()
	if err != nil {
		return tok, err
	}
	switch t := tok.(type) {
	case xml.StartElement:
		switch {
		case t.Name.Local == "Fallback":
			r.fallbackDepth++
		case t.Name.Local == "p" && isWordNamespace(t.Name.Space) && r.fallbackDepth == 0:
			r.paragraphs++
		}
	case xml.EndElement:
		if t.Name.Local == "Fallback" && r.fallbackDepth > 0 {
			r.fallbackDepth--
		}
	}
	return tok, nil
}

// isWordNamespace reports whether an element namespace is WordprocessingML
// (transitional or strict), as opposed to e.g. DrawingML, which also has a:p
func isWordNamespace(space string) bool {
	return space == "w" ||
		strings.HasSuffix(space, "/wordprocessingml/2006/main") ||
		strings.HasSuffix(space, "/wordprocessingml/main")
}

// parseDocumentBody parses the document body into LineItems
func (e *Extractor) parseDocumentBody(data []byte) ([]interface{}, error) {
	var items []interface{}

	// Use streaming parser to handle mixed content
	decoder := &bodyReader{Decoder: xml.NewDecoder(bytes.NewReader(data))}
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
					paraNum := decoder.paragraphs
					para, err := e.parseParagraphElement(decoder)
					if err != nil {
						return nil, err
					}
					if para != nil {
						para.Paragraph = paraNum
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
func (e *Extractor) parseParagraphElement(decoder *bodyReader) (*models.LineItem, error) {
	var words []*models.Word
	var styleID string
	var numPr *NumberingPr

	var currentBold, currentItalic bool
	var nested int // open paragraphs of text boxes inside this paragraph

	// The text of a hyperlink with a target is gathered across its runs and
	// becomes one link word when the hyperlink ends
	var linkTarget string
	var linkText strings.Builder

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		// mc:Fallback repeats the content of the mc:Choice before it
		if decoder.fallbackDepth > 0 {
			continue
		}

		switch t := tok.(type) {
		case xml.StartElement:
			localName := stripNamespacePrefix(t.Name.Local)

			// The properties of a text box paragraph are not this paragraph's
			if nested > 0 && (localName == "pStyle" || localName == "numPr" || localName == "ilvl" || localName == "numId") {
				continue
			}

			switch localName {
			case "p":
				// A text box paragraph (w:pict/v:textbox, w:txbxContent): its
				// text joins this paragraph, separated by a space
				if isWordNamespace(t.Name.Space) {
					nested++
					words = append(words, &models.Word{String: " "})
				}
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
							_, _ = fmt.Sscanf(attr.Value, "%d", &lvl.Val)
							numPr.ILevel = &lvl
						}
					}
				}
			case "numId":
				if numPr != nil {
					for _, attr := range t.Attr {
						if stripNamespacePrefix(attr.Name.Local) == "val" {
							var id NumID
							_, _ = fmt.Sscanf(attr.Value, "%d", &id.Val)
							numPr.NumID = &id
						}
					}
				}
			case "b", "bCs":
				currentBold = true
			case "i", "iCs":
				currentItalic = true
			case "hyperlink":
				for _, attr := range t.Attr {
					if stripNamespacePrefix(attr.Name.Local) == "id" && attr.Value != "" {
						linkTarget = e.relationships.GetTarget(attr.Value)
					}
				}
			case "t":
				// Read text content
				text, err := e.readTextContent(decoder)
				if err != nil {
					return nil, err
				}
				if linkTarget != "" {
					linkText.WriteString(text)
				} else if text != "" {
					words = append(words, &models.Word{
						String: text,
						Format: e.getWordFormat(currentBold, currentItalic),
					})
				}
			case "tab", "br", "cr":
				// Tabs and line breaks separate the words on either side
				if linkTarget != "" {
					linkText.WriteString(" ")
				} else {
					words = append(words, &models.Word{String: " "})
				}
			case "r":
				// Reset run-level formatting
				currentBold = false
				currentItalic = false
			case "drawing":
				// Parse drawing element for images and text boxes
				drawingWords, err := e.parseDrawingElement(decoder)
				if err != nil {
					return nil, err
				}
				words = append(words, drawingWords...)
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
			}

		case xml.EndElement:
			localName := stripNamespacePrefix(t.Name.Local)

			switch {
			case localName == "p" && isWordNamespace(t.Name.Space) && nested > 0:
				nested--
				words = append(words, &models.Word{String: " "})
			case localName == "p" && isWordNamespace(t.Name.Space):
				// End of paragraph
				return e.createLineItem(words, styleID, numPr), nil
			case localName == "hyperlink":
				if linkTarget != "" {
					words = append(words, linkWords(linkText.String(), linkTarget)...)
				}
				linkTarget = ""
				linkText.Reset()
			case localName == "r":
				// Reset run formatting at end of run
				currentBold = false
				currentItalic = false
			}
		}
	}

	return e.createLineItem(words, styleID, numPr), nil
}

// linkWords returns the words for the text of a hyperlink: one LINK word
// wrapping the trimmed text, with the whitespace the text had at its edges
// kept outside the link as separate whitespace words
func linkWords(text, target string) []*models.Word {
	trimmed := trimSpaces(text)
	if trimmed == "" {
		return []*models.Word{{String: text}}
	}
	var words []*models.Word
	if len(trimSpacesLeft(text)) != len(text) {
		words = append(words, &models.Word{String: " "})
	}
	words = append(words, &models.Word{
		String: fmt.Sprintf("[%s](%s)", normalizeSpaces(trimmed), target),
		Type:   &models.WordType{Name: "LINK"},
	})
	if len(trimSpacesRight(text)) != len(text) {
		words = append(words, &models.Word{String: " "})
	}
	return words
}

// readTextContent reads text content until the closing tag
func (e *Extractor) readTextContent(decoder *bodyReader) (string, error) {
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
func (e *Extractor) parseTableElement(decoder *bodyReader) ([]interface{}, error) {
	var items []interface{}
	var currentRow []string
	var rowPara int // Number of the first paragraph in the current row
	isFirstRow := true
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
				rowPara = decoder.paragraphs + 1
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
						Paragraph:     rowPara,
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

// parseTableCell parses a table cell and returns its text content. The
// cell's runs are spaced as body text is (cleanupWordWhitespace); its
// paragraphs, tabs and line breaks, and the cells of a table nested in it,
// are separated by a space.
func (e *Extractor) parseTableCell(decoder *bodyReader) (string, error) {
	var words []*models.Word
	var nested int // open cells of tables nested in this cell

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		// mc:Fallback repeats the content of the mc:Choice before it
		if decoder.fallbackDepth > 0 {
			continue
		}

		switch t := tok.(type) {
		case xml.StartElement:
			localName := stripNamespacePrefix(t.Name.Local)
			switch {
			case localName == "t" && isWordNamespace(t.Name.Space):
				content, err := e.readTextContent(decoder)
				if err != nil {
					return "", err
				}
				words = append(words, &models.Word{String: content})
			case localName == "tab" || localName == "br" || localName == "cr":
				words = append(words, &models.Word{String: " "})
			case localName == "tc" && isWordNamespace(t.Name.Space):
				nested++
			}

		case xml.EndElement:
			localName := stripNamespacePrefix(t.Name.Local)
			if !isWordNamespace(t.Name.Space) {
				continue
			}
			switch {
			case localName == "tc" && nested > 0:
				nested--
				words = append(words, &models.Word{String: " "})
			case localName == "tc":
				return joinWords(words), nil
			case localName == "p":
				words = append(words, &models.Word{String: " "})
			}
		}
	}

	return joinWords(words), nil
}

// joinWords returns the plain text of words, spaced as the renderer spaces
// body text
func joinWords(words []*models.Word) string {
	var text strings.Builder
	for i, w := range cleanupWordWhitespace(words) {
		if i > 0 && (w.Type == nil || !w.Type.AttachWithoutWhitespace) {
			text.WriteString(" ")
		}
		text.WriteString(w.String)
	}
	return text.String()
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
	isHeading := false
	if styleID != "" {
		var level int
		isHeading, level = e.styles.IsHeading(styleID)
		if isHeading {
			lineItem.Type = models.HeadlineByLevel(level)
		}
	}

	// Numbered paragraphs. A heading stays a heading and shows its number
	// in its text; any other numbered paragraph is a list item. The label is
	// a word of its own so it never takes on the formatting of the text.
	numID, level, fromStyle, numbered := e.resolveNumbering(styleID, numPr)
	if numbered {
		label := e.nextNumber(numID, level)
		prefix := listMarker(label)
		if isHeading {
			prefix = headingNumber(label)
		} else {
			lineItem.Type = models.BlockTypeList

			// Store list level on LineItem (indentation handled at render time)
			lineItem.ListLevel = level
			if fromStyle {
				lineItem.ListLevel = max(level, e.styles.ListStyleDepth(styleID))
			}
		}
		if prefix != "" {
			lineItem.Words = append([]*models.Word{{String: prefix}}, lineItem.Words...)
		}
	}

	return lineItem
}

// resolveNumbering returns the list numbering of a paragraph from its direct
// numPr and its paragraph style. A direct numId wins over the style, and a
// numId of 0 means the paragraph is not numbered. When no level is given,
// the level of the numbering definition bound (w:lvl/w:pStyle) to the
// nearest style in the paragraph style's basedOn chain is used, else 0.
// fromStyle reports that the numId came from the style.
func (e *Extractor) resolveNumbering(styleID string, direct *NumberingPr) (numID, level int, fromStyle, numbered bool) {
	var numIDRef *NumID
	var levelRef *ILevel
	if direct != nil {
		numIDRef = direct.NumID
		levelRef = direct.ILevel
	}
	if numIDRef == nil {
		styleNumID, styleLevel := e.styles.StyleNumbering(styleID)
		if styleNumID == nil {
			return 0, 0, false, false
		}
		numIDRef = styleNumID
		fromStyle = true
		if levelRef == nil {
			levelRef = styleLevel
		}
	}
	if numIDRef.Val == 0 {
		return 0, 0, false, false
	}

	numID = numIDRef.Val
	switch {
	case levelRef != nil:
		level = levelRef.Val
	case fromStyle:
		for _, style := range e.styles.styleChain(styleID) {
			if l, ok := e.numbering.LevelForStyle(numID, style.StyleID); ok {
				level = l
				break
			}
		}
	}
	return numID, level, fromStyle, true
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

// cleanupWordWhitespace moves the whitespace of the runs into the spacing
// between words. The renderer puts one space between words unless a word's
// type attaches it to the one before, so each word is trimmed and marked as
// attached when the source had no whitespace before it. Runs thus still
// concatenate without implicit spaces ("an" + "d" stays "and", "un" + bold
// "able" becomes "un**able**"), and a space that sits at the edge of a
// formatted run is written once, outside the formatting. Runs of whitespace
// become a single space. Only ASCII whitespace counts: a no-break space is
// text and is kept.
func cleanupWordWhitespace(words []*models.Word) []*models.Word {
	var result []*models.Word
	spaceBefore := false // whitespace seen since the previous word
	for _, w := range words {
		trimmed := trimSpaces(w.String)
		if trimmed == "" {
			if w.String != "" {
				spaceBefore = true
			}
			continue
		}
		leading := len(trimSpacesLeft(w.String)) != len(w.String)
		trailing := len(trimSpacesRight(w.String)) != len(w.String)

		w.String = normalizeSpaces(trimmed)
		if len(result) > 0 && !spaceBefore && !leading {
			w.Type = attachedType(w.Type)
		}
		result = append(result, w)
		spaceBefore = trailing
	}
	return result
}

// asciiSpace is the whitespace that separates words; a no-break space
// (U+00A0) is not in it
const asciiSpace = " \t\r\n"

func trimSpaces(s string) string      { return strings.Trim(s, asciiSpace) }
func trimSpacesLeft(s string) string  { return strings.TrimLeft(s, asciiSpace) }
func trimSpacesRight(s string) string { return strings.TrimRight(s, asciiSpace) }

// attachedType returns a copy of a word type, or a plain type when t is nil,
// that the renderer writes without a space before it
func attachedType(t *models.WordType) *models.WordType {
	attached := models.WordType{}
	if t != nil {
		attached = *t
	}
	attached.AttachWithoutWhitespace = true
	return &attached
}

// normalizeSpaces replaces each run of ASCII whitespace with a single space
func normalizeSpaces(s string) string {
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool {
		return strings.ContainsRune(asciiSpace, r)
	}), " ")
}

// parseDrawingElement parses a drawing element and returns its words: the
// text of any text box it holds (w:txbxContent), then a Word with type IMAGE
// containing the image ID reference when it holds an image.
func (e *Extractor) parseDrawingElement(decoder *bodyReader) ([]*models.Word, error) {
	type picture struct{ embedID, altText string }
	var pictures []picture
	var altText string
	var words []*models.Word
	var nested int // drawings inside a text box of this drawing

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
				// Alt text of the pictures that follow: the description,
				// else the name
				var descr, name string
				for _, attr := range t.Attr {
					switch stripNamespacePrefix(attr.Name.Local) {
					case "descr":
						descr = attr.Value
					case "name":
						name = attr.Value
					}
				}
				altText = descr
				if altText == "" {
					altText = name
				}
			case "blip":
				// Get the embed relationship ID
				for _, attr := range t.Attr {
					name := stripNamespacePrefix(attr.Name.Local)
					if name == "embed" && attr.Value != "" {
						pictures = append(pictures, picture{attr.Value, altText})
					}
				}
			case "drawing":
				nested++
			case "tab", "br", "cr":
				if isWordNamespace(t.Name.Space) && decoder.fallbackDepth == 0 {
					words = append(words, &models.Word{String: " "})
				}
			case "p", "t":
				if !isWordNamespace(t.Name.Space) || decoder.fallbackDepth > 0 {
					continue
				}
				if localName == "p" {
					// Text box paragraphs are separated by a space
					words = append(words, &models.Word{String: " "})
					continue
				}
				text, err := e.readTextContent(decoder)
				if err != nil {
					return nil, err
				}
				words = append(words, &models.Word{String: text})
			}

		case xml.EndElement:
			localName := stripNamespacePrefix(t.Name.Local)
			switch {
			case localName == "drawing" && nested > 0:
				nested--
			case localName == "drawing":
				for _, pic := range pictures {
					image, err := e.extractAndRegisterImage(pic.embedID, pic.altText)
					if err != nil {
						return nil, err
					}
					if image != nil {
						words = append(words, &models.Word{String: " "}, image, &models.Word{String: " "})
					}
				}
				return words, nil
			case localName == "p" && isWordNamespace(t.Name.Space) && decoder.fallbackDepth == 0:
				words = append(words, &models.Word{String: " "})
			}
		}
	}

	return words, nil
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

	return trimSpaces(text.String())
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
	for _, id := range sortedPartIDs(headers) {
		header := headers[id]
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
	for _, id := range sortedPartIDs(footers) {
		footer := footers[id]
		content := e.extractHeaderFooterContent(footer.Paragraphs, footer.Tables)
		if content != "" {
			result = append(result, HeaderFooterContent{ID: id, Content: content})
		}
	}

	return result, nil
}

// sortedPartIDs returns the IDs of header or footer parts in a stable order:
// by name, with "header2" before "header10".
func sortedPartIDs[T any](parts map[string]T) []string {
	ids := make([]string, 0, len(parts))
	for id := range parts {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if len(ids[i]) != len(ids[j]) {
			return len(ids[i]) < len(ids[j])
		}
		return ids[i] < ids[j]
	})
	return ids
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

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

	// List tracking
	listCounters map[int]map[int]int // numId -> level -> counter
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

	return &Extractor{
		parser:        parser,
		styles:        styles,
		numbering:     numbering,
		relationships: rels,
		listCounters:  make(map[int]map[int]int),
	}, nil
}

// Extract converts the DOCX document to Page format
func (e *Extractor) Extract() (*models.Page, error) {
	// Read raw document XML for custom parsing
	docData, err := e.parser.ReadFile("word/document.xml")
	if err != nil {
		return nil, err
	}

	// Parse document body content
	items, err := e.parseDocumentBody(docData)
	if err != nil {
		return nil, err
	}

	return &models.Page{
		Index: 0,
		Items: items,
	}, nil
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

		// Add indentation for nested lists
		indent := strings.Repeat("  ", level)

		// Prepend prefix to first word
		if len(lineItem.Words) > 0 {
			lineItem.Words[0].String = indent + prefix + lineItem.Words[0].String
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

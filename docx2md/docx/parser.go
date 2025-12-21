package docx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"
)

// Parser handles DOCX file parsing
type Parser struct {
	data      []byte
	zipReader *zip.Reader
	files     map[string]*zip.File

	// Cached parsed content
	document      *Document
	styles        *Styles
	numbering     *Numbering
	relationships *Relationships
	footnotes     *Footnotes
	endnotes      *Endnotes
	headers       map[string]*Header
	footers       map[string]*Footer
}

// NewParser creates a parser from byte data
func NewParser(data []byte) (*Parser, error) {
	reader := bytes.NewReader(data)
	zipReader, err := zip.NewReader(reader, int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("opening ZIP archive: %w", err)
	}

	p := &Parser{
		data:      data,
		zipReader: zipReader,
		files:     make(map[string]*zip.File),
	}

	// Index files by name
	for _, f := range zipReader.File {
		p.files[f.Name] = f
	}

	return p, nil
}

// NewParserFromFile creates a parser from a file path
func NewParserFromFile(path string) (*Parser, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}
	return NewParser(data)
}

// Parse parses the DOCX structure
func (p *Parser) Parse() error {
	// Verify this is a valid DOCX file
	if _, ok := p.files["word/document.xml"]; !ok {
		return fmt.Errorf("not a valid DOCX file: missing word/document.xml")
	}
	return nil
}

// GetDocument returns the parsed document content
func (p *Parser) GetDocument() (*Document, error) {
	if p.document != nil {
		return p.document, nil
	}

	doc := &Document{}
	if err := p.readXML("word/document.xml", doc); err != nil {
		return nil, err
	}

	p.document = doc
	return doc, nil
}

// GetStyles returns the parsed styles
func (p *Parser) GetStyles() (*Styles, error) {
	if p.styles != nil {
		return p.styles, nil
	}

	styles := &Styles{}
	if err := p.readXML("word/styles.xml", styles); err != nil {
		// Styles file is optional
		return &Styles{}, nil
	}

	p.styles = styles
	return styles, nil
}

// GetNumbering returns the parsed numbering definitions
func (p *Parser) GetNumbering() (*Numbering, error) {
	if p.numbering != nil {
		return p.numbering, nil
	}

	numbering := &Numbering{}
	if err := p.readXML("word/numbering.xml", numbering); err != nil {
		// Numbering file is optional
		return &Numbering{}, nil
	}

	p.numbering = numbering
	return numbering, nil
}

// GetRelationships returns the parsed document relationships
func (p *Parser) GetRelationships() (*Relationships, error) {
	if p.relationships != nil {
		return p.relationships, nil
	}

	rels := &Relationships{}
	if err := p.readXML("word/_rels/document.xml.rels", rels); err != nil {
		// Relationships file is optional
		return &Relationships{}, nil
	}

	p.relationships = rels
	return rels, nil
}

// GetFootnotes returns the parsed footnotes
func (p *Parser) GetFootnotes() (*Footnotes, error) {
	if p.footnotes != nil {
		return p.footnotes, nil
	}

	footnotes := &Footnotes{}
	if err := p.readXML("word/footnotes.xml", footnotes); err != nil {
		// Footnotes file is optional
		return &Footnotes{}, nil
	}

	p.footnotes = footnotes
	return footnotes, nil
}

// GetEndnotes returns the parsed endnotes
func (p *Parser) GetEndnotes() (*Endnotes, error) {
	if p.endnotes != nil {
		return p.endnotes, nil
	}

	endnotes := &Endnotes{}
	if err := p.readXML("word/endnotes.xml", endnotes); err != nil {
		// Endnotes file is optional
		return &Endnotes{}, nil
	}

	p.endnotes = endnotes
	return endnotes, nil
}

// GetHeaders returns all parsed headers
// Headers are stored in word/header1.xml, word/header2.xml, etc.
func (p *Parser) GetHeaders() (map[string]*Header, error) {
	if p.headers != nil {
		return p.headers, nil
	}

	p.headers = make(map[string]*Header)

	// Find all header files
	for name := range p.files {
		if strings.HasPrefix(name, "word/header") && strings.HasSuffix(name, ".xml") {
			header := &Header{}
			if err := p.readXML(name, header); err != nil {
				continue // Skip invalid headers
			}
			// Extract the header ID from filename (e.g., "header1" from "word/header1.xml")
			id := strings.TrimPrefix(name, "word/")
			id = strings.TrimSuffix(id, ".xml")
			p.headers[id] = header
		}
	}

	return p.headers, nil
}

// GetFooters returns all parsed footers
// Footers are stored in word/footer1.xml, word/footer2.xml, etc.
func (p *Parser) GetFooters() (map[string]*Footer, error) {
	if p.footers != nil {
		return p.footers, nil
	}

	p.footers = make(map[string]*Footer)

	// Find all footer files
	for name := range p.files {
		if strings.HasPrefix(name, "word/footer") && strings.HasSuffix(name, ".xml") {
			footer := &Footer{}
			if err := p.readXML(name, footer); err != nil {
				continue // Skip invalid footers
			}
			// Extract the footer ID from filename (e.g., "footer1" from "word/footer1.xml")
			id := strings.TrimPrefix(name, "word/")
			id = strings.TrimSuffix(id, ".xml")
			p.footers[id] = footer
		}
	}

	return p.footers, nil
}

// readXML reads and parses an XML file from the ZIP archive
func (p *Parser) readXML(filename string, v interface{}) error {
	f, ok := p.files[filename]
	if !ok {
		return fmt.Errorf("file not found: %s", filename)
	}

	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("opening %s: %w", filename, err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filename, err)
	}

	// Use custom decoder that handles Word XML namespaces
	if err := unmarshalWordXML(data, v); err != nil {
		return fmt.Errorf("parsing %s: %w", filename, err)
	}

	return nil
}

// ReadFile reads a raw file from the ZIP archive
func (p *Parser) ReadFile(filename string) ([]byte, error) {
	f, ok := p.files[filename]
	if !ok {
		return nil, fmt.Errorf("file not found: %s", filename)
	}

	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", filename, err)
	}
	defer rc.Close()

	return io.ReadAll(rc)
}

// ListFiles returns all file paths in the archive
func (p *Parser) ListFiles() []string {
	var files []string
	for name := range p.files {
		files = append(files, name)
	}
	return files
}

// unmarshalWordXML handles Word's XML with namespaces
func unmarshalWordXML(data []byte, v interface{}) error {
	// Word XML uses namespaces that need to be handled
	// The main namespace is "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	// We'll strip namespaces for simpler parsing

	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = false
	decoder.Entity = xml.HTMLEntity

	// Create a custom token reader that strips namespace prefixes
	return decodeWithNamespaceStripping(decoder, v)
}

// decodeWithNamespaceStripping decodes XML while handling namespaces
func decodeWithNamespaceStripping(decoder *xml.Decoder, v interface{}) error {
	// We need to handle Word XML namespaces properly
	// The main document elements are in the "w" namespace
	// We'll use a custom unmarshal approach

	var tokens []xml.Token
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Transform token to strip namespace prefixes from local names
		// CRITICAL: CharData and other byte-based tokens must be copied since
		// xml.Decoder reuses its internal buffer between Token() calls
		switch t := tok.(type) {
		case xml.StartElement:
			t.Name.Local = stripNamespacePrefix(t.Name.Local)
			t.Name.Space = ""
			for i := range t.Attr {
				t.Attr[i].Name.Local = stripNamespacePrefix(t.Attr[i].Name.Local)
				t.Attr[i].Name.Space = ""
			}
			tok = t
		case xml.EndElement:
			t.Name.Local = stripNamespacePrefix(t.Name.Local)
			t.Name.Space = ""
			tok = t
		case xml.CharData:
			tok = xml.CharData(append([]byte(nil), t...))
		case xml.Comment:
			tok = xml.Comment(append([]byte(nil), t...))
		case xml.ProcInst:
			t.Inst = append([]byte(nil), t.Inst...)
			tok = t
		case xml.Directive:
			tok = xml.Directive(append([]byte(nil), t...))
		}
		tokens = append(tokens, tok)
	}

	// Re-encode tokens to XML and decode into struct
	var buf bytes.Buffer
	encoder := xml.NewEncoder(&buf)
	for _, tok := range tokens {
		if err := encoder.EncodeToken(tok); err != nil {
			return err
		}
	}
	if err := encoder.Flush(); err != nil {
		return err
	}

	return xml.Unmarshal(buf.Bytes(), v)
}

// stripNamespacePrefix removes namespace prefix from element names
func stripNamespacePrefix(name string) string {
	if idx := strings.Index(name, ":"); idx != -1 {
		return name[idx+1:]
	}
	return name
}

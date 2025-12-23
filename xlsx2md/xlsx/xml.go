package xlsx

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
)

func unmarshalXLSXXML(data []byte, v interface{}) error {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = false
	decoder.Entity = xml.HTMLEntity

	return decodeWithNamespaceStripping(decoder, v)
}

func decodeWithNamespaceStripping(decoder *xml.Decoder, v interface{}) error {
	var tokens []xml.Token
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

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

func stripNamespacePrefix(name string) string {
	if idx := strings.Index(name, ":"); idx != -1 {
		return name[idx+1:]
	}
	return name
}

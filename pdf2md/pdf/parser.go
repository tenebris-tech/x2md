// Package pdf provides low-level PDF parsing functionality
package pdf

import (
	"bytes"
	"compress/lzw"
	"compress/zlib"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// Object represents a PDF object
type Object struct {
	Type    string
	Dict    map[string]interface{}
	Stream  []byte
	Array   []interface{}
	Number  float64
	String  string
	Boolean bool
	Ref     *Reference
	ObjNum  int // Object number (for decryption)
	GenNum  int // Generation number (for decryption)
}

// Reference represents a PDF object reference
type Reference struct {
	ObjectNum int
	GenNum    int
}

// ObjectStreamRef represents an object stored in an object stream
type ObjectStreamRef struct {
	StreamObjNum int // Object number of the containing object stream
	Index        int // Index within the object stream
}

// Parser parses PDF files
type Parser struct {
	data           []byte
	objects        map[int]*Object
	xref           map[int]int64
	objStreamRefs  map[int]*ObjectStreamRef // Objects stored in object streams
	trailer        map[string]interface{}
	parsedObjStms  map[int]map[int]*Object   // Cached parsed object streams
	encryption     *EncryptionHandler        // Encryption handler (nil if not encrypted)
}

// NewParser creates a new PDF parser
func NewParser(data []byte) *Parser {
	return &Parser{
		data:          data,
		objects:       make(map[int]*Object),
		xref:          make(map[int]int64),
		objStreamRefs: make(map[int]*ObjectStreamRef),
		parsedObjStms: make(map[int]map[int]*Object),
	}
}

// Parse parses the PDF document
func (p *Parser) Parse() error {
	// Find and parse xref table
	if err := p.parseXRef(); err != nil {
		return fmt.Errorf("parsing xref: %w", err)
	}

	// Initialize encryption handler if document is encrypted
	if err := p.initEncryption(); err != nil {
		return fmt.Errorf("initializing encryption: %w", err)
	}

	return nil
}

// initEncryption initializes encryption handling if the PDF is encrypted
func (p *Parser) initEncryption() error {
	if p.trailer == nil {
		return nil
	}

	encryptRef, hasEncrypt := p.trailer["Encrypt"]
	if !hasEncrypt {
		return nil // Not encrypted
	}

	// Get the Encrypt dictionary
	var encryptDict map[string]interface{}

	switch e := encryptRef.(type) {
	case *Reference:
		obj, err := p.getObjectDirect(e.ObjectNum)
		if err != nil {
			return fmt.Errorf("getting Encrypt object: %w", err)
		}
		encryptDict = obj.Dict
	case map[string]interface{}:
		encryptDict = e
	default:
		return fmt.Errorf("invalid Encrypt entry type")
	}

	if encryptDict == nil {
		return fmt.Errorf("empty Encrypt dictionary")
	}

	// Get document ID from trailer
	var docID []byte
	if idArray, ok := p.trailer["ID"].([]interface{}); ok && len(idArray) > 0 {
		if idStr, ok := idArray[0].(string); ok {
			docID = []byte(idStr)
		}
	}

	// Create encryption handler
	handler, err := NewEncryptionHandler(encryptDict, docID)
	if err != nil {
		return fmt.Errorf("creating encryption handler: %w", err)
	}

	// Try authenticating with empty password
	if !handler.TryEmptyPassword() {
		// PDF requires a password - we can't handle this
		return nil // Leave encryption as nil, IsEncrypted will return true
	}

	// Successfully authenticated - store handler for decryption
	p.encryption = handler
	return nil
}

// getObjectDirect retrieves an object without decryption (for bootstrapping)
func (p *Parser) getObjectDirect(objNum int) (*Object, error) {
	if obj, ok := p.objects[objNum]; ok {
		return obj, nil
	}

	if offset, ok := p.xref[objNum]; ok {
		obj, _, err := p.parseObjectAt(int(offset))
		if err != nil {
			return nil, err
		}
		return obj, nil
	}

	return nil, fmt.Errorf("object %d not found", objNum)
}

// parseXRef finds and parses the xref table
func (p *Parser) parseXRef() error {
	// Find the LAST startxref in the file (PDF spec requires reading from end)
	// PDFs can have multiple xref sections (linearized PDFs, incremental updates)
	startxrefRe := regexp.MustCompile(`startxref\s+(\d+)`)
	allMatches := startxrefRe.FindAllSubmatch(p.data, -1)
	if len(allMatches) == 0 {
		return fmt.Errorf("startxref not found")
	}

	// Use the last startxref entry
	matches := allMatches[len(allMatches)-1]
	xrefOffset, _ := strconv.ParseInt(string(matches[1]), 10, 64)

	// Check if it's an xref table or xref stream
	pos := int(xrefOffset)
	if pos >= len(p.data) {
		return fmt.Errorf("invalid xref offset")
	}

	// Skip whitespace
	for pos < len(p.data) && (p.data[pos] == ' ' || p.data[pos] == '\n' || p.data[pos] == '\r') {
		pos++
	}

	if pos+4 <= len(p.data) && string(p.data[pos:pos+4]) == "xref" {
		return p.parseXRefTable(pos)
	}

	// It's an xref stream (PDF 1.5+)
	return p.parseXRefStream(pos)
}

// parseXRefTable parses a traditional xref table
func (p *Parser) parseXRefTable(pos int) error {
	// Skip "xref" and newline
	pos += 4
	for pos < len(p.data) && (p.data[pos] == ' ' || p.data[pos] == '\n' || p.data[pos] == '\r') {
		pos++
	}

	// Parse xref sections
	for pos < len(p.data) {
		// Check for trailer
		if pos+7 <= len(p.data) && string(p.data[pos:pos+7]) == "trailer" {
			pos += 7
			break
		}

		// Parse section header: startobj count
		line, newPos := p.readLine(pos)
		pos = newPos

		parts := strings.Fields(line)
		if len(parts) < 2 {
			break
		}

		startObj, _ := strconv.Atoi(parts[0])
		count, _ := strconv.Atoi(parts[1])

		// Parse entries
		for i := 0; i < count; i++ {
			entryLine, newPos := p.readLine(pos)
			pos = newPos

			entryParts := strings.Fields(entryLine)
			if len(entryParts) >= 3 && entryParts[2] == "n" {
				offset, _ := strconv.ParseInt(entryParts[0], 10, 64)
				p.xref[startObj+i] = offset
			}
		}
	}

	// Parse trailer dictionary
	currentTrailer := p.parseDictAt(pos)

	// If this is the first trailer we've seen, save it
	// Otherwise, just use it to get the Prev pointer
	if p.trailer == nil {
		p.trailer = currentTrailer
	}

	// Follow Prev if present (for incremental updates and linearized PDFs)
	if prev, ok := currentTrailer["Prev"].(float64); ok {
		prevPos := int(prev)
		// Skip whitespace at prev position
		for prevPos < len(p.data) && (p.data[prevPos] == ' ' || p.data[prevPos] == '\n' || p.data[prevPos] == '\r') {
			prevPos++
		}
		// Parse the previous xref (could be table or stream)
		if prevPos+4 <= len(p.data) && string(p.data[prevPos:prevPos+4]) == "xref" {
			return p.parseXRefTable(prevPos)
		} else {
			return p.parseXRefStream(prevPos)
		}
	}

	return nil
}

// parseXRefStream parses an xref stream (PDF 1.5+)
func (p *Parser) parseXRefStream(pos int) error {
	// Parse the xref stream object
	obj, _, err := p.parseObjectAt(pos)
	if err != nil {
		return err
	}

	if obj.Dict == nil {
		return fmt.Errorf("xref stream has no dictionary")
	}

	// If this is the first trailer we've seen, save it
	if p.trailer == nil {
		p.trailer = obj.Dict
	}

	// Decode the xref stream
	stream := obj.Stream
	if filter, ok := obj.Dict["Filter"]; ok {
		if filterName, ok := filter.(string); ok && filterName == "/FlateDecode" {
			// Check for predictor in DecodeParms
			predictor := 1
			columns := 0
			if decodeParms, ok := obj.Dict["DecodeParms"].(map[string]interface{}); ok {
				if pred, ok := decodeParms["Predictor"].(float64); ok {
					predictor = int(pred)
				}
				if cols, ok := decodeParms["Columns"].(float64); ok {
					columns = int(cols)
				}
			}

			if predictor > 1 && columns > 0 {
				decoded, err := p.decodeFlateDecodeWithPredictor(stream, predictor, columns)
				if err != nil {
					return fmt.Errorf("decoding xref stream with predictor: %w", err)
				}
				stream = decoded
			} else {
				decoded, err := p.decodeFlateDecode(stream)
				if err != nil {
					return fmt.Errorf("decoding xref stream: %w", err)
				}
				stream = decoded
			}
		}
	}

	// Parse W array (field widths)
	wArray, ok := obj.Dict["W"].([]interface{})
	if !ok || len(wArray) < 3 {
		return fmt.Errorf("invalid W array in xref stream")
	}

	w := make([]int, 3)
	for i := 0; i < 3; i++ {
		if num, ok := wArray[i].(float64); ok {
			w[i] = int(num)
		}
	}

	entrySize := w[0] + w[1] + w[2]
	if entrySize == 0 {
		return fmt.Errorf("invalid entry size in xref stream")
	}

	// Parse Index array
	var indexes []int
	if indexArray, ok := obj.Dict["Index"].([]interface{}); ok {
		for _, v := range indexArray {
			if num, ok := v.(float64); ok {
				indexes = append(indexes, int(num))
			}
		}
	} else if size, ok := obj.Dict["Size"].(float64); ok {
		indexes = []int{0, int(size)}
	}

	// Parse entries
	offset := 0
	entriesAdded := 0
	for i := 0; i < len(indexes); i += 2 {
		if i+1 >= len(indexes) {
			break // Malformed index array
		}
		startObj := indexes[i]
		count := indexes[i+1]

		for j := 0; j < count; j++ {
			if offset+entrySize > len(stream) {
				break
			}

			// Read entry fields
			var fields [3]int64
			fieldOffset := offset
			for f := 0; f < 3; f++ {
				var val int64
				for k := 0; k < w[f]; k++ {
					val = (val << 8) | int64(stream[fieldOffset])
					fieldOffset++
				}
				fields[f] = val
			}

			objNum := startObj + j
			objType := fields[0]
			if w[0] == 0 {
				objType = 1 // Default type
			}

			if objType == 1 {
				// Regular object - store file offset
				p.xref[objNum] = fields[1]
				entriesAdded++
			} else if objType == 2 {
				// Object in object stream - store stream reference
				p.objStreamRefs[objNum] = &ObjectStreamRef{
					StreamObjNum: int(fields[1]),
					Index:        int(fields[2]),
				}
				entriesAdded++
			}
			// Type 0 = free object, skip

			offset += entrySize
		}
	}

	// Follow Prev if present
	if prev, ok := obj.Dict["Prev"].(float64); ok {
		prevPos := int(prev)
		// Skip whitespace at prev position
		for prevPos < len(p.data) && (p.data[prevPos] == ' ' || p.data[prevPos] == '\n' || p.data[prevPos] == '\r') {
			prevPos++
		}
		// Parse the previous xref (could be table or stream)
		if prevPos+4 <= len(p.data) && string(p.data[prevPos:prevPos+4]) == "xref" {
			return p.parseXRefTable(prevPos)
		}
		return p.parseXRefStream(prevPos)
	}

	return nil
}

// readLine reads a line from the data
func (p *Parser) readLine(pos int) (string, int) {
	start := pos
	for pos < len(p.data) && p.data[pos] != '\n' && p.data[pos] != '\r' {
		pos++
	}
	line := string(p.data[start:pos])
	// Skip newline characters
	for pos < len(p.data) && (p.data[pos] == '\n' || p.data[pos] == '\r') {
		pos++
	}
	return line, pos
}

// GetObject retrieves an object by number
func (p *Parser) GetObject(objNum int) (*Object, error) {
	if obj, ok := p.objects[objNum]; ok {
		return obj, nil
	}

	// Check if object is in regular xref
	if offset, ok := p.xref[objNum]; ok {
		obj, _, err := p.parseObjectAt(int(offset))
		if err != nil {
			return nil, err
		}
		p.objects[objNum] = obj
		return obj, nil
	}

	// Check if object is in an object stream
	if ref, ok := p.objStreamRefs[objNum]; ok {
		return p.getObjectFromStream(objNum, ref)
	}

	return nil, fmt.Errorf("object %d not found in xref", objNum)
}

// getObjectFromStream retrieves an object from an object stream
func (p *Parser) getObjectFromStream(objNum int, ref *ObjectStreamRef) (*Object, error) {
	// Check if we've already parsed this object stream
	if streamObjs, ok := p.parsedObjStms[ref.StreamObjNum]; ok {
		if obj, ok := streamObjs[objNum]; ok {
			return obj, nil
		}
		return nil, fmt.Errorf("object %d not found in object stream %d", objNum, ref.StreamObjNum)
	}

	// Get the object stream
	streamObj, err := p.GetObject(ref.StreamObjNum)
	if err != nil {
		return nil, fmt.Errorf("getting object stream %d: %w", ref.StreamObjNum, err)
	}

	// Parse the object stream
	if err := p.parseObjectStream(ref.StreamObjNum, streamObj); err != nil {
		return nil, fmt.Errorf("parsing object stream %d: %w", ref.StreamObjNum, err)
	}

	// Now get the object from the parsed stream
	if streamObjs, ok := p.parsedObjStms[ref.StreamObjNum]; ok {
		if obj, ok := streamObjs[objNum]; ok {
			return obj, nil
		}
	}

	return nil, fmt.Errorf("object %d not found in object stream %d", objNum, ref.StreamObjNum)
}

// parseObjectStream parses an object stream and caches all objects within it
func (p *Parser) parseObjectStream(streamObjNum int, streamObj *Object) error {
	if streamObj.Dict == nil {
		return fmt.Errorf("object stream has no dictionary")
	}

	// Get N (number of objects) and First (offset to first object)
	n, ok := streamObj.Dict["N"].(float64)
	if !ok {
		return fmt.Errorf("object stream missing N")
	}
	first, ok := streamObj.Dict["First"].(float64)
	if !ok {
		return fmt.Errorf("object stream missing First")
	}

	// Decode the stream - must decrypt first if encryption is active
	stream := streamObj.Stream

	// Decrypt if encryption is active
	if p.encryption != nil && p.encryption.IsAuthenticated() {
		decrypted, err := p.encryption.DecryptStream(stream, streamObj.ObjNum, streamObj.GenNum)
		if err == nil {
			stream = decrypted
		}
	}

	if filter, ok := streamObj.Dict["Filter"]; ok {
		if filterName, ok := filter.(string); ok && filterName == "/FlateDecode" {
			// Check for predictor in DecodeParms
			predictor := 1
			columns := 0
			if decodeParms, ok := streamObj.Dict["DecodeParms"].(map[string]interface{}); ok {
				if pred, ok := decodeParms["Predictor"].(float64); ok {
					predictor = int(pred)
				}
				if cols, ok := decodeParms["Columns"].(float64); ok {
					columns = int(cols)
				}
			}

			var decoded []byte
			var err error
			if predictor > 1 && columns > 0 {
				decoded, err = p.decodeFlateDecodeWithPredictor(stream, predictor, columns)
			} else {
				decoded, err = p.decodeFlateDecode(stream)
			}
			if err != nil {
				return fmt.Errorf("decoding object stream: %w", err)
			}
			stream = decoded
		}
	}

	// Parse the index section (pairs of objNum, offset)
	numObjs := int(n)
	firstOffset := int(first)
	indexData := string(stream[:firstOffset])

	// Parse object numbers and offsets
	type objEntry struct {
		objNum int
		offset int
	}
	entries := make([]objEntry, 0, numObjs)

	pos := 0
	for i := 0; i < numObjs; i++ {
		// Skip whitespace
		for pos < len(indexData) && (indexData[pos] == ' ' || indexData[pos] == '\n' || indexData[pos] == '\r' || indexData[pos] == '\t') {
			pos++
		}
		// Read object number
		start := pos
		for pos < len(indexData) && indexData[pos] >= '0' && indexData[pos] <= '9' {
			pos++
		}
		if start == pos {
			break
		}
		objNum, _ := strconv.Atoi(indexData[start:pos])

		// Skip whitespace
		for pos < len(indexData) && (indexData[pos] == ' ' || indexData[pos] == '\n' || indexData[pos] == '\r' || indexData[pos] == '\t') {
			pos++
		}
		// Read offset
		start = pos
		for pos < len(indexData) && indexData[pos] >= '0' && indexData[pos] <= '9' {
			pos++
		}
		if start == pos {
			break
		}
		offset, _ := strconv.Atoi(indexData[start:pos])

		entries = append(entries, objEntry{objNum: objNum, offset: offset})
	}

	// Parse each object
	objectData := stream[firstOffset:]
	streamObjs := make(map[int]*Object)

	for i, entry := range entries {
		// Calculate end offset
		endOffset := len(objectData)
		if i+1 < len(entries) {
			endOffset = entries[i+1].offset
		}

		// Parse the object value
		objData := objectData[entry.offset:endOffset]
		obj := &Object{}
		p.parseObjectValue(objData, obj)

		streamObjs[entry.objNum] = obj
		p.objects[entry.objNum] = obj
	}

	p.parsedObjStms[streamObjNum] = streamObjs
	return nil
}

// parseObjectValue parses an object value from bytes
func (p *Parser) parseObjectValue(data []byte, obj *Object) {
	pos := 0
	// Skip whitespace
	for pos < len(data) && (data[pos] == ' ' || data[pos] == '\n' || data[pos] == '\r' || data[pos] == '\t') {
		pos++
	}

	if pos >= len(data) {
		return
	}

	// Temporarily replace p.data to parse
	origData := p.data
	p.data = data

	value, _ := p.parseValue(pos)

	p.data = origData

	// Set object based on value type
	switch v := value.(type) {
	case map[string]interface{}:
		obj.Type = "dict"
		obj.Dict = v
	case []interface{}:
		obj.Type = "array"
		obj.Array = v
	case float64:
		obj.Type = "number"
		obj.Number = v
	case string:
		obj.Type = "string"
		obj.String = v
	case bool:
		obj.Type = "boolean"
		obj.Boolean = v
	case *Reference:
		obj.Type = "ref"
		obj.Ref = v
	}
}

// parseObjectAt parses an object at the given offset
func (p *Parser) parseObjectAt(pos int) (*Object, int, error) {
	// Skip whitespace
	for pos < len(p.data) && (p.data[pos] == ' ' || p.data[pos] == '\n' || p.data[pos] == '\r' || p.data[pos] == '\t') {
		pos++
	}

	// Parse object header: objnum gennum obj
	objNumStr, pos := p.readToken(pos)
	genNumStr, pos := p.readToken(pos)
	objKeyword, pos := p.readToken(pos)

	if objKeyword != "obj" {
		return nil, pos, fmt.Errorf("expected 'obj' keyword, got '%s'", objKeyword)
	}

	objNum, _ := strconv.Atoi(objNumStr)
	genNum, _ := strconv.Atoi(genNumStr)

	// Parse object content
	obj := &Object{
		ObjNum: objNum,
		GenNum: genNum,
	}
	pos = p.skipWhitespace(pos)

	// Determine object type and parse
	if pos < len(p.data) {
		switch p.data[pos] {
		case '<':
			if pos+1 < len(p.data) && p.data[pos+1] == '<' {
				// Dictionary
				obj.Type = "dict"
				obj.Dict, pos = p.parseDictionary(pos)
			} else {
				// Hex string
				obj.Type = "string"
				obj.String, pos = p.parseHexString(pos)
			}
		case '[':
			// Array
			obj.Type = "array"
			obj.Array, pos = p.parseArray(pos)
		case '(':
			// Literal string
			obj.Type = "string"
			obj.String, pos = p.parseLiteralString(pos)
		case '/':
			// Name
			obj.Type = "name"
			obj.String, pos = p.parseName(pos)
		default:
			if p.data[pos] >= '0' && p.data[pos] <= '9' || p.data[pos] == '-' || p.data[pos] == '+' || p.data[pos] == '.' {
				obj.Type = "number"
				obj.Number, pos = p.parseNumber(pos)
			} else {
				token, newPos := p.readToken(pos)
				pos = newPos
				if token == "true" {
					obj.Type = "boolean"
					obj.Boolean = true
				} else if token == "false" {
					obj.Type = "boolean"
					obj.Boolean = false
				} else if token == "null" {
					obj.Type = "null"
				}
			}
		}
	}

	// Check for stream
	pos = p.skipWhitespace(pos)
	if pos+6 <= len(p.data) && string(p.data[pos:pos+6]) == "stream" {
		pos += 6
		// Skip single newline (CR, LF, or CRLF)
		if pos < len(p.data) && p.data[pos] == '\r' {
			pos++
		}
		if pos < len(p.data) && p.data[pos] == '\n' {
			pos++
		}

		// Get stream length
		var streamLen int
		if obj.Dict != nil {
			if lenVal, ok := obj.Dict["Length"]; ok {
				switch v := lenVal.(type) {
				case float64:
					streamLen = int(v)
				case *Reference:
					lenObj, err := p.GetObject(v.ObjectNum)
					if err == nil && lenObj.Type == "number" {
						streamLen = int(lenObj.Number)
					}
				}
			}
		}

		if streamLen > 0 && pos+streamLen <= len(p.data) {
			obj.Stream = p.data[pos : pos+streamLen]
			pos += streamLen
		} else {
			// Find endstream
			endstreamPos := bytes.Index(p.data[pos:], []byte("endstream"))
			if endstreamPos != -1 {
				obj.Stream = bytes.TrimRight(p.data[pos:pos+endstreamPos], "\r\n")
				pos += endstreamPos
			}
		}
	}

	p.objects[objNum] = obj
	return obj, pos, nil
}

// parseDictAt parses a dictionary at the given position
func (p *Parser) parseDictAt(pos int) map[string]interface{} {
	pos = p.skipWhitespace(pos)
	dict, _ := p.parseDictionary(pos)
	return dict
}

// parseDictionary parses a PDF dictionary
func (p *Parser) parseDictionary(pos int) (map[string]interface{}, int) {
	dict := make(map[string]interface{})

	if pos+2 > len(p.data) || p.data[pos] != '<' || p.data[pos+1] != '<' {
		return dict, pos
	}
	pos += 2

	for pos < len(p.data) {
		pos = p.skipWhitespace(pos)

		// Check for end of dictionary
		if pos+2 <= len(p.data) && p.data[pos] == '>' && p.data[pos+1] == '>' {
			return dict, pos + 2
		}

		// Parse key (name)
		if pos >= len(p.data) || p.data[pos] != '/' {
			break
		}
		key, newPos := p.parseName(pos)
		pos = newPos

		// Strip leading "/" from key for easier access
		if strings.HasPrefix(key, "/") {
			key = key[1:]
		}

		// Parse value
		pos = p.skipWhitespace(pos)
		value, newPos := p.parseValue(pos)
		pos = newPos

		dict[key] = value
	}

	return dict, pos
}

// parseArray parses a PDF array
func (p *Parser) parseArray(pos int) ([]interface{}, int) {
	var arr []interface{}

	if pos >= len(p.data) || p.data[pos] != '[' {
		return arr, pos
	}
	pos++

	for pos < len(p.data) {
		pos = p.skipWhitespace(pos)

		if p.data[pos] == ']' {
			return arr, pos + 1
		}

		value, newPos := p.parseValue(pos)
		pos = newPos
		arr = append(arr, value)
	}

	return arr, pos
}

// parseValue parses a PDF value
func (p *Parser) parseValue(pos int) (interface{}, int) {
	pos = p.skipWhitespace(pos)

	if pos >= len(p.data) {
		return nil, pos
	}

	switch p.data[pos] {
	case '<':
		if pos+1 < len(p.data) && p.data[pos+1] == '<' {
			return p.parseDictionary(pos)
		}
		return p.parseHexString(pos)
	case '[':
		return p.parseArray(pos)
	case '(':
		return p.parseLiteralString(pos)
	case '/':
		return p.parseName(pos)
	case 't':
		if pos+4 <= len(p.data) && string(p.data[pos:pos+4]) == "true" {
			return true, pos + 4
		}
	case 'f':
		if pos+5 <= len(p.data) && string(p.data[pos:pos+5]) == "false" {
			return false, pos + 5
		}
	case 'n':
		if pos+4 <= len(p.data) && string(p.data[pos:pos+4]) == "null" {
			return nil, pos + 4
		}
	}

	// Number or reference
	if (p.data[pos] >= '0' && p.data[pos] <= '9') || p.data[pos] == '-' || p.data[pos] == '+' || p.data[pos] == '.' {
		// Check if it's a reference (num gen R)
		startPos := pos
		num1, pos := p.parseNumber(pos)
		pos = p.skipWhitespace(pos)

		if pos < len(p.data) && p.data[pos] >= '0' && p.data[pos] <= '9' {
			num2, tempPos := p.parseNumber(pos)
			tempPos = p.skipWhitespace(tempPos)
			if tempPos < len(p.data) && p.data[tempPos] == 'R' {
				return &Reference{ObjectNum: int(num1), GenNum: int(num2)}, tempPos + 1
			}
		}

		// Just a number
		return p.parseNumber(startPos)
	}

	return nil, pos
}

// parseName parses a PDF name object
func (p *Parser) parseName(pos int) (string, int) {
	if pos >= len(p.data) || p.data[pos] != '/' {
		return "", pos
	}

	start := pos
	pos++

	for pos < len(p.data) {
		c := p.data[pos]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '/' ||
			c == '[' || c == ']' || c == '<' || c == '>' || c == '(' || c == ')' {
			break
		}
		pos++
	}

	return string(p.data[start:pos]), pos
}

// parseNumber parses a PDF number
func (p *Parser) parseNumber(pos int) (float64, int) {
	start := pos

	if pos < len(p.data) && (p.data[pos] == '-' || p.data[pos] == '+') {
		pos++
	}

	for pos < len(p.data) && ((p.data[pos] >= '0' && p.data[pos] <= '9') || p.data[pos] == '.') {
		pos++
	}

	num, _ := strconv.ParseFloat(string(p.data[start:pos]), 64)
	return num, pos
}

// parseHexString parses a hex string
func (p *Parser) parseHexString(pos int) (string, int) {
	if pos >= len(p.data) || p.data[pos] != '<' {
		return "", pos
	}
	pos++

	var hexBytes []byte
	for pos < len(p.data) && p.data[pos] != '>' {
		if p.data[pos] != ' ' && p.data[pos] != '\n' && p.data[pos] != '\r' && p.data[pos] != '\t' {
			hexBytes = append(hexBytes, p.data[pos])
		}
		pos++
	}

	if pos < len(p.data) {
		pos++ // Skip '>'
	}

	// Convert hex to bytes
	if len(hexBytes)%2 != 0 {
		hexBytes = append(hexBytes, '0')
	}

	var result []byte
	for i := 0; i < len(hexBytes); i += 2 {
		b, _ := strconv.ParseUint(string(hexBytes[i:i+2]), 16, 8)
		result = append(result, byte(b))
	}

	return string(result), pos
}

// parseLiteralString parses a literal string
func (p *Parser) parseLiteralString(pos int) (string, int) {
	if pos >= len(p.data) || p.data[pos] != '(' {
		return "", pos
	}
	pos++

	var result []byte
	depth := 1

	for pos < len(p.data) && depth > 0 {
		c := p.data[pos]
		if c == '\\' && pos+1 < len(p.data) {
			pos++
			escaped := p.data[pos]
			switch escaped {
			case 'n':
				result = append(result, '\n')
			case 'r':
				result = append(result, '\r')
			case 't':
				result = append(result, '\t')
			case 'b':
				result = append(result, '\b')
			case 'f':
				result = append(result, '\f')
			case '(':
				result = append(result, '(')
			case ')':
				result = append(result, ')')
			case '\\':
				result = append(result, '\\')
			default:
				// Octal or line continuation
				if escaped >= '0' && escaped <= '7' {
					octal := string(escaped)
					for i := 0; i < 2 && pos+1 < len(p.data) && p.data[pos+1] >= '0' && p.data[pos+1] <= '7'; i++ {
						pos++
						octal += string(p.data[pos])
					}
					val, _ := strconv.ParseUint(octal, 8, 8)
					result = append(result, byte(val))
				} else if escaped == '\n' || escaped == '\r' {
					// Line continuation - skip
					if escaped == '\r' && pos+1 < len(p.data) && p.data[pos+1] == '\n' {
						pos++
					}
				} else {
					result = append(result, escaped)
				}
			}
		} else if c == '(' {
			depth++
			result = append(result, c)
		} else if c == ')' {
			depth--
			if depth > 0 {
				result = append(result, c)
			}
		} else {
			result = append(result, c)
		}
		pos++
	}

	return string(result), pos
}

// readToken reads a token (word) from the data
func (p *Parser) readToken(pos int) (string, int) {
	pos = p.skipWhitespace(pos)
	start := pos

	for pos < len(p.data) {
		c := p.data[pos]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' ||
			c == '/' || c == '[' || c == ']' || c == '<' || c == '>' ||
			c == '(' || c == ')' || c == '{' || c == '}' {
			break
		}
		pos++
	}

	return string(p.data[start:pos]), pos
}

// skipWhitespace skips whitespace and comments
func (p *Parser) skipWhitespace(pos int) int {
	for pos < len(p.data) {
		c := p.data[pos]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\x00' {
			pos++
		} else if c == '%' {
			// Skip comment
			for pos < len(p.data) && p.data[pos] != '\n' && p.data[pos] != '\r' {
				pos++
			}
		} else {
			break
		}
	}
	return pos
}

// decodeFlateDecode decodes FlateDecode compressed data
func (p *Parser) decodeFlateDecode(data []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()

	return io.ReadAll(r)
}

// decodeFlateDecodeWithPredictor decodes FlateDecode with PNG predictor
func (p *Parser) decodeFlateDecodeWithPredictor(data []byte, predictor, columns int) ([]byte, error) {
	// First decompress
	decompressed, err := p.decodeFlateDecode(data)
	if err != nil {
		return nil, err
	}

	// If no predictor or predictor 1 (no prediction), return as-is
	if predictor <= 1 {
		return decompressed, nil
	}

	// PNG predictors (10-15)
	if predictor >= 10 && predictor <= 15 {
		return p.decodePNGPredictor(decompressed, columns)
	}

	// TIFF predictor (2) - not implemented for xref streams
	return decompressed, nil
}

// decodePNGPredictor decodes PNG predictor filtered data
func (p *Parser) decodePNGPredictor(data []byte, columns int) ([]byte, error) {
	if columns <= 0 {
		return nil, fmt.Errorf("invalid columns value: %d", columns)
	}
	if len(data) == 0 {
		return nil, nil
	}
	rowSize := columns + 1 // +1 for the filter byte
	if len(data)%rowSize != 0 {
		// Try without the extra filter byte per row
		if len(data)%columns == 0 {
			return data, nil
		}
		return nil, fmt.Errorf("invalid predictor data size: %d not divisible by row size %d", len(data), rowSize)
	}

	numRows := len(data) / rowSize
	result := make([]byte, numRows*columns)
	prevRow := make([]byte, columns)

	for row := 0; row < numRows; row++ {
		srcOffset := row * rowSize
		dstOffset := row * columns
		filterType := data[srcOffset]
		rowData := data[srcOffset+1 : srcOffset+rowSize]

		switch filterType {
		case 0: // None
			copy(result[dstOffset:], rowData)
		case 1: // Sub
			for i := 0; i < columns; i++ {
				left := byte(0)
				if i > 0 {
					left = result[dstOffset+i-1]
				}
				result[dstOffset+i] = rowData[i] + left
			}
		case 2: // Up
			for i := 0; i < columns; i++ {
				result[dstOffset+i] = rowData[i] + prevRow[i]
			}
		case 3: // Average
			for i := 0; i < columns; i++ {
				left := byte(0)
				if i > 0 {
					left = result[dstOffset+i-1]
				}
				result[dstOffset+i] = rowData[i] + byte((int(left)+int(prevRow[i]))/2)
			}
		case 4: // Paeth
			for i := 0; i < columns; i++ {
				left := byte(0)
				if i > 0 {
					left = result[dstOffset+i-1]
				}
				up := prevRow[i]
				upLeft := byte(0)
				if i > 0 {
					upLeft = prevRow[i-1]
				}
				result[dstOffset+i] = rowData[i] + paethPredictor(left, up, upLeft)
			}
		default:
			// Unknown filter, just copy
			copy(result[dstOffset:], rowData)
		}

		copy(prevRow, result[dstOffset:dstOffset+columns])
	}

	return result, nil
}

// paethPredictor implements the Paeth predictor algorithm
func paethPredictor(a, b, c byte) byte {
	p := int(a) + int(b) - int(c)
	pa := abs(p - int(a))
	pb := abs(p - int(b))
	pc := abs(p - int(c))
	if pa <= pb && pa <= pc {
		return a
	} else if pb <= pc {
		return b
	}
	return c
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// GetTrailer returns the trailer dictionary (for debugging)
func (p *Parser) GetTrailer() map[string]interface{} {
	return p.trailer
}

// IsEncrypted returns true if the PDF requires a password to read.
// PDFs with only permission restrictions (owner password, no user password)
// return false because we can decrypt them with an empty password.
func (p *Parser) IsEncrypted() bool {
	if p.trailer == nil {
		return false
	}

	_, hasEncrypt := p.trailer["Encrypt"]
	if !hasEncrypt {
		return false // No encryption at all
	}

	// If we have an encryption handler, we successfully authenticated
	// (with empty password), so the PDF is readable
	if p.encryption != nil && p.encryption.IsAuthenticated() {
		return false
	}

	// Has Encrypt but couldn't authenticate - truly encrypted
	return true
}

// GetPageCount returns the number of pages in the document
func (p *Parser) GetPageCount() (int, error) {
	if p.trailer == nil {
		return 0, fmt.Errorf("PDF not parsed: trailer is nil")
	}
	rootRef, ok := p.trailer["Root"].(*Reference)
	if !ok {
		return 0, fmt.Errorf("no Root in trailer")
	}

	root, err := p.GetObject(rootRef.ObjectNum)
	if err != nil {
		return 0, err
	}

	pagesRef, ok := root.Dict["Pages"].(*Reference)
	if !ok {
		return 0, fmt.Errorf("no Pages in root")
	}

	pages, err := p.GetObject(pagesRef.ObjectNum)
	if err != nil {
		return 0, err
	}

	if count, ok := pages.Dict["Count"].(float64); ok {
		return int(count), nil
	}

	return 0, fmt.Errorf("no Count in pages")
}

// GetPage retrieves a page object by index (0-based)
func (p *Parser) GetPage(index int) (*Object, error) {
	if p.trailer == nil {
		return nil, fmt.Errorf("PDF not parsed: trailer is nil")
	}
	rootRef, ok := p.trailer["Root"].(*Reference)
	if !ok {
		return nil, fmt.Errorf("no Root in trailer")
	}

	root, err := p.GetObject(rootRef.ObjectNum)
	if err != nil {
		return nil, err
	}

	pagesRef, ok := root.Dict["Pages"].(*Reference)
	if !ok {
		return nil, fmt.Errorf("no Pages in root")
	}

	pages, err := p.GetObject(pagesRef.ObjectNum)
	if err != nil {
		return nil, err
	}

	return p.findPage(pages, index, 0, 0)
}

// maxPageTreeDepth is the maximum recursion depth for page tree traversal
const maxPageTreeDepth = 100

// findPage recursively finds a page in the page tree
func (p *Parser) findPage(node *Object, targetIndex, currentIndex, depth int) (*Object, error) {
	if depth > maxPageTreeDepth {
		return nil, fmt.Errorf("page tree exceeds maximum depth of %d (possible circular reference)", maxPageTreeDepth)
	}
	nodeType, _ := node.Dict["Type"].(string)

	if nodeType == "/Page" {
		if currentIndex == targetIndex {
			return node, nil
		}
		return nil, nil
	}

	// It's a Pages node
	kids, ok := node.Dict["Kids"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("no Kids in Pages node")
	}

	for _, kidRef := range kids {
		ref, ok := kidRef.(*Reference)
		if !ok {
			continue
		}

		kid, err := p.GetObject(ref.ObjectNum)
		if err != nil {
			continue
		}

		kidType, _ := kid.Dict["Type"].(string)

		if kidType == "/Page" {
			if currentIndex == targetIndex {
				return kid, nil
			}
			currentIndex++
		} else if kidType == "/Pages" {
			count := 0
			if c, ok := kid.Dict["Count"].(float64); ok {
				count = int(c)
			}

			if targetIndex >= currentIndex && targetIndex < currentIndex+count {
				return p.findPage(kid, targetIndex, currentIndex, depth+1)
			}
			currentIndex += count
		}
	}

	return nil, fmt.Errorf("page %d not found", targetIndex)
}

// DecodeStream decodes a stream based on its filters
func (p *Parser) DecodeStream(obj *Object) ([]byte, error) {
	if obj.Stream == nil {
		return nil, nil
	}

	data := obj.Stream

	// Decrypt if encryption is active (skip Crypt filter handling)
	if p.encryption != nil && p.encryption.IsAuthenticated() {
		// Check if stream uses identity crypt filter
		if !p.streamUsesIdentityCrypt(obj) {
			decrypted, err := p.encryption.DecryptStream(data, obj.ObjNum, obj.GenNum)
			if err != nil {
				// If decryption fails, try with original data
				// (might be unencrypted metadata or XRef stream)
			} else {
				data = decrypted
			}
		}
	}

	filter := obj.Dict["Filter"]
	if filter == nil {
		return data, nil
	}

	// Handle single filter or array of filters
	var filters []string
	switch f := filter.(type) {
	case string:
		filters = []string{f}
	case []interface{}:
		for _, item := range f {
			if s, ok := item.(string); ok {
				filters = append(filters, s)
			}
		}
	}

	for _, f := range filters {
		var err error
		switch f {
		case "/FlateDecode":
			data, err = p.decodeFlateDecode(data)
		case "/ASCII85Decode":
			data, err = p.decodeASCII85(data)
		case "/ASCIIHexDecode":
			data, err = p.decodeASCIIHex(data)
		case "/LZWDecode":
			data, err = p.decodeLZW(data)
		case "/Crypt":
			// Already handled above or identity filter
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("decoding %s: %w", f, err)
		}
	}

	return data, nil
}

// streamUsesIdentityCrypt checks if a stream uses the Identity crypt filter
func (p *Parser) streamUsesIdentityCrypt(obj *Object) bool {
	if obj.Dict == nil {
		return false
	}

	// Check DecodeParms for Crypt filter with Identity
	decodeParms := obj.Dict["DecodeParms"]
	if decodeParms == nil {
		return false
	}

	var parms map[string]interface{}
	switch dp := decodeParms.(type) {
	case map[string]interface{}:
		parms = dp
	case []interface{}:
		// Array of decode params - check each
		for _, item := range dp {
			if m, ok := item.(map[string]interface{}); ok {
				if name, ok := m["Name"].(string); ok && name == "/Identity" {
					return true
				}
			}
		}
		return false
	default:
		return false
	}

	if name, ok := parms["Name"].(string); ok {
		return name == "/Identity"
	}
	return false
}

// decodeASCII85 decodes ASCII85 encoded data
func (p *Parser) decodeASCII85(data []byte) ([]byte, error) {
	// Remove whitespace
	var cleaned []byte
	for _, b := range data {
		if b != ' ' && b != '\t' && b != '\n' && b != '\r' {
			cleaned = append(cleaned, b)
		}
	}

	// Remove ~> trailer if present
	if len(cleaned) >= 2 && cleaned[len(cleaned)-2] == '~' && cleaned[len(cleaned)-1] == '>' {
		cleaned = cleaned[:len(cleaned)-2]
	}

	var result []byte
	var group [5]byte
	groupLen := 0

	for i := 0; i < len(cleaned); i++ {
		c := cleaned[i]

		if c == 'z' {
			if groupLen != 0 {
				return nil, fmt.Errorf("z not at group boundary")
			}
			result = append(result, 0, 0, 0, 0)
			continue
		}

		if c < '!' || c > 'u' {
			continue
		}

		group[groupLen] = c - '!'
		groupLen++

		if groupLen == 5 {
			val := uint32(group[0])*85*85*85*85 +
				uint32(group[1])*85*85*85 +
				uint32(group[2])*85*85 +
				uint32(group[3])*85 +
				uint32(group[4])

			result = append(result,
				byte(val>>24),
				byte(val>>16),
				byte(val>>8),
				byte(val))
			groupLen = 0
		}
	}

	// Handle remaining bytes
	if groupLen > 0 {
		for i := groupLen; i < 5; i++ {
			group[i] = 84 // 'u' - '!'
		}

		val := uint32(group[0])*85*85*85*85 +
			uint32(group[1])*85*85*85 +
			uint32(group[2])*85*85 +
			uint32(group[3])*85 +
			uint32(group[4])

		n := groupLen - 1
		for i := 0; i < n; i++ {
			result = append(result, byte(val>>(24-8*i)))
		}
	}

	return result, nil
}

// decodeASCIIHex decodes ASCIIHex encoded data
func (p *Parser) decodeASCIIHex(data []byte) ([]byte, error) {
	var result []byte
	var hex []byte

	for _, b := range data {
		if b == '>' {
			break
		}
		if (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F') {
			hex = append(hex, b)
		}
	}

	if len(hex)%2 != 0 {
		hex = append(hex, '0')
	}

	for i := 0; i < len(hex); i += 2 {
		val, _ := strconv.ParseUint(string(hex[i:i+2]), 16, 8)
		result = append(result, byte(val))
	}

	return result, nil
}

// decodeLZW decodes LZW compressed data
// PDF uses LZW with MSB bit order and a literal width of 8
func (p *Parser) decodeLZW(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}

	// PDF uses MSB (Most Significant Bit first) order
	// litWidth of 8 means codes start at 9 bits
	r := lzw.NewReader(bytes.NewReader(data), lzw.MSB, 8)
	defer r.Close()

	decoded, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("LZW decompression failed: %w", err)
	}

	return decoded, nil
}

// decodeLZWWithPredictor decodes LZW with PNG predictor
func (p *Parser) decodeLZWWithPredictor(data []byte, predictor, columns int) ([]byte, error) {
	// First decompress
	decompressed, err := p.decodeLZW(data)
	if err != nil {
		return nil, err
	}

	// If no predictor or predictor 1 (no prediction), return as-is
	if predictor <= 1 {
		return decompressed, nil
	}

	// PNG predictors (10-15)
	if predictor >= 10 && predictor <= 15 {
		return p.decodePNGPredictor(decompressed, columns)
	}

	// TIFF predictor (2) - not implemented
	return decompressed, nil
}

// ImageData contains extracted image information from a PDF
type ImageData struct {
	Data             []byte
	Width            int
	Height           int
	BitsPerComponent int
	ColorSpace       string
	Filter           string // Original filter type
	Format           string // Output format (jpeg, png)
}

// GetXObjects returns the XObject dictionary from a page's resources
func (p *Parser) GetXObjects(page *Object) (map[string]interface{}, error) {
	resources := p.getPageResources(page)
	if resources == nil {
		return nil, nil
	}

	xobjects, ok := resources["XObject"]
	if !ok {
		return nil, nil
	}

	switch x := xobjects.(type) {
	case map[string]interface{}:
		return x, nil
	case *Reference:
		obj, err := p.GetObject(x.ObjectNum)
		if err != nil {
			return nil, err
		}
		return obj.Dict, nil
	}

	return nil, nil
}

// getPageResources gets the resources dictionary for a page
func (p *Parser) getPageResources(page *Object) map[string]interface{} {
	if resources, ok := page.Dict["Resources"]; ok {
		switch r := resources.(type) {
		case map[string]interface{}:
			return r
		case *Reference:
			resObj, err := p.GetObject(r.ObjectNum)
			if err == nil {
				return resObj.Dict
			}
		}
	}
	return nil
}

// GetImageXObject retrieves an image XObject by name from a page
func (p *Parser) GetImageXObject(page *Object, name string) (*Object, error) {
	xobjects, err := p.GetXObjects(page)
	if err != nil || xobjects == nil {
		return nil, err
	}

	// Try with and without leading slash
	var ref *Reference
	if r, ok := xobjects[name].(*Reference); ok {
		ref = r
	} else if r, ok := xobjects["/"+name].(*Reference); ok {
		ref = r
	} else if r, ok := xobjects[strings.TrimPrefix(name, "/")].(*Reference); ok {
		ref = r
	}

	if ref == nil {
		return nil, fmt.Errorf("XObject %s not found", name)
	}

	obj, err := p.GetObject(ref.ObjectNum)
	if err != nil {
		return nil, err
	}

	// Verify it's an image
	subtype, _ := obj.Dict["Subtype"].(string)
	if subtype != "/Image" {
		return nil, fmt.Errorf("XObject %s is not an image (subtype: %s)", name, subtype)
	}

	return obj, nil
}

// ExtractImage extracts image data from an image XObject
func (p *Parser) ExtractImage(imgObj *Object) (*ImageData, error) {
	if imgObj == nil || imgObj.Dict == nil {
		return nil, fmt.Errorf("invalid image object")
	}

	// Get image properties
	width := 0
	if w, ok := imgObj.Dict["Width"].(float64); ok {
		width = int(w)
	}
	height := 0
	if h, ok := imgObj.Dict["Height"].(float64); ok {
		height = int(h)
	}
	bitsPerComponent := 8
	if b, ok := imgObj.Dict["BitsPerComponent"].(float64); ok {
		bitsPerComponent = int(b)
	}

	// Get color space
	colorSpace := "DeviceRGB"
	if cs, ok := imgObj.Dict["ColorSpace"].(string); ok {
		colorSpace = strings.TrimPrefix(cs, "/")
	} else if csRef, ok := imgObj.Dict["ColorSpace"].(*Reference); ok {
		// Color space might be a reference to an array (e.g., Indexed)
		csObj, err := p.GetObject(csRef.ObjectNum)
		if err == nil && csObj.Array != nil && len(csObj.Array) > 0 {
			if csName, ok := csObj.Array[0].(string); ok {
				colorSpace = strings.TrimPrefix(csName, "/")
			}
		}
	} else if csArr, ok := imgObj.Dict["ColorSpace"].([]interface{}); ok {
		if len(csArr) > 0 {
			if csName, ok := csArr[0].(string); ok {
				colorSpace = strings.TrimPrefix(csName, "/")
			}
		}
	}

	// Get filter
	filter := ""
	if f, ok := imgObj.Dict["Filter"].(string); ok {
		filter = f
	} else if fArr, ok := imgObj.Dict["Filter"].([]interface{}); ok {
		if len(fArr) > 0 {
			if fName, ok := fArr[0].(string); ok {
				filter = fName
			}
		}
	}

	// Extract and decode the image data
	data := imgObj.Stream
	format := "bin"

	switch filter {
	case "/DCTDecode":
		// JPEG data - can be written directly
		format = "jpeg"
		// DCTDecode streams are already JPEG, no decoding needed

	case "/FlateDecode":
		// Compressed raw pixels - decode
		decoded, err := p.decodeFlateDecode(data)
		if err != nil {
			return nil, fmt.Errorf("decoding FlateDecode: %w", err)
		}
		data = decoded
		format = "png" // Will need to be wrapped in PNG format

	case "/JPXDecode":
		// JPEG 2000 - can be written directly
		format = "jp2"

	case "":
		// No filter - raw data
		format = "png" // Raw data will need PNG wrapping

	default:
		// Try to decode using DecodeStream
		decoded, err := p.DecodeStream(imgObj)
		if err != nil {
			return nil, fmt.Errorf("unsupported filter %s: %w", filter, err)
		}
		data = decoded
		format = "png"
	}

	return &ImageData{
		Data:             data,
		Width:            width,
		Height:           height,
		BitsPerComponent: bitsPerComponent,
		ColorSpace:       colorSpace,
		Filter:           filter,
		Format:           format,
	}, nil
}

// GetAllPageImages extracts all images from a page
func (p *Parser) GetAllPageImages(pageIndex int) ([]*ImageData, []string, error) {
	page, err := p.GetPage(pageIndex)
	if err != nil {
		return nil, nil, err
	}

	xobjects, err := p.GetXObjects(page)
	if err != nil || xobjects == nil {
		return nil, nil, nil
	}

	var images []*ImageData
	var names []string

	for name, xobjRef := range xobjects {
		ref, ok := xobjRef.(*Reference)
		if !ok {
			continue
		}

		obj, err := p.GetObject(ref.ObjectNum)
		if err != nil {
			continue
		}

		// Check if it's an image
		subtype, _ := obj.Dict["Subtype"].(string)
		if subtype != "/Image" {
			continue
		}

		imgData, err := p.ExtractImage(obj)
		if err != nil {
			continue
		}

		images = append(images, imgData)
		names = append(names, strings.TrimPrefix(name, "/"))
	}

	return images, names, nil
}

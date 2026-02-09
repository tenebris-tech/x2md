package pdf

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"
)

// TextItem represents an extracted text element with position
type TextItem struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
	Text   string
	Font   string
}

// Font represents a PDF font
type Font struct {
	Name       string
	BaseFont   string
	Encoding   string
	ToUnicode  *CMap
	Widths     map[int]float64
	FirstChar  int
	LastChar   int
	IsEmbedded bool
}

// TextExtractor extracts text from PDF pages
type TextExtractor struct {
	parser    *Parser
	fonts     map[string]*Font
	xobjects  map[string]*Object // Form XObjects for current page
	pageIndex int
}

// NewTextExtractor creates a new text extractor
func NewTextExtractor(parser *Parser) *TextExtractor {
	return &TextExtractor{
		parser:   parser,
		fonts:    make(map[string]*Font),
		xobjects: make(map[string]*Object),
	}
}

// ExtractPage extracts text items from a page
func (e *TextExtractor) ExtractPage(pageIndex int) ([]TextItem, error) {
	e.pageIndex = pageIndex

	page, err := e.parser.GetPage(pageIndex)
	if err != nil {
		return nil, fmt.Errorf("getting page %d: %w", pageIndex, err)
	}

	// Load fonts for this page
	if err := e.loadPageFonts(page); err != nil {
		return nil, fmt.Errorf("loading fonts: %w", err)
	}

	// Load XObjects for this page (Form XObjects may contain text)
	e.loadPageXObjects(page)

	// Get page content stream(s)
	content, err := e.getPageContent(page)
	if err != nil {
		return nil, fmt.Errorf("getting content: %w", err)
	}

	// Get page dimensions for coordinate transformation
	mediaBox := e.getMediaBox(page)

	// Parse content stream and extract text
	items, err := e.parseContentStream(content, mediaBox)
	if err != nil {
		return nil, fmt.Errorf("parsing content: %w", err)
	}

	return items, nil
}

// loadPageFonts loads all fonts used on a page
func (e *TextExtractor) loadPageFonts(page *Object) error {
	resources := e.getResources(page)
	if resources == nil {
		return nil
	}

	fontsDict, ok := resources["Font"]
	if !ok {
		return nil
	}

	var fonts map[string]interface{}
	switch f := fontsDict.(type) {
	case map[string]interface{}:
		fonts = f
	case *Reference:
		fontObj, err := e.parser.GetObject(f.ObjectNum)
		if err != nil {
			return err
		}
		fonts = fontObj.Dict
	}

	for name, fontRef := range fonts {
		fontName := name
		if strings.HasPrefix(fontName, "/") {
			fontName = fontName[1:]
		}

		var fontDict map[string]interface{}
		switch f := fontRef.(type) {
		case *Reference:
			fontObj, err := e.parser.GetObject(f.ObjectNum)
			if err != nil {
				continue
			}
			fontDict = fontObj.Dict
		case map[string]interface{}:
			fontDict = f
		default:
			continue
		}

		font := e.parseFont(fontDict)
		font.Name = fontName
		e.fonts[fontName] = font
	}

	return nil
}

// parseFont parses a font dictionary
func (e *TextExtractor) parseFont(dict map[string]interface{}) *Font {
	font := &Font{
		Widths: make(map[int]float64),
	}

	if baseFont, ok := dict["BaseFont"].(string); ok {
		font.BaseFont = strings.TrimPrefix(baseFont, "/")
	}

	if encoding, ok := dict["Encoding"].(string); ok {
		font.Encoding = strings.TrimPrefix(encoding, "/")
	}

	if firstChar, ok := dict["FirstChar"].(float64); ok {
		font.FirstChar = int(firstChar)
	}

	if lastChar, ok := dict["LastChar"].(float64); ok {
		font.LastChar = int(lastChar)
	}

	// Parse widths
	if widths, ok := dict["Widths"]; ok {
		var widthArray []interface{}
		switch w := widths.(type) {
		case []interface{}:
			widthArray = w
		case *Reference:
			widthObj, err := e.parser.GetObject(w.ObjectNum)
			if err == nil && widthObj.Array != nil {
				widthArray = widthObj.Array
			}
		}

		for i, w := range widthArray {
			if width, ok := w.(float64); ok {
				font.Widths[font.FirstChar+i] = width
			}
		}
	}

	// Parse ToUnicode CMap
	if toUnicode, ok := dict["ToUnicode"]; ok {
		if ref, ok := toUnicode.(*Reference); ok {
			toUnicodeObj, err := e.parser.GetObject(ref.ObjectNum)
			if err == nil && toUnicodeObj.Stream != nil {
				stream, err := e.parser.DecodeStream(toUnicodeObj)
				if err == nil {
					font.ToUnicode = ParseCMap(stream)
				}
			}
		}
	}

	return font
}

// getResources gets the resources dictionary for a page
func (e *TextExtractor) getResources(page *Object) map[string]interface{} {
	if resources, ok := page.Dict["Resources"]; ok {
		switch r := resources.(type) {
		case map[string]interface{}:
			return r
		case *Reference:
			resObj, err := e.parser.GetObject(r.ObjectNum)
			if err == nil {
				return resObj.Dict
			}
		}
	}
	return nil
}

// loadPageXObjects loads XObjects from page resources
// Form XObjects may contain text that needs to be extracted
func (e *TextExtractor) loadPageXObjects(page *Object) {
	// Clear previous page's XObjects
	e.xobjects = make(map[string]*Object)

	resources := e.getResources(page)
	if resources == nil {
		return
	}

	xobjects, ok := resources["XObject"]
	if !ok {
		return
	}

	var xobjectDict map[string]interface{}
	switch x := xobjects.(type) {
	case map[string]interface{}:
		xobjectDict = x
	case *Reference:
		xobj, err := e.parser.GetObject(x.ObjectNum)
		if err != nil {
			return
		}
		xobjectDict = xobj.Dict
	default:
		return
	}

	for name, ref := range xobjectDict {
		if r, ok := ref.(*Reference); ok {
			obj, err := e.parser.GetObject(r.ObjectNum)
			if err != nil {
				continue
			}

			// Check if it's a Form XObject
			if subtype, ok := obj.Dict["Subtype"].(string); ok && subtype == "/Form" {
				e.xobjects[name] = obj

				// Load fonts from Form XObject resources
				e.loadFormXObjectFonts(obj)
			}
		}
	}
}

// loadFormXObjectFonts loads fonts from a Form XObject's resources
func (e *TextExtractor) loadFormXObjectFonts(formObj *Object) {
	resources, ok := formObj.Dict["Resources"]
	if !ok {
		return
	}

	var resDict map[string]interface{}
	switch r := resources.(type) {
	case map[string]interface{}:
		resDict = r
	case *Reference:
		resObj, err := e.parser.GetObject(r.ObjectNum)
		if err != nil {
			return
		}
		resDict = resObj.Dict
	default:
		return
	}

	fontsDict, ok := resDict["Font"]
	if !ok {
		return
	}

	var fonts map[string]interface{}
	switch f := fontsDict.(type) {
	case map[string]interface{}:
		fonts = f
	case *Reference:
		fontObj, err := e.parser.GetObject(f.ObjectNum)
		if err != nil {
			return
		}
		fonts = fontObj.Dict
	default:
		return
	}

	for fontName, fontRef := range fonts {
		if _, exists := e.fonts[fontName]; exists {
			continue // Already loaded
		}

		if ref, ok := fontRef.(*Reference); ok {
			fontObj, err := e.parser.GetObject(ref.ObjectNum)
			if err != nil {
				continue
			}
			font := e.parseFont(fontObj.Dict)
			e.fonts[fontName] = font
		}
	}
}

// getPageContent gets the content stream(s) for a page
func (e *TextExtractor) getPageContent(page *Object) ([]byte, error) {
	contents, ok := page.Dict["Contents"]
	if !ok {
		return nil, nil
	}

	var result []byte

	switch c := contents.(type) {
	case *Reference:
		contentObj, err := e.parser.GetObject(c.ObjectNum)
		if err != nil {
			return nil, err
		}
		decoded, err := e.parser.DecodeStream(contentObj)
		if err != nil {
			return nil, err
		}
		result = decoded

	case []interface{}:
		// Array of content streams
		for _, ref := range c {
			if r, ok := ref.(*Reference); ok {
				contentObj, err := e.parser.GetObject(r.ObjectNum)
				if err != nil {
					continue
				}
				decoded, err := e.parser.DecodeStream(contentObj)
				if err != nil {
					continue
				}
				result = append(result, decoded...)
				result = append(result, '\n')
			}
		}
	}

	return result, nil
}

// getMediaBox gets the page media box
func (e *TextExtractor) getMediaBox(page *Object) [4]float64 {
	defaultBox := [4]float64{0, 0, 612, 792} // Letter size

	mediaBox, ok := page.Dict["MediaBox"]
	if !ok {
		return defaultBox
	}

	var box []interface{}
	switch m := mediaBox.(type) {
	case []interface{}:
		box = m
	case *Reference:
		boxObj, err := e.parser.GetObject(m.ObjectNum)
		if err == nil && boxObj.Array != nil {
			box = boxObj.Array
		}
	}

	if len(box) >= 4 {
		var result [4]float64
		for i := 0; i < 4; i++ {
			if v, ok := box[i].(float64); ok {
				result[i] = v
			}
		}
		return result
	}

	return defaultBox
}

// GetPageDimensions returns the width and height of a page
func (e *TextExtractor) GetPageDimensions(pageIndex int) (width, height float64, err error) {
	page, err := e.parser.GetPage(pageIndex)
	if err != nil {
		return 0, 0, fmt.Errorf("getting page %d: %w", pageIndex, err)
	}

	mediaBox := e.getMediaBox(page)
	width = mediaBox[2] - mediaBox[0]
	height = mediaBox[3] - mediaBox[1]
	return width, height, nil
}

// GraphicsState represents the current graphics state
type GraphicsState struct {
	CTM          [6]float64 // Current transformation matrix
	TextMatrix   [6]float64
	LineMatrix   [6]float64
	FontName     string
	FontSize     float64
	CharSpacing  float64
	WordSpacing  float64
	Leading      float64
	TextRise     float64
	HorizScaling float64
}

// parseContentStream parses a content stream and extracts text items
func (e *TextExtractor) parseContentStream(content []byte, mediaBox [4]float64) (items []TextItem, err error) {
	// Recover from panics caused by malformed content streams
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("content stream parsing panic: %v", r)
		}
	}()

	// Initialize graphics state
	gs := &GraphicsState{
		CTM:          [6]float64{1, 0, 0, 1, 0, 0},
		TextMatrix:   [6]float64{1, 0, 0, 1, 0, 0},
		LineMatrix:   [6]float64{1, 0, 0, 1, 0, 0},
		HorizScaling: 100,
	}

	var gsStack []*GraphicsState

	// Tokenize and parse
	tokens := e.tokenize(content)
	var operandStack []interface{}

	for _, token := range tokens {
		if e.isOperator(token) {
			items = e.executeOperator(token, operandStack, gs, &gsStack, items, mediaBox)
			operandStack = []interface{}{}
		} else {
			operandStack = append(operandStack, e.parseToken(token))
		}
	}

	return items, nil
}

// parseFormXObject parses a Form XObject's content stream
// It inherits the CTM from the parent graphics state
func (e *TextExtractor) parseFormXObject(content []byte, mediaBox [4]float64, parentGS *GraphicsState) (items []TextItem, err error) {
	// Recover from panics
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("form xobject parsing panic: %v", r)
		}
	}()

	// Initialize graphics state, inheriting CTM from parent
	gs := &GraphicsState{
		CTM:          parentGS.CTM,
		TextMatrix:   [6]float64{1, 0, 0, 1, 0, 0},
		LineMatrix:   [6]float64{1, 0, 0, 1, 0, 0},
		HorizScaling: 100,
	}

	var gsStack []*GraphicsState

	// Tokenize and parse
	tokens := e.tokenize(content)
	var operandStack []interface{}

	for _, token := range tokens {
		if e.isOperator(token) {
			items = e.executeOperator(token, operandStack, gs, &gsStack, items, mediaBox)
			operandStack = []interface{}{}
		} else {
			operandStack = append(operandStack, e.parseToken(token))
		}
	}

	return items, nil
}

// tokenize splits content stream into tokens
func (e *TextExtractor) tokenize(content []byte) []string {
	var tokens []string
	i := 0

	for i < len(content) {
		// Skip whitespace
		for i < len(content) && (content[i] == ' ' || content[i] == '\t' || content[i] == '\n' || content[i] == '\r') {
			i++
		}

		if i >= len(content) {
			break
		}

		// Comment
		if content[i] == '%' {
			for i < len(content) && content[i] != '\n' && content[i] != '\r' {
				i++
			}
			continue
		}

		// String
		if content[i] == '(' {
			start := i
			i++
			depth := 1
			for i < len(content) && depth > 0 {
				if content[i] == '\\' && i+1 < len(content) {
					i += 2
					continue
				}
				if content[i] == '(' {
					depth++
				} else if content[i] == ')' {
					depth--
				}
				i++
			}
			tokens = append(tokens, string(content[start:i]))
			continue
		}

		// Hex string
		if content[i] == '<' {
			if i+1 < len(content) && content[i+1] == '<' {
				// Dictionary
				tokens = append(tokens, "<<")
				i += 2
				continue
			}
			start := i
			for i < len(content) && content[i] != '>' {
				i++
			}
			if i < len(content) {
				i++
			}
			tokens = append(tokens, string(content[start:i]))
			continue
		}

		if content[i] == '>' {
			if i+1 < len(content) && content[i+1] == '>' {
				tokens = append(tokens, ">>")
				i += 2
				continue
			}
			i++
			continue
		}

		// Array
		if content[i] == '[' {
			tokens = append(tokens, "[")
			i++
			continue
		}
		if content[i] == ']' {
			tokens = append(tokens, "]")
			i++
			continue
		}

		// Name
		if content[i] == '/' {
			start := i
			i++
			for i < len(content) && !e.isDelimiter(content[i]) {
				i++
			}
			tokens = append(tokens, string(content[start:i]))
			continue
		}

		// Number or operator
		start := i
		for i < len(content) && !e.isDelimiter(content[i]) {
			i++
		}
		if start != i {
			tokens = append(tokens, string(content[start:i]))
		}
	}

	return tokens
}

func (e *TextExtractor) isDelimiter(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' ||
		b == '(' || b == ')' || b == '<' || b == '>' ||
		b == '[' || b == ']' || b == '/' || b == '%'
}

func (e *TextExtractor) isOperator(token string) bool {
	if len(token) == 0 {
		return false
	}

	// Operators are alphabetic or specific symbols
	c := token[0]
	if c == '/' || c == '(' || c == '<' || c == '[' {
		return false
	}
	if (c >= '0' && c <= '9') || c == '-' || c == '+' || c == '.' {
		return false
	}

	// Known operators
	operators := map[string]bool{
		"BT": true, "ET": true,
		"Tf": true, "Tm": true, "Td": true, "TD": true, "T*": true,
		"Tj": true, "TJ": true, "'": true, "\"": true,
		"Tc": true, "Tw": true, "TL": true, "Ts": true, "Tz": true,
		"q": true, "Q": true, "cm": true,
		"BDC": true, "BMC": true, "EMC": true,
		"BI": true, "ID": true, "EI": true,
		"Do": true, "gs": true, "CS": true, "cs": true, "SC": true, "sc": true,
		"G": true, "g": true, "RG": true, "rg": true, "K": true, "k": true,
		"m": true, "l": true, "c": true, "v": true, "y": true, "h": true,
		"re": true, "S": true, "s": true, "f": true, "F": true, "f*": true,
		"B": true, "B*": true, "b": true, "b*": true, "n": true,
		"W": true, "W*": true, "d": true, "i": true, "j": true, "J": true,
		"M": true, "w": true, "ri": true, "sh": true,
	}

	return operators[token]
}

func (e *TextExtractor) parseToken(token string) interface{} {
	if strings.HasPrefix(token, "/") {
		return token
	}

	if strings.HasPrefix(token, "(") {
		// Literal string
		return e.parseLiteralStringToken(token)
	}

	if strings.HasPrefix(token, "<") && !strings.HasPrefix(token, "<<") {
		// Hex string
		return e.parseHexStringToken(token)
	}

	if token == "[" || token == "]" || token == "<<" || token == ">>" {
		return token
	}

	// Try to parse as number
	if num, err := strconv.ParseFloat(token, 64); err == nil {
		return num
	}

	return token
}

func (e *TextExtractor) parseLiteralStringToken(token string) string {
	if len(token) < 2 {
		return ""
	}

	// Remove parentheses
	content := token[1 : len(token)-1]
	var result []byte

	i := 0
	for i < len(content) {
		if content[i] == '\\' && i+1 < len(content) {
			i++
			switch content[i] {
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
				if content[i] >= '0' && content[i] <= '7' {
					// Octal
					octal := string(content[i])
					for j := 0; j < 2 && i+1 < len(content) && content[i+1] >= '0' && content[i+1] <= '7'; j++ {
						i++
						octal += string(content[i])
					}
					val, _ := strconv.ParseInt(octal, 8, 32)
					result = append(result, byte(val))
				} else {
					result = append(result, content[i])
				}
			}
		} else {
			result = append(result, content[i])
		}
		i++
	}

	return string(result)
}

func (e *TextExtractor) parseHexStringToken(token string) string {
	if len(token) < 2 {
		return ""
	}

	// Remove angle brackets
	hex := token[1 : len(token)-1]
	hex = strings.ReplaceAll(hex, " ", "")
	hex = strings.ReplaceAll(hex, "\n", "")
	hex = strings.ReplaceAll(hex, "\r", "")

	if len(hex)%2 != 0 {
		hex += "0"
	}

	var result []byte
	for i := 0; i < len(hex); i += 2 {
		val, _ := strconv.ParseInt(hex[i:i+2], 16, 32)
		result = append(result, byte(val))
	}

	return string(result)
}

func (e *TextExtractor) executeOperator(op string, operands []interface{}, gs *GraphicsState, gsStack *[]*GraphicsState, items []TextItem, mediaBox [4]float64) []TextItem {
	switch op {
	case "q":
		// Save graphics state
		saved := *gs
		*gsStack = append(*gsStack, &saved)

	case "Q":
		// Restore graphics state
		if len(*gsStack) > 0 {
			*gs = *(*gsStack)[len(*gsStack)-1]
			*gsStack = (*gsStack)[:len(*gsStack)-1]
		}

	case "cm":
		// Concatenate matrix
		if len(operands) >= 6 {
			a := e.getFloat(operands[0])
			b := e.getFloat(operands[1])
			c := e.getFloat(operands[2])
			d := e.getFloat(operands[3])
			ee := e.getFloat(operands[4])
			f := e.getFloat(operands[5])

			gs.CTM = e.multiplyMatrix([6]float64{a, b, c, d, ee, f}, gs.CTM)
		}

	case "BT":
		// Begin text object
		gs.TextMatrix = [6]float64{1, 0, 0, 1, 0, 0}
		gs.LineMatrix = [6]float64{1, 0, 0, 1, 0, 0}

	case "ET":
		// End text object

	case "Tf":
		// Set font
		if len(operands) >= 2 {
			if fontName, ok := operands[0].(string); ok {
				gs.FontName = strings.TrimPrefix(fontName, "/")
			}
			gs.FontSize = e.getFloat(operands[1])
		}

	case "Tc":
		// Character spacing
		if len(operands) >= 1 {
			gs.CharSpacing = e.getFloat(operands[0])
		}

	case "Tw":
		// Word spacing
		if len(operands) >= 1 {
			gs.WordSpacing = e.getFloat(operands[0])
		}

	case "TL":
		// Leading
		if len(operands) >= 1 {
			gs.Leading = e.getFloat(operands[0])
		}

	case "Ts":
		// Text rise
		if len(operands) >= 1 {
			gs.TextRise = e.getFloat(operands[0])
		}

	case "Tz":
		// Horizontal scaling
		if len(operands) >= 1 {
			gs.HorizScaling = e.getFloat(operands[0])
		}

	case "Tm":
		// Set text matrix
		if len(operands) >= 6 {
			gs.TextMatrix = [6]float64{
				e.getFloat(operands[0]),
				e.getFloat(operands[1]),
				e.getFloat(operands[2]),
				e.getFloat(operands[3]),
				e.getFloat(operands[4]),
				e.getFloat(operands[5]),
			}
			gs.LineMatrix = gs.TextMatrix
		}

	case "Td":
		// Move text position
		if len(operands) >= 2 {
			tx := e.getFloat(operands[0])
			ty := e.getFloat(operands[1])
			gs.TextMatrix = e.multiplyMatrix([6]float64{1, 0, 0, 1, tx, ty}, gs.LineMatrix)
			gs.LineMatrix = gs.TextMatrix
		}

	case "TD":
		// Move text position and set leading
		if len(operands) >= 2 {
			tx := e.getFloat(operands[0])
			ty := e.getFloat(operands[1])
			gs.Leading = -ty
			gs.TextMatrix = e.multiplyMatrix([6]float64{1, 0, 0, 1, tx, ty}, gs.LineMatrix)
			gs.LineMatrix = gs.TextMatrix
		}

	case "T*":
		// Move to start of next line
		gs.TextMatrix = e.multiplyMatrix([6]float64{1, 0, 0, 1, 0, -gs.Leading}, gs.LineMatrix)
		gs.LineMatrix = gs.TextMatrix

	case "Tj":
		// Show text
		if len(operands) >= 1 {
			if text, ok := operands[0].(string); ok {
				item := e.showText(text, gs, mediaBox)
				if item.Text != "" {
					items = append(items, item)
				}
			}
		}

	case "TJ":
		// Show text with positioning
		items = e.showTextArray(operands, gs, items, mediaBox)

	case "'":
		// Move to next line and show text
		gs.TextMatrix = e.multiplyMatrix([6]float64{1, 0, 0, 1, 0, -gs.Leading}, gs.LineMatrix)
		gs.LineMatrix = gs.TextMatrix
		if len(operands) >= 1 {
			if text, ok := operands[0].(string); ok {
				item := e.showText(text, gs, mediaBox)
				if item.Text != "" {
					items = append(items, item)
				}
			}
		}

	case "\"":
		// Set spacing, move to next line, and show text
		if len(operands) >= 3 {
			gs.WordSpacing = e.getFloat(operands[0])
			gs.CharSpacing = e.getFloat(operands[1])
			gs.TextMatrix = e.multiplyMatrix([6]float64{1, 0, 0, 1, 0, -gs.Leading}, gs.LineMatrix)
			gs.LineMatrix = gs.TextMatrix
			if text, ok := operands[2].(string); ok {
				item := e.showText(text, gs, mediaBox)
				if item.Text != "" {
					items = append(items, item)
				}
			}
		}

	case "Do":
		// Paint XObject - if it's a Form XObject, extract text from it
		if len(operands) >= 1 {
			if xobjName, ok := operands[0].(string); ok {
				xobjName = strings.TrimPrefix(xobjName, "/")
				if formObj, exists := e.xobjects[xobjName]; exists {
					// Get Form XObject content stream
					stream, err := e.parser.DecodeStream(formObj)
					if err == nil && len(stream) > 0 {
						// Get Form XObject's BBox for coordinate transformation
						formMediaBox := mediaBox
						if bbox, ok := formObj.Dict["BBox"].([]interface{}); ok && len(bbox) >= 4 {
							formMediaBox = [4]float64{
								e.getFloat(bbox[0]),
								e.getFloat(bbox[1]),
								e.getFloat(bbox[2]),
								e.getFloat(bbox[3]),
							}
						}

						// Parse Form XObject content with current graphics state
						formItems, _ := e.parseFormXObject(stream, formMediaBox, gs)
						items = append(items, formItems...)
					}
				}
			}
		}
	}

	return items
}

func (e *TextExtractor) getFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	}
	return 0
}

func (e *TextExtractor) multiplyMatrix(a, b [6]float64) [6]float64 {
	return [6]float64{
		a[0]*b[0] + a[1]*b[2],
		a[0]*b[1] + a[1]*b[3],
		a[2]*b[0] + a[3]*b[2],
		a[2]*b[1] + a[3]*b[3],
		a[4]*b[0] + a[5]*b[2] + b[4],
		a[4]*b[1] + a[5]*b[3] + b[5],
	}
}

func (e *TextExtractor) showText(text string, gs *GraphicsState, mediaBox [4]float64) TextItem {
	// Decode text using font encoding
	decodedText := e.decodeText(text, gs.FontName)

	// Calculate position
	tm := e.multiplyMatrix(gs.TextMatrix, gs.CTM)

	x := tm[4]
	y := tm[5]

	// Transform Y coordinate (PDF origin is bottom-left)
	pageHeight := mediaBox[3] - mediaBox[1]
	y = pageHeight - y

	// Calculate dimensions
	fontSize := gs.FontSize * math.Sqrt(tm[0]*tm[0]+tm[1]*tm[1])
	width := e.calculateTextWidth(text, gs)

	// Advance text position
	gs.TextMatrix = e.multiplyMatrix([6]float64{1, 0, 0, 1, width, 0}, gs.TextMatrix)

	return TextItem{
		X:      x,
		Y:      y,
		Width:  width,
		Height: fontSize,
		Text:   decodedText,
		Font:   gs.FontName,
	}
}

func (e *TextExtractor) showTextArray(operands []interface{}, gs *GraphicsState, items []TextItem, mediaBox [4]float64) []TextItem {
	// Find the array in operands
	var textArray []interface{}

	// Look for array content
	inArray := false
	for _, op := range operands {
		if str, ok := op.(string); ok {
			if str == "[" {
				inArray = true
				textArray = []interface{}{}
				continue
			}
			if str == "]" {
				inArray = false
				continue
			}
		}
		if inArray {
			textArray = append(textArray, op)
		}
	}

	// If no explicit array markers, treat all operands as the array
	if len(textArray) == 0 {
		textArray = operands
	}

	// Process array elements
	var currentText strings.Builder
	startX := gs.TextMatrix[4]
	startY := gs.TextMatrix[5]

	for _, elem := range textArray {
		switch v := elem.(type) {
		case string:
			if v != "[" && v != "]" {
				currentText.WriteString(e.decodeText(v, gs.FontName))
				width := e.calculateTextWidth(v, gs)
				gs.TextMatrix = e.multiplyMatrix([6]float64{1, 0, 0, 1, width, 0}, gs.TextMatrix)
			}
		case float64:
			// Negative value moves text right, positive moves left
			adjustment := -v / 1000.0 * gs.FontSize * (gs.HorizScaling / 100.0)

			// If significant adjustment (space), emit current text
			// Normal word spacing is ~250-333/1000 em (0.25-0.33 em)
			// Use 300 as threshold to avoid false word breaks from kerning
			// Kerning adjustments are typically -50 to -150 units
			if math.Abs(v) > 300 && currentText.Len() > 0 {
				// Add space
				currentText.WriteString(" ")
			}

			gs.TextMatrix = e.multiplyMatrix([6]float64{1, 0, 0, 1, adjustment, 0}, gs.TextMatrix)
		}
	}

	if currentText.Len() > 0 {
		tm := e.multiplyMatrix([6]float64{1, 0, 0, 1, startX, startY}, gs.CTM)

		x := tm[4]
		y := tm[5]

		pageHeight := mediaBox[3] - mediaBox[1]
		y = pageHeight - y

		fontSize := gs.FontSize * math.Sqrt(gs.CTM[0]*gs.CTM[0]+gs.CTM[1]*gs.CTM[1])
		width := gs.TextMatrix[4] - startX

		items = append(items, TextItem{
			X:      x,
			Y:      y,
			Width:  width,
			Height: fontSize,
			Text:   currentText.String(),
			Font:   gs.FontName,
		})
	}

	return items
}

func (e *TextExtractor) decodeText(text string, fontName string) string {
	font, ok := e.fonts[fontName]
	if !ok {
		// No font info, try basic decoding
		return e.basicDecode(text, "")
	}

	// Use CMap if available
	if font.ToUnicode != nil {
		return font.ToUnicode.DecodeString([]byte(text))
	}

	// Use encoding-aware decoding based on font information
	encoding := font.Encoding
	// Check if this is a Symbol font (Symbol encoding uses different character mappings)
	if strings.Contains(strings.ToLower(font.BaseFont), "symbol") {
		encoding = "Symbol"
	}

	return e.basicDecode(text, encoding)
}

func (e *TextExtractor) basicDecode(text string, encoding string) string {
	data := []byte(text)

	// Check for UTF-16BE BOM
	if len(data) >= 2 && data[0] == 0xFE && data[1] == 0xFF {
		// UTF-16BE
		if len(data)%2 != 0 {
			data = append(data, 0)
		}
		var u16 []uint16
		for i := 2; i < len(data); i += 2 {
			u16 = append(u16, uint16(data[i])<<8|uint16(data[i+1]))
		}
		return string(utf16.Decode(u16))
	}

	// Check for MacRoman encoding (0xA5 = bullet in MacRoman)
	isMacRoman := encoding == "MacRomanEncoding" || encoding == "MacRoman"
	// Symbol fonts use a different character set
	isSymbol := encoding == "Symbol" || strings.Contains(strings.ToLower(encoding), "symbol")

	// PDFDocEncoding / WinAnsiEncoding (CP1252) fallback
	var result strings.Builder
	for _, b := range data {
		if b >= 32 && b <= 126 {
			result.WriteByte(b)
		} else if b >= 128 {
			// Handle encoding-specific mappings
			if isSymbol {
				// Symbol encoding character mappings
				r := decodeSymbolChar(b)
				if r != 0 {
					result.WriteRune(r)
				}
				continue
			}

			if isMacRoman {
				// MacRoman encoding - 0xA5 is bullet
				r := decodeMacRomanChar(b)
				if r != 0 {
					result.WriteRune(r)
				}
				continue
			}

			// WinAnsi (CP1252) / MacRoman character mappings
			// Full CP1252 mapping for bytes 0x80-0x9F (undefined in ISO-8859-1)
			switch b {
			case 0x80:
				result.WriteRune(0x20AC) // Euro sign
			case 0x82:
				result.WriteRune(0x201A) // single low-9 quotation mark
			case 0x83:
				result.WriteRune(0x0192) // Latin small f with hook
			case 0x84:
				result.WriteRune(0x201E) // double low-9 quotation mark
			case 0x85:
				result.WriteRune(0x2026) // horizontal ellipsis
			case 0x86:
				result.WriteRune(0x2020) // dagger
			case 0x87:
				result.WriteRune(0x2021) // double dagger
			case 0x88:
				result.WriteRune(0x02C6) // modifier letter circumflex accent
			case 0x89:
				result.WriteRune(0x2030) // per mille sign
			case 0x8A:
				result.WriteRune(0x0160) // Latin capital S with caron
			case 0x8B:
				result.WriteRune(0x2039) // single left-pointing angle quotation
			case 0x8C:
				result.WriteRune(0x0152) // Latin capital ligature OE
			case 0x8E:
				result.WriteRune(0x017D) // Latin capital Z with caron
			case 0x91:
				result.WriteRune(0x2018) // left single quotation mark
			case 0x92:
				result.WriteRune(0x2019) // right single quotation mark (apostrophe)
			case 0x93:
				result.WriteRune(0x201C) // left double quotation mark
			case 0x94:
				result.WriteRune(0x201D) // right double quotation mark
			case 0x95:
				result.WriteRune(0x2022) // bullet
			case 0x96:
				result.WriteRune(0x2013) // en dash
			case 0x97:
				result.WriteRune(0x2014) // em dash
			case 0x98:
				result.WriteRune(0x02DC) // small tilde
			case 0x99:
				result.WriteRune(0x2122) // trade mark sign
			case 0x9A:
				result.WriteRune(0x0161) // Latin small s with caron
			case 0x9B:
				result.WriteRune(0x203A) // single right-pointing angle quotation
			case 0x9C:
				result.WriteRune(0x0153) // Latin small ligature oe
			case 0x9E:
				result.WriteRune(0x017E) // Latin small z with caron
			case 0x9F:
				result.WriteRune(0x0178) // Latin capital Y with diaeresis
			// Bytes 0xA0-0xFF map directly to Unicode in CP1252/Latin-1
			case 0xA0:
				result.WriteRune(0x00A0) // non-breaking space
			case 0xA5:
				result.WriteRune(0x00A5) // yen sign
			case 0xA6:
				result.WriteRune(0x00A6) // broken bar
			case 0xA7:
				result.WriteRune(0x00A7) // section sign
			case 0xA9:
				result.WriteRune(0x00A9) // copyright sign
			case 0xAE:
				result.WriteRune(0x00AE) // registered sign
			case 0xB0:
				result.WriteRune(0x00B0) // degree sign
			case 0xB7:
				result.WriteRune(0x00B7) // middle dot
			default:
				// For bytes 0xA0-0xFF, they map directly to Unicode in Latin-1/CP1252
				if b >= 0xA0 {
					result.WriteRune(rune(b))
				}
				// For undefined bytes 0x81, 0x8D, 0x8F, 0x90, 0x9D, skip them
				// (they are undefined in CP1252)
			}
		} else if b == '\n' || b == '\r' || b == '\t' {
			result.WriteByte(b)
		}
	}
	return result.String()
}

// decodeSymbolChar decodes a byte using Symbol encoding
func decodeSymbolChar(b byte) rune {
	// Symbol encoding maps - common characters used in PDFs
	symbolMap := map[byte]rune{
		0xA5: 0x221E, // infinity (in some symbol fonts)
		0xB7: 0x2022, // bullet
		0xD7: 0x00D7, // multiplication sign
		// Add more as needed
	}
	if r, ok := symbolMap[b]; ok {
		return r
	}
	// Default: treat as Latin-1 for unmapped chars
	if b >= 0xA0 {
		return rune(b)
	}
	return 0
}

// decodeMacRomanChar decodes a byte using MacRoman encoding
func decodeMacRomanChar(b byte) rune {
	// MacRoman encoding differences from Latin-1
	macRomanMap := map[byte]rune{
		0x80: 0x00C4, // Ä
		0x81: 0x00C5, // Å
		0x82: 0x00C7, // Ç
		0x83: 0x00C9, // É
		0x84: 0x00D1, // Ñ
		0x85: 0x00D6, // Ö
		0x86: 0x00DC, // Ü
		0x87: 0x00E1, // á
		0x88: 0x00E0, // à
		0x89: 0x00E2, // â
		0x8A: 0x00E4, // ä
		0x8B: 0x00E3, // ã
		0x8C: 0x00E5, // å
		0x8D: 0x00E7, // ç
		0x8E: 0x00E9, // é
		0x8F: 0x00E8, // è
		0x90: 0x00EA, // ê
		0x91: 0x00EB, // ë
		0x92: 0x00ED, // í
		0x93: 0x00EC, // ì
		0x94: 0x00EE, // î
		0x95: 0x00EF, // ï
		0x96: 0x00F1, // ñ
		0x97: 0x00F3, // ó
		0x98: 0x00F2, // ò
		0x99: 0x00F4, // ô
		0x9A: 0x00F6, // ö
		0x9B: 0x00F5, // õ
		0x9C: 0x00FA, // ú
		0x9D: 0x00F9, // ù
		0x9E: 0x00FB, // û
		0x9F: 0x00FC, // ü
		0xA0: 0x2020, // dagger
		0xA1: 0x00B0, // degree sign
		0xA2: 0x00A2, // cent sign
		0xA3: 0x00A3, // pound sign
		0xA4: 0x00A7, // section sign
		0xA5: 0x2022, // BULLET - key difference from CP1252
		0xA6: 0x00B6, // pilcrow sign
		0xA7: 0x00DF, // sharp s
		0xA8: 0x00AE, // registered sign
		0xA9: 0x00A9, // copyright sign
		0xAA: 0x2122, // trademark
		0xAB: 0x00B4, // acute accent
		0xAC: 0x00A8, // diaeresis
		0xAD: 0x2260, // not equal
		0xAE: 0x00C6, // Æ
		0xAF: 0x00D8, // Ø
		0xB0: 0x221E, // infinity
		0xB1: 0x00B1, // plus-minus
		0xB2: 0x2264, // less than or equal
		0xB3: 0x2265, // greater than or equal
		0xB4: 0x00A5, // yen sign
		0xB5: 0x00B5, // micro sign
		0xB7: 0x2211, // summation
		0xD0: 0x2013, // en dash
		0xD1: 0x2014, // em dash
		0xD2: 0x201C, // left double quotation
		0xD3: 0x201D, // right double quotation
		0xD4: 0x2018, // left single quotation
		0xD5: 0x2019, // right single quotation
	}
	if r, ok := macRomanMap[b]; ok {
		return r
	}
	// Default: treat as Latin-1 for unmapped chars
	if b >= 0xA0 {
		return rune(b)
	}
	return 0
}

func (e *TextExtractor) calculateTextWidth(text string, gs *GraphicsState) float64 {
	font, ok := e.fonts[gs.FontName]
	if !ok {
		// Estimate width
		return float64(len(text)) * gs.FontSize * 0.5 * (gs.HorizScaling / 100.0)
	}

	var width float64
	data := []byte(text)

	for _, b := range data {
		charCode := int(b)
		if w, ok := font.Widths[charCode]; ok {
			width += w
		} else {
			width += 500 // Default width
		}

		if b == ' ' {
			width += gs.WordSpacing * 1000 / gs.FontSize
		}
		width += gs.CharSpacing * 1000 / gs.FontSize
	}

	return width / 1000.0 * gs.FontSize * (gs.HorizScaling / 100.0)
}

// GetFonts returns the loaded fonts
func (e *TextExtractor) GetFonts() map[string]*Font {
	return e.fonts
}

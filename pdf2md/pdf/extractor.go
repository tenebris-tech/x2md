/******************************************************************************
 * Copyright (c) 2025-2026 Tenebris Technologies Inc.                         *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package pdf

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// TextItem represents an extracted text element with position. X and Y are
// where the text starts on the page (Y grows downward, Y is the baseline);
// Width is how far the text advances along its baseline, in the same page
// units. For upright text the baseline is horizontal and the text ends at
// X+Width; Rotated marks text whose baseline is not (a diagonal watermark,
// a vertical label).
type TextItem struct {
	X       float64
	Y       float64
	Width   float64
	Height  float64
	Text    string
	Font    string
	Rotated bool

	// glyphRun marks text that began as a glyph fragment, which following
	// fragments may be joined onto (see mergeGlyphRuns)
	glyphRun bool
}

// Font represents a PDF font
type Font struct {
	Name      string
	BaseFont  string
	Encoding  string
	ToUnicode *CMap
	// Widths holds glyph widths in glyph space by character code, or by CID
	// for a Type0 font
	Widths     map[int]float64
	FirstChar  int
	LastChar   int
	IsEmbedded bool

	// missingWidth is the width of a code with no entry in Widths
	// (MissingWidth, or DW for a Type0 font)
	missingWidth float64
	// widthScale converts glyph space widths to text space: 0.001, or the
	// FontMatrix scale of a Type3 font
	widthScale float64
	// std holds built-in widths for a font without a Widths array
	std *[224]uint16
	// twoByte is set for Type0 fonts, whose codes are 2 bytes
	twoByte bool
}

// TextExtractor extracts text from PDF pages
type TextExtractor struct {
	parser   *Parser
	fonts    map[string]*Font
	xobjects map[string]*Object // Form XObjects for current page

	// xobjectFonts holds the fonts each Form XObject on the current page
	// declares in its own resources, by XObject name then font name. A form's
	// font names are its own: two forms may both call a different font "F1".
	xobjectFonts map[string]map[string]*Font
	pageIndex    int

	// path is the path under construction; rules collects the lines the
	// current page paints (see Rules)
	path  pathBuilder
	rules []Rule
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
	e.path = pathBuilder{}
	e.rules = nil

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

	return visibleText(mergeGlyphRuns(items), mediaBox), nil
}

// visibleText drops text that cannot be seen: a position or size that is not
// a finite number (from a degenerate matrix), or a place far off the page
// (more than a page's width or height outside its box)
func visibleText(items []TextItem, mediaBox [4]float64) []TextItem {
	width, height := mediaBox[2]-mediaBox[0], mediaBox[3]-mediaBox[1]
	kept := items[:0]
	for _, item := range items {
		values := []float64{item.X, item.Y, item.Width, item.Height}
		finite := true
		for _, v := range values {
			finite = finite && !math.IsNaN(v) && !math.IsInf(v, 0)
		}
		if !finite {
			continue
		}
		if width > 0 && height > 0 && (item.X > mediaBox[2]+width || item.X+item.Width < mediaBox[0]-width ||
			item.Y > 2*height || item.Y < -height) {
			continue
		}
		kept = append(kept, item)
	}
	return kept
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

	if firstChar, ok := e.resolveNumber(dict["FirstChar"]); ok {
		font.FirstChar = int(firstChar)
	}

	if lastChar, ok := e.resolveNumber(dict["LastChar"]); ok {
		font.LastChar = int(lastChar)
	}

	e.loadWidths(font, dict)

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
	e.xobjectFonts = make(map[string]map[string]*Font)

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

	// Visit the XObjects in name order, so that which form's font ends up in
	// the document-wide font map does not depend on map iteration order
	names := make([]string, 0, len(xobjectDict))
	for name := range xobjectDict {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if r, ok := xobjectDict[name].(*Reference); ok {
			obj, err := e.parser.GetObject(r.ObjectNum)
			if err != nil {
				continue
			}

			// Check if it's a Form XObject
			if subtype, ok := obj.Dict["Subtype"].(string); ok && subtype == "/Form" {
				e.xobjects[name] = obj

				// Load fonts from Form XObject resources
				fonts := e.loadFormXObjectFonts(obj)
				e.xobjectFonts[name] = fonts

				// Make them known document-wide too (for formatting
				// detection), without replacing a font of the same name
				fontNames := make([]string, 0, len(fonts))
				for fontName := range fonts {
					fontNames = append(fontNames, fontName)
				}
				sort.Strings(fontNames)
				for _, fontName := range fontNames {
					if _, exists := e.fonts[fontName]; !exists {
						e.fonts[fontName] = fonts[fontName]
					}
				}
			}
		}
	}
}

// loadFormXObjectFonts returns the fonts a Form XObject declares in its own
// resources, by font name
func (e *TextExtractor) loadFormXObjectFonts(formObj *Object) map[string]*Font {
	result := make(map[string]*Font)

	resources, ok := formObj.Dict["Resources"]
	if !ok {
		return result
	}

	var resDict map[string]interface{}
	switch r := resources.(type) {
	case map[string]interface{}:
		resDict = r
	case *Reference:
		resObj, err := e.parser.GetObject(r.ObjectNum)
		if err != nil {
			return result
		}
		resDict = resObj.Dict
	default:
		return result
	}

	fontsDict, ok := resDict["Font"]
	if !ok {
		return result
	}

	var fonts map[string]interface{}
	switch f := fontsDict.(type) {
	case map[string]interface{}:
		fonts = f
	case *Reference:
		fontObj, err := e.parser.GetObject(f.ObjectNum)
		if err != nil {
			return result
		}
		fonts = fontObj.Dict
	default:
		return result
	}

	for fontName, fontRef := range fonts {
		if ref, ok := fontRef.(*Reference); ok {
			fontObj, err := e.parser.GetObject(ref.ObjectNum)
			if err != nil {
				continue
			}
			result[fontName] = e.parseFont(fontObj.Dict)
		}
	}

	return result
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
	LineWidth    float64 // stroke width in user space (w)
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
		LineWidth:    1,
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

	// The form inherits the whole graphics state, text state included
	// (font, size, Tc, Tw, Tz, TL, Ts); the text matrices belong to a text
	// object, which a form cannot continue
	inherited := *parentGS
	gs := &inherited
	gs.TextMatrix = [6]float64{1, 0, 0, 1, 0, 0}
	gs.LineMatrix = [6]float64{1, 0, 0, 1, 0, 0}

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
		"SCN": true, "scn": true, "Tr": true, "d0": true, "d1": true,
		"BX": true, "EX": true, "MP": true, "DP": true,
	}

	return operators[token]
}

// delimiter is an array or dictionary delimiter in a content stream, kept
// apart from strings: a literal string "(])" is a string, not the end of an
// array
type delimiter string

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
		return delimiter(token)
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

	case "w":
		// Line width
		if v, ok := e.lastFloats(operands, 1); ok {
			gs.LineWidth = v[0]
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
				items = append(items, e.showText(text, gs, mediaBox)...)
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
				items = append(items, e.showText(text, gs, mediaBox)...)
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
				items = append(items, e.showText(text, gs, mediaBox)...)
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
						// The form's text is placed on the page by the CTM (and
						// its Matrix), so its Y is flipped about the page's
						// height like the page's own text, not the form's BBox

						// Inside the form, its own fonts take precedence over
						// fonts of the same name declared elsewhere
						outerFonts := e.fonts
						if formFonts := e.xobjectFonts[xobjName]; len(formFonts) > 0 {
							e.fonts = make(map[string]*Font, len(outerFonts)+len(formFonts))
							for fontName, font := range outerFonts {
								e.fonts[fontName] = font
							}
							for fontName, font := range formFonts {
								e.fonts[fontName] = font
							}
						}

						// The form's Matrix maps form space into the user
						// space in which the form is drawn
						formGS := *gs
						if m := e.resolveArray(formObj.Dict["Matrix"]); len(m) == 6 {
							var matrix [6]float64
							valid := true
							for i := range matrix {
								var ok bool
								matrix[i], ok = e.resolveNumber(m[i])
								valid = valid && ok
							}
							// A malformed Matrix is the identity
							if valid {
								formGS.CTM = e.multiplyMatrix(matrix, gs.CTM)
							}
						}

						// Parse Form XObject content with current graphics state
						formItems, _ := e.parseFormXObject(stream, mediaBox, &formGS)
						e.fonts = outerFonts
						items = append(items, formItems...)
					}
				}
			}
		}

	default:
		e.pathOperator(op, operands, gs, mediaBox)
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

func (e *TextExtractor) showText(text string, gs *GraphicsState, mediaBox [4]float64) []TextItem {
	// A space wider than tjColumnGap ems (set wide with word spacing)
	// separates columns, as a wide move in a TJ array does: the text on
	// either side becomes a separate item
	var items []TextItem
	for _, part := range e.splitAtWideSpaces(text, gs) {
		if item := e.showTextPart(part, gs, mediaBox); item.Text != "" {
			items = append(items, item)
		}
	}
	return items
}

// splitAtWideSpaces splits the codes of a string after each single-byte
// space whose advance is wider than tjColumnGap ems
func (e *TextExtractor) splitAtWideSpaces(text string, gs *GraphicsState) []string {
	if font, ok := e.fonts[gs.FontName]; (ok && font.twoByte) || gs.FontSize == 0 {
		return []string{text}
	}
	if e.textAdvance(" ", gs) <= tjColumnGap*math.Abs(gs.FontSize) {
		return []string{text}
	}
	var parts []string
	start := 0
	for i := 0; i+1 < len(text); i++ {
		if text[i] == ' ' {
			parts = append(parts, text[start:i+1])
			start = i + 1
		}
	}
	return append(parts, text[start:])
}

// showTextPart shows text as one item and advances the text position
func (e *TextExtractor) showTextPart(text string, gs *GraphicsState, mediaBox [4]float64) TextItem {
	// Decode text using font encoding
	decodedText := e.decodeText(text, gs.FontName)

	// Calculate position
	tm := e.multiplyMatrix(gs.TextMatrix, gs.CTM)

	x := tm[4]
	y := tm[5]

	// Transform Y coordinate (PDF origin is bottom-left)
	pageHeight := mediaBox[3] - mediaBox[1]
	y = pageHeight - y

	// Calculate dimensions: text space scales to the page by the length of
	// the text matrix's x axis on the page
	scale := math.Hypot(tm[0], tm[1])
	fontSize := gs.FontSize * scale
	advance := e.textAdvance(text, gs)

	// Advance text position
	gs.TextMatrix = e.multiplyMatrix([6]float64{1, 0, 0, 1, advance, 0}, gs.TextMatrix)

	return TextItem{
		X:       x,
		Y:       y,
		Width:   advance * scale,
		Height:  fontSize,
		Text:    decodedText,
		Font:    gs.FontName,
		Rotated: !upright(tm),
	}
}

// upright reports whether text drawn with a text rendering matrix runs left
// to right along a horizontal baseline
func upright(tm [6]float64) bool {
	return tm[0] > 0 && math.Abs(tm[1]) <= uprightTolerance*tm[0] && math.Abs(tm[2]) <= uprightTolerance*math.Abs(tm[3])
}

const (
	// tjColumnGap is the widest move, in ems, inside a TJ array that is
	// still a space within a line of text; a wider one separates columns
	tjColumnGap = 2.0

	// uprightTolerance is how far the text matrix may shear or rotate the
	// baseline, as a share of its scale, for text still to count as upright
	uprightTolerance = 0.01

	// glyphRunMaxRunes is the longest text that is treated as a glyph
	// fragment and joined onto the text before it
	glyphRunMaxRunes = 3

	// glyphRunMinShare is the share of a page's text items that must be
	// glyph fragments for the page to count as drawn glyph by glyph
	glyphRunMinShare = 0.8

	// glyphRunMinItems is the fewest text items a page needs for that share
	// to mean anything
	glyphRunMinItems = 20

	// glyphRunJoinGap is the largest gap, as a fraction of the font size,
	// between two fragments of the same word
	glyphRunJoinGap = 0.18

	// glyphRunSpaceGap is the largest gap, as a fraction of the font size,
	// that is a space between two words rather than a column gap
	glyphRunSpaceGap = 0.6
)

// mergeGlyphRuns joins text that a PDF draws one glyph (or a few) at a time.
//
// Some producers position every glyph with its own show-text operation and
// draw no space characters at all: words are separated only by a wider gap.
// Left as separate items, the gaps are too small for later stages to tell
// apart from kerning, and the words run together. Here the exact glyph
// widths are still known, so fragments that touch are joined and a space is
// inserted where the gap is the size of a word space.
//
// This applies only to pages drawn almost entirely that way, and only to
// runs made up of short fragments. Pages that draw whole words or lines per
// operation are returned unchanged, short fragments included.
func mergeGlyphRuns(items []TextItem) []TextItem {
	if !drawnGlyphByGlyph(items) {
		return items
	}

	merged := make([]TextItem, 0, len(items))

	for _, item := range items {
		if len(merged) > 0 {
			prev := &merged[len(merged)-1]
			if gap, ok := glyphRunGap(prev, &item); ok {
				if gap > glyphRunJoinGap*prev.Height && !strings.HasSuffix(prev.Text, " ") &&
					!strings.HasPrefix(item.Text, " ") {
					prev.Text += " "
				}
				prev.Text += item.Text
				prev.Width = item.X + item.Width - prev.X
				continue
			}
		}
		item.glyphRun = isGlyphFragment(&item)
		merged = append(merged, item)
	}

	return merged
}

// drawnGlyphByGlyph reports whether nearly all text on a page is drawn as
// separate glyph fragments
func drawnGlyphByGlyph(items []TextItem) bool {
	if len(items) < glyphRunMinItems {
		return false
	}
	fragments := 0
	for i := range items {
		if isGlyphFragment(&items[i]) {
			fragments++
		}
	}
	return float64(fragments) >= glyphRunMinShare*float64(len(items))
}

// isGlyphFragment reports whether an item is a glyph or two drawn on its own
func isGlyphFragment(item *TextItem) bool {
	return item.Width > 0 && utf8.RuneCountInString(item.Text) <= glyphRunMaxRunes
}

// glyphRunGap returns the horizontal gap between two items if next is a
// glyph fragment that continues the line of text prev is on
func glyphRunGap(prev, next *TextItem) (float64, bool) {
	if !prev.glyphRun || !isGlyphFragment(next) || prev.Height <= 0 {
		return 0, false
	}
	if prev.Font != next.Font || math.Abs(prev.Height-next.Height) > 0.01*prev.Height {
		return 0, false
	}
	if math.Abs(prev.Y-next.Y) > 0.1*prev.Height {
		return 0, false
	}
	gap := next.X - (prev.X + prev.Width)
	if gap < -glyphRunJoinGap*prev.Height || gap > glyphRunSpaceGap*prev.Height {
		return 0, false
	}
	return gap, true
}

func (e *TextExtractor) showTextArray(operands []interface{}, gs *GraphicsState, items []TextItem, mediaBox [4]float64) []TextItem {
	// Find the array in operands
	var textArray []interface{}

	// Look for array content
	inArray := false
	for _, op := range operands {
		if d, ok := op.(delimiter); ok {
			if d == "[" {
				inArray = true
				textArray = []interface{}{}
				continue
			}
			if d == "]" {
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

	// Process array elements. pos is the text position in text space relative
	// to where the array starts; an item runs from the first glyph shown to
	// the end of the last, without leading or trailing adjustments. A move
	// wider than tjColumnGap ems is a gap between columns, not a word space:
	// the text on either side becomes a separate item.
	var currentText strings.Builder
	startTM := gs.TextMatrix
	var pos, textStart, textEnd float64
	shown := false

	emit := func() {
		if currentText.Len() == 0 {
			return
		}
		// Text space to page: the size on the page comes from the text
		// matrix as well as the CTM, as many producers set a font size of 1
		// and scale with Tm
		tm := e.multiplyMatrix(startTM, gs.CTM)
		scale := math.Hypot(tm[0], tm[1])
		fontSize := gs.FontSize * scale

		x := textStart*tm[0] + tm[4]
		y := textStart*tm[1] + tm[5]

		pageHeight := mediaBox[3] - mediaBox[1]
		y = pageHeight - y

		items = append(items, TextItem{
			X:       x,
			Y:       y,
			Width:   (textEnd - textStart) * scale,
			Height:  fontSize,
			Text:    currentText.String(),
			Font:    gs.FontName,
			Rotated: !upright(tm),
		})
		currentText.Reset()
		shown = false
	}

	for _, elem := range textArray {
		switch v := elem.(type) {
		case string:
			// A space set wider than tjColumnGap ems ends an item, as
			// in a Tj string
			parts := e.splitAtWideSpaces(v, gs)
			for k, part := range parts {
				currentText.WriteString(e.decodeText(part, gs.FontName))
				if !shown && part != "" {
					textStart = pos
					shown = true
				}
				advance := e.textAdvance(part, gs)
				pos += advance
				textEnd = pos
				gs.TextMatrix = e.multiplyMatrix([6]float64{1, 0, 0, 1, advance, 0}, gs.TextMatrix)
				if k < len(parts)-1 {
					emit()
				}
			}
		case float64:
			// Negative value moves text right, positive moves left
			adjustment := -v / 1000.0 * gs.FontSize * (gs.HorizScaling / 100.0)
			pos += adjustment

			if -v > tjColumnGap*1000 {
				emit()
			} else if math.Abs(v) > 300 && currentText.Len() > 0 {
				// If significant adjustment (space), emit current text
				// Normal word spacing is ~250-333/1000 em (0.25-0.33 em)
				// Use 300 as threshold to avoid false word breaks from kerning
				// Kerning adjustments are typically -50 to -150 units
				currentText.WriteString(" ")
			}

			gs.TextMatrix = e.multiplyMatrix([6]float64{1, 0, 0, 1, adjustment, 0}, gs.TextMatrix)
		}
	}
	emit()

	return items
}

func (e *TextExtractor) decodeText(text string, fontName string) string {
	font, ok := e.fonts[fontName]
	if !ok {
		// No font info, try basic decoding
		return e.basicDecode(text, "")
	}

	// Use CMap if available. A Type0 font's codes are 2 bytes whatever
	// codespace its ToUnicode CMap declares (some declare 1-byte codes too,
	// which would split every code in two)
	if font.ToUnicode != nil {
		if font.twoByte {
			return font.ToUnicode.decodeTwoByte([]byte(text))
		}
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

// GetFonts returns the loaded fonts
func (e *TextExtractor) GetFonts() map[string]*Font {
	return e.fonts
}

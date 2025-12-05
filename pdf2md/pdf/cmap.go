package pdf

import (
	"regexp"
	"strconv"
	"strings"
)

// CMap represents a character code to Unicode mapping
type CMap struct {
	// codespaceRanges stores ranges for 1-byte, 2-byte, 3-byte, 4-byte codes
	codespaceRanges [4][][2]int
	// mapping stores character code to Unicode string mappings
	mapping map[int]string
	// name is the CMap name
	name string
	// vertical indicates vertical writing mode
	vertical bool
}

// NewCMap creates a new CMap
func NewCMap() *CMap {
	return &CMap{
		codespaceRanges: [4][][2]int{{}, {}, {}, {}},
		mapping:         make(map[int]string),
	}
}

// AddCodespaceRange adds a codespace range for n-byte codes
func (c *CMap) AddCodespaceRange(n int, low, high int) {
	if n >= 1 && n <= 4 {
		c.codespaceRanges[n-1] = append(c.codespaceRanges[n-1], [2]int{low, high})
	}
}

// MapOne maps a single character code to a Unicode string
func (c *CMap) MapOne(src int, dst string) {
	c.mapping[src] = dst
}

// MapBfRange maps a range of character codes to sequential Unicode values
func (c *CMap) MapBfRange(low, high int, dstLow string) {
	// Limit range to prevent hangs on malformed data
	const maxRange = 0xFFFFFF
	if high-low > maxRange {
		return
	}

	dst := []byte(dstLow)
	for code := low; code <= high; code++ {
		c.mapping[code] = string(dst)
		// Increment the destination string (treating as big-endian number)
		incrementBytes(dst)
	}
}

// MapBfRangeToArray maps a range of character codes to an array of Unicode strings
func (c *CMap) MapBfRangeToArray(low, high int, array []string) {
	for i, code := 0, low; code <= high && i < len(array); i, code = i+1, code+1 {
		c.mapping[code] = array[i]
	}
}

// Lookup returns the Unicode string for a character code
func (c *CMap) Lookup(code int) (string, bool) {
	dst, ok := c.mapping[code]
	return dst, ok
}

// GetCharCodeLength returns the byte length for a given character code
func (c *CMap) GetCharCodeLength(charCode int) int {
	for n := 0; n < 4; n++ {
		for _, r := range c.codespaceRanges[n] {
			if charCode >= r[0] && charCode <= r[1] {
				return n + 1
			}
		}
	}
	return 1
}

// incrementBytes increments a byte slice as a big-endian number
func incrementBytes(b []byte) {
	for i := len(b) - 1; i >= 0; i-- {
		b[i]++
		if b[i] != 0 {
			break
		}
	}
}

// ParseCMap parses a ToUnicode CMap stream
func ParseCMap(data []byte) *CMap {
	cmap := NewCMap()
	content := string(data)

	// Parse codespace ranges
	parseCodespaceRanges(content, cmap)

	// Parse bfchar mappings (must be done within beginbfchar/endbfchar sections)
	parseBfChar(content, cmap)

	// Parse bfrange mappings (must be done within beginbfrange/endbfrange sections)
	parseBfRange(content, cmap)

	return cmap
}

// parseCodespaceRanges parses codespace range definitions
func parseCodespaceRanges(content string, cmap *CMap) {
	// Find codespacerange sections
	csRangeRe := regexp.MustCompile(`(?s)begincodespacerange\s*(.*?)\s*endcodespacerange`)
	sections := csRangeRe.FindAllStringSubmatch(content, -1)

	hexPairRe := regexp.MustCompile(`<([0-9A-Fa-f]+)>\s*<([0-9A-Fa-f]+)>`)

	for _, section := range sections {
		matches := hexPairRe.FindAllStringSubmatch(section[1], -1)
		for _, match := range matches {
			low := hexStringToInt(match[1])
			high := hexStringToInt(match[2])
			// Byte length is determined by hex string length / 2
			byteLen := len(match[1]) / 2
			if byteLen < 1 {
				byteLen = 1
			}
			cmap.AddCodespaceRange(byteLen, low, high)
		}
	}
}

// parseBfChar parses bfchar mappings
func parseBfChar(content string, cmap *CMap) {
	// Find bfchar sections
	bfcharRe := regexp.MustCompile(`(?s)beginbfchar\s*(.*?)\s*endbfchar`)
	sections := bfcharRe.FindAllStringSubmatch(content, -1)

	// Match pairs of hex strings: <src> <dst>
	pairRe := regexp.MustCompile(`<([0-9A-Fa-f]+)>\s*<([0-9A-Fa-f]+)>`)

	for _, section := range sections {
		matches := pairRe.FindAllStringSubmatch(section[1], -1)
		for _, match := range matches {
			src := hexStringToInt(match[1])
			dst := hexStringToBytes(match[2])
			cmap.MapOne(src, string(dst))
		}
	}
}

// parseBfRange parses bfrange mappings
func parseBfRange(content string, cmap *CMap) {
	// Find bfrange sections
	bfrangeRe := regexp.MustCompile(`(?s)beginbfrange\s*(.*?)\s*endbfrange`)
	sections := bfrangeRe.FindAllStringSubmatch(content, -1)

	// Match range definitions: <low> <high> <dst> or <low> <high> [array]
	// Simple range: <low> <high> <dst>
	simpleRangeRe := regexp.MustCompile(`<([0-9A-Fa-f]+)>\s*<([0-9A-Fa-f]+)>\s*<([0-9A-Fa-f]+)>`)
	// Array range: <low> <high> [<val1><val2>...]
	arrayRangeRe := regexp.MustCompile(`<([0-9A-Fa-f]+)>\s*<([0-9A-Fa-f]+)>\s*\[(.*?)\]`)

	for _, section := range sections {
		sectionContent := section[1]

		// Parse array ranges first (they're more specific)
		arrayMatches := arrayRangeRe.FindAllStringSubmatch(sectionContent, -1)
		for _, match := range arrayMatches {
			low := hexStringToInt(match[1])
			high := hexStringToInt(match[2])
			arrayContent := match[3]

			// Extract hex values from array
			hexValRe := regexp.MustCompile(`<([0-9A-Fa-f]+)>`)
			hexMatches := hexValRe.FindAllStringSubmatch(arrayContent, -1)
			var array []string
			for _, hm := range hexMatches {
				array = append(array, string(hexStringToBytes(hm[1])))
			}
			cmap.MapBfRangeToArray(low, high, array)
		}

		// Remove array ranges from content to avoid double-matching
		cleanedContent := arrayRangeRe.ReplaceAllString(sectionContent, "")

		// Parse simple ranges
		simpleMatches := simpleRangeRe.FindAllStringSubmatch(cleanedContent, -1)
		for _, match := range simpleMatches {
			low := hexStringToInt(match[1])
			high := hexStringToInt(match[2])
			dst := hexStringToBytes(match[3])
			cmap.MapBfRange(low, high, string(dst))
		}
	}
}

// hexStringToInt converts a hex string to an integer
func hexStringToInt(hex string) int {
	val, _ := strconv.ParseInt(hex, 16, 64)
	return int(val)
}

// hexStringToBytes converts a hex string to bytes
// This preserves the byte representation for proper Unicode handling
func hexStringToBytes(hex string) []byte {
	// Ensure even length
	if len(hex)%2 != 0 {
		hex = "0" + hex
	}

	result := make([]byte, len(hex)/2)
	for i := 0; i < len(hex); i += 2 {
		val, _ := strconv.ParseInt(hex[i:i+2], 16, 32)
		result[i/2] = byte(val)
	}
	return result
}

// DecodeString decodes a string using this CMap
func (c *CMap) DecodeString(data []byte) string {
	var result strings.Builder

	// Determine if we should use 2-byte or 1-byte decoding
	// by checking the codespace ranges
	maxByteLen := 1
	for n := 3; n >= 0; n-- {
		if len(c.codespaceRanges[n]) > 0 {
			maxByteLen = n + 1
			break
		}
	}

	i := 0
	for i < len(data) {
		// Try to match the longest possible code first
		matched := false
		for byteLen := min(maxByteLen, len(data)-i); byteLen >= 1; byteLen-- {
			code := 0
			for j := 0; j < byteLen; j++ {
				code = (code << 8) | int(data[i+j])
			}

			// Check if this code is in a valid codespace range
			inRange := false
			if byteLen <= 4 {
				for _, r := range c.codespaceRanges[byteLen-1] {
					if code >= r[0] && code <= r[1] {
						inRange = true
						break
					}
				}
			}

			if inRange || byteLen == 1 {
				if dst, ok := c.mapping[code]; ok {
					// The destination is stored as bytes representing Unicode
					// Convert to proper UTF-8
					result.WriteString(bytesToUTF8(dst))
					i += byteLen
					matched = true
					break
				}
			}
		}

		if !matched {
			// No mapping found, try single byte
			code := int(data[i])
			if dst, ok := c.mapping[code]; ok {
				result.WriteString(bytesToUTF8(dst))
			} else {
				// Output the raw byte as-is
				result.WriteByte(data[i])
			}
			i++
		}
	}

	return result.String()
}

// bytesToUTF8 converts CMap destination bytes to UTF-8 string
// CMap destinations are stored as big-endian Unicode code points
func bytesToUTF8(s string) string {
	data := []byte(s)

	// Handle based on length
	switch len(data) {
	case 1:
		// Single byte - treat as Latin-1 / Unicode code point
		return string(rune(data[0]))
	case 2:
		// Two bytes - UTF-16BE code unit
		codePoint := int(data[0])<<8 | int(data[1])
		return string(rune(codePoint))
	case 4:
		// Four bytes - could be a surrogate pair or two code points
		// Check if it's a surrogate pair
		high := int(data[0])<<8 | int(data[1])
		low := int(data[2])<<8 | int(data[3])
		if high >= 0xD800 && high <= 0xDBFF && low >= 0xDC00 && low <= 0xDFFF {
			// Surrogate pair
			codePoint := 0x10000 + ((high-0xD800)<<10 | (low - 0xDC00))
			return string(rune(codePoint))
		}
		// Two separate code points
		return string(rune(high)) + string(rune(low))
	default:
		// For other lengths, try to decode as UTF-16BE
		var result strings.Builder
		for i := 0; i+1 < len(data); i += 2 {
			codePoint := int(data[i])<<8 | int(data[i+1])
			result.WriteRune(rune(codePoint))
		}
		// Handle odd trailing byte
		if len(data)%2 != 0 {
			result.WriteByte(data[len(data)-1])
		}
		return result.String()
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

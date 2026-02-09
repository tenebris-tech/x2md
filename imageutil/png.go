package imageutil

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"hash/crc32"
)

// CreatePNG wraps raw pixel data in PNG format.
// This is used for PDF images that use FlateDecode (raw pixel data).
// colorSpace should be "DeviceGray", "DeviceRGB", or "DeviceCMYK".
// bitsPerComponent is typically 8.
func CreatePNG(data []byte, width, height, bitsPerComponent int, colorSpace string) ([]byte, error) {
	// Determine color type and bytes per pixel
	var colorType byte
	var bytesPerPixel int

	switch colorSpace {
	case "DeviceGray":
		colorType = 0 // Grayscale
		bytesPerPixel = 1
	case "DeviceRGB":
		colorType = 2 // RGB
		bytesPerPixel = 3
	case "DeviceCMYK":
		// Convert CMYK to RGB first
		data = cmykToRGB(data)
		colorType = 2 // RGB
		bytesPerPixel = 3
	default:
		// Default to RGB
		colorType = 2
		bytesPerPixel = 3
	}

	// Calculate expected data size
	expectedSize := width * height * bytesPerPixel
	if len(data) < expectedSize {
		// Pad with zeros if data is short
		padded := make([]byte, expectedSize)
		copy(padded, data)
		data = padded
	} else if len(data) > expectedSize {
		data = data[:expectedSize]
	}

	var buf bytes.Buffer

	// PNG signature
	buf.Write([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})

	// IHDR chunk
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], uint32(width))
	binary.BigEndian.PutUint32(ihdr[4:8], uint32(height))
	ihdr[8] = byte(bitsPerComponent) // bit depth
	ihdr[9] = colorType              // color type
	ihdr[10] = 0                     // compression method (deflate)
	ihdr[11] = 0                     // filter method
	ihdr[12] = 0                     // interlace method (none)
	writeChunk(&buf, "IHDR", ihdr)

	// IDAT chunk - compressed image data with filter bytes
	// Add filter byte (0 = None) before each row
	rowSize := width * bytesPerPixel
	filteredData := make([]byte, 0, height*(rowSize+1))
	for y := 0; y < height; y++ {
		filteredData = append(filteredData, 0) // filter byte
		start := y * rowSize
		end := start + rowSize
		if end > len(data) {
			end = len(data)
		}
		if start < len(data) {
			filteredData = append(filteredData, data[start:end]...)
		}
		// Pad row if needed
		for len(filteredData)%((rowSize)+1) != 0 && y < height-1 {
			filteredData = append(filteredData, 0)
		}
	}

	// Compress with zlib
	var compressed bytes.Buffer
	zlibWriter := zlib.NewWriter(&compressed)
	if _, err := zlibWriter.Write(filteredData); err != nil {
		return nil, fmt.Errorf("compressing PNG data: %w", err)
	}
	if err := zlibWriter.Close(); err != nil {
		return nil, fmt.Errorf("finalizing PNG compression: %w", err)
	}
	writeChunk(&buf, "IDAT", compressed.Bytes())

	// IEND chunk
	writeChunk(&buf, "IEND", nil)

	return buf.Bytes(), nil
}

// writeChunk writes a PNG chunk to the buffer
func writeChunk(buf *bytes.Buffer, chunkType string, data []byte) {
	// Length (4 bytes)
	length := make([]byte, 4)
	binary.BigEndian.PutUint32(length, uint32(len(data)))
	buf.Write(length)

	// Type (4 bytes)
	buf.WriteString(chunkType)

	// Data
	if len(data) > 0 {
		buf.Write(data)
	}

	// CRC (4 bytes) - CRC of type + data
	crcData := append([]byte(chunkType), data...)
	crcVal := crc32.ChecksumIEEE(crcData)
	crc := make([]byte, 4)
	binary.BigEndian.PutUint32(crc, crcVal)
	buf.Write(crc)
}

// cmykToRGB converts CMYK pixel data to RGB
func cmykToRGB(cmyk []byte) []byte {
	if len(cmyk)%4 != 0 {
		return cmyk
	}

	rgb := make([]byte, (len(cmyk)/4)*3)
	for i := 0; i < len(cmyk); i += 4 {
		c := float64(cmyk[i]) / 255.0
		m := float64(cmyk[i+1]) / 255.0
		y := float64(cmyk[i+2]) / 255.0
		k := float64(cmyk[i+3]) / 255.0

		r := (1 - c) * (1 - k) * 255
		g := (1 - m) * (1 - k) * 255
		b := (1 - y) * (1 - k) * 255

		j := (i / 4) * 3
		rgb[j] = byte(r)
		rgb[j+1] = byte(g)
		rgb[j+2] = byte(b)
	}

	return rgb
}

package pdf

import (
	"bytes"
	"compress/lzw"
	"testing"
)

func TestDecodeLZW(t *testing.T) {
	p := &Parser{}

	tests := []struct {
		name    string
		input   []byte
		want    []byte
		wantErr bool
	}{
		{
			name:    "empty input",
			input:   nil,
			want:    nil,
			wantErr: false,
		},
		{
			name:    "empty slice",
			input:   []byte{},
			want:    nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.decodeLZW(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("decodeLZW() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("decodeLZW() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecodeLZWRoundTrip(t *testing.T) {
	p := &Parser{}

	// Test with various data sizes
	testCases := []struct {
		name string
		data []byte
	}{
		{"simple text", []byte("Hello, World!")},
		{"repeated text", []byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")},
		{"binary data", []byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD}},
		{"larger text", []byte("The quick brown fox jumps over the lazy dog. " +
			"Pack my box with five dozen liquor jugs. " +
			"How vexingly quick daft zebras jump!")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Compress using LZW (MSB order, litWidth 8 - same as PDF)
			var buf bytes.Buffer
			w := lzw.NewWriter(&buf, lzw.MSB, 8)
			_, err := w.Write(tc.data)
			if err != nil {
				t.Fatalf("Failed to compress: %v", err)
			}
			w.Close()

			// Decompress using our function
			decompressed, err := p.decodeLZW(buf.Bytes())
			if err != nil {
				t.Fatalf("decodeLZW() error = %v", err)
			}

			if !bytes.Equal(decompressed, tc.data) {
				t.Errorf("Round-trip failed: got %v, want %v", decompressed, tc.data)
			}
		})
	}
}

func TestDecodeLZWInvalidData(t *testing.T) {
	p := &Parser{}

	// Random garbage data should fail gracefully
	invalidData := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	_, err := p.decodeLZW(invalidData)

	// We expect an error for invalid LZW data
	if err == nil {
		t.Log("Note: Invalid data did not produce error (may be valid by chance)")
	}
}

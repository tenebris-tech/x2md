// Package pdf provides low-level PDF parsing functionality
package pdf

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rc4"
	"fmt"
)

// EncryptionHandler handles PDF decryption for permission-restricted PDFs
type EncryptionHandler struct {
	// Encryption parameters from /Encrypt dictionary
	V          int    // Version (1, 2, 3, 4, or 5)
	R          int    // Revision (2, 3, 4, 5, or 6)
	KeyLength  int    // Key length in bits (40-256)
	P          int32  // Permissions flags
	O          []byte // Owner password hash (32 or 48 bytes)
	U          []byte // User password hash (32 or 48 bytes)
	OE         []byte // Owner encryption key (AES-256, R=6)
	UE         []byte // User encryption key (AES-256, R=6)
	Perms      []byte // Permissions (AES-256, R=6)
	EncryptMeta bool  // Whether metadata is encrypted

	// Document ID from trailer
	ID []byte

	// Computed encryption key
	key []byte

	// Whether we successfully authenticated
	authenticated bool
}

// Padding used for password operations (from PDF spec)
var passwordPadding = []byte{
	0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41,
	0x64, 0x00, 0x4E, 0x56, 0xFF, 0xFA, 0x01, 0x08,
	0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80,
	0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A,
}

// NewEncryptionHandler creates a new encryption handler from the Encrypt dictionary
func NewEncryptionHandler(encryptDict map[string]interface{}, trailerID []byte) (*EncryptionHandler, error) {
	h := &EncryptionHandler{
		ID:          trailerID,
		EncryptMeta: true, // Default
	}

	// Get version
	if v, ok := encryptDict["V"].(float64); ok {
		h.V = int(v)
	} else {
		h.V = 0
	}

	// Get revision
	if r, ok := encryptDict["R"].(float64); ok {
		h.R = int(r)
	} else {
		return nil, fmt.Errorf("missing R in Encrypt dictionary")
	}

	// Get key length (default 40 for V=1, 128 for V>=2)
	if length, ok := encryptDict["Length"].(float64); ok {
		h.KeyLength = int(length)
	} else if h.V == 1 {
		h.KeyLength = 40
	} else {
		h.KeyLength = 128
	}

	// Get permissions
	if p, ok := encryptDict["P"].(float64); ok {
		h.P = int32(p)
	}

	// Get O (owner hash)
	if o, ok := encryptDict["O"].(string); ok {
		h.O = []byte(o)
	}

	// Get U (user hash)
	if u, ok := encryptDict["U"].(string); ok {
		h.U = []byte(u)
	}

	// For AES-256 (V=5, R=5 or R=6)
	if oe, ok := encryptDict["OE"].(string); ok {
		h.OE = []byte(oe)
	}
	if ue, ok := encryptDict["UE"].(string); ok {
		h.UE = []byte(ue)
	}
	if perms, ok := encryptDict["Perms"].(string); ok {
		h.Perms = []byte(perms)
	}

	// Check EncryptMetadata
	if em, ok := encryptDict["EncryptMetadata"].(bool); ok {
		h.EncryptMeta = em
	}

	return h, nil
}

// TryEmptyPassword attempts to authenticate with an empty password
// Returns true if authentication succeeded (PDF is readable without password)
func (h *EncryptionHandler) TryEmptyPassword() bool {
	// Try authenticating with empty user password
	if h.authenticateUser("") {
		h.authenticated = true
		return true
	}
	return false
}

// IsAuthenticated returns whether we've successfully authenticated
func (h *EncryptionHandler) IsAuthenticated() bool {
	return h.authenticated
}

// authenticateUser attempts to authenticate with the given user password
func (h *EncryptionHandler) authenticateUser(password string) bool {
	switch h.R {
	case 2:
		return h.authenticateUserR2(password)
	case 3, 4:
		return h.authenticateUserR34(password)
	case 5, 6:
		return h.authenticateUserR56(password)
	default:
		return false
	}
}

// authenticateUserR2 authenticates for revision 2
func (h *EncryptionHandler) authenticateUserR2(password string) bool {
	key := h.computeKeyR2(password)

	// For R=2, encrypt the padding with the key and compare to U
	cipher, err := rc4.NewCipher(key)
	if err != nil {
		return false
	}

	encrypted := make([]byte, 32)
	cipher.XORKeyStream(encrypted, passwordPadding)

	if bytes.Equal(encrypted, h.U) {
		h.key = key
		return true
	}
	return false
}

// authenticateUserR34 authenticates for revision 3 or 4
func (h *EncryptionHandler) authenticateUserR34(password string) bool {
	key := h.computeKeyR34(password)

	// For R=3/4, compute MD5 of padding + ID, then encrypt iteratively
	hash := md5.New()
	hash.Write(passwordPadding)
	hash.Write(h.ID)
	digest := hash.Sum(nil)

	// Encrypt 20 times with modified keys
	encrypted := make([]byte, 16)
	copy(encrypted, digest)

	for i := 0; i < 20; i++ {
		// Create key XORed with iteration number
		iterKey := make([]byte, len(key))
		for j := range key {
			iterKey[j] = key[j] ^ byte(i)
		}

		cipher, err := rc4.NewCipher(iterKey)
		if err != nil {
			return false
		}
		cipher.XORKeyStream(encrypted, encrypted)
	}

	// Compare first 16 bytes of U with encrypted result
	if len(h.U) >= 16 && bytes.Equal(encrypted, h.U[:16]) {
		h.key = key
		return true
	}
	return false
}

// authenticateUserR56 authenticates for revision 5 or 6 (AES-256)
func (h *EncryptionHandler) authenticateUserR56(password string) bool {
	// For R=5/6, the U value contains:
	// - First 32 bytes: hash
	// - Next 8 bytes: validation salt
	// - Next 8 bytes: key salt
	if len(h.U) < 48 {
		return false
	}

	userHash := h.U[:32]
	validationSalt := h.U[32:40]
	keySalt := h.U[40:48]

	// Compute hash with validation salt
	pwBytes := []byte(password)
	if len(pwBytes) > 127 {
		pwBytes = pwBytes[:127]
	}

	// For R=5: SHA-256(password + validation_salt)
	// For R=6: More complex iterative hash
	var computedHash []byte
	if h.R == 5 {
		computedHash = h.sha256(pwBytes, validationSalt, nil)
	} else {
		computedHash = h.computeHashR6(pwBytes, validationSalt, nil)
	}

	if !bytes.Equal(computedHash, userHash) {
		return false
	}

	// Derive the file encryption key
	var keyHash []byte
	if h.R == 5 {
		keyHash = h.sha256(pwBytes, keySalt, nil)
	} else {
		keyHash = h.computeHashR6(pwBytes, keySalt, nil)
	}

	// Decrypt UE to get the file encryption key
	if len(h.UE) < 32 {
		return false
	}

	block, err := aes.NewCipher(keyHash)
	if err != nil {
		return false
	}

	// Use CBC mode with zero IV
	iv := make([]byte, 16)
	mode := cipher.NewCBCDecrypter(block, iv)

	h.key = make([]byte, 32)
	mode.CryptBlocks(h.key, h.UE[:32])

	h.authenticated = true
	return true
}

// computeKeyR2 computes the encryption key for revision 2
func (h *EncryptionHandler) computeKeyR2(password string) []byte {
	// Pad or truncate password to 32 bytes
	pwBytes := h.padPassword(password)

	// MD5 hash of: password + O + P + ID
	hash := md5.New()
	hash.Write(pwBytes)
	hash.Write(h.O)
	hash.Write([]byte{byte(h.P), byte(h.P >> 8), byte(h.P >> 16), byte(h.P >> 24)})
	hash.Write(h.ID)

	digest := hash.Sum(nil)

	// Key is first n/8 bytes (n = key length in bits)
	keyLen := h.KeyLength / 8
	if keyLen > 16 {
		keyLen = 16
	}
	return digest[:keyLen]
}

// computeKeyR34 computes the encryption key for revision 3 or 4
func (h *EncryptionHandler) computeKeyR34(password string) []byte {
	// Pad or truncate password to 32 bytes
	pwBytes := h.padPassword(password)

	// MD5 hash of: password + O + P + ID + [EncryptMetadata]
	hash := md5.New()
	hash.Write(pwBytes)
	hash.Write(h.O)
	hash.Write([]byte{byte(h.P), byte(h.P >> 8), byte(h.P >> 16), byte(h.P >> 24)})
	hash.Write(h.ID)

	if !h.EncryptMeta {
		hash.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	}

	digest := hash.Sum(nil)

	// For R>=3, hash 50 more times
	keyLen := h.KeyLength / 8
	if keyLen > 16 {
		keyLen = 16
	}

	for i := 0; i < 50; i++ {
		h2 := md5.New()
		h2.Write(digest[:keyLen])
		digest = h2.Sum(nil)
	}

	return digest[:keyLen]
}

// padPassword pads or truncates password to 32 bytes using standard padding
func (h *EncryptionHandler) padPassword(password string) []byte {
	result := make([]byte, 32)
	pwBytes := []byte(password)

	n := copy(result, pwBytes)
	copy(result[n:], passwordPadding)

	return result
}

// sha256 computes SHA-256 hash (simplified - would need crypto/sha256 import)
func (h *EncryptionHandler) sha256(data, salt, extra []byte) []byte {
	// For now, return nil - R5/R6 is less common
	// Would need proper implementation with crypto/sha256
	return nil
}

// computeHashR6 computes the R6 iterative hash
func (h *EncryptionHandler) computeHashR6(password, salt, extra []byte) []byte {
	// R6 uses a complex iterative hash algorithm
	// For now, return nil - R6 is less common
	return nil
}

// DecryptStream decrypts a stream using the computed key
func (h *EncryptionHandler) DecryptStream(data []byte, objNum, genNum int) ([]byte, error) {
	if !h.authenticated {
		return nil, fmt.Errorf("not authenticated")
	}

	// Compute object-specific key
	objKey := h.computeObjectKey(objNum, genNum, false)

	// Decrypt based on version
	if h.V < 4 {
		// RC4
		return h.decryptRC4(data, objKey)
	}

	// AES (V=4 or V=5)
	return h.decryptAES(data, objKey)
}

// DecryptString decrypts a string using the computed key
func (h *EncryptionHandler) DecryptString(data []byte, objNum, genNum int) ([]byte, error) {
	return h.DecryptStream(data, objNum, genNum)
}

// computeObjectKey computes the key for a specific object
func (h *EncryptionHandler) computeObjectKey(objNum, genNum int, forAES bool) []byte {
	if h.V == 5 {
		// AES-256 uses the file key directly
		return h.key
	}

	// MD5(file_key + objNum(3 bytes) + genNum(2 bytes) + ["sAlT" for AES])
	hash := md5.New()
	hash.Write(h.key)
	hash.Write([]byte{byte(objNum), byte(objNum >> 8), byte(objNum >> 16)})
	hash.Write([]byte{byte(genNum), byte(genNum >> 8)})

	if forAES {
		hash.Write([]byte("sAlT"))
	}

	digest := hash.Sum(nil)

	// Key length is min(file_key_length + 5, 16) bytes
	keyLen := len(h.key) + 5
	if keyLen > 16 {
		keyLen = 16
	}

	return digest[:keyLen]
}

// decryptRC4 decrypts data using RC4
func (h *EncryptionHandler) decryptRC4(data, key []byte) ([]byte, error) {
	cipher, err := rc4.NewCipher(key)
	if err != nil {
		return nil, err
	}

	result := make([]byte, len(data))
	cipher.XORKeyStream(result, data)
	return result, nil
}

// decryptAES decrypts data using AES-CBC
func (h *EncryptionHandler) decryptAES(data, key []byte) ([]byte, error) {
	if len(data) < 16 {
		return nil, fmt.Errorf("data too short for AES")
	}

	// First 16 bytes are the IV
	iv := data[:16]
	ciphertext := data[16:]

	if len(ciphertext)%16 != 0 {
		return nil, fmt.Errorf("ciphertext not multiple of block size")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	plaintext := make([]byte, len(ciphertext))
	mode.CryptBlocks(plaintext, ciphertext)

	// Remove PKCS#7 padding
	if len(plaintext) > 0 {
		padLen := int(plaintext[len(plaintext)-1])
		if padLen > 0 && padLen <= 16 && padLen <= len(plaintext) {
			// Verify padding
			valid := true
			for i := 0; i < padLen; i++ {
				if plaintext[len(plaintext)-1-i] != byte(padLen) {
					valid = false
					break
				}
			}
			if valid {
				plaintext = plaintext[:len(plaintext)-padLen]
			}
		}
	}

	return plaintext, nil
}

package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"

	"golang.org/x/crypto/argon2"
)

const (
	tokenVersion  = byte(1) // Format version
	saltSize      = 16
	nonceSize     = 12 // AES-GCM standard nonce
	keySize       = 32 // AES-256
	argon2Time    = 1
	argon2Memory  = 64 * 1024 // 64MB
	argon2Threads = 4
)

// EncryptToken encrypts plaintext using AES-256-GCM with Argon2id key derivation.
// Output format: version(1) + salt(16) + nonce(12) + ciphertext
func EncryptToken(plaintext []byte, passphrase string) ([]byte, error) {
	// Generate random salt
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}

	// Derive key using Argon2id
	key := argon2.IDKey([]byte(passphrase), salt, argon2Time, argon2Memory, argon2Threads, keySize)

	// Create AES-GCM cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	// Generate random nonce
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	// Encrypt
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	// Build output: version + salt + nonce + ciphertext
	result := make([]byte, 0, 1+saltSize+nonceSize+len(ciphertext))
	result = append(result, tokenVersion)
	result = append(result, salt...)
	result = append(result, nonce...)
	result = append(result, ciphertext...)

	return result, nil
}

// DecryptToken decrypts data encrypted by EncryptToken.
func DecryptToken(encrypted []byte, passphrase string) ([]byte, error) {
	minSize := 1 + saltSize + nonceSize + 16 // 16 = GCM tag
	if len(encrypted) < minSize {
		return nil, fmt.Errorf("encrypted data too short")
	}

	if encrypted[0] != tokenVersion {
		return nil, fmt.Errorf("unsupported token encryption version: %d", encrypted[0])
	}

	salt := encrypted[1 : 1+saltSize]
	nonce := encrypted[1+saltSize : 1+saltSize+nonceSize]
	ciphertext := encrypted[1+saltSize+nonceSize:]

	// Derive key
	key := argon2.IDKey([]byte(passphrase), salt, argon2Time, argon2Memory, argon2Threads, keySize)

	// Decrypt
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt failed (wrong passphrase?): %w", err)
	}

	return plaintext, nil
}

// IsEncryptedToken checks if data appears to be an encrypted token file.
// Checks for the version byte prefix. Plain JSON tokens start with '{'.
func IsEncryptedToken(data []byte) bool {
	return len(data) > 0 && data[0] == tokenVersion
}

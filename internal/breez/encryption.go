package breez

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	log "github.com/sirupsen/logrus"
)

// EncryptMnemonic encrypts a mnemonic phrase using AES-256-GCM
// Returns the encrypted data as a hex-encoded string
func EncryptMnemonic(mnemonic, keyHex string) (string, error) {
	if mnemonic == "" {
		return "", fmt.Errorf("mnemonic cannot be empty")
	}

	if keyHex == "" {
		return "", fmt.Errorf("encryption key cannot be empty")
	}

	// Decode the hex key
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return "", fmt.Errorf("failed to decode encryption key: %w", err)
	}

	// Validate key length (must be 32 bytes for AES-256)
	if len(key) != 32 {
		return "", fmt.Errorf("encryption key must be 32 bytes (64 hex chars), got %d bytes", len(key))
	}

	// Create AES cipher block
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher block: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate a random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the mnemonic
	// The result will be: nonce + ciphertext + tag
	ciphertext := gcm.Seal(nonce, nonce, []byte(mnemonic), nil)

	// Encode to hex for storage
	encrypted := hex.EncodeToString(ciphertext)

	log.Debug("[Breez] Mnemonic encrypted successfully")
	return encrypted, nil
}

// DecryptMnemonic decrypts a mnemonic phrase that was encrypted with EncryptMnemonic
func DecryptMnemonic(encryptedHex, keyHex string) (string, error) {
	if encryptedHex == "" {
		return "", fmt.Errorf("encrypted data cannot be empty")
	}

	if keyHex == "" {
		return "", fmt.Errorf("encryption key cannot be empty")
	}

	// Decode the hex key
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return "", fmt.Errorf("failed to decode encryption key: %w", err)
	}

	// Validate key length
	if len(key) != 32 {
		return "", fmt.Errorf("encryption key must be 32 bytes (64 hex chars), got %d bytes", len(key))
	}

	// Decode the encrypted data from hex
	ciphertext, err := hex.DecodeString(encryptedHex)
	if err != nil {
		return "", fmt.Errorf("failed to decode encrypted data: %w", err)
	}

	// Create AES cipher block
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher block: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Validate ciphertext length
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	// Extract nonce and actual ciphertext
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt mnemonic: %w", err)
	}

	log.Debug("[Breez] Mnemonic decrypted successfully")
	return string(plaintext), nil
}

package breez

import (
	"crypto/rand"
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/tyler-smith/go-bip39"
)

// GenerateWallet generates a new wallet with a random mnemonic
func (c *Client) GenerateWallet() (*WalletInfo, error) {
	if c.initialized {
		return nil, fmt.Errorf("client already initialized with existing wallet")
	}

	log.Info("[Breez] Generating new wallet")

	// Generate a new mnemonic using BIP39
	mnemonic, err := GenerateMnemonic()
	if err != nil {
		return nil, fmt.Errorf("failed to generate mnemonic: %w", err)
	}

	// Store the mnemonic
	c.mnemonic = mnemonic
	c.config.Mnemonic = mnemonic

	// Convert mnemonic to seed
	seed := bip39.NewSeed(mnemonic, "") // Empty passphrase

	walletInfo := &WalletInfo{
		Mnemonic:   mnemonic,
		Seed:       seed,
		PublicKey:  "", // Will be available after Initialize()
		Descriptor: "", // Will be available after Initialize()
	}

	log.Info("[Breez] Wallet generated successfully with new mnemonic")
	return walletInfo, nil
}

// ConnectWallet connects to an existing wallet using a mnemonic
func (c *Client) ConnectWallet(mnemonic string) error {
	if c.initialized {
		return fmt.Errorf("client already initialized")
	}

	if mnemonic == "" {
		return fmt.Errorf("mnemonic cannot be empty")
	}

	log.Info("[Breez] Connecting to existing wallet")

	// Store the mnemonic
	c.mnemonic = mnemonic
	c.config.Mnemonic = mnemonic

	// Note: The actual connection happens in Initialize()
	// This function just validates and stores the mnemonic
	log.Info("[Breez] Wallet mnemonic configured successfully")
	return nil
}

// GetMnemonic returns the wallet mnemonic (use with caution!)
func (c *Client) GetMnemonic() string {
	return c.mnemonic
}

// HasMnemonic returns true if the client has a mnemonic configured
func (c *Client) HasMnemonic() bool {
	return c.mnemonic != ""
}

// ValidateMnemonic validates a BIP39 mnemonic
func ValidateMnemonic(mnemonic string) error {
	if mnemonic == "" {
		return fmt.Errorf("mnemonic cannot be empty")
	}

	// Basic validation: check word count (12, 15, 18, 21, or 24 words)
	words := len(splitMnemonic(mnemonic))
	validCounts := []int{12, 15, 18, 21, 24}
	valid := false
	for _, count := range validCounts {
		if words == count {
			valid = true
			break
		}
	}

	if !valid {
		return fmt.Errorf("invalid mnemonic: expected 12, 15, 18, 21, or 24 words, got %d", words)
	}

	log.Debug("[Breez] Mnemonic validation passed")
	return nil
}

// splitMnemonic splits a mnemonic string into words
func splitMnemonic(mnemonic string) []string {
	words := []string{}
	current := ""
	for _, r := range mnemonic {
		if r == ' ' || r == '\t' || r == '\n' {
			if current != "" {
				words = append(words, current)
				current = ""
			}
		} else {
			current += string(r)
		}
	}
	if current != "" {
		words = append(words, current)
	}
	return words
}

// GenerateMnemonic generates a new BIP39 mnemonic using cryptographically secure random entropy
// Returns a 12-word mnemonic (128 bits of entropy) which is the Bitcoin standard
func GenerateMnemonic() (string, error) {
	// Generate 128 bits (16 bytes) of entropy for a 12-word mnemonic
	// 128 bits = 12 words (recommended minimum for Bitcoin wallets)
	// 256 bits = 24 words (maximum security, but 12 words is sufficient)
	entropy := make([]byte, 16)

	// Use crypto/rand for cryptographically secure random bytes
	_, err := rand.Read(entropy)
	if err != nil {
		return "", fmt.Errorf("failed to generate entropy: %w", err)
	}

	// Generate mnemonic from entropy using BIP39
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return "", fmt.Errorf("failed to create mnemonic from entropy: %w", err)
	}

	log.Debug("[Breez] Generated new 12-word BIP39 mnemonic")
	return mnemonic, nil
}

// GenerateMnemonic24 generates a 24-word mnemonic (256 bits of entropy) for maximum security
func GenerateMnemonic24() (string, error) {
	// Generate 256 bits (32 bytes) of entropy for a 24-word mnemonic
	entropy := make([]byte, 32)

	_, err := rand.Read(entropy)
	if err != nil {
		return "", fmt.Errorf("failed to generate entropy: %w", err)
	}

	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return "", fmt.Errorf("failed to create mnemonic from entropy: %w", err)
	}

	log.Debug("[Breez] Generated new 24-word BIP39 mnemonic")
	return mnemonic, nil
}

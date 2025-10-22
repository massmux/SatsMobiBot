package breez

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultConfig returns a default Breez configuration
func DefaultConfig(workingDir string) *Config {
	return &Config{
		WorkingDir: workingDir,
		Network:    NetworkTestnet, // Default to testnet for safety
		APIKey:     os.Getenv("BREEZ_API_KEY"),
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.WorkingDir == "" {
		return fmt.Errorf("working directory cannot be empty")
	}

	if c.Network != NetworkMainnet && c.Network != NetworkTestnet && c.Network != NetworkRegtest {
		return fmt.Errorf("invalid network: %s", c.Network)
	}

	// Create working directory if it doesn't exist
	if err := os.MkdirAll(c.WorkingDir, 0755); err != nil {
		return fmt.Errorf("failed to create working directory: %w", err)
	}

	// Check if working directory is writable
	testFile := filepath.Join(c.WorkingDir, ".test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return fmt.Errorf("working directory not writable: %w", err)
	}
	os.Remove(testFile)

	return nil
}

// IsMainnet returns true if the network is mainnet
func (c *Config) IsMainnet() bool {
	return c.Network == NetworkMainnet
}

// IsTestnet returns true if the network is testnet
func (c *Config) IsTestnet() bool {
	return c.Network == NetworkTestnet
}

// GetNetworkString returns the network as a string
func (c *Config) GetNetworkString() string {
	return string(c.Network)
}

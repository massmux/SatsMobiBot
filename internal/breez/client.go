package breez

import (
	"fmt"
	"os"
	"path/filepath"

	breez_sdk "github.com/SatsRouting/breez-sdk-liquid-go/breez_sdk_liquid"
	log "github.com/sirupsen/logrus"
)

// NewClient creates a new Breez SDK client
func NewClient(config *Config) (*Client, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	client := &Client{
		config:     config,
		workingDir: config.WorkingDir,
		network:    config.Network,
		mnemonic:   config.Mnemonic,
	}

	return client, nil
}

// Initialize initializes the Breez SDK
func (c *Client) Initialize() error {
	if c.initialized {
		log.Warn("[Breez] Client already initialized")
		return nil
	}

	// Create working directory if it doesn't exist
	if err := os.MkdirAll(c.workingDir, 0755); err != nil {
		return fmt.Errorf("failed to create working directory: %w", err)
	}

	// Create state directory
	stateDir := filepath.Join(c.workingDir, "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	log.Infof("[Breez] Initializing SDK in %s (network: %s)", c.workingDir, c.network)

	// Determine network type
	var networkType breez_sdk.LiquidNetwork
	if c.config.IsMainnet() {
		networkType = breez_sdk.LiquidNetworkMainnet
	} else {
		networkType = breez_sdk.LiquidNetworkTestnet
	}

	// Prepare API key pointer (SDK expects *string)
	var apiKey *string
	if c.config.APIKey != "" {
		apiKey = &c.config.APIKey
	}

	// Create SDK configuration
	sdkConfig, sdkErr := breez_sdk.DefaultConfig(networkType, apiKey)
	if sdkErr != nil {
		return fmt.Errorf("failed to create SDK config: %s", sdkErr.AsError().Error())
	}

	// Set working directory
	sdkConfig.WorkingDir = stateDir

	// Prepare mnemonic pointer
	var mnemonicPtr *string
	if c.mnemonic != "" {
		mnemonicPtr = &c.mnemonic
	}

	// Create connect request
	connectRequest := breez_sdk.ConnectRequest{
		Config:     sdkConfig,
		Mnemonic:   mnemonicPtr,
		Passphrase: nil, // No passphrase for now
		Seed:       nil, // Using mnemonic instead of raw seed
	}

	// Connect to Breez SDK
	sdk, connectErr := breez_sdk.Connect(connectRequest)
	if connectErr != nil {
		return fmt.Errorf("failed to connect to Breez SDK: %s", connectErr.AsError().Error())
	}

	c.sdk = sdk
	c.initialized = true
	log.Info("[Breez] SDK initialized successfully")

	return nil
}

// Shutdown gracefully shuts down the Breez SDK
func (c *Client) Shutdown() error {
	if !c.initialized {
		return nil
	}

	log.Info("[Breez] Shutting down SDK")

	// Disconnect from Breez SDK
	if c.sdk != nil {
		disconnectErr := c.sdk.Disconnect()
		if disconnectErr != nil {
			log.Warnf("[Breez] Disconnect error: %s", disconnectErr.AsError().Error())
		}
		c.sdk = nil
	}

	c.initialized = false
	log.Info("[Breez] SDK shutdown complete")
	return nil
}

// IsInitialized returns whether the client is initialized
func (c *Client) IsInitialized() bool {
	return c.initialized
}

// GetNodeInfo returns information about the Breez node
func (c *Client) GetNodeInfo() (map[string]interface{}, error) {
	if !c.initialized {
		return nil, fmt.Errorf("client not initialized")
	}

	// Get info from SDK
	info, infoErr := c.sdk.GetInfo()
	if infoErr != nil {
		return nil, fmt.Errorf("failed to get node info: %s", infoErr.AsError().Error())
	}

	return map[string]interface{}{
		"status":         "initialized",
		"network":        c.network,
		"workingDir":     c.workingDir,
		"balance_sat":    info.WalletInfo.BalanceSat,
		"pending_send":   info.WalletInfo.PendingSendSat,
		"pending_recv":   info.WalletInfo.PendingReceiveSat,
		"pubkey":         info.WalletInfo.Pubkey,
		"fingerprint":    info.WalletInfo.Fingerprint,
		"asset_balances": len(info.WalletInfo.AssetBalances),
	}, nil
}

// GetConfig returns the client configuration
func (c *Client) GetConfig() *Config {
	return c.config
}

// GetNetwork returns the network the client is configured for
func (c *Client) GetNetwork() Network {
	return c.network
}

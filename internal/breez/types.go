package breez

import (
	"fmt"

	breez_sdk "github.com/SatsRouting/breez-sdk-liquid-go/breez_sdk_liquid"
)

// Client wraps the Breez SDK client
type Client struct {
	sdk         *breez_sdk.BindingLiquidSdk
	config      *Config
	workingDir  string
	network     Network
	initialized bool
	mnemonic    string
}

// Config holds Breez SDK configuration
type Config struct {
	// API Key for Breez services
	APIKey string

	// Working directory for SDK data
	WorkingDir string

	// Network to use (mainnet, testnet)
	Network Network

	// Mnemonic for wallet recovery (should be encrypted in production)
	Mnemonic string
}

// Network represents the Bitcoin/Liquid network
type Network string

const (
	NetworkMainnet Network = "mainnet"
	NetworkTestnet Network = "testnet"
	NetworkRegtest Network = "regtest"
)

// String returns the string representation of the network
func (n Network) String() string {
	return string(n)
}

// PaymentInfo represents a Breez payment
type PaymentInfo struct {
	ID          string
	PaymentHash string
	Amount      int64
	Status      PaymentStatus
	Direction   PaymentDirection
	Description string
	CreatedAt   int64
}

// PaymentStatus represents payment state
type PaymentStatus string

const (
	PaymentStatusPending  PaymentStatus = "pending"
	PaymentStatusComplete PaymentStatus = "complete"
	PaymentStatusFailed   PaymentStatus = "failed"
)

// PaymentDirection indicates payment direction
type PaymentDirection string

const (
	PaymentDirectionInbound  PaymentDirection = "inbound"
	PaymentDirectionOutbound PaymentDirection = "outbound"
)

// Invoice represents a Breez invoice
type Invoice struct {
	Bolt11      string
	PaymentHash string
	Amount      int64
	Description string
	ExpiresAt   int64
}

// DecodedInvoice represents a decoded Lightning invoice
type DecodedInvoice struct {
	Bolt11      string
	PaymentHash string
	AmountSats  int64
	Description string
	Payee       string
	Timestamp   int64
	Expiry      int64
}

// WalletInfo represents wallet information
type WalletInfo struct {
	Mnemonic   string
	Seed       []byte
	PublicKey  string
	Descriptor string
}

// BalanceInfo represents wallet balance information
type BalanceInfo struct {
	TotalSats     int64
	ConfirmedSats int64
	PendingSats   int64
}

// ExchangeRateInfo represents fiat exchange rate information
type ExchangeRateInfo struct {
	Currency  string
	Rate      float64
	UpdatedAt int64
}

// Error represents a Breez SDK error
type Error struct {
	Code    string
	Message string
}

func (e Error) Error() string {
	return fmt.Sprintf("[Breez Error %s]: %s", e.Code, e.Message)
}

// NewError creates a new Breez error
func NewError(code, message string) Error {
	return Error{
		Code:    code,
		Message: message,
	}
}

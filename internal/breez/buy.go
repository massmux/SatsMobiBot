package breez

import (
	"fmt"

	breez_sdk "github.com/breez/breez-sdk-liquid-go/breez_sdk_liquid"
	log "github.com/sirupsen/logrus"
)

// OnchainLimits represents onchain payment limits
type OnchainLimits struct {
	MinSat uint64
	MaxSat uint64
}

// FetchOnchainLimits fetches current onchain limits
func (c *Client) FetchOnchainLimits() (*OnchainLimits, error) {
	if !c.initialized {
		return nil, fmt.Errorf("client not initialized")
	}

	limits, err := c.sdk.FetchOnchainLimits()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch onchain limits: %w", err)
	}

	return &OnchainLimits{
		MinSat: limits.Receive.MinSat,
		MaxSat: limits.Receive.MaxSat,
	}, nil
}

// PrepareBuyBitcoin prepares a Bitcoin purchase
func (c *Client) PrepareBuyBitcoin(req breez_sdk.PrepareBuyBitcoinRequest) (*breez_sdk.PrepareBuyBitcoinResponse, error) {
	if !c.initialized {
		return nil, fmt.Errorf("client not initialized")
	}

	resp, err := c.sdk.PrepareBuyBitcoin(req)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare buy bitcoin: %w", err)
	}

	log.Debugf("[Breez] Prepared buy bitcoin: fees=%d sats", resp.FeesSat)
	return &resp, nil
}

// BuyBitcoin generates a purchase URL for buying Bitcoin
func (c *Client) BuyBitcoin(req breez_sdk.BuyBitcoinRequest) (string, error) {
	if !c.initialized {
		return "", fmt.Errorf("client not initialized")
	}

	url, err := c.sdk.BuyBitcoin(req)
	if err != nil {
		return "", fmt.Errorf("failed to buy bitcoin: %w", err)
	}

	log.Infof("[Breez] Generated buy Bitcoin URL")
	return url, nil
}

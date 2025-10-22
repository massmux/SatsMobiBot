package breez

import (
	"fmt"

	log "github.com/sirupsen/logrus"
)

// GetBalance returns the current wallet balance in satoshis
func (c *Client) GetBalance() (int64, error) {
	if !c.initialized {
		return 0, fmt.Errorf("client not initialized")
	}

	log.Debug("[Breez] Fetching wallet balance")

	// Get info from SDK
	info, infoErr := c.sdk.GetInfo()
	if infoErr != nil {
		return 0, fmt.Errorf("failed to get wallet info: %s", infoErr.AsError().Error())
	}

	// Get balance in satoshis
	balanceSats := int64(info.WalletInfo.BalanceSat)

	log.Debugf("[Breez] Balance: %d sats", balanceSats)
	return balanceSats, nil
}

// GetBalanceInfo returns detailed balance information
func (c *Client) GetBalanceInfo() (*BalanceInfo, error) {
	if !c.initialized {
		return nil, fmt.Errorf("client not initialized")
	}

	log.Debug("[Breez] Fetching detailed balance info")

	// Get info from SDK
	info, infoErr := c.sdk.GetInfo()
	if infoErr != nil {
		return nil, fmt.Errorf("failed to get wallet info: %s", infoErr.AsError().Error())
	}

	// Build balance info from WalletInfo
	balanceInfo := &BalanceInfo{
		TotalSats:     int64(info.WalletInfo.BalanceSat),
		ConfirmedSats: int64(info.WalletInfo.BalanceSat - info.WalletInfo.PendingReceiveSat),
		PendingSats:   int64(info.WalletInfo.PendingSendSat + info.WalletInfo.PendingReceiveSat),
	}

	log.Debugf("[Breez] Balance info: %+v", balanceInfo)
	return balanceInfo, nil
}

// RefreshBalance forces a balance refresh from the network
func (c *Client) RefreshBalance() error {
	if !c.initialized {
		return fmt.Errorf("client not initialized")
	}

	log.Info("[Breez] Refreshing balance from network")

	// Sync with the network
	syncErr := c.sdk.Sync()
	if syncErr != nil {
		return fmt.Errorf("failed to sync: %s", syncErr.AsError().Error())
	}

	log.Info("[Breez] Balance refreshed successfully")
	return nil
}

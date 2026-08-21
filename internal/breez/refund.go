package breez

import (
	"fmt"

	breez_sdk "github.com/SatsRouting/breez-sdk-liquid-go/breez_sdk_liquid"
	log "github.com/sirupsen/logrus"
)

// RefundableSwap represents a swap that can be refunded
type RefundableSwap struct {
	SwapAddress string
	Timestamp   int64
	AmountSat   uint64
}

// RecommendedFees represents recommended Bitcoin transaction fees
type RecommendedFees struct {
	FastestFee  uint64 // Fastest confirmation (high priority)
	HalfHourFee uint64 // ~30 minutes
	HourFee     uint64 // ~1 hour
	EconomyFee  uint64 // Low priority
	MinimumFee  uint64 // Minimum acceptable fee
}

// ListRefundables returns a list of swaps that can be refunded
// These are typically failed Bitcoin to Lightning swaps
func (c *Client) ListRefundables() ([]RefundableSwap, error) {
	if !c.initialized {
		return nil, fmt.Errorf("client not initialized")
	}

	log.Info("[Breez] Listing refundable swaps")

	// Get refundables from SDK
	refundables, err := c.sdk.ListRefundables()
	if err != nil {
		return nil, fmt.Errorf("failed to list refundables: %s", err.AsError().Error())
	}

	// Convert to our type
	var swaps []RefundableSwap
	for _, refundable := range refundables {
		swap := RefundableSwap{
			SwapAddress: refundable.SwapAddress,
			Timestamp:   int64(refundable.Timestamp),
			AmountSat:   refundable.AmountSat,
		}
		swaps = append(swaps, swap)
	}

	log.Infof("[Breez] Found %d refundable swaps", len(swaps))
	return swaps, nil
}

// GetRecommendedFees returns recommended Bitcoin transaction fees
func (c *Client) GetRecommendedFees() (*RecommendedFees, error) {
	if !c.initialized {
		return nil, fmt.Errorf("client not initialized")
	}

	log.Debug("[Breez] Fetching recommended fees")

	// Get fees from SDK
	fees, err := c.sdk.RecommendedFees()
	if err != nil {
		return nil, fmt.Errorf("failed to get recommended fees: %s", err.AsError().Error())
	}

	recommendedFees := &RecommendedFees{
		FastestFee:  fees.FastestFee,
		HalfHourFee: fees.HalfHourFee,
		HourFee:     fees.HourFee,
		EconomyFee:  fees.EconomyFee,
		MinimumFee:  fees.MinimumFee,
	}

	log.Debugf("[Breez] Recommended fees: %+v", recommendedFees)
	return recommendedFees, nil
}

// ExecuteRefund executes a refund for a failed swap
// Returns the refund transaction ID
func (c *Client) ExecuteRefund(swapAddress, refundAddress string, feeRateSatPerVbyte uint64) (string, error) {
	if !c.initialized {
		return "", fmt.Errorf("client not initialized")
	}

	log.Infof("[Breez] Executing refund for swap %s to address %s with fee rate %d sat/vbyte",
		swapAddress, refundAddress, feeRateSatPerVbyte)

	// Prepare refund request
	refundRequest := breez_sdk.RefundRequest{
		SwapAddress:        swapAddress,
		RefundAddress:      refundAddress,
		FeeRateSatPerVbyte: uint32(feeRateSatPerVbyte),
	}

	// Execute refund
	result, err := c.sdk.Refund(refundRequest)
	if err != nil {
		return "", fmt.Errorf("failed to execute refund: %s", err.AsError().Error())
	}

	log.Infof("[Breez] Refund successful: txid=%s", result.RefundTxId)
	return result.RefundTxId, nil
}

// HasRefundables checks if there are any refundable swaps
func (c *Client) HasRefundables() (bool, error) {
	refundables, err := c.ListRefundables()
	if err != nil {
		return false, err
	}
	return len(refundables) > 0, nil
}

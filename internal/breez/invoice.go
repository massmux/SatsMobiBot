package breez

import (
	"fmt"
	"time"

	breez_sdk "github.com/breez/breez-sdk-liquid-go/breez_sdk_liquid"
	log "github.com/sirupsen/logrus"
)

// CreateInvoice creates a Lightning invoice for receiving payments
func (c *Client) CreateInvoice(amountSats int64, description string) (*Invoice, error) {
	if !c.initialized {
		return nil, fmt.Errorf("client not initialized")
	}

	if amountSats <= 0 {
		return nil, fmt.Errorf("amount must be positive")
	}

	log.Infof("[Breez] Creating invoice for %d sats", amountSats)

	// Create receive amount using ReceiveAmountBitcoin
	var receiveAmount breez_sdk.ReceiveAmount = breez_sdk.ReceiveAmountBitcoin{
		PayerAmountSat: uint64(amountSats),
	}

	// Prepare receive payment request
	prepareReq := breez_sdk.PrepareReceiveRequest{
		PaymentMethod: breez_sdk.PaymentMethodBolt11Invoice,
		Amount:        &receiveAmount,
	}

	// Prepare the receive payment (validates amount, calculates fees)
	prepareResp, prepareErr := c.sdk.PrepareReceivePayment(prepareReq)
	if prepareErr != nil {
		return nil, fmt.Errorf("failed to prepare receive payment: %s", prepareErr.AsError().Error())
	}

	log.Debugf("[Breez] Prepared receive payment: fees_sat=%d", prepareResp.FeesSat)

	// Create the actual invoice
	receiveReq := breez_sdk.ReceivePaymentRequest{
		PrepareResponse: prepareResp,
		Description:     &description,
	}

	receiveResp, receiveErr := c.sdk.ReceivePayment(receiveReq)
	if receiveErr != nil {
		return nil, fmt.Errorf("failed to receive payment: %s", receiveErr.AsError().Error())
	}

	// Extract the destination (BOLT11 invoice or BIP21)
	destination := receiveResp.Destination

	log.Infof("[Breez] Invoice created successfully: %s", destination)

	// Decode the invoice to extract payment hash
	decodedInvoice, decodeErr := c.DecodeInvoice(destination)
	paymentHash := ""
	if decodeErr == nil && decodedInvoice != nil {
		paymentHash = decodedInvoice.PaymentHash
		log.Debugf("[Breez] Extracted payment hash from invoice: %s", paymentHash)
	} else {
		log.Warnf("[Breez] Failed to decode invoice for payment hash: %v", decodeErr)
	}

	// Create invoice object
	invoice := &Invoice{
		Bolt11:      destination,
		PaymentHash: paymentHash,
		Amount:      amountSats,
		Description: description,
		ExpiresAt:   time.Now().Add(24 * time.Hour).Unix(), // Default 24h expiry
	}

	return invoice, nil
}

// GetInvoiceStatus checks the status of an invoice by payment hash
func (c *Client) GetInvoiceStatus(paymentHash string) (*PaymentInfo, error) {
	if !c.initialized {
		return nil, fmt.Errorf("client not initialized")
	}

	log.Debugf("[Breez] Checking invoice status for payment hash: %s", paymentHash)

	// List payments and find the one with matching payment hash
	payments, listErr := c.sdk.ListPayments(breez_sdk.ListPaymentsRequest{})
	if listErr != nil {
		return nil, fmt.Errorf("failed to list payments: %s", listErr.AsError().Error())
	}

	// Find payment with matching hash
	for _, payment := range payments {
		if payment.TxId != nil && *payment.TxId == paymentHash {
			return convertSDKPaymentToPaymentInfo(payment), nil
		}
	}

	return nil, fmt.Errorf("payment not found: %s", paymentHash)
}

// WaitForInvoicePayment waits for an invoice to be paid (with timeout)
func (c *Client) WaitForInvoicePayment(paymentHash string, timeout time.Duration) (*PaymentInfo, error) {
	if !c.initialized {
		return nil, fmt.Errorf("client not initialized")
	}

	log.Infof("[Breez] Waiting for payment: %s (timeout: %v)", paymentHash, timeout)

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Check payment status
			payment, err := c.GetInvoiceStatus(paymentHash)
			if err == nil && payment.Status == PaymentStatusComplete {
				log.Infof("[Breez] Payment received: %s", paymentHash)
				return payment, nil
			}

			// Check if timeout reached
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("timeout waiting for payment: %s", paymentHash)
			}
		}
	}
}

// convertSDKPaymentToPaymentInfo converts Breez SDK payment to our PaymentInfo type
func convertSDKPaymentToPaymentInfo(payment breez_sdk.Payment) *PaymentInfo {
	var status PaymentStatus
	var direction PaymentDirection

	// Determine status
	switch payment.Status {
	case breez_sdk.PaymentStatePending:
		status = PaymentStatusPending
	case breez_sdk.PaymentStateComplete:
		status = PaymentStatusComplete
	case breez_sdk.PaymentStateFailed:
		status = PaymentStatusFailed
	default:
		status = PaymentStatusPending
	}

	// Determine direction
	switch payment.PaymentType {
	case breez_sdk.PaymentTypeReceive:
		direction = PaymentDirectionInbound
	case breez_sdk.PaymentTypeSend:
		direction = PaymentDirectionOutbound
	default:
		direction = PaymentDirectionInbound
	}

	// Extract description from payment details
	description := extractDescriptionFromPayment(payment)

	// Get transaction ID
	txID := ""
	if payment.TxId != nil {
		txID = *payment.TxId
	}

	return &PaymentInfo{
		ID:          txID,
		PaymentHash: txID,
		Amount:      int64(payment.AmountSat),
		Status:      status,
		Direction:   direction,
		Description: description,
		CreatedAt:   int64(payment.Timestamp),
	}
}

// extractDescriptionFromPayment extracts description from payment details
func extractDescriptionFromPayment(payment breez_sdk.Payment) string {
	// Try to extract description from Details based on payment type
	switch details := payment.Details.(type) {
	case breez_sdk.PaymentDetailsLightning:
		return details.Description
	case breez_sdk.PaymentDetailsLiquid:
		return details.Description
	}
	return ""
}

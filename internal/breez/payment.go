package breez

import (
	"fmt"
	"strings"

	breez_sdk "github.com/breez/breez-sdk-liquid-go/breez_sdk_liquid"
	log "github.com/sirupsen/logrus"
)

// PayInvoice pays a Lightning invoice (BOLT11)
func (c *Client) PayInvoice(bolt11 string) (*PaymentInfo, error) {
	if !c.initialized {
		return nil, fmt.Errorf("client not initialized")
	}

	if bolt11 == "" {
		return nil, fmt.Errorf("invoice cannot be empty")
	}

	// Normalize invoice (remove lightning: prefix if present)
	bolt11 = strings.TrimPrefix(strings.ToLower(bolt11), "lightning:")

	log.Infof("[Breez] Paying invoice: %s", bolt11)

	// Prepare send payment
	prepareReq := breez_sdk.PrepareSendRequest{
		Destination: bolt11,
	}

	prepareResp, prepareErr := c.sdk.PrepareSendPayment(prepareReq)
	if prepareErr != nil {
		return nil, fmt.Errorf("failed to prepare send payment: %s", prepareErr.AsError().Error())
	}

	log.Debugf("[Breez] Prepared send payment: fees_sat=%d", prepareResp.FeesSat)

	// Send the payment
	sendReq := breez_sdk.SendPaymentRequest{
		PrepareResponse: prepareResp,
	}

	sendResp, sendErr := c.sdk.SendPayment(sendReq)
	if sendErr != nil {
		return nil, fmt.Errorf("failed to send payment: %s", sendErr.AsError().Error())
	}

	log.Infof("[Breez] Payment sent successfully: tx_id=%s", *sendResp.Payment.TxId)

	// Convert to our PaymentInfo type
	paymentInfo := convertSDKPaymentToPaymentInfo(sendResp.Payment)

	return paymentInfo, nil
}

// DecodeInvoice decodes a Lightning invoice without paying it
func (c *Client) DecodeInvoice(bolt11 string) (*DecodedInvoice, error) {
	if !c.initialized {
		return nil, fmt.Errorf("client not initialized")
	}

	if bolt11 == "" {
		return nil, fmt.Errorf("invoice cannot be empty")
	}

	// Normalize invoice
	bolt11 = strings.TrimPrefix(strings.ToLower(bolt11), "lightning:")

	log.Debugf("[Breez] Decoding invoice: %s", bolt11)

	// Parse the invoice using ParseInvoice function
	invoice, parseErr := breez_sdk.ParseInvoice(bolt11)
	if parseErr != nil {
		return nil, fmt.Errorf("failed to parse invoice: %s", parseErr.AsError().Error())
	}

	description := ""
	if invoice.Description != nil {
		description = *invoice.Description
	}

	amountSats := int64(0)
	if invoice.AmountMsat != nil {
		amountSats = int64(*invoice.AmountMsat / 1000)
	}

	decoded := &DecodedInvoice{
		Bolt11:      bolt11,
		PaymentHash: invoice.PaymentHash,
		AmountSats:  amountSats,
		Description: description,
		Payee:       invoice.PayeePubkey,
		Timestamp:   int64(invoice.Timestamp),
		Expiry:      int64(invoice.Expiry),
	}

	log.Debugf("[Breez] Decoded invoice: amount=%d sats, description=%s", decoded.AmountSats, decoded.Description)
	return decoded, nil
}

// EstimatePaymentFee estimates the fee for paying an invoice
func (c *Client) EstimatePaymentFee(bolt11 string) (int64, error) {
	if !c.initialized {
		return 0, fmt.Errorf("client not initialized")
	}

	// Normalize invoice
	bolt11 = strings.TrimPrefix(strings.ToLower(bolt11), "lightning:")

	log.Debugf("[Breez] Estimating fee for invoice: %s", bolt11)

	// Prepare send payment to get fee estimate
	prepareReq := breez_sdk.PrepareSendRequest{
		Destination: bolt11,
	}

	prepareResp, prepareErr := c.sdk.PrepareSendPayment(prepareReq)
	if prepareErr != nil {
		return 0, fmt.Errorf("failed to prepare send payment: %s", prepareErr.AsError().Error())
	}

	var feeSats int64
	if prepareResp.FeesSat != nil {
		feeSats = int64(*prepareResp.FeesSat)
	}
	log.Debugf("[Breez] Estimated fee: %d sats", feeSats)

	return feeSats, nil
}

// ListPayments lists recent payments
func (c *Client) ListPayments(limit int) ([]*PaymentInfo, error) {
	if !c.initialized {
		return nil, fmt.Errorf("client not initialized")
	}

	log.Debugf("[Breez] Listing payments (limit: %d)", limit)

	// List payments from SDK
	payments, listErr := c.sdk.ListPayments(breez_sdk.ListPaymentsRequest{})
	if listErr != nil {
		return nil, fmt.Errorf("failed to list payments: %s", listErr.AsError().Error())
	}

	// Convert to our PaymentInfo type
	var paymentInfos []*PaymentInfo
	count := 0
	for _, payment := range payments {
		if limit > 0 && count >= limit {
			break
		}
		paymentInfos = append(paymentInfos, convertSDKPaymentToPaymentInfo(payment))
		count++
	}

	log.Debugf("[Breez] Found %d payments", len(paymentInfos))
	return paymentInfos, nil
}

// GetPayment retrieves a specific payment by ID
func (c *Client) GetPayment(paymentID string) (*PaymentInfo, error) {
	if !c.initialized {
		return nil, fmt.Errorf("client not initialized")
	}

	log.Debugf("[Breez] Getting payment: %s", paymentID)

	// List all payments and find the matching one
	payments, listErr := c.sdk.ListPayments(breez_sdk.ListPaymentsRequest{})
	if listErr != nil {
		return nil, fmt.Errorf("failed to list payments: %s", listErr.AsError().Error())
	}

	for _, payment := range payments {
		if payment.TxId != nil && *payment.TxId == paymentID {
			return convertSDKPaymentToPaymentInfo(payment), nil
		}
	}

	return nil, fmt.Errorf("payment not found: %s", paymentID)
}

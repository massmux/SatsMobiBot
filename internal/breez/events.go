package breez

import (
	"context"
	"fmt"
	"sync"
	"time"

	breez_sdk "github.com/breez/breez-sdk-liquid-go/breez_sdk_liquid"
	log "github.com/sirupsen/logrus"
)

// PaymentEventListener listens for payment events
type PaymentEventListener struct {
	paymentHash   string
	onPaymentPaid func()
	cancelFunc    context.CancelFunc
	once          sync.Once // Ensure callback only runs once
	triggered     bool
	mu            sync.Mutex
}

// OnEvent handles SDK events
func (l *PaymentEventListener) OnEvent(e breez_sdk.SdkEvent) {
	log.Debugf("[Breez Event] Received event type: %T", e)

	// Check if this is a payment received event
	switch event := e.(type) {
	case breez_sdk.SdkEventPaymentWaitingConfirmation:
		// For incoming payments, PaymentWaitingConfirmation means payment received
		log.Debugf("[Breez Event] Payment waiting confirmation - Details: %+v", event.Details)

		// Extract payment hash from the payment details (not TxId!)
		var paymentHash string
		if lightningDetails, ok := event.Details.Details.(breez_sdk.PaymentDetailsLightning); ok {
			if lightningDetails.PaymentHash != nil {
				paymentHash = *lightningDetails.PaymentHash
			}
		}

		log.Debugf("[Breez Event] Payment Hash: %s, Looking for: %s", paymentHash, l.paymentHash)

		// Check if this matches our payment hash
		if paymentHash != "" && paymentHash == l.paymentHash {
			// Use sync.Once to ensure callback only runs once
			l.once.Do(func() {
				log.Infof("[Breez Event] Payment received for hash: %s", l.paymentHash)

				// Mark as triggered
				l.mu.Lock()
				l.triggered = true
				l.mu.Unlock()

				// Execute callback in a safe goroutine
				if l.onPaymentPaid != nil {
					go func() {
						defer func() {
							if r := recover(); r != nil {
								log.Errorf("[Breez Event] Panic in payment callback: %v", r)
							}
						}()
						l.onPaymentPaid()
					}()
				}

				// Cancel the listener
				if l.cancelFunc != nil {
					l.cancelFunc()
				}
			})
		}

	case breez_sdk.SdkEventPaymentSucceeded:
		// Also handle PaymentSucceeded for completeness
		log.Debugf("[Breez Event] Payment succeeded - Details: %+v", event.Details)

		// Extract payment hash from the payment details (not TxId!)
		var paymentHash string
		if lightningDetails, ok := event.Details.Details.(breez_sdk.PaymentDetailsLightning); ok {
			if lightningDetails.PaymentHash != nil {
				paymentHash = *lightningDetails.PaymentHash
			}
		}

		log.Debugf("[Breez Event] Payment Hash: %s, Looking for: %s", paymentHash, l.paymentHash)

		// Check if this matches our payment hash
		if paymentHash != "" && paymentHash == l.paymentHash {
			// Use sync.Once to ensure callback only runs once
			l.once.Do(func() {
				log.Infof("[Breez Event] Payment succeeded for hash: %s", l.paymentHash)

				// Mark as triggered
				l.mu.Lock()
				l.triggered = true
				l.mu.Unlock()

				// Execute callback in a safe goroutine
				if l.onPaymentPaid != nil {
					go func() {
						defer func() {
							if r := recover(); r != nil {
								log.Errorf("[Breez Event] Panic in payment callback: %v", r)
							}
						}()
						l.onPaymentPaid()
					}()
				}

				// Cancel the listener
				if l.cancelFunc != nil {
					l.cancelFunc()
				}
			})
		}
	}
}

// ListenForInvoicePayment listens for a specific invoice payment with timeout
func (c *Client) ListenForInvoicePayment(paymentHash string, timeout time.Duration, onPaymentPaid func()) (string, error) {
	if !c.initialized {
		return "", fmt.Errorf("client not initialized")
	}

	log.Infof("[Breez] Starting payment listener for hash: %s (timeout: %v)", paymentHash, timeout)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	// Create listener
	listener := &PaymentEventListener{
		paymentHash:   paymentHash,
		onPaymentPaid: onPaymentPaid,
		cancelFunc:    cancel,
	}

	// Add event listener
	listenerID, err := c.sdk.AddEventListener(listener)
	if err != nil {
		cancel()
		return "", fmt.Errorf("failed to add event listener: %w", err)
	}

	// Start timeout goroutine
	go func() {
		<-ctx.Done()
		// Remove listener after timeout
		c.sdk.RemoveEventListener(listenerID)
		log.Infof("[Breez] Payment listener removed for hash: %s", paymentHash)
	}()

	return listenerID, nil
}

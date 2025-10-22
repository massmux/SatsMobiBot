package telegram

import (
	"fmt"

	"github.com/massmux/SatsMobiBot/internal"
	"github.com/massmux/SatsMobiBot/internal/lnbits"
	log "github.com/sirupsen/logrus"
)

const MaxCustodialBalance = 100000 // 100k sats

// PaymentRoutingDecision determines where to route an incoming payment
type PaymentRoutingDecision struct {
	RouteToBreez   bool
	Reason         string
	CurrentBalance int64
	IncomingAmount int64
}

// DecidePaymentRouting determines whether to route a payment to LNbits or Breez
// based on the custodial wallet limit (100k sats)
func (bot *TipBot) DecidePaymentRouting(user *lnbits.User, incomingAmount int64) (*PaymentRoutingDecision, error) {
	decision := &PaymentRoutingDecision{
		IncomingAmount: incomingAmount,
	}

	// Get current LNbits balance
	currentBalance, err := bot.GetUserBalance(user)
	if err != nil {
		return nil, fmt.Errorf("failed to get current balance: %w", err)
	}
	decision.CurrentBalance = currentBalance

	// Check if user has Breez available
	userBreez := bot.GetUserBreezClient(user)
	breezAvailable := userBreez != nil && userBreez.IsInitialized()

	// Determine limit to use (from config or constant)
	maxCustodial := internal.Configuration.Pos.Max_balance
	if maxCustodial == 0 {
		maxCustodial = MaxCustodialBalance
	}

	// If adding payment would exceed 100k sats, route to Breez (automatic overflow)
	if currentBalance+incomingAmount > maxCustodial {
		if breezAvailable {
			decision.RouteToBreez = true
			decision.Reason = fmt.Sprintf(
				"Automatic overflow: balance (%d) + incoming (%d) exceeds custodial limit (%d)",
				currentBalance, incomingAmount, maxCustodial,
			)
			log.Infof("[PaymentRouting] Routing %d sats to Breez for user (overflow)", incomingAmount)
		} else {
			// Breez not available, reject payment
			return nil, fmt.Errorf(
				"payment would exceed custodial limit (%d sats) and Breez is not available",
				maxCustodial,
			)
		}
	} else {
		// Within custodial limit, use LNbits
		decision.RouteToBreez = false
		decision.Reason = "Within custodial limit, using LNbits"
		log.Debugf("[PaymentRouting] Routing %d sats to LNbits for user", incomingAmount)
	}

	return decision, nil
}

// GetPaymentRoutingInfo returns human-readable info about routing decision
func (d *PaymentRoutingDecision) GetInfo() string {
	if d.RouteToBreez {
		return fmt.Sprintf(
			"⚡ Payment will be received to your self-custodial Breez wallet\n"+
				"Reason: %s",
			d.Reason,
		)
	}
	return fmt.Sprintf(
		"💰 Payment will be received to your custodial LNbits wallet\n"+
			"Current balance: %d sats",
		d.CurrentBalance,
	)
}

// CheckCustodialLimit checks if a user's custodial balance is approaching or at the limit
func (bot *TipBot) CheckCustodialLimit(user *lnbits.User) (bool, int64, error) {
	balance, err := bot.GetUserBalance(user)
	if err != nil {
		return false, 0, err
	}

	maxCustodial := internal.Configuration.Pos.Max_balance
	if maxCustodial == 0 {
		maxCustodial = MaxCustodialBalance
	}

	atLimit := balance >= maxCustodial
	return atLimit, balance, nil
}

// GetRemainingCustodialCapacity returns how many more sats can be added to custodial wallet
func (bot *TipBot) GetRemainingCustodialCapacity(user *lnbits.User) (int64, error) {
	balance, err := bot.GetUserBalance(user)
	if err != nil {
		return 0, err
	}

	maxCustodial := internal.Configuration.Pos.Max_balance
	if maxCustodial == 0 {
		maxCustodial = MaxCustodialBalance
	}

	remaining := maxCustodial - balance
	if remaining < 0 {
		remaining = 0
	}

	return remaining, nil
}

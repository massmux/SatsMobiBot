package telegram

import (
	"context"
	"fmt"
	"regexp"

	"github.com/massmux/SatsMobiBot/internal/lnbits"
	"github.com/massmux/SatsMobiBot/internal/telegram/intercept"
	log "github.com/sirupsen/logrus"
	tb "gopkg.in/lightningtipbot/telebot.v3"
)

// Bitcoin address regex (supports legacy, segwit, and bech32)
var bitcoinAddressRegex = regexp.MustCompile(`^(bc1|[13]|tb1|[mn2])[a-zA-HJ-NP-Z0-9]{25,90}$`)

// CheckAndNotifyRefunds checks for refundable swaps and notifies the user
func (bot *TipBot) CheckAndNotifyRefunds(ctx intercept.Context, user *lnbits.User) error {
	// Get user's Breez client
	userBreez := bot.GetUserBreezClient(user)

	if userBreez == nil || !userBreez.IsInitialized() {
		log.Debug("[CheckRefunds] Breez not initialized for user")
		return nil
	}

	// Check for refundables
	refundables, err := userBreez.ListRefundables()
	if err != nil {
		log.Warnf("[CheckRefunds] Failed to list refundables: %s", err)
		return err
	}

	if len(refundables) == 0 {
		log.Debug("[CheckRefunds] No refundables found")
		return nil
	}

	// Calculate total refundable amount
	var totalSats uint64
	for _, refundable := range refundables {
		totalSats += refundable.AmountSat
	}

	log.Infof("[CheckRefunds] Found %d refundable swap(s) totaling %d sats", len(refundables), totalSats)

	// Create refund button
	refundButton := tb.InlineButton{
		Unique: "refund_claim",
		Text:   Translate(ctx, "refundButtonMessage"),
	}
	bot.Telegram.Handle(&refundButton, bot.refundClaimHandler)

	// Send notification message
	message := fmt.Sprintf(Translate(ctx, "refundAvailableMessage"), totalSats)
	markup := &tb.ReplyMarkup{
		InlineKeyboard: [][]tb.InlineButton{{refundButton}},
	}

	bot.trySendMessage(ctx.Sender(), message, markup)
	return nil
}

// refundClaimHandler handles the refund claim button click
func (bot *TipBot) refundClaimHandler(c tb.Context) error {
	ctx := intercept.Context{TeleContext: intercept.TeleContext{Context: c}, Context: context.Background()}

	user, err := GetUser(ctx.Sender(), *bot)
	if err != nil {
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "errorTryLaterMessage"))
		return err
	}

	// Set user state to await Bitcoin address
	user.StateKey = lnbits.UserStateRefundAwaitingAddress
	err = UpdateUserRecord(user, *bot)
	if err != nil {
		log.Errorf("[refundClaimHandler] Failed to update user state: %s", err)
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "errorTryLaterMessage"))
		return err
	}

	// Ask user for Bitcoin address
	bot.trySendMessage(ctx.Sender(), Translate(ctx, "refundEnterAddressMessage"))

	// Delete the original refund notification
	bot.tryDeleteMessage(c.Message())

	return nil
}

// handleRefundAddressInput processes the Bitcoin address provided by the user
func (bot *TipBot) handleRefundAddressInput(ctx intercept.Context) (intercept.Context, error) {
	user, err := GetUser(ctx.Sender(), *bot)
	if err != nil {
		return ctx, err
	}

	address := ctx.Message().Text

	// Validate Bitcoin address format
	if !IsValidBitcoinAddress(address) {
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "refundInvalidAddressMessage"))
		return ctx, fmt.Errorf("invalid Bitcoin address")
	}

	// Get user's Breez client
	userBreez := bot.GetUserBreezClient(user)
	if userBreez == nil || !userBreez.IsInitialized() {
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "errorTryLaterMessage"))
		return ctx, fmt.Errorf("Breez not initialized")
	}

	// Get refundables
	refundables, err := userBreez.ListRefundables()
	if err != nil {
		log.Errorf("[handleRefundAddress] Failed to list refundables: %s", err)
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "errorTryLaterMessage"))
		return ctx, err
	}

	if len(refundables) == 0 {
		bot.trySendMessage(ctx.Sender(), "❌ No refundable swaps found.")
		// Clear state
		user.StateKey = 0
		UpdateUserRecord(user, *bot)
		return ctx, nil
	}

	// Send processing message
	processingMsg := bot.trySendMessage(ctx.Sender(), Translate(ctx, "refundProcessingMessage"))

	// Get recommended fees
	fees, err := userBreez.GetRecommendedFees()
	if err != nil {
		log.Errorf("[handleRefundAddress] Failed to get fees: %s", err)
		bot.tryEditMessage(processingMsg, fmt.Sprintf(Translate(ctx, "refundErrorMessage"), "Failed to fetch fees"))
		user.StateKey = 0
		UpdateUserRecord(user, *bot)
		return ctx, err
	}

	// Use half hour fee as default (good balance between speed and cost)
	feeRate := fees.HalfHourFee

	// Execute refund for each refundable swap
	var txIDs []string
	for _, refundable := range refundables {
		log.Infof("[handleRefundAddress] Executing refund for swap %s", refundable.SwapAddress)

		txID, err := userBreez.ExecuteRefund(refundable.SwapAddress, address, feeRate)
		if err != nil {
			log.Errorf("[handleRefundAddress] Refund failed for swap %s: %s", refundable.SwapAddress, err)
			bot.tryEditMessage(processingMsg, fmt.Sprintf(Translate(ctx, "refundErrorMessage"), err.Error()))
			user.StateKey = 0
			UpdateUserRecord(user, *bot)
			return ctx, err
		}

		txIDs = append(txIDs, txID)
		log.Infof("[handleRefundAddress] Refund successful: txid=%s", txID)
	}

	// Send success message
	successMsg := fmt.Sprintf(Translate(ctx, "refundSuccessMessage"), txIDs[0])
	if len(txIDs) > 1 {
		successMsg += fmt.Sprintf("\n\n_Total transactions: %d_", len(txIDs))
	}

	bot.tryEditMessage(processingMsg, successMsg)

	// Clear user state
	user.StateKey = 0
	err = UpdateUserRecord(user, *bot)
	if err != nil {
		log.Errorf("[handleRefundAddress] Failed to clear user state: %s", err)
	}

	return ctx, nil
}

// IsValidBitcoinAddress validates a Bitcoin address format
func IsValidBitcoinAddress(address string) bool {
	return bitcoinAddressRegex.MatchString(address)
}

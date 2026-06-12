package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/massmux/SatsMobiBot/internal"
	"github.com/massmux/SatsMobiBot/internal/breez"
	"github.com/massmux/SatsMobiBot/internal/errors"
	"github.com/massmux/SatsMobiBot/internal/i18n"
	"github.com/massmux/SatsMobiBot/internal/lnbits"
	"github.com/massmux/SatsMobiBot/internal/runtime"
	"github.com/massmux/SatsMobiBot/internal/runtime/mutex"
	"github.com/massmux/SatsMobiBot/internal/storage"
	"github.com/massmux/SatsMobiBot/internal/telegram/intercept"

	log "github.com/sirupsen/logrus"
	tb "gopkg.in/lightningtipbot/telebot.v3"
)

const (
	MinimumSwapAmount = 1000 // Minimum swap amount in sats to avoid fee issues
)

var (
	swapConfirmationMenu    = &tb.ReplyMarkup{ResizeKeyboard: true}
	btnCancelSwap           = swapConfirmationMenu.Data("🚫 Cancel", "cancel_swap")
	btnConfirmSwap          = swapConfirmationMenu.Data("✅ Confirm Swap", "confirm_swap")
	btnConfirmSwapToBreez   = swapConfirmationMenu.Data("✅ Confirm Swap", "confirm_swap_to_breez")
	btnConfirmSwapToLNbits  = swapConfirmationMenu.Data("✅ Confirm Swap", "confirm_swap_to_lnbits")
)

// requireUserBreezClient returns the user's cached Breez client. If the client
// isn't in memory but the user has a PIN-encrypted mnemonic, it stores the
// given operation name and prompts the user for their PIN so the swap can be
// resumed once the PIN is verified (see handleOperationPinInput in pin.go).
// A nil client with a nil error means a PIN prompt was sent and the caller
// should return without further action.
func (bot *TipBot) requireUserBreezClient(ctx intercept.Context, user *lnbits.User, operation string) (*breez.Client, error) {
	userBreez := bot.GetUserBreezClient(user)
	if userBreez != nil && userBreez.IsInitialized() {
		return userBreez, nil
	}

	if user.BreezInitialized && user.HasPin() {
		user.StateKey = lnbits.UserStateEnterPinForOperation
		user.StateData = operation
		UpdateUserRecord(user, *bot)

		bot.trySendMessage(ctx.Sender(),
			"🔐 Your Safer wallet requires your PIN.\n\n"+
				"Enter your PIN to continue:")
		return nil, nil
	}

	bot.trySendMessage(ctx.Sender(), Translate(ctx, "swapBreezNotInitialized"))
	return nil, errors.Create(errors.UserNoWalletError)
}

// SwapData holds information about a swap transaction
type SwapData struct {
	*storage.Base
	From            *lnbits.User `json:"from"`
	Amount          int64        `json:"amount"`
	Invoice         string       `json:"invoice"`
	Message         string       `json:"message"`
	LanguageCode    string       `json:"languagecode"`
	TelegramMessage *tb.Message  `json:"telegrammessage"`
}

// swapHandler handles the /swap command - asks user for amount
func (bot *TipBot) swapHandler(ctx intercept.Context) (intercept.Context, error) {
	// check and print all commands
	bot.anyTextHandler(ctx)

	user := LoadUser(ctx)
	if user.Wallet == nil {
		return ctx, errors.Create(errors.UserNoWalletError)
	}

	userStr := GetUserStr(ctx.Sender())

	// Check if Breez is initialized
	userBreez, err := bot.requireUserBreezClient(ctx, user, "swap")
	if err != nil {
		log.Warnf("[/swap] %s tried to swap but Breez not initialized", userStr)
		return ctx, err
	}
	if userBreez == nil {
		// PIN prompt sent, swap will resume once the PIN is verified
		return ctx, nil
	}

	// Get LNbits balance
	lnbitsBalance, err := bot.GetLNbitsBalance(user)
	if err != nil {
		log.Errorf("[/swap] Error fetching %s's LNbits balance: %s", userStr, err)
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "errorTryLaterMessage"))
		return ctx, err
	}

	// Check if user has sufficient balance
	if lnbitsBalance < MinimumSwapAmount {
		bot.trySendMessage(ctx.Sender(), fmt.Sprintf(Translate(ctx, "swapMinimumAmount"), MinimumSwapAmount))
		log.Warnf("[/swap] %s has insufficient balance for swap: %d sats", userStr, lnbitsBalance)
		return ctx, errors.Create(errors.InvalidAmountError)
	}

	// Prompt user to enter amount
	promptMessage := fmt.Sprintf(Translate(ctx, "swapAmountPrompt"), MinimumSwapAmount, lnbitsBalance)
	bot.trySendMessage(ctx.Sender(), promptMessage)

	// Set user state to wait for amount input
	SetUserState(user, bot, lnbits.UserStateSwapEnterAmount, "")
	log.Infof("[/swap] %s initiated swap, waiting for amount", userStr)

	return ctx, nil
}

// swapToBreezHandler handles the /swaptobreez command - swaps entire LNbits balance to Breez
func (bot *TipBot) swapToBreezHandler(ctx intercept.Context) (intercept.Context, error) {
	// check and print all commands
	bot.anyTextHandler(ctx)

	user := LoadUser(ctx)
	if user.Wallet == nil {
		return ctx, errors.Create(errors.UserNoWalletError)
	}

	userStr := GetUserStr(ctx.Sender())

	// Check if Breez is initialized
	userBreez, err := bot.requireUserBreezClient(ctx, user, "swap_to_breez")
	if err != nil {
		log.Warnf("[/swaptobreez] %s tried to swap but Breez not initialized", userStr)
		return ctx, err
	}
	if userBreez == nil {
		// PIN prompt sent, swap will resume once the PIN is verified
		return ctx, nil
	}

	// Get LNbits balance
	lnbitsBalance, err := bot.GetLNbitsBalance(user)
	if err != nil {
		log.Errorf("[/swaptobreez] Error fetching %s's LNbits balance: %s", userStr, err)
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "errorTryLaterMessage"))
		return ctx, err
	}

	// Check if user has sufficient balance
	if lnbitsBalance < MinimumSwapAmount {
		bot.trySendMessage(ctx.Sender(), fmt.Sprintf(Translate(ctx, "swapToBreezMinimum"), lnbitsBalance, MinimumSwapAmount))
		log.Warnf("[/swaptobreez] %s has insufficient balance for swap: %d sats", userStr, lnbitsBalance)
		return ctx, errors.Create(errors.InvalidAmountError)
	}

	// Estimate fees (1% + 100 sats conservative estimate for LNbits routing fees)
	// LNbits needs to pay: invoiceAmount + routingFee, so we reduce the invoice amount
	estimatedFees := int64(float64(lnbitsBalance)*0.01) + 100
	if estimatedFees < 100 {
		estimatedFees = 100
	}

	// Calculate actual swap amount (what Breez will receive) = balance - fees
	swapAmount := lnbitsBalance - estimatedFees
	if swapAmount < MinimumSwapAmount {
		bot.trySendMessage(ctx.Sender(), fmt.Sprintf(Translate(ctx, "swapToBreezInsufficientAfterFees"), lnbitsBalance, estimatedFees))
		log.Warnf("[/swaptobreez] %s swap amount too low after fees: balance=%d, fees=%d, swapAmount=%d", userStr, lnbitsBalance, estimatedFees, swapAmount)
		return ctx, errors.Create(errors.InvalidAmountError)
	}

	// Show confirmation with fee estimate (show swapAmount, not lnbitsBalance)
	confirmText := fmt.Sprintf(Translate(ctx, "swapToBreezConfirmation"), swapAmount, estimatedFees)
	log.Infof("[/swaptobreez] User: %s, LNbits balance: %d, swap amount: %d, estimated fees: %d",
		userStr, lnbitsBalance, swapAmount, estimatedFees)

	// Create swap confirmation data
	id := fmt.Sprintf("swaptobreez:%d-%d-%s", ctx.Sender().ID, swapAmount, RandStringRunes(5))

	// Create inline buttons
	confirmButton := swapConfirmationMenu.Data(Translate(ctx, "swapButtonConfirm"), "confirm_swap_to_breez", id)
	cancelButton := swapConfirmationMenu.Data(Translate(ctx, "swapButtonCancel"), "cancel_swap", id)

	swapConfirmationMenu.Inline(
		swapConfirmationMenu.Row(
			confirmButton,
			cancelButton),
	)

	swapMessage := bot.trySendMessageEditable(ctx.Chat(), confirmText, swapConfirmationMenu)

	swapData := &SwapData{
		Base:            storage.New(storage.ID(id)),
		From:            user,
		Amount:          swapAmount, // Use reduced amount (after fees) to ensure payment succeeds
		Message:         confirmText,
		LanguageCode:    ctx.Value("publicLanguageCode").(string),
		TelegramMessage: swapMessage,
	}

	// Save swap data
	runtime.IgnoreError(swapData.Set(swapData, bot.Bunt))

	return ctx, nil
}

// swapAllHandler is an alias for swapToBreezHandler for backward compatibility
func (bot *TipBot) swapAllHandler(ctx intercept.Context) (intercept.Context, error) {
	return bot.swapToBreezHandler(ctx)
}

// swapToLNbitsHandler handles the /swaptolnbits command - swaps from Breez to LNbits
func (bot *TipBot) swapToLNbitsHandler(ctx intercept.Context) (intercept.Context, error) {
	// check and print all commands
	bot.anyTextHandler(ctx)

	user := LoadUser(ctx)
	if user.Wallet == nil {
		return ctx, errors.Create(errors.UserNoWalletError)
	}

	userStr := GetUserStr(ctx.Sender())

	// Check if Breez is initialized
	userBreez, err := bot.requireUserBreezClient(ctx, user, "swap_to_lnbits")
	if err != nil {
		log.Warnf("[/swaptolnbits] %s tried to swap but Breez not initialized", userStr)
		return ctx, err
	}
	if userBreez == nil {
		// PIN prompt sent, swap will resume once the PIN is verified
		return ctx, nil
	}

	// Get Breez balance
	breezBalance, err := bot.GetBreezBalance(user)
	if err != nil {
		log.Errorf("[/swaptolnbits] Error fetching %s's Breez balance: %s", userStr, err)
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "errorTryLaterMessage"))
		return ctx, err
	}

	// Check if user has sufficient balance
	if breezBalance < MinimumSwapAmount {
		bot.trySendMessage(ctx.Sender(), fmt.Sprintf(Translate(ctx, "swapToLNbitsMinimum"), MinimumSwapAmount, breezBalance))
		log.Warnf("[/swaptolnbits] %s has insufficient Breez balance for swap: %d sats", userStr, breezBalance)
		return ctx, errors.Create(errors.InvalidAmountError)
	}

	// Get LNbits balance
	lnbitsBalance, err := bot.GetLNbitsBalance(user)
	if err != nil {
		log.Errorf("[/swaptolnbits] Error fetching %s's LNbits balance: %s", userStr, err)
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "errorTryLaterMessage"))
		return ctx, err
	}

	// Get S (LNbits max balance) from config
	S := internal.Configuration.Limits.LNbitsMaxBalance

	// Calculate maximum amount we can swap: S - B
	maxSwapAmount := S - lnbitsBalance

	if maxSwapAmount <= 0 {
		bot.trySendMessage(ctx.Sender(), fmt.Sprintf(Translate(ctx, "swapToLNbitsFull"), lnbitsBalance, S))
		log.Warnf("[/swaptolnbits] %s's LNbits already at capacity: %d >= %d", userStr, lnbitsBalance, S)
		return ctx, errors.Create(errors.InvalidAmountError)
	}

	if maxSwapAmount < MinimumSwapAmount {
		bot.trySendMessage(ctx.Sender(), fmt.Sprintf(Translate(ctx, "swapToLNbitsMinimum"), MinimumSwapAmount, maxSwapAmount))
		log.Warnf("[/swaptolnbits] %s max swap amount too low: %d sats", userStr, maxSwapAmount)
		return ctx, errors.Create(errors.InvalidAmountError)
	}

	// Limit swap amount to available Breez balance
	swapAmount := maxSwapAmount
	if swapAmount > breezBalance {
		swapAmount = breezBalance
	}

	// Estimate fees (1% + 100 sats conservative estimate)
	estimatedFees := int64(float64(swapAmount)*0.01) + 100
	if estimatedFees < 100 {
		estimatedFees = 100
	}

	// Adjust swap amount if fees would exceed Breez balance
	if swapAmount+estimatedFees > breezBalance {
		swapAmount = breezBalance - estimatedFees
	}

	if swapAmount < MinimumSwapAmount {
		bot.trySendMessage(ctx.Sender(), fmt.Sprintf(Translate(ctx, "swapToLNbitsInsufficientAfterFees"), breezBalance, estimatedFees))
		log.Warnf("[/swaptolnbits] %s swap amount too low after fees: %d sats", userStr, swapAmount)
		return ctx, errors.Create(errors.InvalidAmountError)
	}

	// Show confirmation with fee estimate
	newLNbitsBalance := lnbitsBalance + swapAmount
	confirmText := fmt.Sprintf(Translate(ctx, "swapToLNbitsConfirmation"), 
		swapAmount, breezBalance, lnbitsBalance, newLNbitsBalance, estimatedFees)
	log.Infof("[/swaptolnbits] User: %s, swap amount: %d, Breez balance: %d, LNbits: %d->%d, fees: %d",
		userStr, swapAmount, breezBalance, lnbitsBalance, newLNbitsBalance, estimatedFees)

	// Create swap confirmation data
	id := fmt.Sprintf("swaptolnbits:%d-%d-%s", ctx.Sender().ID, swapAmount, RandStringRunes(5))

	// Create inline buttons
	confirmButton := swapConfirmationMenu.Data(Translate(ctx, "swapButtonConfirm"), "confirm_swap_to_lnbits", id)
	cancelButton := swapConfirmationMenu.Data(Translate(ctx, "swapButtonCancel"), "cancel_swap", id)

	swapConfirmationMenu.Inline(
		swapConfirmationMenu.Row(
			confirmButton,
			cancelButton),
	)

	swapMessage := bot.trySendMessageEditable(ctx.Chat(), confirmText, swapConfirmationMenu)

	swapData := &SwapData{
		Base:            storage.New(storage.ID(id)),
		From:            user,
		Amount:          swapAmount,
		Message:         confirmText,
		LanguageCode:    ctx.Value("publicLanguageCode").(string),
		TelegramMessage: swapMessage,
	}

	// Save swap data
	runtime.IgnoreError(swapData.Set(swapData, bot.Bunt))

	return ctx, nil
}

// swapLNHandler handles the /swap-ln command - asks user for amount (Breez/Safe -> LNbits/Hot)
func (bot *TipBot) swapLNHandler(ctx intercept.Context) (intercept.Context, error) {
	// check and print all commands
	bot.anyTextHandler(ctx)

	user := LoadUser(ctx)
	if user.Wallet == nil {
		return ctx, errors.Create(errors.UserNoWalletError)
	}

	userStr := GetUserStr(ctx.Sender())

	// Check if Breez is initialized
	userBreez, err := bot.requireUserBreezClient(ctx, user, "swap_ln")
	if err != nil {
		log.Warnf("[/swap-ln] %s tried to swap-ln but Breez not initialized", userStr)
		return ctx, err
	}
	if userBreez == nil {
		// PIN prompt sent, swap will resume once the PIN is verified
		return ctx, nil
	}

	// Get Breez balance
	breezBalance, err := bot.GetBreezBalance(user)
	if err != nil {
		log.Errorf("[/swap-ln] Error fetching %s's Breez balance: %s", userStr, err)
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "errorTryLaterMessage"))
		return ctx, err
	}

	// Get LNbits balance
	lnbitsBalance, err := bot.GetLNbitsBalance(user)
	if err != nil {
		log.Errorf("[/swap-ln] Error fetching %s's LNbits balance: %s", userStr, err)
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "errorTryLaterMessage"))
		return ctx, err
	}

	// Get S (LNbits max balance) from config
	S := internal.Configuration.Limits.LNbitsMaxBalance

	// LNbits capacity remaining
	capacity := S - lnbitsBalance
	if capacity <= 0 {
		bot.trySendMessage(ctx.Sender(), fmt.Sprintf(Translate(ctx, "swapToLNbitsFull"), lnbitsBalance, S))
		log.Warnf("[/swap-ln] %s's LNbits already at capacity: %d >= %d", userStr, lnbitsBalance, S)
		return ctx, errors.Create(errors.InvalidAmountError)
	}

	// Max by Breez balance with a conservative fee model (1% + 100 sats)
	// Require: amount + (amount*1% + 100) <= breezBalance  => amount <= (breezBalance - 100) / 1.01
	maxByBreez := int64(0)
	if breezBalance > 100 {
		maxByBreez = int64(float64(breezBalance-100) / 1.01)
	}

	maxAllowed := capacity
	if maxByBreez < maxAllowed {
		maxAllowed = maxByBreez
	}
	if maxAllowed > breezBalance {
		maxAllowed = breezBalance
	}

	if maxAllowed < MinimumSwapAmount {
		// If Breez has funds but fees would eat everything, use the dedicated message; otherwise generic minimum
		estimatedFees := int64(float64(maxAllowed)*0.01) + 100
		if estimatedFees < 100 {
			estimatedFees = 100
		}
		if breezBalance >= MinimumSwapAmount && capacity >= MinimumSwapAmount {
			bot.trySendMessage(ctx.Sender(), fmt.Sprintf(Translate(ctx, "swapToLNbitsInsufficientAfterFees"), breezBalance, estimatedFees))
		} else {
			bot.trySendMessage(ctx.Sender(), fmt.Sprintf(Translate(ctx, "swapToLNbitsMinimum"), MinimumSwapAmount, breezBalance))
		}
		log.Warnf("[/swap-ln] %s max allowed too low: max=%d, breez=%d, capacity=%d", userStr, maxAllowed, breezBalance, capacity)
		return ctx, errors.Create(errors.InvalidAmountError)
	}

	promptMessage := fmt.Sprintf(Translate(ctx, "swapLNAmountPrompt"), MinimumSwapAmount, maxAllowed, breezBalance, lnbitsBalance, S)
	bot.trySendMessage(ctx.Sender(), promptMessage)

	// Set user state to wait for amount input
	SetUserState(user, bot, lnbits.UserStateSwapLNEnterAmount, "")
	log.Infof("[/swap-ln] %s initiated swap-ln, waiting for amount (maxAllowed=%d)", userStr, maxAllowed)

	return ctx, nil
}

// enterSwapLNAmountHandler processes user's swap-ln amount input (Breez/Safe -> LNbits/Hot)
func (bot *TipBot) enterSwapLNAmountHandler(ctx intercept.Context) (intercept.Context, error) {
	user := LoadUser(ctx)
	if user.Wallet == nil {
		return ctx, errors.Create(errors.UserNoWalletError)
	}

	userStr := GetUserStr(ctx.Sender())

	// Parse amount from message
	amountStr := strings.TrimSpace(ctx.Message().Text)
	amount, err := strconv.ParseInt(amountStr, 10, 64)
	if err != nil || amount <= 0 {
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "invalidAmountMessage"))
		log.Warnf("[enterSwapLNAmount] %s entered invalid amount: %s", userStr, amountStr)
		return ctx, errors.Create(errors.InvalidAmountError)
	}

	// Check minimum amount
	if amount < MinimumSwapAmount {
		bot.trySendMessage(ctx.Sender(), fmt.Sprintf(Translate(ctx, "swapMinimumAmount"), MinimumSwapAmount))
		log.Warnf("[enterSwapLNAmount] %s entered amount below minimum: %d sats", userStr, amount)
		return ctx, errors.Create(errors.InvalidAmountError)
	}

	// Check Breez initialized + balances + LNbits capacity
	userBreez := bot.GetUserBreezClient(user)
	if userBreez == nil || !userBreez.IsInitialized() {
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "swapBreezNotInitialized"))
		return ctx, errors.Create(errors.UserNoWalletError)
	}

	breezBalance, err := bot.GetBreezBalance(user)
	if err != nil {
		log.Errorf("[enterSwapLNAmount] Error fetching %s's Breez balance: %s", userStr, err)
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "errorTryLaterMessage"))
		return ctx, err
	}

	lnbitsBalance, err := bot.GetLNbitsBalance(user)
	if err != nil {
		log.Errorf("[enterSwapLNAmount] Error fetching %s's LNbits balance: %s", userStr, err)
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "errorTryLaterMessage"))
		return ctx, err
	}

	S := internal.Configuration.Limits.LNbitsMaxBalance

	capacity := S - lnbitsBalance
	if capacity <= 0 {
		bot.trySendMessage(ctx.Sender(), fmt.Sprintf(Translate(ctx, "swapToLNbitsFull"), lnbitsBalance, S))
		log.Warnf("[enterSwapLNAmount] %s's LNbits already at capacity: %d >= %d", userStr, lnbitsBalance, S)
		return ctx, errors.Create(errors.InvalidAmountError)
	}

	if amount > capacity {
		bot.trySendMessage(ctx.Sender(), fmt.Sprintf(Translate(ctx, "enterAmountRangeMessage"), MinimumSwapAmount, capacity))
		log.Warnf("[enterSwapLNAmount] %s entered amount above capacity: amount=%d capacity=%d", userStr, amount, capacity)
		return ctx, errors.Create(errors.InvalidAmountError)
	}

	// Conservative fee check (real fee is enforced at execution time)
	estimatedFees := int64(float64(amount)*0.01) + 100
	if estimatedFees < 100 {
		estimatedFees = 100
	}
	if amount+estimatedFees > breezBalance {
		bot.trySendMessage(ctx.Sender(), fmt.Sprintf(Translate(ctx, "swapToLNbitsInsufficientAfterFees"), breezBalance, estimatedFees))
		log.Warnf("[enterSwapLNAmount] %s insufficient Breez balance after fees: breez=%d amount=%d estFees=%d", userStr, breezBalance, amount, estimatedFees)
		return ctx, errors.Create(errors.InvalidAmountError)
	}

	// Reset user state
	ResetUserState(user, bot)

	// Show confirmation with fee estimate (same format used by /swaptolnbits)
	newLNbitsBalance := lnbitsBalance + amount
	confirmText := fmt.Sprintf(Translate(ctx, "swapToLNbitsConfirmation"),
		amount, breezBalance, lnbitsBalance, newLNbitsBalance, estimatedFees)

	// Create swap confirmation data
	id := fmt.Sprintf("swapln:%d-%d-%s", ctx.Sender().ID, amount, RandStringRunes(5))

	confirmButton := swapConfirmationMenu.Data(Translate(ctx, "swapButtonConfirm"), "confirm_swap_to_lnbits", id)
	cancelButton := swapConfirmationMenu.Data(Translate(ctx, "swapButtonCancel"), "cancel_swap", id)

	swapConfirmationMenu.Inline(
		swapConfirmationMenu.Row(
			confirmButton,
			cancelButton),
	)

	swapMessage := bot.trySendMessageEditable(ctx.Chat(), confirmText, swapConfirmationMenu)

	swapData := &SwapData{
		Base:            storage.New(storage.ID(id)),
		From:            user,
		Amount:          amount,
		Message:         confirmText,
		LanguageCode:    ctx.Value("publicLanguageCode").(string),
		TelegramMessage: swapMessage,
	}

	runtime.IgnoreError(swapData.Set(swapData, bot.Bunt))
	log.Infof("[enterSwapLNAmount] User: %s, amount: %d sat (LNbits %d->%d, estFees=%d)", userStr, amount, lnbitsBalance, newLNbitsBalance, estimatedFees)

	return ctx, nil
}

// enterSwapAmountHandler processes user's swap amount input
func (bot *TipBot) enterSwapAmountHandler(ctx intercept.Context) (intercept.Context, error) {
	user := LoadUser(ctx)
	if user.Wallet == nil {
		return ctx, errors.Create(errors.UserNoWalletError)
	}

	userStr := GetUserStr(ctx.Sender())

	// Parse amount from message
	amountStr := strings.TrimSpace(ctx.Message().Text)
	amount, err := strconv.ParseInt(amountStr, 10, 64)
	if err != nil || amount <= 0 {
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "invalidAmountMessage"))
		log.Warnf("[enterSwapAmount] %s entered invalid amount: %s", userStr, amountStr)
		return ctx, errors.Create(errors.InvalidAmountError)
	}

	// Check minimum amount
	if amount < MinimumSwapAmount {
		bot.trySendMessage(ctx.Sender(), fmt.Sprintf(Translate(ctx, "swapMinimumAmount"), MinimumSwapAmount))
		log.Warnf("[enterSwapAmount] %s entered amount below minimum: %d sats", userStr, amount)
		return ctx, errors.Create(errors.InvalidAmountError)
	}

	// Check if user has sufficient balance
	lnbitsBalance, err := bot.GetLNbitsBalance(user)
	if err != nil {
		log.Errorf("[enterSwapAmount] Error fetching %s's LNbits balance: %s", userStr, err)
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "errorTryLaterMessage"))
		return ctx, err
	}

	if amount > lnbitsBalance {
		bot.trySendMessage(ctx.Sender(), fmt.Sprintf(Translate(ctx, "swapInsufficientFunds"), lnbitsBalance, amount))
		log.Warnf("[enterSwapAmount] %s insufficient balance: has %d, wants %d", userStr, lnbitsBalance, amount)
		return ctx, errors.Create(errors.InvalidAmountError)
	}

	// Reset user state
	ResetUserState(user, bot)

	// Show confirmation
	confirmText := fmt.Sprintf(Translate(ctx, "swapConfirmation"), amount)
	log.Infof("[enterSwapAmount] User: %s, amount: %d sat", userStr, amount)

	// Create swap confirmation data
	id := fmt.Sprintf("swap:%d-%d-%s", ctx.Sender().ID, amount, RandStringRunes(5))

	// Create inline buttons
	confirmButton := swapConfirmationMenu.Data(Translate(ctx, "swapButtonConfirm"), "confirm_swap", id)
	cancelButton := swapConfirmationMenu.Data(Translate(ctx, "swapButtonCancel"), "cancel_swap", id)

	swapConfirmationMenu.Inline(
		swapConfirmationMenu.Row(
			confirmButton,
			cancelButton),
	)

	swapMessage := bot.trySendMessageEditable(ctx.Chat(), confirmText, swapConfirmationMenu)

	swapData := &SwapData{
		Base:            storage.New(storage.ID(id)),
		From:            user,
		Amount:          amount,
		Message:         confirmText,
		LanguageCode:    ctx.Value("publicLanguageCode").(string),
		TelegramMessage: swapMessage,
	}

	// Save swap data
	runtime.IgnoreError(swapData.Set(swapData, bot.Bunt))

	return ctx, nil
}

// confirmSwapHandler executes the actual swap transaction
func (bot *TipBot) confirmSwapHandler(ctx intercept.Context) (intercept.Context, error) {
	tx := &SwapData{Base: storage.New(storage.ID(ctx.Data()))}
	mutex.LockWithContext(ctx, tx.ID)
	defer mutex.UnlockWithContext(ctx, tx.ID)

	sn, err := tx.Get(tx, bot.Bunt)
	if err != nil {
		log.Errorf("[confirmSwapHandler] %s", err.Error())
		return ctx, err
	}
	swapData := sn.(*SwapData)

	// Only the correct user can press
	if swapData.From.Telegram.ID != ctx.Sender().ID {
		return ctx, errors.Create(errors.UnknownError)
	}

	if !swapData.Active {
		log.Errorf("[confirmSwapHandler] swap not active anymore")
		bot.tryEditMessage(ctx.Message(), i18n.Translate(swapData.LanguageCode, "errorTryLaterMessage"), &tb.ReplyMarkup{})
		bot.tryDeleteMessage(ctx.Message())
		return ctx, errors.Create(errors.NotActiveError)
	}
	defer swapData.Set(swapData, bot.Bunt)

	user := LoadUser(ctx)
	if user.Wallet == nil {
		bot.tryDeleteMessage(ctx.Message())
		return ctx, errors.Create(errors.UserNoWalletError)
	}

	userStr := GetUserStr(ctx.Sender())

	// Update button text to show processing
	bot.tryEditMessage(
		ctx.Message(),
		swapData.Message,
		&tb.ReplyMarkup{
			InlineKeyboard: [][]tb.InlineButton{
				{tb.InlineButton{Unique: "processing_swap", Text: i18n.Translate(swapData.LanguageCode, "processingMessage")}},
			},
		},
	)

	log.Infof("[/swap] Executing swap for %s: %d sats", userStr, swapData.Amount)

	// Execute the swap
	err = bot.executeSwap(user, swapData.Amount, ctx)
	if err != nil {
		log.Errorf("[/swap] Swap failed for %s: %s", userStr, err)
		errMsg := fmt.Sprintf(i18n.Translate(swapData.LanguageCode, "swapFailed"), err.Error())
		bot.tryEditMessage(ctx.Message(), errMsg, &tb.ReplyMarkup{})
		return ctx, err
	}

	// Success!
	successMsg := fmt.Sprintf(i18n.Translate(swapData.LanguageCode, "swapSuccess"), swapData.Amount)
	bot.tryDeleteMessage(ctx.Message())
	bot.trySendMessage(ctx.Sender(), successMsg)

	log.Infof("[⚡️ swap] User %s swapped %d sats from LNbits to Breez", userStr, swapData.Amount)

	// Inactivate the swap data
	return ctx, swapData.Inactivate(swapData, bot.Bunt)
}

// cancelSwapHandler cancels swap operation
func (bot *TipBot) cancelSwapHandler(ctx intercept.Context) (intercept.Context, error) {
	tx := &SwapData{Base: storage.New(storage.ID(ctx.Data()))}
	mutex.LockWithContext(ctx, tx.ID)
	defer mutex.UnlockWithContext(ctx, tx.ID)

	sn, err := tx.Get(tx, bot.Bunt)
	if err != nil {
		log.Errorf("[cancelSwapHandler] %s", err.Error())
		return ctx, err
	}
	swapData := sn.(*SwapData)

	// Only the correct user can press
	if swapData.From.Telegram.ID != ctx.Callback().Sender.ID {
		return ctx, errors.Create(errors.UnknownError)
	}

	// Delete message and notify cancellation
	bot.tryDeleteMessage(ctx.Message())
	bot.trySendMessage(ctx.Message().Chat, i18n.Translate(swapData.LanguageCode, "swapCancelled"))

	return ctx, swapData.Inactivate(swapData, bot.Bunt)
}

// confirmSwapToBreezHandler executes the swap from LNbits to Breez
func (bot *TipBot) confirmSwapToBreezHandler(ctx intercept.Context) (intercept.Context, error) {
	tx := &SwapData{Base: storage.New(storage.ID(ctx.Data()))}
	mutex.LockWithContext(ctx, tx.ID)
	defer mutex.UnlockWithContext(ctx, tx.ID)

	sn, err := tx.Get(tx, bot.Bunt)
	if err != nil {
		log.Errorf("[confirmSwapToBreezHandler] %s", err.Error())
		return ctx, err
	}
	swapData := sn.(*SwapData)

	// Only the correct user can press
	if swapData.From.Telegram.ID != ctx.Sender().ID {
		return ctx, errors.Create(errors.UnknownError)
	}

	if !swapData.Active {
		log.Errorf("[confirmSwapToBreezHandler] swap not active anymore")
		bot.tryEditMessage(ctx.Message(), i18n.Translate(swapData.LanguageCode, "errorTryLaterMessage"), &tb.ReplyMarkup{})
		bot.tryDeleteMessage(ctx.Message())
		return ctx, errors.Create(errors.NotActiveError)
	}
	defer swapData.Set(swapData, bot.Bunt)

	user := LoadUser(ctx)
	if user.Wallet == nil {
		bot.tryDeleteMessage(ctx.Message())
		return ctx, errors.Create(errors.UserNoWalletError)
	}

	userStr := GetUserStr(ctx.Sender())

	// Update button text to show processing
	bot.tryEditMessage(
		ctx.Message(),
		swapData.Message,
		&tb.ReplyMarkup{
			InlineKeyboard: [][]tb.InlineButton{
				{tb.InlineButton{Unique: "processing_swap", Text: i18n.Translate(swapData.LanguageCode, "processingMessage")}},
			},
		},
	)

	log.Infof("[/swaptobreez] Executing swap for %s: %d sats", userStr, swapData.Amount)

	// Execute the swap
	err = bot.executeSwap(user, swapData.Amount, ctx)
	if err != nil {
		log.Errorf("[/swaptobreez] Swap failed for %s: %s", userStr, err)
		errMsg := fmt.Sprintf(i18n.Translate(swapData.LanguageCode, "swapFailed"), err.Error())
		bot.tryEditMessage(ctx.Message(), errMsg, &tb.ReplyMarkup{})
		return ctx, err
	}

	// Success!
	successMsg := fmt.Sprintf(i18n.Translate(swapData.LanguageCode, "swapToBreezSuccess"), swapData.Amount)
	bot.tryDeleteMessage(ctx.Message())
	bot.trySendMessage(ctx.Sender(), successMsg)

	log.Infof("[⚡️ swaptobreez] User %s swapped %d sats from LNbits to Breez", userStr, swapData.Amount)

	// Inactivate the swap data
	return ctx, swapData.Inactivate(swapData, bot.Bunt)
}

// paymentStatusPollAttempts/paymentStatusPollInterval control how long we poll
// LNbits for a payment's settlement status after Pay() returns a network-level
// error (e.g. a Cloudflare gateway timeout). In that case LNbits may still be
// processing the payment on its backend even though the HTTP response was lost.
const (
	paymentStatusPollAttempts = 6
	paymentStatusPollInterval = 5 * time.Second
)

// checkPaymentSettled polls LNbits for the status of the payment identified by
// paymentHash and returns true if LNbits confirms it as paid.
func (bot *TipBot) checkPaymentSettled(user *lnbits.User, paymentHash string) bool {
	for i := 0; i < paymentStatusPollAttempts; i++ {
		time.Sleep(paymentStatusPollInterval)
		payment, err := bot.Client.Payment(*user.Wallet, paymentHash)
		if err != nil {
			log.Warnf("[checkPaymentSettled] failed to check status for payment %s: %s", paymentHash, err)
			continue
		}
		if payment.Paid {
			return true
		}
	}
	return false
}

// executeSwap performs the actual swap from LNbits to Breez
func (bot *TipBot) executeSwap(user *lnbits.User, amount int64, ctx intercept.Context) error {
	userStr := GetUserStr(user.Telegram)

	// 1. Check if Breez is initialized
	userBreez := bot.GetUserBreezClient(user)
	if userBreez == nil || !userBreez.IsInitialized() {
		return fmt.Errorf("breez not initialized")
	}

	// 2. Get LNbits balance
	lnbitsBalance, err := bot.GetLNbitsBalance(user)
	if err != nil {
		return fmt.Errorf("failed to get LNbits balance: %w", err)
	}

	// 3. Validate amount
	if amount < MinimumSwapAmount {
		return fmt.Errorf("amount below minimum: %d < %d", amount, MinimumSwapAmount)
	}

	// 4. Calculate fee buffer and validate balance can cover amount + fees
	// LNbits needs to pay: invoiceAmount + routingFee
	// We add a safety buffer (1% of amount + 50 sats) to ensure payment succeeds
	feeBuffer := int64(float64(amount)*0.01) + 50
	if feeBuffer < 50 {
		feeBuffer = 50
	}

	// If balance is too tight, reduce the amount further
	if amount+feeBuffer > lnbitsBalance {
		// Recalculate amount to fit within balance with safety margin
		newAmount := lnbitsBalance - feeBuffer
		if newAmount < MinimumSwapAmount {
			return fmt.Errorf("insufficient LNbits balance after fees: balance=%d, need=%d (amount=%d + buffer=%d)",
				lnbitsBalance, amount+feeBuffer, amount, feeBuffer)
		}
		log.Warnf("[executeSwap] Reducing swap amount from %d to %d to accommodate fees (balance=%d, buffer=%d)",
			amount, newAmount, lnbitsBalance, feeBuffer)
		amount = newAmount
	}

	// 5. Create Breez invoice
	invoice, err := userBreez.CreateInvoice(amount, fmt.Sprintf("Swap from LNbits: %d sats", amount))
	if err != nil {
		return fmt.Errorf("failed to create Breez invoice: %w", err)
	}

	log.Infof("[executeSwap] Created Breez invoice for %s: %s (amount=%d, balance=%d)",
		userStr, invoice.Bolt11, amount, lnbitsBalance)

	// 6. Pay invoice from LNbits
	_, err = user.Wallet.Pay(lnbits.PaymentParams{Out: true, Bolt11: invoice.Bolt11}, bot.Client)
	if err != nil {
		log.Warnf("[executeSwap] Pay request error for %s, checking if payment settled anyway: %s", userStr, err)
		if !bot.checkPaymentSettled(user, invoice.PaymentHash) {
			return fmt.Errorf("failed to pay invoice from LNbits: %w", err)
		}
		log.Infof("[executeSwap] Payment for %s settled despite Pay() error", userStr)
	}

	log.Infof("[executeSwap] Paid invoice from LNbits for %s", userStr)

	// 7. Sync Breez balance
	err = userBreez.RefreshBalance()
	if err != nil {
		log.Warnf("[executeSwap] Failed to sync Breez balance for %s: %s", userStr, err)
		// Don't return error, swap was successful
	}

	log.Infof("[executeSwap] Successfully swapped %d sats for %s", amount, userStr)
	return nil
}

// executeSwapWithContext performs the actual swap from LNbits to Breez (with plain context)
func (bot *TipBot) executeSwapWithContext(user *lnbits.User, amount int64, ctx context.Context) error {
	userStr := GetUserStr(user.Telegram)

	// 1. Check if Breez is initialized
	userBreez := bot.GetUserBreezClient(user)
	if userBreez == nil || !userBreez.IsInitialized() {
		return fmt.Errorf("breez not initialized")
	}

	// 2. Get LNbits balance
	lnbitsBalance, err := bot.GetLNbitsBalance(user)
	if err != nil {
		return fmt.Errorf("failed to get LNbits balance: %w", err)
	}

	// 3. Validate amount
	if amount < MinimumSwapAmount {
		return fmt.Errorf("amount below minimum: %d < %d", amount, MinimumSwapAmount)
	}

	// 4. Calculate fee buffer and validate balance can cover amount + fees
	// LNbits needs to pay: invoiceAmount + routingFee
	// We add a safety buffer (1% of amount + 50 sats) to ensure payment succeeds
	feeBuffer := int64(float64(amount)*0.01) + 50
	if feeBuffer < 50 {
		feeBuffer = 50
	}

	// If balance is too tight, reduce the amount further
	if amount+feeBuffer > lnbitsBalance {
		// Recalculate amount to fit within balance with safety margin
		newAmount := lnbitsBalance - feeBuffer
		if newAmount < MinimumSwapAmount {
			return fmt.Errorf("insufficient LNbits balance after fees: balance=%d, need=%d (amount=%d + buffer=%d)",
				lnbitsBalance, amount+feeBuffer, amount, feeBuffer)
		}
		log.Warnf("[executeSwapWithContext] Reducing swap amount from %d to %d to accommodate fees (balance=%d, buffer=%d)",
			amount, newAmount, lnbitsBalance, feeBuffer)
		amount = newAmount
	}

	// 5. Create Breez invoice
	invoice, err := userBreez.CreateInvoice(amount, fmt.Sprintf("Auto-swap from LNbits: %d sats", amount))
	if err != nil {
		return fmt.Errorf("failed to create Breez invoice: %w", err)
	}

	log.Infof("[executeSwapWithContext] Created Breez invoice for %s: %s (amount=%d, balance=%d)",
		userStr, invoice.Bolt11, amount, lnbitsBalance)

	// 6. Pay invoice from LNbits
	_, err = user.Wallet.Pay(lnbits.PaymentParams{Out: true, Bolt11: invoice.Bolt11}, bot.Client)
	if err != nil {
		log.Warnf("[executeSwapWithContext] Pay request error for %s, checking if payment settled anyway: %s", userStr, err)
		if !bot.checkPaymentSettled(user, invoice.PaymentHash) {
			return fmt.Errorf("failed to pay invoice from LNbits: %w", err)
		}
		log.Infof("[executeSwapWithContext] Payment for %s settled despite Pay() error", userStr)
	}

	log.Infof("[executeSwapWithContext] Paid invoice from LNbits for %s", userStr)

	// 7. Sync Breez balance
	err = userBreez.RefreshBalance()
	if err != nil {
		log.Warnf("[executeSwapWithContext] Failed to sync Breez balance for %s: %s", userStr, err)
		// Don't return error, swap was successful
	}

	// 8. Clear balance cache
	cacheKey := fmt.Sprintf("%s_balance", user.Name)
	bot.Cache.Delete(cacheKey)

	log.Infof("[executeSwapWithContext] Successfully swapped %d sats for %s", amount, userStr)
	return nil
}

// estimateSwapFees estimates the fees for a swap using Breez PrepareReceivePayment
func (bot *TipBot) estimateSwapFees(userBreez interface{}, amount int64) (int64, error) {
	// For now, return a conservative estimate of 1% of the amount
	// This is a simplified approach; ideally we'd use PrepareReceivePayment
	// but that requires exposing more methods from the breez client
	estimatedFees := int64(float64(amount) * 0.01)
	if estimatedFees < 100 {
		estimatedFees = 100 // Minimum 100 sats for fees
	}
	return estimatedFees, nil
}

// confirmSwapToLNbitsHandler executes the swap from Breez to LNbits
func (bot *TipBot) confirmSwapToLNbitsHandler(ctx intercept.Context) (intercept.Context, error) {
	tx := &SwapData{Base: storage.New(storage.ID(ctx.Data()))}
	mutex.LockWithContext(ctx, tx.ID)
	defer mutex.UnlockWithContext(ctx, tx.ID)

	sn, err := tx.Get(tx, bot.Bunt)
	if err != nil {
		log.Errorf("[confirmSwapToLNbitsHandler] %s", err.Error())
		return ctx, err
	}
	swapData := sn.(*SwapData)

	// Only the correct user can press
	if swapData.From.Telegram.ID != ctx.Sender().ID {
		return ctx, errors.Create(errors.UnknownError)
	}

	if !swapData.Active {
		log.Errorf("[confirmSwapToLNbitsHandler] swap not active anymore")
		bot.tryEditMessage(ctx.Message(), i18n.Translate(swapData.LanguageCode, "errorTryLaterMessage"), &tb.ReplyMarkup{})
		bot.tryDeleteMessage(ctx.Message())
		return ctx, errors.Create(errors.NotActiveError)
	}
	defer swapData.Set(swapData, bot.Bunt)

	user := LoadUser(ctx)
	if user.Wallet == nil {
		bot.tryDeleteMessage(ctx.Message())
		return ctx, errors.Create(errors.UserNoWalletError)
	}

	userStr := GetUserStr(ctx.Sender())

	// Update button text to show processing
	bot.tryEditMessage(
		ctx.Message(),
		swapData.Message,
		&tb.ReplyMarkup{
			InlineKeyboard: [][]tb.InlineButton{
				{tb.InlineButton{Unique: "processing_swap", Text: i18n.Translate(swapData.LanguageCode, "processingMessage")}},
			},
		},
	)

	log.Infof("[/swaptolnbits] Executing swap for %s: %d sats", userStr, swapData.Amount)

	// Execute the reverse swap (Breez to LNbits)
	err = bot.executeReverseSwap(user, swapData.Amount, ctx)
	if err != nil {
		log.Errorf("[/swaptolnbits] Swap failed for %s: %s", userStr, err)
		errMsg := fmt.Sprintf(i18n.Translate(swapData.LanguageCode, "swapFailed"), err.Error())
		bot.tryEditMessage(ctx.Message(), errMsg, &tb.ReplyMarkup{})
		return ctx, err
	}

	// Success!
	successMsg := fmt.Sprintf(i18n.Translate(swapData.LanguageCode, "swapToLNbitsSuccess"), swapData.Amount)
	bot.tryDeleteMessage(ctx.Message())
	bot.trySendMessage(ctx.Sender(), successMsg)

	log.Infof("[⚡️ swaptolnbits] User %s swapped %d sats from Breez to LNbits", userStr, swapData.Amount)

	// Inactivate the swap data
	return ctx, swapData.Inactivate(swapData, bot.Bunt)
}

// executeReverseSwap performs the actual swap from Breez to LNbits
func (bot *TipBot) executeReverseSwap(user *lnbits.User, amount int64, ctx intercept.Context) error {
	userStr := GetUserStr(user.Telegram)

	// 1. Check if Breez is initialized
	userBreez := bot.GetUserBreezClient(user)
	if userBreez == nil || !userBreez.IsInitialized() {
		return fmt.Errorf("breez not initialized")
	}

	// 2. Get Breez balance
	breezBalance, err := userBreez.GetBalance()
	if err != nil {
		return fmt.Errorf("failed to get Breez balance: %w", err)
	}

	// 3. Validate amount
	if amount < MinimumSwapAmount {
		return fmt.Errorf("amount below minimum: %d < %d", amount, MinimumSwapAmount)
	}

	// Enforce LNbits max balance (S) at execution time too
	lnbitsBalance, err := bot.GetLNbitsBalance(user)
	if err != nil {
		return fmt.Errorf("failed to get LNbits balance: %w", err)
	}
	S := internal.Configuration.Limits.LNbitsMaxBalance
	capacity := S - lnbitsBalance
	if capacity <= 0 {
		return fmt.Errorf("lnbits wallet full: balance=%d >= max=%d", lnbitsBalance, S)
	}
	if amount > capacity {
		return fmt.Errorf("amount exceeds lnbits capacity: amount=%d > capacity=%d (balance=%d max=%d)", amount, capacity, lnbitsBalance, S)
	}

	// 4. Create LNbits invoice
	invoice, err := user.Wallet.Invoice(
		lnbits.InvoiceParams{
			Out:     false,
			Amount:  amount,
			Memo:    fmt.Sprintf("Swap from Breez: %d sats", amount),
			Webhook: internal.Configuration.Lnbits.WebhookCall,
		},
		bot.Client)
	if err != nil {
		return fmt.Errorf("failed to create LNbits invoice: %w", err)
	}

	log.Infof("[executeReverseSwap] Created LNbits invoice for %s: %s", userStr, invoice.PaymentRequest)

	// 5. Enforce Breez balance including *real* fee estimate
	feeSats, feeErr := userBreez.EstimatePaymentFee(invoice.PaymentRequest)
	if feeErr != nil {
		return fmt.Errorf("failed to estimate Breez fee: %w", feeErr)
	}
	if amount+feeSats > breezBalance {
		return fmt.Errorf("insufficient Breez balance (amount+fee): balance=%d < need=%d (amount=%d fee=%d)", breezBalance, amount+feeSats, amount, feeSats)
	}

	// 6. Pay invoice from Breez
	_, err = userBreez.PayInvoice(invoice.PaymentRequest)
	if err != nil {
		return fmt.Errorf("failed to pay invoice from Breez: %w", err)
	}

	log.Infof("[executeReverseSwap] Paid invoice from Breez for %s", userStr)

	// 7. Sync Breez balance
	err = userBreez.RefreshBalance()
	if err != nil {
		log.Warnf("[executeReverseSwap] Failed to sync Breez balance for %s: %s", userStr, err)
		// Don't return error, swap was successful
	}

	// 8. Clear balance cache
	cacheKey := fmt.Sprintf("%s_balance", user.Name)
	bot.Cache.Delete(cacheKey)

	log.Infof("[executeReverseSwap] Successfully swapped %d sats from Breez to LNbits for %s", amount, userStr)
	return nil
}

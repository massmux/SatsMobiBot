package telegram

import (
	"fmt"
	"strconv"
	"strings"

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
	swapConfirmationMenu = &tb.ReplyMarkup{ResizeKeyboard: true}
	btnCancelSwap        = swapConfirmationMenu.Data("🚫 Cancel", "cancel_swap")
	btnConfirmSwap       = swapConfirmationMenu.Data("✅ Confirm Swap", "confirm_swap")
)

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
	userBreez := bot.GetUserBreezClient(user)
	if userBreez == nil || !userBreez.IsInitialized() {
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "swapBreezNotInitialized"))
		log.Warnf("[/swap] %s tried to swap but Breez not initialized", userStr)
		return ctx, errors.Create(errors.UserNoWalletError)
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

// swapAllHandler handles the /swap-all command - swaps entire LNbits balance
func (bot *TipBot) swapAllHandler(ctx intercept.Context) (intercept.Context, error) {
	// check and print all commands
	bot.anyTextHandler(ctx)

	user := LoadUser(ctx)
	if user.Wallet == nil {
		return ctx, errors.Create(errors.UserNoWalletError)
	}

	userStr := GetUserStr(ctx.Sender())

	// Check if Breez is initialized
	userBreez := bot.GetUserBreezClient(user)
	if userBreez == nil || !userBreez.IsInitialized() {
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "swapBreezNotInitialized"))
		log.Warnf("[/swap-all] %s tried to swap but Breez not initialized", userStr)
		return ctx, errors.Create(errors.UserNoWalletError)
	}

	// Get LNbits balance
	lnbitsBalance, err := bot.GetLNbitsBalance(user)
	if err != nil {
		log.Errorf("[/swap-all] Error fetching %s's LNbits balance: %s", userStr, err)
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "errorTryLaterMessage"))
		return ctx, err
	}

	// Check if user has sufficient balance
	if lnbitsBalance < MinimumSwapAmount {
		bot.trySendMessage(ctx.Sender(), fmt.Sprintf(Translate(ctx, "swapMinimumAmount"), MinimumSwapAmount))
		log.Warnf("[/swap-all] %s has insufficient balance for swap: %d sats", userStr, lnbitsBalance)
		return ctx, errors.Create(errors.InvalidAmountError)
	}

	// Estimate fees by preparing a receive payment
	// We'll create a test invoice amount to get fee estimate
	estimatedFees, err := bot.estimateSwapFees(userBreez, lnbitsBalance)
	if err != nil {
		log.Errorf("[/swap-all] Error estimating fees for %s: %s", userStr, err)
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "errorTryLaterMessage"))
		return ctx, err
	}

	// Calculate the amount to swap (balance - fees to ensure we can pay it)
	// Add a small buffer (1%) to account for fee variations
	swapAmount := lnbitsBalance - estimatedFees - int64(float64(lnbitsBalance)*0.01)

	if swapAmount < MinimumSwapAmount {
		bot.trySendMessage(ctx.Sender(), fmt.Sprintf(Translate(ctx, "swapMinimumAmount"), MinimumSwapAmount))
		log.Warnf("[/swap-all] %s calculated swap amount too low after fees: %d sats", userStr, swapAmount)
		return ctx, errors.Create(errors.InvalidAmountError)
	}

	// Show confirmation
	confirmText := fmt.Sprintf(Translate(ctx, "swapAllConfirmation"), swapAmount, lnbitsBalance, estimatedFees)
	log.Infof("[/swap-all] User: %s, LNbits balance: %d, swap amount: %d, estimated fees: %d",
		userStr, lnbitsBalance, swapAmount, estimatedFees)

	// Create swap confirmation data
	id := fmt.Sprintf("swap:%d-%d-%s", ctx.Sender().ID, swapAmount, RandStringRunes(5))

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
		Amount:          swapAmount,
		Message:         confirmText,
		LanguageCode:    ctx.Value("publicLanguageCode").(string),
		TelegramMessage: swapMessage,
	}

	// Save swap data
	runtime.IgnoreError(swapData.Set(swapData, bot.Bunt))

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

	if amount > lnbitsBalance {
		return fmt.Errorf("insufficient LNbits balance: %d < %d", lnbitsBalance, amount)
	}

	// 4. Create Breez invoice
	invoice, err := userBreez.CreateInvoice(amount, fmt.Sprintf("Swap from LNbits: %d sats", amount))
	if err != nil {
		return fmt.Errorf("failed to create Breez invoice: %w", err)
	}

	log.Infof("[executeSwap] Created Breez invoice for %s: %s", userStr, invoice.Bolt11)

	// 5. Pay invoice from LNbits
	_, err = user.Wallet.Pay(lnbits.PaymentParams{Out: true, Bolt11: invoice.Bolt11}, bot.Client)
	if err != nil {
		return fmt.Errorf("failed to pay invoice from LNbits: %w", err)
	}

	log.Infof("[executeSwap] Paid invoice from LNbits for %s", userStr)

	// 6. Sync Breez balance
	err = userBreez.RefreshBalance()
	if err != nil {
		log.Warnf("[executeSwap] Failed to sync Breez balance for %s: %s", userStr, err)
		// Don't return error, swap was successful
	}

	log.Infof("[executeSwap] Successfully swapped %d sats for %s", amount, userStr)
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

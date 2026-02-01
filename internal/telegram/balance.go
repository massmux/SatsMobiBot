package telegram

import (
	"fmt"
	"strconv"

	"github.com/massmux/SatsMobiBot/internal"
	"github.com/massmux/SatsMobiBot/internal/errors"
	"github.com/massmux/SatsMobiBot/internal/lnbits"
	"github.com/massmux/SatsMobiBot/internal/telegram/intercept"

	log "github.com/sirupsen/logrus"

	tb "gopkg.in/lightningtipbot/telebot.v3"
)

func (bot *TipBot) balanceHandler(ctx intercept.Context) (intercept.Context, error) {
	m := ctx.Message()
	// check and print all commands
	if len(m.Text) > 0 {
		bot.anyTextHandler(ctx)
	}

	// reply only in private message
	if m.Chat.Type != tb.ChatPrivate {
		// delete message
		bot.tryDeleteMessage(m)
	}
	// first check whether the user is initialized
	// Always reload user from database to get latest state
	user, err := GetUser(ctx.Sender(), *bot)
	if err != nil || user.Wallet == nil {
		return ctx, errors.Create(errors.UserNoWalletError)
	}

	if !user.Initialized {
		return bot.startHandler(ctx)
	}

	usrStr := GetUserStr(ctx.Sender())

	// Get LNbits custodial balance (ONLY LNbits, not total)
	lnbitsBalance, err := bot.GetLNbitsBalance(user)
	if err != nil {
		log.Errorf("[/balance] Error fetching %s's LNbits balance: %s", usrStr, err)
		lnbitsBalance = 0 // Continue to show Breez balance even if LNbits fails
	}

	// Get Breez self-custodial balance (if enabled and user has Breez)
	var breezBalance int64 = 0
	userBreez := bot.GetUserBreezClient(user)
	log.Debugf("[/balance] User %s - BreezInitialized: %v, BreezClient exists: %v", usrStr, user.BreezInitialized, userBreez != nil)

	// ✨ NUOVO: Se l'utente ha PIN ma il client non è in memoria, chiedi PIN
	if userBreez == nil && user.BreezInitialized && user.HasPin() {
		log.Infof("[/balance] Breez client not in memory for user with PIN, requesting PIN")

		// Salva lo stato per sapere che stiamo aspettando il PIN per il balance
		user.StateKey = lnbits.UserStateEnterPinForOperation
		user.StateData = "view_balance"
		UpdateUserRecord(user, *bot)

		bot.trySendMessage(user.Telegram,
			"🔐 Your self-custodial wallet requires your PIN.\n\n"+
				"Enter your PIN to view your complete balance:")

		// Mostra solo il balance LNbits per ora
		message := fmt.Sprintf(Translate(ctx, "balanceMessage"), lnbitsBalance)
		bot.trySendMessage(ctx.Message().Chat, message)
		return ctx, nil
	}

	if userBreez != nil && userBreez.IsInitialized() {
		log.Debugf("[/balance] Fetching Breez balance for %s", usrStr)
		breezBalance, err = bot.GetBreezBalance(user)
		if err != nil {
			log.Errorf("[/balance] Error fetching %s's Breez balance: %s", usrStr, err)
			// Continue with 0 Breez balance
		}
	} else {
		log.Debugf("[/balance] Breez not available for %s - userBreez=%v, initialized=%v", usrStr, userBreez != nil, user.BreezInitialized)
	}

	// Format message with dual balance
	totalBalance := lnbitsBalance + breezBalance
	var message string

	userBreezClient := bot.GetUserBreezClient(user)
	if userBreezClient != nil && userBreezClient.IsInitialized() {
		// Show dual balance when user has Breez
		message = formatDualBalanceMessage(ctx, lnbitsBalance, breezBalance, totalBalance)
		log.Infof("[/balance] %s's balance - LNbits: %d sat, Breez: %d sat, Total: %d sat", usrStr, lnbitsBalance, breezBalance, totalBalance)
	} else {
		// Show only LNbits balance when Breez is disabled
		message = fmt.Sprintf(Translate(ctx, "balanceMessage"), lnbitsBalance)
		log.Infof("[/balance] %s's balance: %d sat", usrStr, lnbitsBalance)
	}
	// Check the limit warning for custodial wallet
	if lnbitsBalance >= internal.Configuration.Limits.LNbitsMaxBalance {
		//balanceWarningMessage := fmt.Sprintf(Translate(ctx, "balanceOverMax"), strconv.FormatInt(internal.Configuration.Pos.Max_balance, 10))
		balanceWarningMessage := fmt.Sprintf(Translate(ctx, "balanceOverMax"), strconv.FormatInt(internal.Configuration.Limits.LNbitsMaxBalance, 10))
		message += "\n\n⚠️ " + balanceWarningMessage
		log.Infof("[/balance] User %s over max custodial balance: %d Sats", usrStr, lnbitsBalance)
		// In case we are over the maximum balance, then we propose the swap-all
		bot.trySendMessage(ctx.Sender(), message)
		return bot.swapAllHandler(ctx)
	}

	bot.trySendMessage(ctx.Sender(), message)
	return ctx, nil
}

// GetBreezBalance returns the Breez balance for a user
func (bot *TipBot) GetBreezBalance(user *lnbits.User) (int64, error) {
	userBreez := bot.GetUserBreezClient(user)
	if userBreez == nil || !userBreez.IsInitialized() {
		return 0, fmt.Errorf("user's Breez not initialized")
	}

	// Sync with network first to get latest balance (especially after payments)
	syncErr := userBreez.RefreshBalance()
	if syncErr != nil {
		log.Warnf("[GetBreezBalance] Failed to sync balance for %s: %s", GetUserStr(user.Telegram), syncErr)
		// Continue anyway and return cached balance
	}

	// Get user's personal Breez balance
	return userBreez.GetBalance()
}

// formatDualBalanceMessage formats a message showing both custodial and self-custodial balances
func formatDualBalanceMessage(ctx intercept.Context, custodial, selfCustodial, total int64) string {
	tpl := Translate(ctx, "balanceTotalMessage")
	return fmt.Sprintf(tpl, custodial, selfCustodial, total)

}

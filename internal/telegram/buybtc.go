package telegram

import (
	"fmt"

	breez_sdk "github.com/breez/breez-sdk-liquid-go/breez_sdk_liquid"
	"github.com/massmux/SatsMobiBot/internal/errors"
	"github.com/massmux/SatsMobiBot/internal/telegram/intercept"
	log "github.com/sirupsen/logrus"
	tb "gopkg.in/lightningtipbot/telebot.v3"
)

func (bot *TipBot) buyBtcHandler(ctx intercept.Context) (intercept.Context, error) {
	m := ctx.Message()
	user := LoadUser(ctx)

	// Ensure private chat only
	if !m.Private() {
		bot.tryDeleteMessage(m)
		return ctx, errors.Create(errors.NoPrivateChatError)
	}

	// Get user's Breez client
	userBreez := bot.GetUserBreezClient(user)
	if userBreez == nil || !userBreez.IsInitialized() {
		bot.trySendMessage(m.Sender, Translate(ctx, "buyBtcBreezNotAvailableMessage"))
		return ctx, fmt.Errorf("Breez not available for user")
	}

	// Show loading message
	statusMsg := bot.trySendMessageEditable(m.Sender, Translate(ctx, "buyBtcCheckingLimitsMessage"))

	// Fetch onchain limits
	limits, err := userBreez.FetchOnchainLimits()
	if err != nil {
		bot.tryEditMessage(statusMsg, Translate(ctx, "errorTryLaterMessage"))
		log.Errorf("[/buybtc] Failed to fetch limits: %s", err)
		return ctx, err
	}

	// Show limits to user
	limitsMsg := fmt.Sprintf(
		Translate(ctx, "buyBtcLimitsMessage"),
		limits.MinSat,
		limits.MaxSat,
	)
	bot.tryEditMessage(statusMsg, limitsMsg)

	// Use minimum amount for the purchase
	prepareReq := breez_sdk.PrepareBuyBitcoinRequest{
		Provider:  breez_sdk.BuyBitcoinProviderMoonpay,
		AmountSat: limits.MinSat,
	}

	prepareResp, err := userBreez.PrepareBuyBitcoin(prepareReq)
	if err != nil {
		bot.trySendMessage(m.Sender, Translate(ctx, "errorTryLaterMessage"))
		log.Errorf("[/buybtc] Failed to prepare buy: %s", err)
		return ctx, err
	}

	// Show fees
	feesMsg := fmt.Sprintf(Translate(ctx, "buyBtcFeesMessage"), prepareResp.FeesSat)
	bot.trySendMessage(m.Sender, feesMsg)

	// Generate purchase URL
	buyReq := breez_sdk.BuyBitcoinRequest{
		PrepareResponse: *prepareResp,
	}

	url, err := userBreez.BuyBitcoin(buyReq)
	if err != nil {
		bot.trySendMessage(m.Sender, Translate(ctx, "errorTryLaterMessage"))
		log.Errorf("[/buybtc] Failed to generate buy URL: %s", err)
		return ctx, err
	}

	// Send URL as inline button
	keyboard := &tb.ReplyMarkup{
		InlineKeyboard: [][]tb.InlineButton{
			{tb.InlineButton{Text: Translate(ctx, "buyBtcPurchaseButton"), URL: url}},
		},
	}

	bot.trySendMessage(m.Sender, Translate(ctx, "buyBtcReadyMessage"), keyboard)

	log.Infof("[/buybtc] Generated Moonpay URL for user %s", GetUserStr(user.Telegram))
	return ctx, nil
}

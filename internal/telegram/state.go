package telegram

import (
	"github.com/massmux/SatsMobiBot/internal/lnbits"
	"github.com/massmux/SatsMobiBot/internal/telegram/intercept"
)

type StateCallbackMessage map[lnbits.UserStateKey]func(ctx intercept.Context) (intercept.Context, error)

var stateCallbackMessage StateCallbackMessage

func initializeStateCallbackMessage(bot *TipBot) {
	stateCallbackMessage = StateCallbackMessage{
		lnbits.UserStateLNURLEnterAmount:      bot.enterAmountHandler,
		lnbits.UserEnterAmount:                bot.enterAmountHandler,
		lnbits.UserEnterUser:                  bot.enterUserHandler,
		lnbits.UserEnterShopTitle:             bot.enterShopTitleHandler,
		lnbits.UserStateShopItemSendPhoto:     bot.addShopItemPhoto,
		lnbits.UserStateShopItemSendPrice:     bot.enterShopItemPriceHandler,
		lnbits.UserStateShopItemSendTitle:     bot.enterShopItemTitleHandler,
		lnbits.UserStateShopItemSendItemFile:  bot.addItemFileHandler,
		lnbits.UserEnterShopsDescription:      bot.enterShopsDescriptionHandler,
		lnbits.UserEnterDallePrompt:           bot.confirmGenerateImages,
		lnbits.UserStateRefundAwaitingAddress: bot.handleRefundAddressInput,
		lnbits.UserStateSwapEnterAmount:       bot.enterSwapAmountHandler,
		lnbits.UserStateSwapLNEnterAmount:     bot.enterSwapLNAmountHandler,
		// ✨ PIN states
		lnbits.UserStateConfirmPinWarning:    bot.handlePinInput,
		lnbits.UserStateEnterNewPin:          bot.handlePinInput,
		lnbits.UserStateConfirmNewPin:        bot.handlePinInput,
		lnbits.UserStateEnterPinForOperation: bot.handlePinInput,
		lnbits.UserStateEnterPinForBackup:    bot.handlePinInput,
		lnbits.UserStateEnterPinForPayment:   bot.handlePinInput,
	}
}

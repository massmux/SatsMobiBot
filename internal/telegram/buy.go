package telegram

import (
	"fmt"
	"github.com/LightningTipBot/LightningTipBot/internal/errors"
	"github.com/LightningTipBot/LightningTipBot/internal/telegram/intercept"
	log "github.com/sirupsen/logrus"

	"github.com/almerlucke/go-iban/iban"
	tb "gopkg.in/lightningtipbot/telebot.v3"
)

// todo: this part is to be finished

func (bot *TipBot) buyHandler(ctx intercept.Context) (intercept.Context, error) {
	// commands: /buy IBAN
	m := ctx.Message()
	giveniban, err := getArgumentFromCommand(ctx.Message().Text, 1)
	if m.Chat.Type != tb.ChatPrivate {
		return ctx, errors.Create(errors.NoPrivateChatError)
	}
	log.Infof("[buyHandler] %s", m.Text)
	if err != nil {
		NewMessage(ctx.Message(), WithDuration(0, bot))
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "buyHelpText"))
		errmsg := fmt.Sprintf("[/buy] Error: Could not getArgumentFromCommand: %s", err.Error())
		log.Errorln(errmsg)
		return ctx, errors.New(errors.InvalidSyntaxError, err)
	}
	user := LoadUser(ctx)
	// load user settings
	user, err = GetLnbitsUserWithSettings(user.Telegram, *bot)
	if user.Wallet == nil {
		return ctx, errors.Create(errors.UserNoWalletError)
	}
	userStr := GetUserStr(user.Telegram)
	if m.Chat.Type != tb.ChatPrivate {
		// delete message
		bot.tryDeleteMessage(m)
		return ctx, errors.Create(errors.NoPrivateChatError)
	}
	iban, err := iban.NewIBAN(giveniban)
	if err != nil {
		fmt.Printf("%v\n", err)
		//bot.trySendMessage(ctx.Sender(), Translate(ctx, "activateScrubHelpText"))
		errmsg := fmt.Sprintf("[/buy] Error: invalid IBAN provided: %s", err.Error())
		log.Errorln(errmsg)
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "invalidIBANHelpText"))
		return ctx, errors.New(errors.InvalidSyntaxError, err)
	}
	log.Infof("[buyHandler] valid iban provided: %s", iban.Code)
	bot.trySendMessage(m.Sender, fmt.Sprintf("buy invoked from %s %s", userStr, giveniban))
	return ctx, err
}

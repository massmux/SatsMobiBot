package telegram

import (
	"context"
	"fmt"
	"github.com/LightningTipBot/LightningTipBot/internal"
	"github.com/LightningTipBot/LightningTipBot/internal/errors"
	"github.com/LightningTipBot/LightningTipBot/internal/telegram/intercept"
	log "github.com/sirupsen/logrus"
	"strconv"
)

func helpActivatecardUsage(ctx context.Context, errormsg string) string {
	if len(errormsg) > 0 {
		return fmt.Sprintf(Translate(ctx, "activatecardHelpText"), fmt.Sprintf("%s", errormsg))
	} else {
		return fmt.Sprintf(Translate(ctx, "activatecardHelpText"), "")
	}
}

// activate NFC card function
func (bot *TipBot) activatecardHandler(ctx intercept.Context) (intercept.Context, error) {
	cardID, err := getArgumentFromCommand(ctx.Message().Text, 1)
	if err != nil {
		NewMessage(ctx.Message(), WithDuration(0, bot))
		bot.trySendMessage(ctx.Sender(), helpActivatecardUsage(ctx, ""))
		errmsg := fmt.Sprintf("[/activatecard] Error: Could not getArgumentFromCommand: %s", err.Error())
		log.Errorln(errmsg)
		return ctx, errors.New(errors.InvalidSyntaxError, err)
	}

	// send confirmation to final user
	cardIdMsg := fmt.Sprintf(Translate(ctx, "activatecardSendText"), cardID)
	bot.trySendMessage(ctx.Sender(), cardIdMsg)

	// send activation request to bot admin
	userID := strconv.FormatInt(ctx.Sender().ID, 10)
	cardIdAdminMsg := fmt.Sprintf(Translate(ctx, "activatecardAdminSendText"), cardID, userID, ctx.Sender().Username, cardID)
	toUserDb, err := GetUserByTelegramUsername(internal.Configuration.Bot.Botadmin, *bot)
	bot.trySendMessage(toUserDb.Telegram, cardIdAdminMsg)

	return ctx, nil
}

func (bot *TipBot) confirmcardHandler(ctx intercept.Context) (intercept.Context, error) {
	if ctx.Sender().Username != internal.Configuration.Bot.Botadmin {
		// Not admin. do nothing exit
		return ctx, nil
	}
	userID, err := getArgumentFromCommand(ctx.Message().Text, 1)
	cardID, err := getArgumentFromCommand(ctx.Message().Text, 2)
	toUserDb, err := GetUserByTelegramUsername(userID, *bot)
	adminUserID, err := GetUserByTelegramUsername(internal.Configuration.Bot.Botadmin, *bot)
	if err != nil {
		NewMessage(ctx.Message(), WithDuration(0, bot))
		bot.trySendMessage(ctx.Sender(), helpActivatecardUsage(ctx, ""))
		errmsg := fmt.Sprintf("[/confirmcard] Error: Could not getArgumentFromCommand: %s", err.Error())
		log.Errorln(errmsg)
		return ctx, errors.New(errors.InvalidSyntaxError, err)
	}
	cardIdConfirmMsg := fmt.Sprintf(Translate(ctx, "confirmCardAdminSendText"), cardID)
	bot.trySendMessage(toUserDb.Telegram, cardIdConfirmMsg)
	bot.trySendMessage(adminUserID.Telegram, "Message sent!")
	return ctx, nil
}

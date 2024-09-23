package telegram

import (
	"fmt"
	"github.com/massmux/SatsMobiBot/internal"
	"strconv"

	"github.com/massmux/SatsMobiBot/internal/errors"
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
	user := LoadUser(ctx)
	if user.Wallet == nil {
		return ctx, errors.Create(errors.UserNoWalletError)
	}

	if !user.Initialized {
		return bot.startHandler(ctx)
	}

	usrStr := GetUserStr(ctx.Sender())
	balance, err := bot.GetUserBalance(user)
	if err != nil {
		log.Errorf("[/balance] Error fetching %s's balance: %s", usrStr, err)
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "balanceErrorMessage"))
		return ctx, err
	}

	log.Infof("[/balance] %s's balance: %d sat\n", usrStr, balance)
	bot.trySendMessage(ctx.Sender(), fmt.Sprintf(Translate(ctx, "balanceMessage"), balance))

	// check user balance. if more than Maximum allowed (in config) then send a warning message
	if balance >= internal.Configuration.Pos.Max_balance {
		balanceWarningMessage := fmt.Sprintf(Translate(ctx, "balanceOverMax"), strconv.FormatInt(internal.Configuration.Pos.Max_balance, 10))
		bot.trySendMessage(ctx.Sender(), balanceWarningMessage)
		log.Infof("[/balance] User %s over max balance: %d Sats\n", usrStr, balance)
	}

	return ctx, nil
}

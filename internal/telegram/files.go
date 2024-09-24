package telegram

import (
	"github.com/massmux/SatsMobiBot/internal/errors"
	"github.com/massmux/SatsMobiBot/internal/runtime"
	"github.com/massmux/SatsMobiBot/internal/telegram/intercept"
	tb "gopkg.in/lightningtipbot/telebot.v3"
	"time"
)

func (bot *TipBot) fileHandler(ctx intercept.Context) (intercept.Context, error) {
	m := ctx.Message()
	if m.Chat.Type != tb.ChatPrivate {
		return ctx, errors.Create(errors.NoPrivateChatError)
	}
	user := LoadUser(ctx)
	if c := stateCallbackMessage[user.StateKey]; c != nil {
		// found ctx for this state
		// now looking for user state reset ticker
		ticker := runtime.GetFunction(user.ID, runtime.WithTicker(time.NewTicker(runtime.DefaultTickerDuration)))
		if !ticker.Started {
			ticker.Do(func() {
				ResetUserState(user, bot)
				// removing ticker asap done
				bot.shopViewDeleteAllStatusMsgs(ctx, user)
				runtime.RemoveTicker(user.ID)
			})
		} else {
			ticker.ResetChan <- struct{}{}
		}

		return c(ctx)
	}
	return ctx, errors.Create(errors.NoFileFoundError)
}

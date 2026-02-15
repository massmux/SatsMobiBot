package telegram

import (
	"context"
	"fmt"

	"github.com/massmux/SatsMobiBot/internal/telegram/intercept"
	log "github.com/sirupsen/logrus"

	tb "gopkg.in/lightningtipbot/telebot.v3"
)

func (bot TipBot) makeHelpMessage(ctx context.Context, m *tb.Message) string {
	fromUser := LoadUser(ctx)
	dynamicHelpMessage := ""
	// user has no username set
	if len(m.Sender.Username) == 0 {
		// return fmt.Sprintf(helpMessage, fmt.Sprintf("%s\n\n", helpNoUsernameMessage))
		dynamicHelpMessage = dynamicHelpMessage + "\n" + Translate(ctx, "helpNoUsernameMessage")
	}
	lnaddr, _ := bot.UserGetLightningAddress(fromUser)
	if len(lnaddr) > 0 {
		dynamicHelpMessage = dynamicHelpMessage + "\n" + fmt.Sprintf(Translate(ctx, "infoYourLightningAddress"), lnaddr)
	}

	if len(dynamicHelpMessage) > 0 {
		dynamicHelpMessage = Translate(ctx, "infoHelpMessage") + dynamicHelpMessage
	}
	helpMessage := Translate(ctx, "helpMessage")
	return fmt.Sprintf(helpMessage, dynamicHelpMessage)
}

func (bot TipBot) helpHandler(ctx intercept.Context) (intercept.Context, error) {
	user := LoadUser(ctx)

	// Verify user is initialized
	if user == nil || !user.Initialized {
		log.Infof("[/help] User not initialized, redirecting to /start")
		return bot.startHandler(ctx)
	}

	// ✨ NUOVO: Check if user needs to set PIN first
	if !user.HasPin() {
		log.Infof("[/help] User %s needs to set PIN first", GetUserStr(user.Telegram))
		msg := "🔐 *Welcome to Your Self-Custodial Wallet*\n\n" +
			"To protect your funds, you need to set a PIN.\n\n" +
			"This PIN will:\n" +
			"• Encrypt your seed phrase\n" +
			"• Protect your wallet access\n" +
			"• Cannot be recovered if forgotten\n\n" +
			"Use /setpin to continue."
		bot.trySendMessage(ctx.Sender(), msg, tb.ModeMarkdown)
		return ctx, nil
	}

	// helpMessage has 2 placeholders for the bot username
	//helpMsg := fmt.Sprintf(Translate(ctx, "helpMessage"), bot.Telegram.Me.Username, bot.Telegram.Me.Username)
	// metti qui i lightning address e lnurl
	helpMsg := fmt.Sprintf(Translate(ctx, "helpMessage"), bot.Telegram.Me.Username)

	// Fetch standard (Hot) Lightning Address
	lnaddr, _ := bot.UserGetLightningAddress(user)
	if len(lnaddr) > 0 {
		helpMsg = helpMsg + "\n\n" + fmt.Sprintf(Translate(ctx, "infoYourLightningAddress"), lnaddr)
	}

	// Fetch anonymous Lightning Address
	anonLnaddr, err := bot.UserGetAnonLightningAddress(user)
	if err == nil && len(anonLnaddr) > 0 {
		helpMsg = helpMsg + "\n" + fmt.Sprintf(Translate(ctx, "infoYourAnonLightningAddress"), anonLnaddr)
	}

	// Fetch anonymous LNURL
	lnurl, err := UserGetAnonLNURL(user)
	if err == nil && len(lnurl) > 0 {
		// infoYourAnonLNURL must have exactly one %s in the .toml file
		helpMsg = helpMsg + "\n" + fmt.Sprintf(Translate(ctx, "infoYourAnonLNURL"), lnurl)
	}

	bot.trySendMessage(ctx.Sender(), helpMsg)
	return ctx, nil
}

func (bot TipBot) basicsHandler(ctx intercept.Context) (intercept.Context, error) {
	// check and print all commands
	bot.anyTextHandler(ctx)
	if !ctx.Message().Private() {
		// delete message
		bot.tryDeleteMessage(ctx.Message())
	}
	bot.trySendMessage(ctx.Sender(), Translate(ctx, "basicsMessage"), tb.NoPreview)
	return ctx, nil
}

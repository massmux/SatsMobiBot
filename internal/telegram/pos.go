package telegram

import (
	"fmt"
	"github.com/LightningTipBot/LightningTipBot/internal"
	"github.com/LightningTipBot/LightningTipBot/internal/lnbits"
	"github.com/LightningTipBot/LightningTipBot/internal/telegram/intercept"
	log "github.com/sirupsen/logrus"
)

// activate POS for the current User
func (bot *TipBot) posHandler(ctx intercept.Context) (intercept.Context, error) {
	m := ctx.Message()
	if internal.Configuration.Lnbits.LnbitsPublicUrl == "" {
		bot.trySendMessage(m.Sender, Translate(ctx, "couldNotLinkMessage"))
		return ctx, fmt.Errorf("invalid configuration")
	}
	// first check whether the user is initialized
	fromUser := LoadUser(ctx)

	posManager := lnbits.Tpos{ApiKey: fromUser.Wallet.Adminkey, LnbitsPublicUrl: internal.Configuration.Lnbits.LnbitsPublicUrl}
	createPos := posManager.PosCreate(ctx.Sender().Username, internal.Configuration.Pos.Currency)
	log.Infof("[/pos] User: %s, posID: %s ", ctx.Sender().Username, createPos)

	// send confirmation to final user
	posIdMsg := fmt.Sprintf(Translate(ctx, "posSendText"), ctx.Sender().Username, internal.Configuration.Lnbits.LnbitsPublicUrl, createPos)
	bot.trySendMessage(ctx.Sender(), posIdMsg)

	return ctx, nil
}

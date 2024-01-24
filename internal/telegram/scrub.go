package telegram

import (
	"fmt"
	"github.com/LightningTipBot/LightningTipBot/internal"
	"github.com/LightningTipBot/LightningTipBot/internal/errors"
	"github.com/LightningTipBot/LightningTipBot/internal/lnbits"
	"github.com/LightningTipBot/LightningTipBot/internal/telegram/intercept"
	log "github.com/sirupsen/logrus"
	"strings"
)

// activate Scrub for the current User
func (bot *TipBot) scrubHandler(ctx intercept.Context) (intercept.Context, error) {
	lnaddress, err := getArgumentFromCommand(ctx.Message().Text, 1)
	if err != nil {
		NewMessage(ctx.Message(), WithDuration(0, bot))
		bot.trySendMessage(ctx.Sender(), helpActivatecardUsage(ctx, ""))
		errmsg := fmt.Sprintf("[/scrub] Error: Could not getArgumentFromCommand: %s", err.Error())
		log.Errorln(errmsg)
		return ctx, errors.New(errors.InvalidSyntaxError, err)
	}

	// walletID string, lnAddress string, scrubName
	m := ctx.Message()
	if internal.Configuration.Lnbits.LnbitsPublicUrl == "" {
		bot.trySendMessage(m.Sender, Translate(ctx, "couldNotLinkMessage"))
		return ctx, fmt.Errorf("invalid configuration")
	}
	fromUser := LoadUser(ctx)

	// remove scrub from wallet
	if strings.ToLower(lnaddress) == "off" {
		scrubManager := lnbits.Scrub{ApiKey: fromUser.Wallet.Adminkey, LnbitsPublicURL: internal.Configuration.Lnbits.LnbitsPublicUrl}
		scrub := scrubManager.ScrubExists(ctx.Sender().Username)
		scrubID := ""
		if scrub != nil {
			scrubManager.ScrubDelete(scrub["id"].(string))
			scrubID = scrub["id"].(string)
		}
		log.Infof("[/scrub] Delete User: %s, ScrubID: %s ", ctx.Sender().Username, scrubID)

		// send confirmation of deactivation to final user
		scrubOffMsg := fmt.Sprintf(Translate(ctx, "scrubOffSendText"), ctx.Sender().Username)
		bot.trySendMessage(ctx.Sender(), scrubOffMsg)
		return ctx, nil
	}

	// activate scrub into the wallet
	scrubManager := lnbits.Scrub{ApiKey: fromUser.Wallet.Adminkey, LnbitsPublicURL: internal.Configuration.Lnbits.LnbitsPublicUrl}
	wID := scrubManager.WalletID()
	createScrub := scrubManager.ScrubCreate(wID, lnaddress, ctx.Sender().Username)
	log.Infof("[/scrub] Activate User: %s, WalletID: %s %s", ctx.Sender().Username, wID, createScrub)

	// send confirmation of activation to final user
	scrubIdMsg := fmt.Sprintf(Translate(ctx, "scrubSendText"), ctx.Sender().Username, lnaddress)
	bot.trySendMessage(ctx.Sender(), scrubIdMsg)

	return ctx, nil
}

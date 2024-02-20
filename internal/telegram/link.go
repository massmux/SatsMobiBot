package telegram

import (
	"bytes"
	"fmt"
	"time"

	"github.com/LightningTipBot/LightningTipBot/internal/telegram/intercept"

	"github.com/LightningTipBot/LightningTipBot/internal"

	log "github.com/sirupsen/logrus"
	"github.com/skip2/go-qrcode"
	tb "gopkg.in/lightningtipbot/telebot.v3"
)

func (bot *TipBot) lndhubHandler(ctx intercept.Context) (intercept.Context, error) {
	m := ctx.Message()
	if internal.Configuration.Lnbits.LnbitsPublicUrl == "" {
		bot.trySendMessage(m.Sender, Translate(ctx, "couldNotLinkMessage"))
		return ctx, fmt.Errorf("invalid configuration")
	}
	// first check whether the user is initialized
	fromUser := LoadUser(ctx)
	linkmsg := bot.trySendMessageEditable(m.Sender, Translate(ctx, "walletConnectMessage"))

	lndhubUrl := fmt.Sprintf("lndhub://admin:%s@%slndhub/ext/", fromUser.Wallet.Adminkey, internal.Configuration.Lnbits.LnbitsPublicUrl)

	lndhubpassword := fromUser.Wallet.Adminkey
	lndhuburl := fmt.Sprintf("%slndhub/ext/", internal.Configuration.Lnbits.LnbitsPublicUrl)

	lndhubdetails := fmt.Sprintf("\nLndhub details\n\nUser: `admin`\nPassword: `%s`\nURL: `%s`", lndhubpassword, lndhuburl)

	// create qr code
	qr, err := qrcode.Encode(lndhubUrl, qrcode.Medium, 256)
	if err != nil {
		errmsg := fmt.Sprintf("[/invoice] Failed to create QR code for invoice: %s", err.Error())
		log.Errorln(errmsg)
		return ctx, err
	}

	// send the link to the user
	qrmsg := bot.trySendMessage(m.Sender, &tb.Photo{File: tb.File{FileReader: bytes.NewReader(qr)}, Caption: fmt.Sprintf("`%s`\n%s", lndhubUrl, lndhubdetails)})
	// auto delete
	go func() {
		time.Sleep(time.Second * 60)
		bot.tryDeleteMessage(qrmsg)
		bot.tryEditMessage(linkmsg, Translate(ctx, "linkHiddenMessage"), tb.Silent)
	}()
	return ctx, nil
}

func (bot *TipBot) apiHandler(ctx intercept.Context) (intercept.Context, error) {
	m := ctx.Message()
	fromUser := LoadUser(ctx)
	apimesg := bot.trySendMessageEditable(m.Sender, fmt.Sprintf(Translate(ctx, "apiConnectMessage"), fromUser.Wallet.Adminkey, fromUser.Wallet.Inkey))
	// auto delete
	go func() {
		time.Sleep(time.Second * 60)
		bot.tryEditMessage(apimesg, Translate(ctx, "apiHiddenMessage"))
	}()
	return ctx, nil
}

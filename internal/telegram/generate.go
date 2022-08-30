package telegram

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/LightningTipBot/LightningTipBot/internal"
	"github.com/LightningTipBot/LightningTipBot/internal/dalle"
	"github.com/LightningTipBot/LightningTipBot/internal/lnbits"
	"github.com/LightningTipBot/LightningTipBot/internal/runtime"
	"github.com/LightningTipBot/LightningTipBot/internal/telegram/intercept"
	log "github.com/sirupsen/logrus"
	"github.com/skip2/go-qrcode"
	tb "gopkg.in/lightningtipbot/telebot.v3"
)

// generateImages is called when the user enters /generate or /generate <prompt>
// asks the user for a prompt if not given
func (bot *TipBot) generateImages(ctx intercept.Context) (intercept.Context, error) {
	user := LoadUser(ctx)
	if user.Wallet == nil {
		return ctx, fmt.Errorf("user has no wallet")
	}

	if len(strings.Split(ctx.Message().Text, " ")) < 2 {
		// We need to save the pay state in the user state so we can load the payment in the next handler
		SetUserState(user, bot, lnbits.UserEnterDallePrompt, "")
		bot.trySendMessage(ctx.Message().Sender, "⌨️ Enter image prompt.", tb.ForceReply)
		return ctx, nil
	}
	// write the prompt into the command and call confirm
	m := ctx.Message()
	m.Text = GetMemoFromCommand(m.Text, 1)
	return bot.confirmGenerateImages(ctx)
}

// confirmGenerateImages is called when the user has entered a prompt through /generate <prompt>
// or because he answered to the request to enter it in generateImages()
// confirmGenerateImages will create an invoice that the user can pay and if they pay
// generateDalleImages will fetch the images and send it to the user
func (bot *TipBot) confirmGenerateImages(ctx intercept.Context) (intercept.Context, error) {
	user := LoadUser(ctx)

	ResetUserState(user, bot)
	m := ctx.Message()
	prompt := m.Text
	if user.Wallet == nil {
		return ctx, fmt.Errorf("user has no wallet")
	}
	me, err := GetUser(bot.Telegram.Me, *bot)
	if err != nil {
		return ctx, err
	}
	invoice, err := bot.createInvoiceWithEvent(ctx, me, internal.Configuration.Generate.DallePrice, fmt.Sprintf("DALLE2 %s", GetUserStr(user.Telegram)), InvoiceCallbackGenerateDalle, prompt)
	invoice.Payer = user
	if err != nil {
		return ctx, err
	}

	runtime.IgnoreError(bot.Bunt.Set(invoice))

	balance, err := bot.GetUserBalance(user)
	if err != nil {
		errmsg := fmt.Sprintf("[inlineReceive] Error: Could not get user balance: %s", err.Error())
		log.Warnln(errmsg)
	}

	bot.trySendMessage(ctx.Message().Sender, Translate(ctx, "generateDallePayInvoiceMessage"))

	// invoke internal pay if enough balance
	if balance >= internal.Configuration.Generate.DallePrice {
		m.Text = fmt.Sprintf("/pay %s", invoice.PaymentRequest)
		return bot.payHandler(ctx)
	}

	// create qr code
	qr, err := qrcode.Encode(invoice.PaymentRequest, qrcode.Medium, 256)
	if err != nil {
		bot.tryEditMessage(invoice.Message, Translate(ctx, "errorTryLaterMessage"))
		return ctx, err
	}

	// send the invoice data to user
	msg := bot.trySendMessage(ctx.Message().Sender, &tb.Photo{File: tb.File{FileReader: bytes.NewReader(qr)}, Caption: fmt.Sprintf("`%s`", invoice.PaymentRequest)})
	invoice.InvoiceMessage = msg
	runtime.IgnoreError(bot.Bunt.Set(invoice))
	return ctx, nil
}

// generateDalleImages is called by the invoice event when the user has paid
func (bot *TipBot) generateDalleImages(event Event) {
	invoiceEvent := event.(*InvoiceEvent)
	user := invoiceEvent.Payer
	if user == nil || user.Wallet == nil {
		return
	}
	// create the client with the bearer token api key

	dalleClient, err := dalle.NewHTTPClient(internal.Configuration.Generate.DalleKey)
	// handle err
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()
	// generate a task to create an image with a prompt
	task, err := dalleClient.Generate(ctx, invoiceEvent.CallbackData)
	if err != nil {

	}

	// poll the task.ID until status is succeeded
	var t *dalle.Task
	for {
		time.Sleep(time.Second * 3)

		t, err = dalleClient.GetTask(ctx, task.ID)
		// handle err

		if t.Status == dalle.StatusSucceeded {
			fmt.Println("task succeeded")
			break
		} else if t.Status == dalle.StatusRejected {
			log.Fatal("rejected: ", t.ID)
		}

		fmt.Println("task still pending")
	}

	// download the first generated image
	for _, data := range t.Generations.Data {

		reader, err := dalleClient.Download(ctx, data.ID)
		if err != nil {
			return
		}
		defer reader.Close()

		file, err := os.Create("images/" + data.ID + ".png")
		if err != nil {
			return
		}
		defer file.Close()
		_, err = io.Copy(file, reader)
		if err != nil {
			return
		}
		f, err := os.OpenFile("images/"+data.ID+".png", 0, os.ModePerm)
		if err != nil {
			return
		}
		bot.trySendMessage(invoiceEvent.Payer.Telegram, &tb.Photo{File: tb.File{FileReader: f}})
	}

	// handle err and close readCloser
}

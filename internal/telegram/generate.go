package telegram

import (
	"bytes"
	"context"
	"fmt"
	"github.com/LightningTipBot/LightningTipBot/internal/runtime"
	"github.com/LightningTipBot/LightningTipBot/internal/telegram/intercept"
	"github.com/dillonstreator/dalle"
	log "github.com/sirupsen/logrus"
	"github.com/skip2/go-qrcode"
	tb "gopkg.in/lightningtipbot/telebot.v3"
	"os"
	"time"
)

func (bot *TipBot) generateImages(ctx intercept.Context) (intercept.Context, error) {
	user := LoadUser(ctx)
	if user.Wallet == nil {
		return ctx, fmt.Errorf("user has no wallet")
	}
	me, err := GetUser(bot.Telegram.Me, *bot)
	if err != nil {
		return ctx, err
	}
	invoice, err := bot.createInvoiceWithEvent(ctx, me, 1, fmt.Sprintf("DALLE2 %s", GetUserStr(user.Telegram)), InvoiceCallbackGenerateDalle, "")

	if err != nil {
		return ctx, err
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

func (bot *TipBot) generateDalleImages(event Event) {
	invoiceEvent := event.(*InvoiceEvent)
	user := invoiceEvent.User
	if user.Wallet == nil {
		return
	}
	// create the client with the bearer token api key
	dalleClient, err := dalle.NewHTTPClient("")
	// handle err
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()
	// generate a task to create an image with a prompt
	task, err := dalleClient.Generate(ctx, "monkey printing bitcoin with money printing machine, cyberpunk")
	// handle err

	// poll the task.ID until status is succeeded
	var t *dalle.Task
	for {
		time.Sleep(time.Second * 3)

		t, err = dalleClient.GetTask(ctx, task.ID)
		// handle err
		t.Status = dalle.StatusSucceeded

		if t.Status == dalle.StatusSucceeded {
			fmt.Println("task succeeded")
			break
		} else if t.Status == dalle.StatusRejected {
			log.Fatal("rejected: ", t.ID)
		}

		fmt.Println("task still pending")
	}
	/*
		// download the first generated image
		for _, data := range t.Generations.Data {
			reader, err := dalleClient.Download(ctx, data.ID)
			if err != nil {
				continue
			}
			bot.trySendMessage(user.Telegram, &tb.Photo{File: tb.File{FileReader: reader}, Caption: fmt.Sprintf("Result")})
		}*/
	reader, err := os.OpenFile("image", 0, os.ModePerm)
	if err != nil {
		panic(err)
	}
	bot.trySendMessage(user.Telegram, &tb.Photo{File: tb.File{FileReader: reader}, Caption: fmt.Sprintf("Result")})
	// handle err and close readCloser
}

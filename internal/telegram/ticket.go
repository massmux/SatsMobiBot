package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/LightningTipBot/LightningTipBot/internal"
	"github.com/LightningTipBot/LightningTipBot/internal/errors"
	"github.com/LightningTipBot/LightningTipBot/internal/lnbits"
	"github.com/LightningTipBot/LightningTipBot/internal/runtime"
	"github.com/LightningTipBot/LightningTipBot/internal/storage"
	"github.com/LightningTipBot/LightningTipBot/internal/telegram/intercept"
	log "github.com/sirupsen/logrus"
	"github.com/skip2/go-qrcode"
	"github.com/tidwall/buntdb"
	tb "gopkg.in/lightningtipbot/telebot.v3"
	"strconv"
	"time"
)

var defaultTicketDuration = time.Minute * 15

// handleTelegramNewMember is invoked when users join groups.
// handler will create a new invoice and send it to the group chat.
// a ticket callback timer function is stored in the blunt db.
func (bot *TipBot) handleTelegramNewMember(ctx intercept.Context) (intercept.Context, error) {
	id := strconv.FormatInt(ctx.Chat().ID, 10)
	group, err := bot.loadGroup(id)
	if err != nil {
		return ctx, err
	}
	if !bot.isAdmin(ctx.Chat(), bot.Telegram.Me) {
		fmt.Println("[TICKET] no admin")
		return ctx, fmt.Errorf("no admin rights")
	}
	user := LoadUser(ctx)
	ownerUser, err := GetUser(group.Owner, *bot)

	ticket := JoinTicket{Sender: ctx.Sender(),
		Chat: ctx.Chat(),

		Ticket: group.Ticket}

	/*	err = bot.Bunt.Get(&ticket)
		if err != nil {
			if err.Error() != "not found" {
				return ctx, err
			}
		}*/

	// group owner creates invoice
	invoice, err := ownerUser.Wallet.Invoice(
		lnbits.InvoiceParams{
			Out:     false,
			Amount:  ticket.Ticket.Price,
			Memo:    ticket.Ticket.Memo,
			Webhook: internal.Configuration.Lnbits.WebhookServer},
		bot.Client)
	if err != nil {
		errmsg := fmt.Sprintf("[handleTelegramNewMember] Could not create an invoice: %s", err.Error())
		log.Errorln(errmsg)
		return ctx, err
	}

	// create qr code
	qr, err := qrcode.Encode(invoice.PaymentRequest, qrcode.Medium, 256)
	if err != nil {
		return ctx, err
	}

	// send the owner invoice to chat

	ticketEvent := TicketEvent{
		InvoiceEvent: &InvoiceEvent{
			Base: storage.New(storage.ID(id)),
			Invoice: &Invoice{PaymentHash: invoice.PaymentHash,
				PaymentRequest: invoice.PaymentRequest,
				Amount:         ticket.Ticket.Price,
				Memo:           ticket.Ticket.Memo},
			Payer:        user,
			Chat:         ctx.Chat(),
			Callback:     InvoiceCallbackPayJoinTicket,
			CallbackData: "",
			LanguageCode: ctx.Value("publicLanguageCode").(string),
		},
		Group: group,
		Base:  storage.New(storage.ID(id)),
	}
	captionText := fmt.Sprintf("⚠️ %s, this group requires that you pay %d sat to be able to join.\n\nYou have 15 minutes to do it or you'll be kicked and banned for one day", GetUserStrMd(ctx.Message().Sender), ticket.Ticket.Price)
	entryMessage := &tb.Photo{
		File:    tb.File{FileReader: bytes.NewReader(qr)},
		Caption: captionText}

	// // if the user has enough balance, we send him a payment button
	balance, err := bot.GetUserBalance(user)
	if err != nil {
		errmsg := fmt.Sprintf("[/group] Error: Could not get user balance: %s", err.Error())
		log.Errorln(errmsg)
		bot.trySendMessage(ctx.Message().Sender, Translate(ctx, "errorTryLaterMessage"))
		return ctx, errors.New(errors.GetBalanceError, err)
	}
	var msg *tb.Message
	if balance >= group.Ticket.Price {
		confirm, menu := bot.getSendPayButton(ctx, ticketEvent)
		msg = bot.trySendMessageEditable(ctx.Chat(), fmt.Sprintf("%s\n%s", entryMessage.Caption, confirm), menu)
	} else {
		entryMessage.Caption = fmt.Sprintf("%s\n`%s`", entryMessage.Caption, invoice.PaymentRequest)
		msg = bot.trySendMessageEditable(ctx.Chat(), entryMessage)
	}
	ticketEvent.Message = msg
	ticket.Message = msg
	ticket.CreatedTimestamp = time.Now()
	err = bot.Bunt.Set(&ticket)
	if err != nil {
		return ctx, err
	}
	// save invoice struct for later use
	err = ticketEvent.Set(&ticketEvent, bot.Bunt)
	if err != nil {
		return ctx, err
	}
	err = ticketEvent.InvoiceEvent.Set(ticketEvent.InvoiceEvent, bot.Bunt)
	if err != nil {
		return ctx, err
	}
	bot.startTicketCallbackFunctionTimer(ticket)
	fmt.Println(ctx.Message())
	return ctx, nil
}

// stopTicketTimer will load the timer function based on the event.
// should stop the ticker timer function and remove ticket from bluntDB.
func (bot *TipBot) stopJoinTicketTimer(event Event) {
	ev := event.(*InvoiceEvent)
	ticket := JoinTicket{Sender: ev.Payer.Telegram, Chat: ev.Chat}
	err := bot.Bunt.Get(&ticket)
	if err != nil {
		return
	}
	me, err := GetUser(bot.Telegram.Me, *bot)
	invoice, err := me.Wallet.Invoice(
		lnbits.InvoiceParams{
			Out:     false,
			Amount:  ticket.Ticket.Cut,
			Memo:    ticket.Ticket.Memo,
			Webhook: internal.Configuration.Lnbits.WebhookServer},
		bot.Client)
	ticket.Ticket.Creator.Wallet.Pay(lnbits.PaymentParams{Bolt11: invoice.PaymentRequest, Out: true}, bot.Client)
	d := time.Until(time.Now().Add(defaultTicketDuration))
	bot.tryDeleteMessage(ev.Message)
	t := runtime.GetFunction(ticket.Key(), runtime.WithDuration(d))
	t.StopChan <- struct{}{}
	err = bot.Bunt.Delete(ticket.Key(), &ticket)
	if err != nil {
		log.Errorln(err)
	}
}

// startTicketCallbackFunctionTimer will start a ticket which will ban users if the timer runs out.
func (bot *TipBot) startTicketCallbackFunctionTimer(ticket JoinTicket) {
	// check if ticket is already expired
	if ticket.CreatedTimestamp.Add(defaultTicketDuration).Before(time.Now()) {
		return
	}
	// ticket is valid. create duration until kick
	ticketDuration := time.Until(ticket.CreatedTimestamp.Add(defaultTicketDuration))
	// create function timer
	t := runtime.NewResettableFunction(ticket.Key(),
		runtime.WithTimer(time.NewTimer(ticketDuration)))
	// run the ticket callback function
	t.Do(func() {
		// ticket expired
		member, err := bot.Telegram.ChatMemberOf(ticket.Chat, ticket.Sender)
		if err != nil {
			log.Errorln("🧨 could not fetch / ban chat member")
			return
		}
		// ban user
		err = bot.Telegram.Ban(ticket.Chat, member)
		if err != nil {
			log.Errorln(err)
		}
		err = bot.Bunt.Delete(ticket.Key(), &ticket)
		if err != nil {
			log.Errorln(err)
		}
		bot.tryDeleteMessage(ticket.Message)
	})
}

func (bot *TipBot) restartPersistedTickets() {
	bot.Bunt.View(func(tx *buntdb.Tx) error {
		err := tx.Ascend("join-ticket", func(key, value string) bool {
			ticket := JoinTicket{}
			json.Unmarshal([]byte(value), &ticket)
			bot.startTicketCallbackFunctionTimer(ticket)
			return true // continue iteration
		})
		return err
	})
}

package telegram

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/massmux/SatsMobiBot/internal/telegram/intercept"
	"github.com/nbd-wtf/go-nostr"

	"github.com/massmux/SatsMobiBot/internal/errors"
	"github.com/massmux/SatsMobiBot/internal/storage"

	"github.com/massmux/SatsMobiBot/internal"
	"github.com/massmux/SatsMobiBot/internal/i18n"
	"github.com/massmux/SatsMobiBot/internal/lnbits"
	"github.com/massmux/SatsMobiBot/internal/runtime"
	"github.com/massmux/SatsMobiBot/internal/str"

	log "github.com/sirupsen/logrus"
	"github.com/skip2/go-qrcode"
	tb "gopkg.in/lightningtipbot/telebot.v3"
)

type InvoiceEventCallback map[int]EventHandler

type EventHandler struct {
	Function func(event Event)
	Type     EventType
}

var InvoiceCallback InvoiceEventCallback

func initInvoiceEventCallbacks(bot *TipBot) {
	InvoiceCallback = InvoiceEventCallback{
		InvoiceCallbackGeneric:         EventHandler{Function: bot.notifyInvoiceReceivedEvent, Type: EventTypeInvoice},
		InvoiceCallbackInlineReceive:   EventHandler{Function: bot.inlineReceiveEvent, Type: EventTypeInvoice},
		InvoiceCallbackLNURLPayReceive: EventHandler{Function: bot.lnurlReceiveEvent, Type: EventTypeInvoice},
		InvoiceCallbackGroupTicket:     EventHandler{Function: bot.groupGetInviteLinkHandler, Type: EventTypeInvoice},
		InvoiceCallbackSatdressProxy:   EventHandler{Function: bot.satdressProxyRelayPaymentHandler, Type: EventTypeInvoice},
		InvoiceCallbackGenerateDalle:   EventHandler{Function: bot.generateDalleImages, Type: EventTypeInvoice},
		InvoiceCallbackPayJoinTicket:   EventHandler{Function: bot.stopJoinTicketTimer, Type: EventTypeInvoice},
	}
}

type InvoiceEventKey int

const (
	InvoiceCallbackGeneric = iota + 1
	InvoiceCallbackInlineReceive
	InvoiceCallbackLNURLPayReceive
	InvoiceCallbackGroupTicket
	InvoiceCallbackSatdressProxy
	InvoiceCallbackGenerateDalle
	InvoiceCallbackPayJoinTicket
)

const (
	EventTypeInvoice       EventType = "invoice"
	EventTypeTicketInvoice EventType = "ticket-invoice"
)

type EventType string

func AssertEventType(event Event, eventType EventType) error {
	if event.Type() != eventType {
		return fmt.Errorf("invalid event type")
	}
	return nil
}

type Invoice struct {
	PaymentHash    string `json:"payment_hash"`
	PaymentRequest string `json:"payment_request"`
	Amount         int64  `json:"amount"`
	Memo           string `json:"memo"`
}
type InvoiceEvent struct {
	*Invoice
	*storage.Base
	User           *lnbits.User `json:"user"`                      // the user that is being paid
	Message        *tb.Message  `json:"message,omitempty"`         // the message that the invoice replies to
	InvoiceMessage *tb.Message  `json:"invoice_message,omitempty"` // the message that displays the invoice
	WaitingMessage *tb.Message  `json:"waiting_message,omitempty"` // the "waiting for payment" message
	LanguageCode   string       `json:"languagecode"`              // language code of the user
	Callback       int          `json:"func"`                      // which function to call if the invoice is paid
	CallbackData   string       `json:"callbackdata"`              // add some data for the callback
	Chat           *tb.Chat     `json:"chat,omitempty"`            // if invoice is supposed to be sent to a particular chat
	Payer          *lnbits.User `json:"payer,omitempty"`           // if a particular user is supposed to pay this
	UserCurrency   string       `json:"usercurrency,omitempty"`    // the currency a user selected
}

func (invoiceEvent InvoiceEvent) Type() EventType {
	return EventTypeInvoice
}

type Event interface {
	Type() EventType
}

func (invoiceEvent InvoiceEvent) Key() string {
	return fmt.Sprintf("invoice:%s", invoiceEvent.PaymentHash)
}

func helpInvoiceUsage(ctx context.Context, errormsg string) string {
	if len(errormsg) > 0 {
		return fmt.Sprintf(Translate(ctx, "invoiceHelpText"), errormsg)
	} else {
		return fmt.Sprintf(Translate(ctx, "invoiceHelpText"), "")
	}
}

func (bot *TipBot) invoiceHandler(ctx intercept.Context) (intercept.Context, error) {
	m := ctx.Message()
	// check and print all commands
	bot.anyTextHandler(ctx)
	user := LoadUser(ctx)
	// load user settings
	user, err := GetLnbitsUserWithSettings(user.Telegram, *bot)
	if user.Wallet == nil {
		return ctx, errors.Create(errors.UserNoWalletError)
	}
	userStr := GetUserStr(user.Telegram)
	// we prevent the user from creating an invoice if the balance is over the imposed limit
	balance, err := bot.GetUserBalance(user)
	if balance >= internal.Configuration.Pos.Max_balance {
		balanceWarningMessage := fmt.Sprintf(Translate(ctx, "balanceOverMax"), strconv.FormatInt(internal.Configuration.Pos.Max_balance, 10))
		bot.trySendMessage(m.Sender, balanceWarningMessage)
		errmsg := fmt.Sprintf("[/balance] User %s over max balance: %d Sats", userStr, balance)
		log.Errorln(errmsg)
		return ctx, err
	}
	if m.Chat.Type != tb.ChatPrivate {
		// delete message
		bot.tryDeleteMessage(m)
		return ctx, errors.Create(errors.NoPrivateChatError)
	}

	// if no amount is in the command, ask for it
	amount, err := decodeAmountFromCommand(m.Text)
	if (err != nil || amount < 1) && m.Chat.Type == tb.ChatPrivate {
		// // no amount was entered, set user state and ask fo""r amount
		_, err = bot.askForAmount(ctx, "", "CreateInvoiceState", 0, 0, m.Text)
		return ctx, err
	}

	// Check Breez minimum amount (1000 sats) if Breez is enabled and will be used
	userBreezClient := bot.GetUserBreezClient(user)
	if internal.Configuration.Breez.Enabled && userBreezClient != nil && userBreezClient.IsInitialized() {
		const BreezMinAmount = 1000
		const BreezMaxAmount = 25000000

		if amount < BreezMinAmount {
			errorMsg := fmt.Sprintf(
				"⚠️ Minimum invoice amount: **%d sats**\n\n"+
					"Breez Lightning Network requires a minimum of %d sats for invoice creation.\n\n"+
					"Please use `/invoice %d` or higher.",
				BreezMinAmount, BreezMinAmount, BreezMinAmount,
			)
			bot.trySendMessage(m.Sender, errorMsg)
			return ctx, fmt.Errorf("amount below Breez minimum: %d < %d", amount, BreezMinAmount)
		}

		if amount > BreezMaxAmount {
			errorMsg := fmt.Sprintf(
				"⚠️ Maximum invoice amount: **%d sats** (%.2f BTC)\n\n"+
					"Breez Lightning Network allows a maximum of %d sats per invoice.\n\n"+
					"Please use `/invoice %d` or lower.",
				BreezMaxAmount, float64(BreezMaxAmount)/100000000, BreezMaxAmount, BreezMaxAmount,
			)
			bot.trySendMessage(m.Sender, errorMsg)
			return ctx, fmt.Errorf("amount above Breez maximum: %d > %d", amount, BreezMaxAmount)
		}
	}

	// check for memo in command
	memo := fmt.Sprintf("Powered by %s %s", internal.Configuration.Bot.Name, internal.Configuration.Bot.Username)
	if len(strings.Split(m.Text, " ")) > 2 {
		memo = GetMemoFromCommand(m.Text, 2)
		tag := fmt.Sprintf("(%s)", internal.Configuration.Bot.Username)
		memoMaxLen := 159 - len(tag)
		if len(memo) > memoMaxLen {
			memo = memo[:memoMaxLen-len(tag)]
		}
		memo = memo + tag
	}

	creatingMsg := bot.trySendMessageEditable(m.Sender, Translate(ctx, "lnurlGettingUserMessage"))
	log.Debugf("[/invoice] Creating invoice for %s of %d sat.", userStr, amount)

	currency := user.Settings.Display.DisplayCurrency
	if currency == "" {
		currency = "BTC"
	}

	invoice, err := bot.createInvoiceWithEvent(ctx, user, amount, memo, currency, InvoiceCallbackGeneric, "")
	if err != nil {
		errmsg := fmt.Sprintf("[/invoice] Could not create an invoice: %s", err.Error())
		bot.tryEditMessage(creatingMsg, Translate(ctx, "errorTryLaterMessage"))
		log.Errorln(errmsg)
		return ctx, err
	}

	// create qr code
	qr, err := qrcode.Encode(invoice.PaymentRequest, qrcode.Medium, 256)
	if err != nil {
		errmsg := fmt.Sprintf("[/invoice] Failed to create QR code for invoice: %s", err.Error())
		bot.tryEditMessage(creatingMsg, Translate(ctx, "errorTryLaterMessage"))
		log.Errorln(errmsg)
		return ctx, err
	}

	// deleting messages will delete the main menu.
	//bot.tryDeleteMessage(creatingMsg)

	// send the invoice data to user
	bot.trySendMessage(m.Sender, &tb.Photo{File: tb.File{FileReader: bytes.NewReader(qr)}, Caption: fmt.Sprintf("`%s`", invoice.PaymentRequest)})

	// Send waiting message
	waitingMsg := bot.trySendMessage(m.Sender, Translate(ctx, "invoiceWaitingMessage"))

	// Store waiting message in invoice event
	invoice.WaitingMessage = waitingMsg
	runtime.IgnoreError(bot.Bunt.Set(invoice))

	log.Printf("[/invoice] Invoice created. User: %s, amount: %d sat.", userStr, amount)
	return ctx, nil
}

func (bot *TipBot) createInvoiceWithEvent(ctx context.Context, user *lnbits.User, amount int64, memo string, currency string, callback int, callbackData string) (InvoiceEvent, error) {
	var paymentHash string
	var paymentRequest string

	// Decide whether to use Breez or LNbits based on routing logic
	useBreez := bot.shouldUseBreezForInvoice(user, amount)

	if useBreez {
		// Get user's Breez client
		userBreez := bot.GetUserBreezClient(user)
		if userBreez != nil && userBreez.IsInitialized() {
			// Create invoice using Breez SDK
			log.Infof("[/invoice] Creating Breez invoice for %s of %d sat", GetUserStr(user.Telegram), amount)
			breezInvoice, breezErr := userBreez.CreateInvoice(amount, memo)
			if breezErr != nil {
				errmsg := fmt.Sprintf("[/invoice] Breez invoice creation failed: %s, falling back to LNbits", breezErr.Error())
				log.Warnln(errmsg)
				// Fall back to LNbits
				useBreez = false
			} else {
				paymentRequest = breezInvoice.Bolt11
				paymentHash = breezInvoice.PaymentHash
				log.Infof("[/invoice] Breez invoice created successfully")

				// Translate message before callback to avoid context issues in goroutine
				paidMessage := Translate(ctx, "invoicePaidConfirmedMessage")

				// Start listening for payment events (120 second timeout)
				listenerID, listenerErr := userBreez.ListenForInvoicePayment(
					paymentHash,
					120*time.Second,
					func() {
						// Payment received callback (panic recovery handled in event listener)
						log.Infof("[Breez] Invoice paid callback triggered: %s", paymentHash)

						// Retrieve invoice event to get waiting message and user info
						invoiceEvent := InvoiceEvent{Invoice: &Invoice{PaymentHash: paymentHash}}
						if err := bot.Bunt.Get(&invoiceEvent); err == nil {
							// Sync Breez balance to get immediate update
							if invoiceEvent.User != nil {
								userBreez := bot.GetUserBreezClient(invoiceEvent.User)
								if userBreez != nil && userBreez.IsInitialized() {
									if syncErr := userBreez.RefreshBalance(); syncErr != nil {
										log.Warnf("[Breez] Failed to sync balance after payment: %s", syncErr)
									} else {
										log.Infof("[Breez] Balance synced after payment for user %s", invoiceEvent.User.Name)
									}
								}

								// Clear the user's balance cache so next balance check gets fresh data
								cacheKey := fmt.Sprintf("%s_balance", invoiceEvent.User.Name)
								bot.Cache.Delete(cacheKey)
								log.Debugf("[Breez] Cleared balance cache for user %s", invoiceEvent.User.Name)
							}

							// Send a new message to notify the user that the invoice was paid
							if invoiceEvent.User != nil {
								bot.trySendMessage(invoiceEvent.User.Telegram, paidMessage)
								log.Infof("[Breez] Sent invoice paid notification to user %s", invoiceEvent.User.Name)
							}

							// Delete the waiting message if it exists
							if invoiceEvent.WaitingMessage != nil {
								bot.tryDeleteMessage(invoiceEvent.WaitingMessage)
							}
						}
					},
				)
				if listenerErr != nil {
					log.Warnf("[/invoice] Failed to start payment listener: %s", listenerErr)
				} else {
					// Set up timeout to delete waiting message and remove listener if not paid
					go func(hash, listenerID string) {
						time.Sleep(120 * time.Second)

						// Retrieve invoice event
						invoiceEvent := InvoiceEvent{Invoice: &Invoice{PaymentHash: hash}}
						if err := bot.Bunt.Get(&invoiceEvent); err == nil {
							// Delete waiting message if timeout reached
							if invoiceEvent.WaitingMessage != nil {
								bot.tryDeleteMessage(invoiceEvent.WaitingMessage)
							}
						}

						log.Debugf("[Breez] Payment listener timeout for hash: %s", hash)
					}(paymentHash, listenerID)
				}
			}
		} else {
			useBreez = false
		}
	}

	// Use LNbits if Breez is not available or failed
	if !useBreez || paymentRequest == "" {
		log.Debugf("[/invoice] Creating LNbits invoice for %s of %d sat", GetUserStr(user.Telegram), amount)
		invoice, lnbitsErr := user.Wallet.Invoice(
			lnbits.InvoiceParams{
				Out:     false,
				Amount:  int64(amount),
				Memo:    memo,
				Webhook: internal.Configuration.Lnbits.WebhookCall},
			bot.Client)
		if lnbitsErr != nil {
			errmsg := fmt.Sprintf("[/invoice] Could not create an invoice: %s", lnbitsErr.Error())
			log.Errorln(errmsg)
			return InvoiceEvent{}, lnbitsErr
		}
		paymentHash = invoice.PaymentHash
		paymentRequest = invoice.PaymentRequest
	}

	invoiceEvent := InvoiceEvent{
		Invoice: &Invoice{PaymentHash: paymentHash,
			PaymentRequest: paymentRequest,
			Amount:         amount,
			Memo:           memo},
		User:         user,
		Callback:     callback,
		CallbackData: callbackData,
		LanguageCode: ctx.Value("publicLanguageCode").(string),
		UserCurrency: currency,
	}
	// save invoice struct for later use
	runtime.IgnoreError(bot.Bunt.Set(invoiceEvent))
	return invoiceEvent, nil
}

// shouldUseBreezForInvoice determines if Breez should be used for invoice creation
// SELF-CUSTODIAL FIRST: Always use Breez if available, fallback to LNbits if not initialized
func (bot *TipBot) shouldUseBreezForInvoice(user *lnbits.User, amount int64) bool {
	// Check if user has Breez available
	userBreez := bot.GetUserBreezClient(user)
	if userBreez == nil || !userBreez.IsInitialized() {
		log.Debugf("[shouldUseBreezForInvoice] User's Breez not available, falling back to LNbits")
		return false
	}

	// SELF-CUSTODIAL FIRST: Always prefer Breez for receiving if it's initialized
	log.Infof("[shouldUseBreezForInvoice] Breez is available, routing invoice to Breez (self-custodial default)")
	return true
}

// invoicelnHandler creates invoices ONLY using LNbits (bypasses Breez)
func (bot *TipBot) invoicelnHandler(ctx intercept.Context) (intercept.Context, error) {
	m := ctx.Message()
	// check and print all commands
	bot.anyTextHandler(ctx)
	user := LoadUser(ctx)
	// load user settings
	user, err := GetLnbitsUserWithSettings(user.Telegram, *bot)
	if user.Wallet == nil {
		return ctx, errors.Create(errors.UserNoWalletError)
	}
	userStr := GetUserStr(user.Telegram)
	// we prevent the user from creating an invoice if the balance is over the imposed limit
	balance, err := bot.GetUserBalance(user)
	if balance >= internal.Configuration.Pos.Max_balance {
		balanceWarningMessage := fmt.Sprintf(Translate(ctx, "balanceOverMax"), strconv.FormatInt(internal.Configuration.Pos.Max_balance, 10))
		bot.trySendMessage(m.Sender, balanceWarningMessage)
		errmsg := fmt.Sprintf("[/invoiceln] User %s over max balance: %d Sats", userStr, balance)
		log.Errorln(errmsg)
		return ctx, err
	}
	if m.Chat.Type != tb.ChatPrivate {
		// delete message
		bot.tryDeleteMessage(m)
		return ctx, errors.Create(errors.NoPrivateChatError)
	}

	// if no amount is in the command, ask for it
	amount, err := decodeAmountFromCommand(m.Text)
	if (err != nil || amount < 1) && m.Chat.Type == tb.ChatPrivate {
		// no amount was entered, set user state and ask for amount
		_, err = bot.askForAmount(ctx, "", "CreateInvoiceLNState", 0, 0, m.Text)
		return ctx, err
	}

	// check for memo in command
	memo := fmt.Sprintf("Powered by %s %s (LNbits)", internal.Configuration.Bot.Name, internal.Configuration.Bot.Username)
	if len(strings.Split(m.Text, " ")) > 2 {
		memo = GetMemoFromCommand(m.Text, 2)
		tag := fmt.Sprintf("(%s)", internal.Configuration.Bot.Username)
		memoMaxLen := 159 - len(tag)
		if len(memo) > memoMaxLen {
			memo = memo[:memoMaxLen-len(tag)]
		}
		memo = memo + tag
	}

	creatingMsg := bot.trySendMessageEditable(m.Sender, Translate(ctx, "lnurlGettingUserMessage"))
	log.Debugf("[/invoiceln] Creating LNbits invoice for %s of %d sat.", userStr, amount)

	currency := user.Settings.Display.DisplayCurrency
	if currency == "" {
		currency = "BTC"
	}

	// Create invoice directly with LNbits (bypass Breez)
	invoice, err := bot.createLNbitsInvoiceWithEvent(ctx, user, amount, memo, currency, InvoiceCallbackGeneric, "")
	if err != nil {
		errmsg := fmt.Sprintf("[/invoiceln] Could not create LNbits invoice: %s", err.Error())
		bot.tryEditMessage(creatingMsg, Translate(ctx, "errorTryLaterMessage"))
		log.Errorln(errmsg)
		return ctx, err
	}

	// create qr code
	qr, err := qrcode.Encode(invoice.PaymentRequest, qrcode.Medium, 256)
	if err != nil {
		errmsg := fmt.Sprintf("[/invoiceln] Failed to create QR code for invoice: %s", err.Error())
		bot.tryEditMessage(creatingMsg, Translate(ctx, "errorTryLaterMessage"))
		log.Errorln(errmsg)
		return ctx, err
	}

	// send the invoice data to user
	bot.trySendMessage(m.Sender, &tb.Photo{File: tb.File{FileReader: bytes.NewReader(qr)}, Caption: fmt.Sprintf("`%s`", invoice.PaymentRequest)})

	// Send waiting message
	waitingMsg := bot.trySendMessage(m.Sender, Translate(ctx, "invoiceWaitingMessage"))

	// Store waiting message in invoice event
	invoice.WaitingMessage = waitingMsg
	runtime.IgnoreError(bot.Bunt.Set(invoice))

	log.Printf("[/invoiceln] LNbits invoice created. User: %s, amount: %d sat.", userStr, amount)
	return ctx, nil
}

// createLNbitsInvoiceWithEvent creates an invoice using ONLY LNbits (no Breez fallback)
func (bot *TipBot) createLNbitsInvoiceWithEvent(ctx context.Context, user *lnbits.User, amount int64, memo string, currency string, callback int, callbackData string) (InvoiceEvent, error) {
	log.Debugf("[createLNbitsInvoice] Creating LNbits invoice for %s of %d sat", GetUserStr(user.Telegram), amount)

	invoice, err := user.Wallet.Invoice(
		lnbits.InvoiceParams{
			Out:     false,
			Amount:  int64(amount),
			Memo:    memo,
			Webhook: internal.Configuration.Lnbits.WebhookCall},
		bot.Client)
	if err != nil {
		errmsg := fmt.Sprintf("[createLNbitsInvoice] Could not create an invoice: %s", err.Error())
		log.Errorln(errmsg)
		return InvoiceEvent{}, err
	}

	invoiceEvent := InvoiceEvent{
		Invoice: &Invoice{PaymentHash: invoice.PaymentHash,
			PaymentRequest: invoice.PaymentRequest,
			Amount:         amount,
			Memo:           memo},
		User:         user,
		Callback:     callback,
		CallbackData: callbackData,
		LanguageCode: ctx.Value("publicLanguageCode").(string),
		UserCurrency: currency,
	}
	// save invoice struct for later use
	runtime.IgnoreError(bot.Bunt.Set(invoiceEvent))
	return invoiceEvent, nil
}

func (bot *TipBot) notifyInvoiceReceivedEvent(event Event) {
	invoiceEvent := event.(*InvoiceEvent)

	// Clear balance cache first to ensure we get fresh balance
	if invoiceEvent.User != nil {
		cacheKey := fmt.Sprintf("%s_balance", invoiceEvent.User.Name)
		bot.Cache.Delete(cacheKey)
		log.Debugf("[notifyInvoiceReceivedEvent] Cleared balance cache for user %s", invoiceEvent.User.Name)
	}

	// do balance check for keyboard update (will fetch fresh balance now)
	_, err := bot.GetUserBalance(invoiceEvent.User)
	if err != nil {
		errmsg := fmt.Sprintf("could not get balance of user %s", GetUserStr(invoiceEvent.User.Telegram))
		log.Errorln(errmsg)
	}

	if invoiceEvent.UserCurrency == "" || strings.ToLower(invoiceEvent.UserCurrency) == "btc" {
		bot.trySendMessage(invoiceEvent.User.Telegram, fmt.Sprintf(i18n.Translate(invoiceEvent.User.Telegram.LanguageCode, "invoiceReceivedMessage"), invoiceEvent.Amount))
	} else {
		fiatAmount, err := SatoshisToFiat(invoiceEvent.Amount, strings.ToUpper(invoiceEvent.UserCurrency))
		if err != nil {
			log.Errorln(err)
			// fallback to satoshis
			bot.trySendMessage(invoiceEvent.User.Telegram, fmt.Sprintf(i18n.Translate(invoiceEvent.User.Telegram.LanguageCode, "invoiceReceivedMessage"), invoiceEvent.Amount))
			return
		}
		bot.trySendMessage(invoiceEvent.User.Telegram, fmt.Sprintf(i18n.Translate(invoiceEvent.User.Telegram.LanguageCode, "invoiceReceivedCurrencyMessage"), invoiceEvent.Amount, fiatAmount, strings.ToUpper(invoiceEvent.UserCurrency)))
	}

	// Check for auto-swap if balance exceeds S2 threshold
	bot.checkAndPerformAutoSwap(invoiceEvent.User)
}

// checkAndPerformAutoSwap checks if LNbits balance exceeds S2 and performs automatic swap
func (bot *TipBot) checkAndPerformAutoSwap(user *lnbits.User) {
	// Check if user has Breez available
	userBreez := bot.GetUserBreezClient(user)
	if userBreez == nil || !userBreez.IsInitialized() {
		log.Debugf("[AutoSwap] User %s doesn't have Breez initialized, skipping auto-swap", GetUserStr(user.Telegram))
		return
	}

	// Get current LNbits balance
	lnbitsBalance, err := bot.GetLNbitsBalance(user)
	if err != nil {
		log.Errorf("[AutoSwap] Failed to get LNbits balance for %s: %s", GetUserStr(user.Telegram), err)
		return
	}

	// Get thresholds from config
	S := internal.Configuration.Limits.LNbitsMaxBalance
	S2 := internal.Configuration.Limits.AutoSwapThreshold

	// Check if balance exceeds auto-swap threshold
	if lnbitsBalance <= S2 {
		log.Debugf("[AutoSwap] LNbits balance %d <= threshold %d, no auto-swap needed", lnbitsBalance, S2)
		return
	}

	// Calculate swap amount: B - S
	swapAmount := lnbitsBalance - S
	if swapAmount < 1000 {
		log.Debugf("[AutoSwap] Swap amount %d too small (< 1000 sats), skipping", swapAmount)
		return
	}

	userStr := GetUserStr(user.Telegram)
	log.Infof("[AutoSwap] Triggering auto-swap for %s: %d sats (balance=%d, S=%d, S2=%d)",
		userStr, swapAmount, lnbitsBalance, S, S2)

	// Notify user about auto-swap
	bot.trySendMessage(user.Telegram, fmt.Sprintf(
		"⚡ *Auto-Swap Triggered*\n\n"+
			"Your LNbits balance (%d sats) exceeded the threshold (%d sats).\n"+
			"Automatically swapping %d sats to your Breez wallet...",
		lnbitsBalance, S2, swapAmount))

	// Perform the swap
	ctx := context.Background()
	err = bot.executeSwapWithContext(user, swapAmount, ctx)
	if err != nil {
		log.Errorf("[AutoSwap] Failed to execute auto-swap for %s: %s", userStr, err)
		bot.trySendMessage(user.Telegram, fmt.Sprintf(
			"❌ Auto-swap failed: %s\n\n"+
				"Please try manual swap with /swaptosafe", err.Error()))
		return
	}

	// Success notification
	bot.trySendMessage(user.Telegram, fmt.Sprintf(
		"✅ *Auto-Swap Complete*\n\n"+
			"Successfully swapped %d sats from LNbits to Breez!\n"+
			"Check your balance with /balance", swapAmount))
	log.Infof("[AutoSwap] Successfully completed auto-swap for %s: %d sats", userStr, swapAmount)
}

type LNURLInvoice struct {
	*Invoice
	Comment            string       `json:"comment"`
	User               *lnbits.User `json:"user"`
	CreatedAt          time.Time    `json:"created_at"`
	Paid               bool         `json:"paid"`
	PaidAt             time.Time    `json:"paid_at"`
	From               string       `json:"from"`
	Nip57Receipt       nostr.Event  `json:"nip57_receipt"`
	Nip57ReceiptRelays []string     `json:"nip57_receipt_relays"`
}

func (lnurlInvoice LNURLInvoice) Key() string {
	return fmt.Sprintf("lnurl-p:%s", lnurlInvoice.PaymentHash)
}

func (bot *TipBot) lnurlReceiveEvent(event Event) {
	invoiceEvent := event.(*InvoiceEvent)
	bot.notifyInvoiceReceivedEvent(invoiceEvent)

	tx := &LNURLInvoice{Invoice: &Invoice{PaymentHash: invoiceEvent.PaymentHash}}
	err := bot.Bunt.Get(tx)
	log.Debugf("[lnurl-p] Received invoice for %s of %d sat.", GetUserStr(invoiceEvent.User.Telegram), tx.Amount)
	if err == nil {
		// filter: if tx.Comment includes a URL, return if tx.Amount is less than 100 sat
		if len(tx.Comment) > 0 && tx.Amount < 100 {
			if strings.Contains(tx.Comment, "http") {
				log.Debugf("[lnurl-p] Filtered LNURL comment for %s of %d sat.", GetUserStr(invoiceEvent.User.Telegram), tx.Amount)
				return
			}
		}

		if tx.Amount < 21 {
			log.Debugf("[lnurl-p] Filtered LNURL comment for %s of %d sat.", GetUserStr(invoiceEvent.User.Telegram), tx.Amount)
			return
		}

		// notify user with LNURL comment and sender Information
		if len(tx.Comment) > 0 {
			if len(tx.From) == 0 {
				//bot.trySendMessage(tx.User.Telegram, fmt.Sprintf("✉️ %s", str.MarkdownEscape(tx.Comment)))
				bot.trySendMessage(tx.User.Telegram, fmt.Sprintf("✉️ %s", str.MarkdownEscape(tx.Comment)), tb.NoPreview)
			} else {
				//bot.trySendMessage(tx.User.Telegram, fmt.Sprintf("✉️ From `%s`: %s", tx.From, str.MarkdownEscape(tx.Comment)))
				bot.trySendMessage(tx.User.Telegram, fmt.Sprintf("✉️ From `%s`: %s", tx.From, str.MarkdownEscape(tx.Comment)), tb.NoPreview)
			}
		} else if len(tx.From) > 0 {
			//bot.trySendMessage(tx.User.Telegram, fmt.Sprintf("From `%s`", str.MarkdownEscape(tx.From)))
			bot.trySendMessage(tx.User.Telegram, fmt.Sprintf("From `%s`", str.MarkdownEscape(tx.From)), tb.NoPreview)
		}
		// send out NIP57 zap receipt
		if len(tx.Nip57Receipt.Sig) > 0 {
			// zapEventSerialized, _ := json.Marshal(tx.Nip57Receipt)
			bot.trySendMessage(tx.User.Telegram, "💜 This was a zap on nostr.")
			go bot.publishNostrEvent(tx.Nip57Receipt, tx.Nip57ReceiptRelays)
		}
	}
}

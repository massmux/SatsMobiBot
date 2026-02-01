package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/massmux/SatsMobiBot/internal/telegram/intercept"

	"github.com/massmux/SatsMobiBot/internal/errors"

	"github.com/massmux/SatsMobiBot/internal/runtime/mutex"
	"github.com/massmux/SatsMobiBot/internal/storage"

	"github.com/massmux/SatsMobiBot/internal/i18n"
	"github.com/massmux/SatsMobiBot/internal/lnbits"
	"github.com/massmux/SatsMobiBot/internal/runtime"

	lnurl "github.com/fiatjaf/go-lnurl"
	decodepay "github.com/fiatjaf/ln-decodepay"
	"github.com/massmux/SatsMobiBot/internal/str"
	log "github.com/sirupsen/logrus"
	tb "gopkg.in/lightningtipbot/telebot.v3"
)

var (
	paymentConfirmationMenu = &tb.ReplyMarkup{ResizeKeyboard: true}
	btnCancelPay            = paymentConfirmationMenu.Data("🚫 Cancel", "cancel_pay")
	btnPay                  = paymentConfirmationMenu.Data("✅ Pay", "confirm_pay")
)

func helpPayInvoiceUsage(ctx context.Context, errormsg string) string {
	if len(errormsg) > 0 {
		return fmt.Sprintf(Translate(ctx, "payHelpText"), errormsg)
	} else {
		return fmt.Sprintf(Translate(ctx, "payHelpText"), "")
	}
}

type PayData struct {
	*storage.Base
	From            *lnbits.User         `json:"from"`
	Invoice         string               `json:"invoice"`
	Hash            string               `json:"hash"`
	Proof           string               `json:"proof"`
	Memo            string               `json:"memo"`
	Message         string               `json:"message"`
	Amount          int64                `json:"amount"`
	LanguageCode    string               `json:"languagecode"`
	SuccessAction   *lnurl.SuccessAction `json:"successAction"`
	TelegramMessage *tb.Message          `json:"telegrammessage"`
}

// payHandler invoked on "/pay lnbc..." command
func (bot *TipBot) payHandler(ctx intercept.Context) (intercept.Context, error) {
	// check and print all commands
	bot.anyTextHandler(ctx)
	user := LoadUser(ctx)
	if user.Wallet == nil {
		return ctx, errors.Create(errors.UserNoWalletError)
	}
	if len(strings.Split(ctx.Message().Text, " ")) < 2 {
		NewMessage(ctx.Message(), WithDuration(0, bot))
		bot.trySendMessage(ctx.Sender(), helpPayInvoiceUsage(ctx, ""))
		return ctx, errors.Create(errors.InvalidSyntaxError)
	}
	userStr := GetUserStr(ctx.Sender())
	paymentRequest, err := getArgumentFromCommand(ctx.Message().Text, 1)
	if err != nil {
		NewMessage(ctx.Message(), WithDuration(0, bot))
		bot.trySendMessage(ctx.Sender(), helpPayInvoiceUsage(ctx, Translate(ctx, "invalidInvoiceHelpMessage")))
		errmsg := fmt.Sprintf("[/pay] Error: Could not getArgumentFromCommand: %s", err.Error())
		log.Errorln(errmsg)
		return ctx, errors.New(errors.InvalidSyntaxError, err)
	}
	paymentRequest = strings.ToLower(paymentRequest)
	// get rid of the URI prefix
	paymentRequest = strings.TrimPrefix(paymentRequest, "lightning:")

	// decode invoice
	bolt11, err := decodepay.Decodepay(paymentRequest)
	if err != nil {
		bot.trySendMessage(ctx.Sender(), helpPayInvoiceUsage(ctx, Translate(ctx, "invalidInvoiceHelpMessage")))
		errmsg := fmt.Sprintf("[/pay] Error: Could not decode invoice: %s", err.Error())
		log.Errorln(errmsg)
		return ctx, errors.New(errors.InvalidSyntaxError, err)
	}
	amount := int64(bolt11.MSatoshi / 1000)

	if amount <= 0 {
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "invoiceNoAmountMessage"))
		errmsg := fmt.Sprint("[/pay] Error: invoice without amount")
		log.Warnln(errmsg)
		return ctx, errors.Create(errors.InvalidAmountError)
	}

	// check user balance first
	balance, err := bot.GetUserBalance(user)
	if err != nil {
		NewMessage(ctx.Message(), WithDuration(0, bot))
		errmsg := fmt.Sprintf("[/pay] Error: Could not get user balance: %s", err.Error())
		log.Errorln(errmsg)
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "errorTryLaterMessage"))
		return ctx, errors.New(errors.GetBalanceError, err)
	}

	if amount > balance {
		NewMessage(ctx.Message(), WithDuration(0, bot))
		bot.trySendMessage(ctx.Sender(), fmt.Sprintf(Translate(ctx, "insufficientFundsMessage"), balance, amount))
		return ctx, errors.Create(errors.InvalidSyntaxError)
	}
	// send warning that the invoice might fail due to missing fee reserve
	if float64(amount) > float64(balance)*0.98 {
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "feeReserveMessage"))
	}

	confirmText := fmt.Sprintf(Translate(ctx, "confirmPayInvoiceMessage"), amount)
	if len(bolt11.Description) > 0 {
		confirmText = confirmText + fmt.Sprintf(Translate(ctx, "confirmPayAppendMemo"), str.MarkdownEscape(bolt11.Description))
	}

	log.Infof("[/pay] Invoice entered. User: %s, amount: %d sat.", userStr, amount)

	// object that holds all information about the send payment
	id := fmt.Sprintf("pay:%d-%d-%s", ctx.Sender().ID, amount, RandStringRunes(5))

	// // // create inline buttons
	payButton := paymentConfirmationMenu.Data(Translate(ctx, "payButtonMessage"), "confirm_pay", id)
	cancelButton := paymentConfirmationMenu.Data(Translate(ctx, "cancelButtonMessage"), "cancel_pay", id)

	paymentConfirmationMenu.Inline(
		paymentConfirmationMenu.Row(
			payButton,
			cancelButton),
	)
	payMessage := bot.trySendMessageEditable(ctx.Chat(), confirmText, paymentConfirmationMenu)
	// read successaction
	sa, ok := ctx.Value("SuccessAction").(*lnurl.SuccessAction)
	if !ok {
		sa = &lnurl.SuccessAction{}
	}

	payData := &PayData{
		Base:            storage.New(storage.ID(id)),
		From:            user,
		Invoice:         paymentRequest,
		Amount:          int64(amount),
		Memo:            bolt11.Description,
		Message:         confirmText,
		LanguageCode:    ctx.Value("publicLanguageCode").(string),
		SuccessAction:   sa,
		TelegramMessage: payMessage,
	}
	// add result to persistent struct
	runtime.IgnoreError(payData.Set(payData, bot.Bunt))

	SetUserState(user, bot, lnbits.UserStateConfirmPayment, paymentRequest)
	return ctx, nil
}

// confirmPayHandler when user clicked pay on payment confirmation
func (bot *TipBot) confirmPayHandler(ctx intercept.Context) (intercept.Context, error) {
	tx := &PayData{Base: storage.New(storage.ID(ctx.Data()))}
	mutex.LockWithContext(ctx, tx.ID)
	defer mutex.UnlockWithContext(ctx, tx.ID)
	sn, err := tx.Get(tx, bot.Bunt)
	// immediatelly set intransaction to block duplicate calls
	if err != nil {
		log.Errorf("[confirmPayHandler] %s", err.Error())
		return ctx, err
	}
	payData := sn.(*PayData)

	// onnly the correct user can press
	if payData.From.Telegram.ID != ctx.Sender().ID {
		return ctx, errors.Create(errors.UnknownError)
	}
	if !payData.Active {
		log.Errorf("[confirmPayHandler] send not active anymore")
		bot.tryEditMessage(ctx.Message(), i18n.Translate(payData.LanguageCode, "errorTryLaterMessage"), &tb.ReplyMarkup{})
		bot.tryDeleteMessage(ctx.Message())
		return ctx, errors.Create(errors.NotActiveError)
	}
	defer payData.Set(payData, bot.Bunt)

	// remove buttons from confirmation message
	// bot.tryEditMessage(handler.Message(), MarkdownEscape(payData.Message), &tb.ReplyMarkup{})

	user := LoadUser(ctx)
	if user.Wallet == nil {
		bot.tryDeleteMessage(ctx.Message())
		return ctx, errors.Create(errors.UserNoWalletError)
	}

	// ✨ MODIFICATO: Check if user has Breez initialized and enough balance
	// If client is not in memory but user has PIN, request PIN first
	if user.BreezInitialized && user.HasPin() {
		userBreezCheck := bot.GetUserBreezClient(user)

		// Se il client non è in memoria, chiedi il PIN per inizializzarlo
		if userBreezCheck == nil {
			log.Infof("[/pay] User %s has Breez but client not in memory, requesting PIN", GetUserStr(ctx.Sender()))

			// Salva i dati del pagamento nello stato utente
			user.StateKey = lnbits.UserStateEnterPinForPayment
			user.StateData = string(payData.ID)
			UpdateUserRecord(user, *bot)

			// Chiedi il PIN
			bot.trySendMessage(user.Telegram,
				"🔐 Your self-custodial wallet requires your PIN.\n\n"+
					"Enter your PIN to process this payment:")

			return ctx, nil
		}

		// Se il client è in memoria, verifica se ha abbastanza saldo per il pagamento
		if userBreezCheck.IsInitialized() {
			breezBalance, err := userBreezCheck.GetBalance()
			if err == nil {
				requiredBalance := int64(float64(payData.Amount) * 1.01) // 1% buffer for fees
				willUseBreez := breezBalance >= requiredBalance

				if willUseBreez {
					// Breez ha abbastanza saldo, chiedi il PIN per confermare
					log.Infof("[/pay] Breez has sufficient balance, requesting PIN from user %s", GetUserStr(ctx.Sender()))

					user.StateKey = lnbits.UserStateEnterPinForPayment
					user.StateData = string(payData.ID)
					UpdateUserRecord(user, *bot)

					bot.trySendMessage(user.Telegram,
						"🔐 This payment will use your self-custodial wallet.\n\n"+
							"Please enter your PIN to confirm:")

					return ctx, nil
				} else {
					log.Infof("[/pay] Breez balance insufficient (%d < %d), will use LNbits", breezBalance, requiredBalance)
				}
			}
		}
	}

	// ✨ Se arriviamo qui, o non ha Breez o non ha abbastanza saldo -> usa LNbits senza PIN

	// reset state immediately
	ResetUserState(user, bot)

	userStr := GetUserStr(ctx.Sender())

	// update button text
	bot.tryEditMessage(
		ctx.Message(),
		payData.Message,
		&tb.ReplyMarkup{
			InlineKeyboard: [][]tb.InlineButton{
				{tb.InlineButton{Unique: "attempt_payment", Text: i18n.Translate(payData.LanguageCode, "lnurlGettingUserMessage")}},
			},
		},
	)

	log.Infof("[/pay] Attempting %s's invoice %s (%d sat)", userStr, payData.ID, payData.Amount)

	// Pay invoice using appropriate wallet (Breez or LNbits)
	var paymentHash string
	var paymentErr error

	userBreez := bot.GetUserBreezClient(user)
	if bot.shouldUseBreezForPayment(user, payData.Amount) && userBreez != nil && userBreez.IsInitialized() {
		// Pay using Breez SDK
		log.Infof("[/pay] Paying with Breez: %d sats", payData.Amount)
		breezPayment, breezErr := userBreez.PayInvoice(payData.Invoice)
		if breezErr != nil {
			log.Warnf("[/pay] Breez payment failed: %s, falling back to LNbits", breezErr)
			// Fall back to LNbits
			invoice, lnbitsErr := user.Wallet.Pay(lnbits.PaymentParams{Out: true, Bolt11: payData.Invoice}, bot.Client)
			if lnbitsErr != nil {
				paymentErr = lnbitsErr
			} else {
				paymentHash = invoice.PaymentHash
			}
		} else {
			paymentHash = breezPayment.PaymentHash
			log.Infof("[/pay] Breez payment successful: %s", paymentHash)
		}
	} else {
		// Pay using LNbits
		log.Debugf("[/pay] Paying with LNbits: %d sats", payData.Amount)
		invoice, lnbitsErr := user.Wallet.Pay(lnbits.PaymentParams{Out: true, Bolt11: payData.Invoice}, bot.Client)
		if lnbitsErr != nil {
			paymentErr = lnbitsErr
		} else {
			paymentHash = invoice.PaymentHash
		}
	}

	if paymentErr != nil {
		errmsg := fmt.Sprintf("[/pay] Could not pay invoice of %s: %s", userStr, paymentErr)
		err = fmt.Errorf(i18n.Translate(payData.LanguageCode, "invoiceUndefinedErrorMessage"))
		bot.tryEditMessage(ctx.Message(), fmt.Sprintf(i18n.Translate(payData.LanguageCode, "invoicePaymentFailedMessage"), err.Error()), &tb.ReplyMarkup{})
		log.Errorln(errmsg)
		return ctx, paymentErr
	}
	payData.Hash = paymentHash

	// do balance check for keyboard update
	_, err = bot.GetUserBalance(user)
	if err != nil {
		errmsg := fmt.Sprintf("could not get balance of user %s", userStr)
		log.Errorln(errmsg)
	}

	if ctx.Message().Private() {
		// if the command was invoked in private chat
		// the edit below was cool, but we need to pop up the keyboard again
		// bot.tryEditMessage(c.Message, i18n.Translate(payData.LanguageCode, "invoicePaidMessage"), &tb.ReplyMarkup{})
		bot.tryDeleteMessage(ctx.Message())
		bot.trySendMessage(ctx.Sender(), i18n.Translate(payData.LanguageCode, "invoicePaidMessage"))
	} else {
		// if the command was invoked in group chat
		bot.trySendMessage(ctx.Sender(), i18n.Translate(payData.LanguageCode, "invoicePaidMessage"))
		bot.tryEditMessage(ctx.Message(), fmt.Sprintf(i18n.Translate(payData.LanguageCode, "invoicePublicPaidMessage"), userStr), &tb.ReplyMarkup{})
	}

	// display LNURL success action if present
	sa := payData.SuccessAction
	if sa.Tag == "message" && len(sa.Message) > 0 {
		bot.trySendMessage(ctx.Sender(), fmt.Sprintf("✉️: `%s`", sa.Message))
	} else if sa.Tag == "url" && len(sa.URL) > 0 {
		bot.trySendMessage(ctx.Sender(), fmt.Sprintf("🔗: %s", str.MarkdownEscape(sa.URL)), tb.NoPreview)
		if len(sa.Description) > 0 {
			bot.trySendMessage(ctx.Sender(), fmt.Sprintf("✉️: %s", sa.Description))
		}
	}

	log.Infof("[⚡️ pay] User %s paid invoice %s (%d sat)", userStr, payData.ID, payData.Amount)
	return ctx, nil
}

// shouldUseBreezForPayment determines if Breez should be used for sending payments
func (bot *TipBot) shouldUseBreezForPayment(user *lnbits.User, amount int64) bool {
	// Get user's Breez client
	userBreez := bot.GetUserBreezClient(user)
	if userBreez == nil || !userBreez.IsInitialized() {
		return false
	}

	// Get user's Breez balance
	breezBalance, err := userBreez.GetBalance()
	if err != nil {
		log.Warnf("[shouldUseBreezForPayment] Could not get user's Breez balance: %s", err)
		return false
	}

	// Use Breez if it has sufficient balance
	// Add 1% buffer for fees
	requiredBalance := int64(float64(amount) * 1.01)
	return breezBalance >= requiredBalance
}

// ✨ MODIFICATA: executeBreezPaymentWithPin esegue il pagamento Breez dopo verifica PIN
func (bot *TipBot) executeBreezPaymentWithPin(ctx intercept.Context, user *lnbits.User, payDataID string) error {
	// Recupera i dati del pagamento
	tx := &PayData{Base: storage.New(storage.ID(payDataID))}
	sn, err := tx.Get(tx, bot.Bunt)
	if err != nil {
		log.Errorf("[executeBreezPaymentWithPin] Failed to get payment data: %s", err)
		bot.trySendMessage(user.Telegram, "❌ Payment data not found. Please try again.")
		return err
	}
	payData := sn.(*PayData)

	if !payData.Active {
		bot.trySendMessage(user.Telegram, "❌ Payment is no longer active.")
		return fmt.Errorf("payment not active")
	}

	userStr := GetUserStr(user.Telegram)
	log.Infof("[/pay] Executing Breez payment for %s: %d sats", userStr, payData.Amount)

	// Mostra messaggio "processing"
	processingMsg := bot.trySendMessage(user.Telegram, "⚡ Processing payment...")

	// ✨ NUOVO: Verifica che il client Breez sia inizializzato
	// Il PIN è già stato verificato in handlePaymentPinInput, quindi possiamo usarlo
	userBreez := bot.GetUserBreezClient(user)
	if userBreez == nil || !userBreez.IsInitialized() {
		log.Warnf("[/pay] Breez client not in memory, attempting to initialize")

		// Il client deve essere inizializzato con il PIN prima di pagare
		// Questo non dovrebbe succedere se il flusso è corretto, ma come fallback
		bot.tryDeleteMessage(processingMsg)
		bot.trySendMessage(user.Telegram,
			"❌ Breez wallet not ready. Please try the payment again.")
		return fmt.Errorf("breez not initialized")
	}

	breezPayment, breezErr := userBreez.PayInvoice(payData.Invoice)
	if breezErr != nil {
		bot.tryDeleteMessage(processingMsg)
		log.Errorf("[/pay] Breez payment failed: %s", breezErr)
		bot.trySendMessage(user.Telegram,
			fmt.Sprintf("❌ Payment failed: %s", breezErr.Error()))
		return breezErr
	}

	// Pagamento riuscito
	payData.Hash = breezPayment.PaymentHash
	payData.Set(payData, bot.Bunt)

	bot.tryDeleteMessage(processingMsg)
	bot.trySendMessage(user.Telegram,
		"✅ Payment successful!\n\n"+
			fmt.Sprintf("Payment hash: `%s`", breezPayment.PaymentHash),
		tb.ModeMarkdown)

	log.Infof("[/pay] Breez payment successful: %s", breezPayment.PaymentHash)

	return nil
}

// cancelPaymentHandler invoked when user clicked cancel on payment confirmation
func (bot *TipBot) cancelPaymentHandler(ctx intercept.Context) (intercept.Context, error) {
	// reset state immediately
	user := LoadUser(ctx)
	ResetUserState(user, bot)
	tx := &PayData{Base: storage.New(storage.ID(ctx.Data()))}
	mutex.LockWithContext(ctx, tx.ID)
	defer mutex.UnlockWithContext(ctx, tx.ID)
	// immediatelly set intransaction to block duplicate calls
	sn, err := tx.Get(tx, bot.Bunt)
	if err != nil {
		log.Errorf("[cancelPaymentHandler] %s", err.Error())
		return ctx, err
	}
	payData := sn.(*PayData)
	// onnly the correct user can press
	if payData.From.Telegram.ID != ctx.Callback().Sender.ID {
		return ctx, errors.Create(errors.UnknownError)
	}
	// delete and send instead of edit for the keyboard to pop up after sending
	bot.tryDeleteMessage(ctx.Message())
	bot.trySendMessage(ctx.Message().Chat, i18n.Translate(payData.LanguageCode, "paymentCancelledMessage"))
	// bot.tryEditMessage(c.Message, i18n.Translate(payData.LanguageCode, "paymentCancelledMessage"), &tb.ReplyMarkup{})
	return ctx, payData.Inactivate(payData, bot.Bunt)

}

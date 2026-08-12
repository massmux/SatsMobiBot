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

	decodepay "github.com/fiatjaf/ln-decodepay"
	"github.com/massmux/SatsMobiBot/internal/str"
	log "github.com/sirupsen/logrus"
	tb "gopkg.in/lightningtipbot/telebot.v3"
)

// payLnConfirmationMenu is a dedicated menu/buttons so /payln does not share
// callback state with /pay (which may route through Breez + PIN).
var (
	payLnConfirmationMenu = &tb.ReplyMarkup{ResizeKeyboard: true}
	btnCancelPayLn        = payLnConfirmationMenu.Data("🚫 Cancel", "cancel_payln")
	btnPayLn              = payLnConfirmationMenu.Data("✅ Pay", "confirm_payln")
)

func helpPayLnInvoiceUsage(ctx context.Context, errormsg string) string {
	if len(errormsg) > 0 {
		return fmt.Sprintf(Translate(ctx, "paylnHelpText"), errormsg)
	}
	return fmt.Sprintf(Translate(ctx, "paylnHelpText"), "")
}

// payLnHandler is invoked on "/payln lnbc...". Unlike /pay, this command
// always pays from the LNbits ("Hot") balance and never touches Breez,
// so it keeps working even if the Breez/Safer wallet is not set up,
// not initialized, or currently broken.
func (bot *TipBot) payLnHandler(ctx intercept.Context) (intercept.Context, error) {
	bot.anyTextHandler(ctx)
	user := LoadUser(ctx)
	if user.Wallet == nil {
		return ctx, errors.Create(errors.UserNoWalletError)
	}
	if len(strings.Split(ctx.Message().Text, " ")) < 2 {
		NewMessage(ctx.Message(), WithDuration(0, bot))
		bot.trySendMessage(ctx.Sender(), helpPayLnInvoiceUsage(ctx, ""))
		return ctx, errors.Create(errors.InvalidSyntaxError)
	}
	userStr := GetUserStr(ctx.Sender())
	paymentRequest, err := getArgumentFromCommand(ctx.Message().Text, 1)
	if err != nil {
		NewMessage(ctx.Message(), WithDuration(0, bot))
		bot.trySendMessage(ctx.Sender(), helpPayLnInvoiceUsage(ctx, Translate(ctx, "invalidInvoiceHelpMessage")))
		errmsg := fmt.Sprintf("[/payln] Error: Could not getArgumentFromCommand: %s", err.Error())
		log.Errorln(errmsg)
		return ctx, errors.New(errors.InvalidSyntaxError, err)
	}
	paymentRequest = strings.ToLower(paymentRequest)
	// get rid of the URI prefix
	paymentRequest = strings.TrimPrefix(paymentRequest, "lightning:")

	// decode invoice
	bolt11, err := decodepay.Decodepay(paymentRequest)
	if err != nil {
		bot.trySendMessage(ctx.Sender(), helpPayLnInvoiceUsage(ctx, Translate(ctx, "invalidInvoiceHelpMessage")))
		errmsg := fmt.Sprintf("[/payln] Error: Could not decode invoice: %s", err.Error())
		log.Errorln(errmsg)
		return ctx, errors.New(errors.InvalidSyntaxError, err)
	}
	amount := int64(bolt11.MSatoshi / 1000)

	if amount <= 0 {
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "invoiceNoAmountMessage"))
		errmsg := fmt.Sprint("[/payln] Error: invoice without amount")
		log.Warnln(errmsg)
		return ctx, errors.Create(errors.InvalidAmountError)
	}

	// check the LNbits ("Hot") balance only, on purpose: this command must
	// keep working regardless of the state of the Breez/Safer wallet.
	balance, err := bot.GetLNbitsBalance(user)
	if err != nil {
		NewMessage(ctx.Message(), WithDuration(0, bot))
		errmsg := fmt.Sprintf("[/payln] Error: Could not get LNbits balance: %s", err.Error())
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

	confirmText := fmt.Sprintf(Translate(ctx, "confirmPayLnInvoiceMessage"), amount)
	if len(bolt11.Description) > 0 {
		confirmText = confirmText + fmt.Sprintf(Translate(ctx, "confirmPayAppendMemo"), str.MarkdownEscape(bolt11.Description))
	}

	log.Infof("[/payln] Invoice entered. User: %s, amount: %d sat.", userStr, amount)

	// object that holds all information about the send payment
	id := fmt.Sprintf("payln:%d-%d-%s", ctx.Sender().ID, amount, RandStringRunes(5))

	// create inline buttons
	payButton := payLnConfirmationMenu.Data(Translate(ctx, "payButtonMessage"), "confirm_payln", id)
	cancelButton := payLnConfirmationMenu.Data(Translate(ctx, "cancelButtonMessage"), "cancel_payln", id)

	payLnConfirmationMenu.Inline(
		payLnConfirmationMenu.Row(
			payButton,
			cancelButton),
	)
	payMessage := bot.trySendMessageEditable(ctx.Chat(), confirmText, payLnConfirmationMenu)

	payData := &PayData{
		Base:            storage.New(storage.ID(id)),
		From:            user,
		Invoice:         paymentRequest,
		Amount:          int64(amount),
		Memo:            bolt11.Description,
		Message:         confirmText,
		LanguageCode:    ctx.Value("publicLanguageCode").(string),
		TelegramMessage: payMessage,
	}
	// add result to persistent struct
	runtime.IgnoreError(payData.Set(payData, bot.Bunt))

	return ctx, nil
}

// confirmPayLnHandler is invoked when the user confirms a /payln payment.
// It always pays from LNbits and never checks or requires Breez/PIN.
func (bot *TipBot) confirmPayLnHandler(ctx intercept.Context) (intercept.Context, error) {
	tx := &PayData{Base: storage.New(storage.ID(ctx.Data()))}
	mutex.LockWithContext(ctx, tx.ID)
	defer mutex.UnlockWithContext(ctx, tx.ID)
	sn, err := tx.Get(tx, bot.Bunt)
	if err != nil {
		log.Errorf("[confirmPayLnHandler] %s", err.Error())
		return ctx, err
	}
	payData := sn.(*PayData)

	// only the correct user can press
	if payData.From.Telegram.ID != ctx.Sender().ID {
		return ctx, errors.Create(errors.UnknownError)
	}
	if !payData.Active {
		log.Errorf("[confirmPayLnHandler] send not active anymore")
		bot.tryEditMessage(ctx.Message(), i18n.Translate(payData.LanguageCode, "errorTryLaterMessage"), &tb.ReplyMarkup{})
		bot.tryDeleteMessage(ctx.Message())
		return ctx, errors.Create(errors.NotActiveError)
	}
	defer payData.Set(payData, bot.Bunt)

	user := LoadUser(ctx)
	if user.Wallet == nil {
		bot.tryDeleteMessage(ctx.Message())
		return ctx, errors.Create(errors.UserNoWalletError)
	}

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

	log.Infof("[/payln] Attempting %s's invoice %s (%d sat) via LNbits", userStr, payData.ID, payData.Amount)

	// always pay via LNbits ("Hot") wallet, regardless of Breez state
	invoice, lnbitsErr := user.Wallet.Pay(lnbits.PaymentParams{Out: true, Bolt11: payData.Invoice}, bot.Client)
	var paymentHash string
	if lnbitsErr != nil {
		// LNbits/Cloudflare can time out on a payment that actually settles
		// (see /swaptobreez fix): verify before declaring failure.
		if decoded, decodeErr := decodepay.Decodepay(payData.Invoice); decodeErr == nil {
			if bot.checkPaymentSettled(user, decoded.PaymentHash) {
				paymentHash = decoded.PaymentHash
				lnbitsErr = nil
			}
		}
	} else {
		paymentHash = invoice.PaymentHash
	}

	if lnbitsErr != nil {
		errmsg := fmt.Sprintf("[/payln] Could not pay invoice of %s: %s", userStr, lnbitsErr)
		err = fmt.Errorf(i18n.Translate(payData.LanguageCode, "invoiceUndefinedErrorMessage"))
		bot.tryEditMessage(ctx.Message(), fmt.Sprintf(i18n.Translate(payData.LanguageCode, "invoicePaymentFailedMessage"), err.Error()), &tb.ReplyMarkup{})
		log.Errorln(errmsg)
		return ctx, lnbitsErr
	}
	payData.Hash = paymentHash

	// do balance check for keyboard update
	_, err = bot.GetUserBalance(user)
	if err != nil {
		errmsg := fmt.Sprintf("could not get balance of user %s", userStr)
		log.Errorln(errmsg)
	}

	if ctx.Message().Private() {
		bot.tryDeleteMessage(ctx.Message())
		bot.trySendMessage(ctx.Sender(), i18n.Translate(payData.LanguageCode, "invoicePaidMessage"))
	} else {
		bot.trySendMessage(ctx.Sender(), i18n.Translate(payData.LanguageCode, "invoicePaidMessage"))
		bot.tryEditMessage(ctx.Message(), fmt.Sprintf(i18n.Translate(payData.LanguageCode, "invoicePublicPaidMessage"), userStr), &tb.ReplyMarkup{})
	}

	log.Infof("[⚡️ payln] User %s paid invoice %s (%d sat) via LNbits", userStr, payData.ID, payData.Amount)
	return ctx, nil
}

// cancelPayLnHandler is invoked when the user cancels a /payln confirmation.
func (bot *TipBot) cancelPayLnHandler(ctx intercept.Context) (intercept.Context, error) {
	tx := &PayData{Base: storage.New(storage.ID(ctx.Data()))}
	mutex.LockWithContext(ctx, tx.ID)
	defer mutex.UnlockWithContext(ctx, tx.ID)
	sn, err := tx.Get(tx, bot.Bunt)
	if err != nil {
		log.Errorf("[cancelPayLnHandler] %s", err.Error())
		return ctx, err
	}
	payData := sn.(*PayData)
	if payData.From.Telegram.ID != ctx.Callback().Sender.ID {
		return ctx, errors.Create(errors.UnknownError)
	}
	bot.tryDeleteMessage(ctx.Message())
	bot.trySendMessage(ctx.Message().Chat, i18n.Translate(payData.LanguageCode, "paymentCancelledMessage"))
	return ctx, payData.Inactivate(payData, bot.Bunt)
}

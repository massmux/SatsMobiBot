package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"

	"github.com/massmux/SatsMobiBot/internal/network"
	"github.com/massmux/SatsMobiBot/internal/telegram/intercept"

	"github.com/massmux/SatsMobiBot/internal/errors"

	"github.com/massmux/SatsMobiBot/internal/runtime/mutex"
	"github.com/massmux/SatsMobiBot/internal/storage"

	"github.com/massmux/SatsMobiBot/internal"
	"github.com/massmux/SatsMobiBot/internal/i18n"
	"github.com/massmux/SatsMobiBot/internal/lnbits"
	"github.com/massmux/SatsMobiBot/internal/runtime"

	lnurl "github.com/fiatjaf/go-lnurl"
	"github.com/massmux/SatsMobiBot/internal/str"
	log "github.com/sirupsen/logrus"
	tb "gopkg.in/lightningtipbot/telebot.v3"
)

var (
	withdrawConfirmationMenu = &tb.ReplyMarkup{ResizeKeyboard: true}
	btnCancelWithdraw        = paymentConfirmationMenu.Data("🚫 Cancel", "cancel_withdraw")
	btnWithdraw              = paymentConfirmationMenu.Data("✅ Withdraw", "confirm_withdraw")
)

// LnurlWithdrawState saves the state of the user for an LNURL payment
type LnurlWithdrawState struct {
	*storage.Base
	From                  *lnbits.User                `json:"from"`
	LNURLWithdrawResponse lnurl.LNURLWithdrawResponse `json:"LNURLWithdrawResponse"`
	LNURResponse          lnurl.LNURLResponse         `json:"LNURLResponse"`
	Amount                int64                       `json:"amount"`
	Comment               string                      `json:"comment"`
	LanguageCode          string                      `json:"languagecode"`
	Success               bool                        `json:"success"`
	Invoice               lnbits.Invoice              `json:"invoice"`
	Message               string                      `json:"message"`
}

type EditSingleButtonParams struct {
	Message    string
	ButtonText string
	Data       string
	URL        string
}

// editSingleButton edits a message to display a single button (for something like a progress indicator)
func (bot *TipBot) editSingleButton(ctx context.Context, m *tb.Message, params EditSingleButtonParams) (*tb.Message, error) {
	if len(params.URL) > 0 && len(params.Data) > 0 {
		return &tb.Message{}, fmt.Errorf("URL and Data cannot be set at the same time.")
	}
	if len(params.URL) == 0 && len(params.Data) == 0 {
		params.Data = "placeholder"
	}
	return bot.tryEditMessage(
		m,
		params.Message,
		&tb.ReplyMarkup{
			InlineKeyboard: [][]tb.InlineButton{
				{tb.InlineButton{Text: params.ButtonText, Data: params.Data, URL: params.URL}},
			},
		},
	)

}

// lnurlWithdrawHandler is invoked when the first lnurl response was a lnurl-withdraw response
// at this point, the user hans't necessarily entered an amount yet
func (bot *TipBot) lnurlWithdrawHandler(ctx intercept.Context, withdrawParams *LnurlWithdrawState) {
	m := ctx.Message()
	user := LoadUser(ctx)
	if user.Wallet == nil {
		return
	}
	// object that holds all information about the send payment
	id := fmt.Sprintf("lnurlw-%d-%s", m.Sender.ID, RandStringRunes(5))

	withdrawParams.Base = storage.New(storage.ID(id))
	withdrawParams.From = user
	withdrawParams.LanguageCode = ctx.Value("publicLanguageCode").(string)

	// first we check whether an amount is present in the command
	amount, amount_err := decodeAmountFromCommand(m.Text)

	// amount is already present in the command, i.e., /lnurl <amount> <LNURL>
	// amount not in allowed range from LNURL
	if amount_err == nil &&
		(int64(amount) > (withdrawParams.LNURLWithdrawResponse.MaxWithdrawable/1000) || int64(amount) < (withdrawParams.LNURLWithdrawResponse.MinWithdrawable/1000)) &&
		(withdrawParams.LNURLWithdrawResponse.MaxWithdrawable != 0 && withdrawParams.LNURLWithdrawResponse.MinWithdrawable != 0) { // only if max and min are set
		err := fmt.Errorf("amount not in range")
		log.Warnf("[lnurlWithdrawHandler] Error: %s", err.Error())
		bot.trySendMessage(m.Sender, fmt.Sprintf(Translate(ctx, "lnurlInvalidAmountRangeMessage"), withdrawParams.LNURLWithdrawResponse.MinWithdrawable/1000, withdrawParams.LNURLWithdrawResponse.MaxWithdrawable/1000))
		ResetUserState(user, bot)
		return
	}

	// if no amount is entered, and if only one amount is possible, we use it
	if amount_err != nil && withdrawParams.LNURLWithdrawResponse.MaxWithdrawable == withdrawParams.LNURLWithdrawResponse.MinWithdrawable {
		amount = int64(withdrawParams.LNURLWithdrawResponse.MaxWithdrawable / 1000)
		amount_err = nil
	}

	// set also amount in the state of the user
	withdrawParams.Amount = amount * 1000 // save as mSat

	// add result to persistent struct
	runtime.IgnoreError(withdrawParams.Set(withdrawParams, bot.Bunt))

	// now we actualy check whether the amount was in the command and if not, ask for it
	if amount_err != nil || amount < 1 {
		// // no amount was entered, set user state and ask for amount
		bot.askForAmount(ctx, id, "LnurlWithdrawState", withdrawParams.LNURLWithdrawResponse.MinWithdrawable, withdrawParams.LNURLWithdrawResponse.MaxWithdrawable, m.Text)
		return
	}

	// We need to save the pay state in the user state so we can load the payment in the next ctx
	paramsJson, err := json.Marshal(withdrawParams)
	if err != nil {
		log.Errorf("[lnurlWithdrawHandler] Error: %s", err.Error())
		// bot.trySendMessage(m.Sender, err.Error())
		return
	}
	SetUserState(user, bot, lnbits.UserHasEnteredAmount, string(paramsJson))
	// directly go to confirm
	bot.lnurlWithdrawHandlerWithdraw(ctx)
	return
}

// lnurlWithdrawHandlerWithdraw is invoked when the user has delivered an amount and is ready to pay
func (bot *TipBot) lnurlWithdrawHandlerWithdraw(ctx intercept.Context) (intercept.Context, error) {
	m := ctx.Message()
	user := LoadUser(ctx)
	if user.Wallet == nil {
		return ctx, errors.Create(errors.UserNoWalletError)
	}
	statusMsg := bot.trySendMessageEditable(m.Sender, Translate(ctx, "lnurlPreparingWithdraw"))

	// assert that user has entered an amount
	if user.StateKey != lnbits.UserHasEnteredAmount {
		log.Errorln("[lnurlWithdrawHandlerWithdraw] state keys don't match")
		bot.tryEditMessage(statusMsg, Translate(ctx, "errorTryLaterMessage"))
		return ctx, fmt.Errorf("wrong state key")
	}

	// read the enter amount state from user.StateData
	var enterAmountData EnterAmountStateData
	err := json.Unmarshal([]byte(user.StateData), &enterAmountData)
	if err != nil {
		log.Errorf("[lnurlWithdrawHandlerWithdraw] Error: %s", err.Error())
		bot.tryEditMessage(statusMsg, Translate(ctx, "errorTryLaterMessage"))
		return ctx, err
	}

	// use the enter amount state of the user to load the LNURL payment state
	tx := &LnurlWithdrawState{Base: storage.New(storage.ID(enterAmountData.ID))}
	mutex.LockWithContext(ctx, tx.ID)
	defer mutex.UnlockWithContext(ctx, tx.ID)
	fn, err := tx.Get(tx, bot.Bunt)
	if err != nil {
		log.Errorf("[lnurlWithdrawHandlerWithdraw] Error: %s", err.Error())
		bot.tryEditMessage(statusMsg, Translate(ctx, "errorTryLaterMessage"))
		return ctx, err
	}
	var lnurlWithdrawState *LnurlWithdrawState
	switch fn.(type) {
	case *LnurlWithdrawState:
		lnurlWithdrawState = fn.(*LnurlWithdrawState)
	default:
		log.Errorf("[lnurlWithdrawHandlerWithdraw] invalid type")
		bot.tryEditMessage(statusMsg, Translate(ctx, "errorTryLaterMessage"))
		return ctx, fmt.Errorf("invalid type")
	}

	confirmText := fmt.Sprintf(Translate(ctx, "confirmLnurlWithdrawMessage"), lnurlWithdrawState.Amount/1000)
	if len(lnurlWithdrawState.LNURLWithdrawResponse.DefaultDescription) > 0 {
		confirmText = confirmText + fmt.Sprintf(Translate(ctx, "confirmPayAppendMemo"), str.MarkdownEscape(lnurlWithdrawState.LNURLWithdrawResponse.DefaultDescription))
	}
	lnurlWithdrawState.Message = confirmText

	// create inline buttons
	withdrawButton := paymentConfirmationMenu.Data(Translate(ctx, "withdrawButtonMessage"), "confirm_withdraw", lnurlWithdrawState.ID)
	btnCancelWithdraw := paymentConfirmationMenu.Data(Translate(ctx, "cancelButtonMessage"), "cancel_withdraw", lnurlWithdrawState.ID)

	withdrawConfirmationMenu.Inline(
		withdrawConfirmationMenu.Row(
			withdrawButton,
			btnCancelWithdraw),
	)

	bot.tryEditMessage(statusMsg, confirmText, withdrawConfirmationMenu)

	// // add response to persistent struct
	// lnurlWithdrawState.LNURResponse = response2
	runtime.IgnoreError(lnurlWithdrawState.Set(lnurlWithdrawState, bot.Bunt))
	return ctx, nil
}

// confirmPayHandler when user clicked pay on payment confirmation
func (bot *TipBot) confirmWithdrawHandler(ctx intercept.Context) (intercept.Context, error) {
	c := ctx.Callback()
	tx := &LnurlWithdrawState{Base: storage.New(storage.ID(c.Data))}
	mutex.LockWithContext(ctx, tx.ID)
	defer mutex.UnlockWithContext(ctx, tx.ID)
	fn, err := tx.Get(tx, bot.Bunt)
	if err != nil {
		log.Errorf("[confirmWithdrawHandler] Error: %s", err.Error())
		return ctx, err
	}

	var lnurlWithdrawState *LnurlWithdrawState
	switch fn.(type) {
	case *LnurlWithdrawState:
		lnurlWithdrawState = fn.(*LnurlWithdrawState)
	default:
		log.Errorf("[confirmWithdrawHandler] invalid type")
		return ctx, errors.Create(errors.InvalidTypeError)
	}
	// onnly the correct user can press
	if lnurlWithdrawState.From.Telegram.ID != c.Sender.ID {
		return ctx, errors.Create(errors.UnknownError)
	}
	if !lnurlWithdrawState.Active {
		log.Errorf("[confirmPayHandler] send not active anymore")
		bot.tryEditMessage(c, i18n.Translate(lnurlWithdrawState.LanguageCode, "errorTryLaterMessage"), &tb.ReplyMarkup{})
		bot.tryDeleteMessage(c)
		return ctx, errors.Create(errors.NotActiveError)
	}
	defer lnurlWithdrawState.Set(lnurlWithdrawState, bot.Bunt)

	user := LoadUser(ctx)
	if user.Wallet == nil {
		bot.tryDeleteMessage(c)
		return ctx, errors.Create(errors.UserNoWalletError)
	}

	// reset state immediately
	ResetUserState(user, bot)

	// update button text
	bot.editSingleButton(ctx, c.Message, EditSingleButtonParams{Message: lnurlWithdrawState.Message, ButtonText: i18n.Translate(lnurlWithdrawState.LanguageCode, "lnurlPreparingWithdraw")})

	callbackUrl, err := url.Parse(lnurlWithdrawState.LNURLWithdrawResponse.Callback)
	if err != nil {
		log.Errorf("[lnurlWithdrawHandlerWithdraw] Error: %s", err.Error())
		// bot.trySendMessage(c.Sender, Translate(handler.Ctx, "errorTryLaterMessage"))
		bot.editSingleButton(ctx, c.Message, EditSingleButtonParams{Message: lnurlWithdrawState.Message, ButtonText: i18n.Translate(lnurlWithdrawState.LanguageCode, "errorTryLaterMessage")})
		return ctx, err
	}

	// generate an invoice and add the pr to the request
	// generate invoice
	invoice, err := user.Wallet.Invoice(
		lnbits.InvoiceParams{
			Out:     false,
			Amount:  int64(lnurlWithdrawState.Amount) / 1000,
			Memo:    "Withdraw",
			Webhook: internal.Configuration.Lnbits.WebhookCall},
		bot.Client)
	if err != nil {
		errmsg := fmt.Sprintf("[lnurlWithdrawHandlerWithdraw] Could not create an invoice: %s", err.Error())
		log.Errorln(errmsg)
		bot.editSingleButton(ctx, c.Message, EditSingleButtonParams{Message: lnurlWithdrawState.Message, ButtonText: i18n.Translate(lnurlWithdrawState.LanguageCode, "errorTryLaterMessage")})
		return ctx, err
	}
	lnurlWithdrawState.Invoice = invoice

	qs := callbackUrl.Query()
	// add amount to query string
	qs.Set("pr", invoice.PaymentRequest)
	qs.Set("k1", lnurlWithdrawState.LNURLWithdrawResponse.K1)
	callbackUrl.RawQuery = qs.Encode()

	// lnurlWithdrawState loaded
	client, err := network.GetClientForScheme(callbackUrl)
	if err != nil {
		log.Errorf("[lnurlWithdrawHandlerWithdraw] Error: %s", err.Error())
		// bot.trySendMessage(c.Sender, Translate(ctx, "errorTryLaterMessage"))
		bot.editSingleButton(ctx, c.Message, EditSingleButtonParams{Message: lnurlWithdrawState.Message, ButtonText: i18n.Translate(lnurlWithdrawState.LanguageCode, "errorTryLaterMessage")})
		return ctx, err
	}
	res, err := client.Get(callbackUrl.String())
	if err != nil || res.StatusCode >= 300 {
		log.Errorf("[lnurlWithdrawHandlerWithdraw] Failed.")
		// bot.trySendMessage(c.Sender, Translate(handler.Ctx, "errorTryLaterMessage"))
		bot.editSingleButton(ctx, c.Message, EditSingleButtonParams{Message: lnurlWithdrawState.Message, ButtonText: i18n.Translate(lnurlWithdrawState.LanguageCode, "errorTryLaterMessage")})
		return ctx, errors.New(errors.UnknownError, err)
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Errorf("[lnurlWithdrawHandlerWithdraw] Error: %s", err.Error())
		// bot.trySendMessage(c.Sender, Translate(handler.Ctx, "errorTryLaterMessage"))
		bot.editSingleButton(ctx, c.Message, EditSingleButtonParams{Message: lnurlWithdrawState.Message, ButtonText: i18n.Translate(lnurlWithdrawState.LanguageCode, "errorTryLaterMessage")})
		return ctx, err
	}

	// parse the response
	var response2 lnurl.LNURLResponse
	json.Unmarshal(body, &response2)
	if response2.Status == "OK" {
		// update button text
		bot.editSingleButton(ctx, c.Message, EditSingleButtonParams{Message: lnurlWithdrawState.Message, ButtonText: i18n.Translate(lnurlWithdrawState.LanguageCode, "lnurlWithdrawSuccess")})

	} else {
		log.Errorf("[lnurlWithdrawHandlerWithdraw] LNURLWithdraw failed.")
		// update button text
		bot.editSingleButton(ctx, c.Message, EditSingleButtonParams{Message: lnurlWithdrawState.Message, ButtonText: i18n.Translate(lnurlWithdrawState.LanguageCode, "lnurlWithdrawFailed")})
		return ctx, errors.New(errors.UnknownError, fmt.Errorf("LNURLWithdraw failed"))
	}

	// add response to persistent struct
	lnurlWithdrawState.LNURResponse = response2
	return ctx, lnurlWithdrawState.Set(lnurlWithdrawState, bot.Bunt)

}

// cancelPaymentHandler invoked when user clicked cancel on payment confirmation
func (bot *TipBot) cancelWithdrawHandler(ctx intercept.Context) (intercept.Context, error) {
	c := ctx.Callback()
	// reset state immediately
	user := LoadUser(ctx)
	ResetUserState(user, bot)
	tx := &LnurlWithdrawState{Base: storage.New(storage.ID(c.Data))}
	mutex.LockWithContext(ctx, tx.ID)
	defer mutex.UnlockWithContext(ctx, tx.ID)
	fn, err := tx.Get(tx, bot.Bunt)
	if err != nil {
		log.Errorf("[cancelWithdrawHandler] Error: %s", err.Error())
		return ctx, err
	}
	var lnurlWithdrawState *LnurlWithdrawState
	switch fn.(type) {
	case *LnurlWithdrawState:
		lnurlWithdrawState = fn.(*LnurlWithdrawState)
	default:
		log.Errorf("[cancelWithdrawHandler] invalid type")
	}
	// onnly the correct user can press
	if lnurlWithdrawState.From.Telegram.ID != c.Sender.ID {
		return ctx, errors.Create(errors.UnknownError)
	}
	bot.tryEditMessage(c, i18n.Translate(lnurlWithdrawState.LanguageCode, "lnurlWithdrawCancelled"), &tb.ReplyMarkup{})
	return ctx, lnurlWithdrawState.Inactivate(lnurlWithdrawState, bot.Bunt)
}

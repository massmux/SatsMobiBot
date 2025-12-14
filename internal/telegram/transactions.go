package telegram

import (
	"fmt"
	"sort"
	"time"

	"github.com/eko/gocache/store"
	"github.com/massmux/SatsMobiBot/internal/breez"
	"github.com/massmux/SatsMobiBot/internal/lnbits"
	"github.com/massmux/SatsMobiBot/internal/str"
	"github.com/massmux/SatsMobiBot/internal/telegram/intercept"
	log "github.com/sirupsen/logrus"
	tb "gopkg.in/lightningtipbot/telebot.v3"
)

// UnifiedPayment represents a payment from either LNbits or Breez
type UnifiedPayment struct {
	Time    int64
	Amount  int64
	Fee     int64
	Memo    string
	Pending bool
	Source  string // "lnbits" or "breez"
}

type TransactionsList struct {
	ID           string           `json:"id"`
	User         *lnbits.User     `json:"from"`
	Payments     []UnifiedPayment `json:"payments"`
	LanguageCode string           `json:"languagecode"`
	CurrentPage  int              `json:"currentpage"`
	MaxPages     int              `json:"maxpages"`
	TxPerPage    int              `json:"txperpage"`
}

// getMergedPayments merges LNbits and Breez payments into a unified list
func (bot *TipBot) getMergedPayments(user *lnbits.User) ([]UnifiedPayment, error) {
	var unified []UnifiedPayment

	// Get LNbits payments
	lnbitsPayments, lnbitsErr := bot.Client.Payments(*user.Wallet)
	if lnbitsErr == nil {
		for _, p := range lnbitsPayments {
			unified = append(unified, UnifiedPayment{
				Time:    int64(p.Time),
				Amount:  p.Amount / 1000, // Convert msat to sat
				Fee:     p.Fee,
				Memo:    p.Memo,
				Pending: p.Pending,
				Source:  "lnbits",
			})
		}
	} else {
		log.Warnf("[getMergedPayments] Failed to get LNbits payments: %s", lnbitsErr)
	}

	// Get Breez payments if user has Breez
	userBreez := bot.GetUserBreezClient(user)
	if userBreez != nil && userBreez.IsInitialized() {
		breezPayments, breezErr := userBreez.ListPayments(60)
		if breezErr == nil {
			for _, p := range breezPayments {
				amount := p.Amount
				if p.Direction == breez.PaymentDirectionOutbound {
					amount = -amount // Negative for outbound
				}
				unified = append(unified, UnifiedPayment{
					Time:    p.CreatedAt,
					Amount:  amount,
					Fee:     0, // Breez fees handled differently
					Memo:    p.Description,
					Pending: p.Status == breez.PaymentStatusPending,
					Source:  "breez",
				})
			}
		} else {
			log.Warnf("[getMergedPayments] Failed to get Breez payments: %s", breezErr)
		}
	}

	// Sort by timestamp (newest first)
	sort.Slice(unified, func(i, j int) bool {
		return unified[i].Time > unified[j].Time
	})

	return unified, nil
}

func (txlist *TransactionsList) printTransactions(ctx intercept.Context) string {
	txstr := ""
	payments := txlist.Payments
	pagenr := txlist.CurrentPage
	tx_per_page := txlist.TxPerPage
	if pagenr > (len(payments)+1)/tx_per_page {
		pagenr = 0
	}
	if len(payments) < tx_per_page {
		tx_per_page = len(payments)
	}
	start := pagenr * (tx_per_page - 1)
	end := start + tx_per_page
	if end >= len(payments) {
		end = len(payments) - 1
	}
	for i := start; i <= end; i++ {
		p := payments[i]

		// Add source indicator
		sourceEmoji := "🔵" // LNbits custodial
		if p.Source == "breez" {
			sourceEmoji = "⚡" // Breez self-custodial
		}

		if p.Pending {
			txstr += "🔄"
		} else {
			if p.Amount < 0 {
				txstr += "🔴"
			} else {
				txstr += "🟢"
			}
		}

		txstr += sourceEmoji // Add source indicator

		timestr := time.Unix(p.Time, 0).UTC().Format("2 Jan 06 15:04")
		txstr += fmt.Sprintf(" `%s`", timestr)
		txstr += fmt.Sprintf(" `%+d sat`", p.Amount)
		if p.Fee > 0 {
			fee := p.Fee
			if fee < 1000 {
				fee = 1000
			}
			txstr += fmt.Sprintf(" _(fee: %d sat)_", fee/1000)
		}
		memo := p.Memo
		memo_maxlen := 50
		if len(memo) > memo_maxlen {
			memo = memo[:memo_maxlen] + "..."
		}
		if len(memo) > 0 {
			txstr += fmt.Sprintf("\n✉️ %s", str.MarkdownEscape(memo))
		}
		txstr += "\n"
	}
	txstr += fmt.Sprintf("\nShowing %d transactions. Page %d of %d.", len(payments), txlist.CurrentPage+1, txlist.MaxPages)
	txstr += "\n\n🔵 = Hot Wallet | ⚡ = Safe Wallet"
	return txstr
}

var (
	transactionsMeno           = &tb.ReplyMarkup{ResizeKeyboard: true}
	btnLeftTransactionsButton  = inlineTipjarMenu.Data("◀️", "left_transactions")
	btnRightTransactionsButton = inlineTipjarMenu.Data("▶️", "right_transactions")
)

func (bot *TipBot) makeTransactionsKeyboard(ctx intercept.Context, txlist TransactionsList) *tb.ReplyMarkup {
	leftTransactionsButton := transactionsMeno.Data("←", "left_transactions", txlist.ID)
	rightTransactionsButton := transactionsMeno.Data("→", "right_transactions", txlist.ID)

	if txlist.CurrentPage == 0 {
		transactionsMeno.Inline(
			transactionsMeno.Row(
				leftTransactionsButton),
		)
	} else if txlist.CurrentPage == txlist.MaxPages-1 {
		transactionsMeno.Inline(
			transactionsMeno.Row(
				rightTransactionsButton),
		)
	} else {
		transactionsMeno.Inline(
			transactionsMeno.Row(
				leftTransactionsButton,
				rightTransactionsButton),
		)
	}
	return transactionsMeno
}

func (bot *TipBot) transactionsHandler(ctx intercept.Context) (intercept.Context, error) {
	m := ctx.Message()
	user := LoadUser(ctx)

	// Get merged payments from both LNbits and Breez
	payments, err := bot.getMergedPayments(user)
	if err != nil {
		log.Errorf("[transactions] Error: %s", err.Error())
		bot.trySendMessage(m.Sender, Translate(ctx, "errorTryLaterMessage"))
		return ctx, err
	}

	tx_per_page := 10
	transactionsList := TransactionsList{
		ID:           fmt.Sprintf("txlist:%d:%s", user.Telegram.ID, RandStringRunes(5)),
		User:         user,
		Payments:     payments,
		LanguageCode: ctx.Value("userLanguageCode").(string),
		CurrentPage:  0,
		TxPerPage:    tx_per_page,
		MaxPages:     (len(payments)+1)/tx_per_page + 1,
	}
	bot.Cache.Set(fmt.Sprintf("%s_transactions", user.Name), transactionsList, &store.Options{Expiration: 1 * time.Minute})
	txstr := transactionsList.printTransactions(ctx)
	bot.trySendMessage(m.Sender, txstr, bot.makeTransactionsKeyboard(ctx, transactionsList))
	return ctx, nil
}

func (bot *TipBot) transactionsScrollLeftHandler(ctx intercept.Context) (intercept.Context, error) {
	c := ctx.Callback()
	user := LoadUser(ctx)
	transactionsListInterface, err := bot.Cache.Get(fmt.Sprintf("%s_transactions", user.Name))
	if err != nil {
		log.Info("Transactions not in cache anymore")
		return ctx, err
	}
	transactionsList := transactionsListInterface.(TransactionsList)

	if c.Sender.ID == transactionsList.User.Telegram.ID {
		if transactionsList.CurrentPage < transactionsList.MaxPages-1 {
			transactionsList.CurrentPage++
		} else {
			return ctx, err
		}
		bot.Cache.Set(fmt.Sprintf("%s_transactions", user.Name), transactionsList, &store.Options{Expiration: 1 * time.Minute})
		bot.tryEditMessage(c.Message, transactionsList.printTransactions(ctx), bot.makeTransactionsKeyboard(ctx, transactionsList))
	}
	return ctx, nil
}

func (bot *TipBot) transactionsScrollRightHandler(ctx intercept.Context) (intercept.Context, error) {
	c := ctx.Callback()
	user := LoadUser(ctx)
	transactionsListInterface, err := bot.Cache.Get(fmt.Sprintf("%s_transactions", user.Name))
	if err != nil {
		log.Info("Transactions not in cache anymore")
		return ctx, err
	}
	transactionsList := transactionsListInterface.(TransactionsList)

	if c.Sender.ID == transactionsList.User.Telegram.ID {
		if transactionsList.CurrentPage > 0 {
			transactionsList.CurrentPage--
		} else {
			return ctx, nil
		}
		bot.Cache.Set(fmt.Sprintf("%s_transactions", user.Name), transactionsList, &store.Options{Expiration: 1 * time.Minute})
		bot.tryEditMessage(c.Message, transactionsList.printTransactions(ctx), bot.makeTransactionsKeyboard(ctx, transactionsList))
	}
	return ctx, nil
}

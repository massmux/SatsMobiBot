package webhook

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"time"

	"github.com/massmux/SatsMobiBot/internal"
	"github.com/massmux/SatsMobiBot/internal/lnbits"
	"github.com/massmux/SatsMobiBot/internal/telegram"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"net/http"

	"github.com/massmux/SatsMobiBot/internal/storage"

	"github.com/gorilla/mux"
	tb "gopkg.in/lightningtipbot/telebot.v3"

	"github.com/massmux/SatsMobiBot/internal/i18n"
)

type Server struct {
	httpServer *http.Server
	bot        *tb.Bot
	c          *lnbits.Client
	database   *gorm.DB
	buntdb     *storage.DB
}

type Webhook struct {
	CheckingID    string      `json:"checking_id"`
	Pending       bool        `json:"pending"`
	Amount        int64       `json:"amount"`
	Fee           int64       `json:"fee"`
	Memo          string      `json:"memo"`
	Time          int64       `json:"time"`
	Bolt11        string      `json:"bolt11"`
	Preimage      string      `json:"preimage"`
	PaymentHash   string      `json:"payment_hash"`
	Extra         struct{}    `json:"extra"`
	WalletID      string      `json:"wallet_id"`
	Webhook       string      `json:"webhook"`
	WebhookStatus interface{} `json:"webhook_status"`
}

func NewServer(bot *telegram.TipBot) *Server {
	srv := &http.Server{
		Addr:         internal.Configuration.Lnbits.WebhookServerUrl.Host,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}
	apiServer := &Server{
		c:          bot.Client,
		database:   bot.DB.Users,
		bot:        bot.Telegram,
		httpServer: srv,
		buntdb:     bot.Bunt,
	}
	apiServer.httpServer.Handler = apiServer.newRouter()
	go apiServer.httpServer.ListenAndServe()
	log.Infof("[Webhook] Server started at %s", internal.Configuration.Lnbits.WebhookServerUrl)
	return apiServer
}

func (w *Server) GetUserByWalletId(walletId string) (*lnbits.User, error) {
	user := &lnbits.User{}
	tx := w.database.Where("wallet_id = ?", walletId).First(user)
	if tx.Error != nil {
		return user, tx.Error
	}
	return user, nil
}

func (w *Server) newRouter() *mux.Router {
	router := mux.NewRouter()
	router.HandleFunc("/{secret}", w.receive).Methods(http.MethodPost)
	return router
}

func (w *Server) receive(writer http.ResponseWriter, request *http.Request) {
	log.Debugln("[Webhook] Received request")

	// authenticate: the path secret is the only thing standing between this
	// endpoint and the internet, so check it before touching the DB or body.
	secret := mux.Vars(request)["secret"]
	if subtle.ConstantTimeCompare([]byte(secret), []byte(internal.Configuration.Lnbits.WebhookSecret)) != 1 {
		log.Warnln("[Webhook] Rejected request: invalid or missing secret")
		writer.WriteHeader(http.StatusNotFound)
		return
	}

	webhookEvent := Webhook{}
	// need to delete the header otherwise the Decode will fail
	request.Header.Del("content-length")
	err := json.NewDecoder(request.Body).Decode(&webhookEvent)
	if err != nil {
		log.Errorf("[Webhook] Error decoding request: %s", err.Error())
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	user, err := w.GetUserByWalletId(webhookEvent.WalletID)
	if err != nil {
		log.Errorf("[Webhook] Error getting user: %s", err.Error())
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	// resolve any registered event for this payment hash. When one exists, its
	// stored wallet is the source of truth for who actually owns this invoice —
	// never the wallet_id supplied in the untrusted POST body.
	txInvoiceEvent := &telegram.InvoiceEvent{Invoice: &telegram.Invoice{PaymentHash: webhookEvent.PaymentHash}}
	hasEvent := w.buntdb.Get(txInvoiceEvent) == nil

	owningWallet := user.Wallet
	if hasEvent {
		if txInvoiceEvent.User == nil || txInvoiceEvent.User.Wallet == nil || txInvoiceEvent.User.Wallet.ID != webhookEvent.WalletID {
			log.Warnf("[Webhook] wallet_id %s does not own payment_hash %s, rejecting", webhookEvent.WalletID, webhookEvent.PaymentHash)
			writer.WriteHeader(http.StatusForbidden)
			return
		}
		owningWallet = txInvoiceEvent.User.Wallet
	}

	// never trust the POST body as proof of payment: confirm settlement with
	// LNbits itself, using the owning wallet's own read key.
	payment, err := w.c.Payment(*owningWallet, webhookEvent.PaymentHash)
	if err != nil || !payment.Paid {
		log.Warnf("[Webhook] payment_hash %s not confirmed paid by LNbits: %v", webhookEvent.PaymentHash, err)
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	if !hasEvent {
		// no registered event for this payment hash (e.g. a deposit made
		// outside the bot's normal invoice flow): just notify the wallet
		// owner, using the amount LNbits confirmed, never the POST body's.
		_, err = w.bot.Send(user.Telegram, fmt.Sprintf(i18n.Translate(user.Telegram.LanguageCode, "invoiceReceivedMessage"), payment.Details.Amount/1000))
		if err != nil {
			log.Errorln(err)
		}
		writer.WriteHeader(http.StatusOK)
		return
	}

	// dedup: make dispatch idempotent against retried/replayed deliveries of
	// the same settlement.
	claimed, err := txInvoiceEvent.ClaimOnce(txInvoiceEvent, w.buntdb)
	if err != nil {
		log.Errorln(err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !claimed {
		log.Debugf("[Webhook] payment_hash %s already claimed, ignoring duplicate delivery", webhookEvent.PaymentHash)
		writer.WriteHeader(http.StatusOK)
		return
	}

	log.Infof("[⚡️ WebHook] User %s (%d) received invoice of %d sat.", telegram.GetUserStr(txInvoiceEvent.User.Telegram), txInvoiceEvent.User.Telegram.ID, txInvoiceEvent.Amount)

	// trigger invoice events synchronously: only respond 200 once the
	// callback's effects have actually completed (bounded by the server's
	// 15s read/write timeout).
	if c := telegram.InvoiceCallback[txInvoiceEvent.Callback]; c.Function != nil {
		if err := telegram.AssertEventType(txInvoiceEvent, c.Type); err != nil {
			log.Errorln(err)
			writer.WriteHeader(http.StatusOK)
			return
		}
		c.Function(txInvoiceEvent)
		writer.WriteHeader(http.StatusOK)
		return
	}

	// fallback: no callback registered for this event type, send a generic message
	_, err = w.bot.Send(txInvoiceEvent.User.Telegram, fmt.Sprintf(i18n.Translate(txInvoiceEvent.User.Telegram.LanguageCode, "invoiceReceivedMessage"), txInvoiceEvent.Amount))
	if err != nil {
		log.Errorln(err)
	}
	writer.WriteHeader(http.StatusOK)
}

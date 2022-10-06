package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/LightningTipBot/LightningTipBot/internal"
	"github.com/LightningTipBot/LightningTipBot/internal/lnbits"
	"github.com/LightningTipBot/LightningTipBot/internal/telegram"
	"github.com/gorilla/mux"
	"github.com/r3labs/sse"
)

type Service struct {
	Bot *telegram.TipBot
}

type ErrorResponse struct {
	Message string `json:"error"`
}

func RespondError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(ErrorResponse{Message: message})
}

func (s Service) Balance(w http.ResponseWriter, r *http.Request) {
	user := telegram.LoadUser(r.Context())
	balance, err := s.Bot.GetUserBalance(user)
	if err != nil {
		RespondError(w, "balance check failed")
		return
	}

	balanceResponse := BalanceResponse{
		Balance: balance,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(balanceResponse)
}

func (s Service) CreateInvoice(w http.ResponseWriter, r *http.Request) {
	user := telegram.LoadUser(r.Context())
	var createInvoiceRequest CreateInvoiceRequest
	err := json.NewDecoder(r.Body).Decode(&createInvoiceRequest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	invoice, err := user.Wallet.Invoice(
		lnbits.InvoiceParams{
			Amount:              createInvoiceRequest.Amount,
			Out:                 false,
			DescriptionHash:     createInvoiceRequest.DescriptionHash,
			UnhashedDescription: createInvoiceRequest.UnhashedDescription,
			Webhook:             internal.Configuration.Lnbits.WebhookServer},
		s.Bot.Client)
	if err != nil {
		RespondError(w, "could not create invoice")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(invoice)
}

func (s Service) PayInvoice(w http.ResponseWriter, r *http.Request) {
	user := telegram.LoadUser(r.Context())
	var payInvoiceRequest PayInvoiceRequest
	err := json.NewDecoder(r.Body).Decode(&payInvoiceRequest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	invoice, err := user.Wallet.Pay(lnbits.PaymentParams{Out: true, Bolt11: payInvoiceRequest.PayRequest}, s.Bot.Client)
	if err != nil {
		RespondError(w, "could not pay invoice: "+err.Error())
		return
	}

	payment, _ := s.Bot.Client.Payment(*user.Wallet, invoice.PaymentHash)
	if err != nil {
		// we assume that it's paid since thre was no error earlier
		payment.Paid = true
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(payment)
}

func (s Service) PaymentStatus(w http.ResponseWriter, r *http.Request) {
	user := telegram.LoadUser(r.Context())
	payment_hash := mux.Vars(r)["payment_hash"]
	payment, err := s.Bot.Client.Payment(*user.Wallet, payment_hash)
	if err != nil {
		RespondError(w, "could not get payment")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(payment)
}

// InvoiceStatus
func (s Service) InvoiceStatus(w http.ResponseWriter, r *http.Request) {
	user := telegram.LoadUser(r.Context())
	payment_hash := mux.Vars(r)["payment_hash"]
	user.Wallet = &lnbits.Wallet{}
	payment, err := s.Bot.Client.Payment(*user.Wallet, payment_hash)
	if err != nil {
		RespondError(w, "could not get invoice")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(payment)
}

type InvoiceStream struct {
	CheckingID  string `json:"checking_id"`
	Pending     bool   `json:"pending"`
	Amount      int    `json:"amount"`
	Fee         int    `json:"fee"`
	Memo        string `json:"memo"`
	Time        int    `json:"time"`
	Bolt11      string `json:"bolt11"`
	Preimage    string `json:"preimage"`
	PaymentHash string `json:"payment_hash"`
	Extra       struct {
	} `json:"extra"`
	WalletID      string      `json:"wallet_id"`
	Webhook       string      `json:"webhook"`
	WebhookStatus interface{} `json:"webhook_status"`
}

func (s Service) InvoiceStream(w http.ResponseWriter, r *http.Request) {
	user := telegram.LoadUser(r.Context())
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}
	client := sse.NewClient(fmt.Sprintf("%s/api/v1/payments/sse", internal.Configuration.Lnbits.Url))
	client.Connection.Transport = &http.Transport{DisableCompression: true}
	client.Headers = map[string]string{"X-Api-Key": user.Wallet.Inkey}
	c := make(chan *sse.Event)
	err := client.SubscribeChan("", c)
	if err != nil {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}
out:
	for {
		select {
		case msg := <-c:
			select {
			case <-r.Context().Done():
				client.Unsubscribe(c)
				break out
			default:
				written, err := fmt.Fprintf(w, "event: %s\n", string(msg.Event))
				if err != nil || written == 0 {
					break out
				}
				written, err = fmt.Fprintf(w, "data: %s\n", string(msg.Data))
				if err != nil || written == 0 {
					break out
				}
				written, err = fmt.Fprint(w, "\n")
				if err != nil || written == 0 {
					break out
				}
				flusher.Flush()
			}
		}

	}
	close(c)
	time.Sleep(time.Second * 5)
}

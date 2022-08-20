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

func (s Service) InvoiceStream(w http.ResponseWriter, r *http.Request) {
	user := telegram.LoadUser(r.Context())

	q := r.URL.Query()
	q.Add("stream", user.Wallet.Inkey)
	r.URL.RawQuery = q.Encode()
	client := sse.NewClient("http://localhost:5050/api/v1/payments/sse")
	// custom header with invoice key
	// invoiceHeader := req.Header{
	// 	"X-Api-Key": user.Wallet.Inkey,
	// }
	client.Headers = map[string]string{"X-Api-Key": user.Wallet.Inkey}
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	flusher.Flush()
	client.Subscribe("", func(msg *sse.Event) {
		// Got some data!

		if msg.ID != nil && len(msg.ID) > 0 {
			fmt.Fprintf(w, "id: %s\n", msg.ID)
		}
		if msg.Event != nil {
			n, err := fmt.Fprintf(w, "event: %s\n", msg.Event)
			if err != nil {
				fmt.Println(err)
				fmt.Println(n)
				return
			}
		}
		if msg.Data != nil {
			fmt.Fprintf(w, "%s", string(msg.Data))
		}
		flusher.Flush()
	})
	time.Sleep(time.Second * 5)
}

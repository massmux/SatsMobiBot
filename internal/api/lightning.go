package api

import (
	"encoding/json"
	"net/http"

	"github.com/LightningTipBot/LightningTipBot/internal"
	"github.com/LightningTipBot/LightningTipBot/internal/lnbits"
	"github.com/LightningTipBot/LightningTipBot/internal/telegram"
)

type Service struct {
	Bot *telegram.TipBot
}

type ApiError struct {
	Message string `json:"error"`
}

func RespondError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(ApiError{Message: message})
}

func (s Service) Balance(w http.ResponseWriter, r *http.Request) {
	user := telegram.LoadUser(r.Context())
	balance, err := s.Bot.GetUserBalance(user)
	if err != nil {
		RespondError(w, "balance check failed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(balance)
}

func (s Service) CreateInvoice(w http.ResponseWriter, r *http.Request) {
	var createInvoiceRequest CreateInvoiceRequest
	err := json.NewDecoder(r.Body).Decode(&createInvoiceRequest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	user := &lnbits.User{}
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
	var payInvoiceRequest PayInvoiceRequest
	err := json.NewDecoder(r.Body).Decode(&payInvoiceRequest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	user := &lnbits.User{}
	invoice, err := user.Wallet.Pay(lnbits.PaymentParams{Out: true, Bolt11: payInvoiceRequest.PayRequest}, s.Bot.Client)
	if err != nil {
		RespondError(w, "could not pay invoice: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(invoice)
}

func (s Service) PaymentStatus(w http.ResponseWriter, r *http.Request) {
	user := &lnbits.User{}
	payment, err := s.Bot.Client.Payment(*user.Wallet, "")
	if err != nil {
		RespondError(w, "could not get payment")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(payment)
}

// InvoiceStatus
func (s Service) InvoiceStatus(w http.ResponseWriter, r *http.Request) {
	user := &lnbits.User{}
	user.Wallet = &lnbits.Wallet{}
	payment, err := s.Bot.Client.Payment(*user.Wallet, "")
	if err != nil {
		RespondError(w, "could not get payment")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(payment)
}

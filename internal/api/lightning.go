package api

import (
	"encoding/json"
	"net/http"

	"github.com/LightningTipBot/LightningTipBot/internal"
	"github.com/LightningTipBot/LightningTipBot/internal/lnbits"
	"github.com/LightningTipBot/LightningTipBot/internal/telegram"
)

type Service struct {
	bot *telegram.TipBot
}

func (s Service) Balance(w http.ResponseWriter, r *http.Request) {
	user := &lnbits.User{}
	balance, err := s.bot.GetUserBalance(user)
	if err != nil {
		// return ctx, errors.Create("balance check failed")
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
		s.bot.Client)
	if err != nil {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(invoice)
}

func (s Service) PayInvoice(w http.ResponseWriter, r *http.Request) {

}

func (s Service) PaymentStatus(w http.ResponseWriter, r *http.Request) {

}

// InvoiceStatus
func (s Service) InvoiceStatus(w http.ResponseWriter, r *http.Request) {

}

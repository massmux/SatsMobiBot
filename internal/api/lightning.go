package api

import (
	"github.com/LightningTipBot/LightningTipBot/internal/telegram"
	"net/http"
)

type Service struct {
	bot *telegram.TipBot
}

func (s Service) Balance(w http.ResponseWriter, r *http.Request) {

}

func (s Service) CreateInvoice(w http.ResponseWriter, r *http.Request) {

}

func (s Service) PayInvoice(w http.ResponseWriter, r *http.Request) {

}

func (s Service) PaymentStatus(w http.ResponseWriter, r *http.Request) {

}

// InvoiceStatus
func (s Service) InvoiceStatus(w http.ResponseWriter, r *http.Request) {

}

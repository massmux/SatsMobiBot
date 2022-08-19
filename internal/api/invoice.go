package api

type BalanceResponse struct {
	Balance int64 `json:"balance"`
}

type InvoiceStatusResponse struct {
	State       string `json:"state,omitempty"`
	PaymentHash string `json:"payment_hash"`
	Preimage    int64  `json:"preimage"`
}

type CreateInvoiceResponse struct {
	PaymentHash string `json:"payment_hash"`
	PayRequest  string `json:"pay_request"`
	Preimage    string `json:"preimage,omitempty"`
}
type CreateInvoiceRequest struct {
	Memo                string `json:"memo"`
	Amount              int64  `json:"amount"`
	DescriptionHash     string `json:"description_hash"`
	UnhashedDescription string `json:"unhashed_description"`
}

type PayInvoiceRequest struct {
	PayRequest string `json:"pay_req"`
}

package api

type InvoiceStatusResponse struct {
	State       string `json:"state,omitempty"`
	PaymentHash string `json:"payment_hash"`
	PreiMage    int64  `json:"preiMage"`
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

type PayInvoice struct {
	PayRequest string `json:"pay_request"`
}

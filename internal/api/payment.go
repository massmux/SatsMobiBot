package api

type PaymentStatusResponse struct {
	State       string `json:"state"`
	FeeMsat     int64  `json:"fee_msat,omitempty"`
	Amount      int64  `json:"amount,omitempty"`
	Preimage    string `json:"preimage,omitempty"`
	PaymentHash string `json:"payment_hash"`
}

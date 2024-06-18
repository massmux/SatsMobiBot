package telegram

import (
	"bytes"
	"encoding/json"
	"github.com/LightningTipBot/LightningTipBot/internal"
	"io/ioutil"
	"net/http"
)

type VoucherBot struct {
	Amount           string `json:"amount"`
	LightningAddress string `json:"lightning_address"`
	Iban             string `json:"iban"`
	APIKey           string `json:"api_key"`
	BitcoinAddress   string `json:"bitcoin_address"`
	Message          string `json:"message"`
	Signature        string `json:"signature"`
}

func (vb *VoucherBot) getChallenge() (*http.Response, error) {
	url := "https://api.gwoq.com/v1/order/challenge"
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", vb.APIKey)
	client := &http.Client{}
	return client.Do(req)
}

func (vb *VoucherBot) getFee() (*http.Response, error) {
	url := "https://api.gwoq.com/v1/order/getfee"
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", vb.APIKey)
	client := &http.Client{}
	return client.Do(req)
}

func (vb *VoucherBot) setLightningRecipient(lightningAddress string, amount string, iban string) {
	vb.Amount = amount
	vb.LightningAddress = lightningAddress
	vb.Iban = iban
}

func (vb *VoucherBot) createLightningOrder() map[string]interface{} {

	url := "https://api.gwoq.com/v1/order/create_lightning"
	payload := map[string]interface{}{
		"event": "order.create",
		"payload": map[string]string{
			"currency":       internal.Configuration.Voucherbot.Currency,
			"email":          "nomail@nomail.com",
			"iban":           vb.Iban,
			"amount":         vb.Amount,
			"recipient":      vb.LightningAddress,
			"recipient_type": "2",
			"public_key":     "npubxx",
			"op_type":        internal.Configuration.Voucherbot.PurchaseType,
		},
	}
	jsonPayload, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", vb.APIKey)
	client := &http.Client{}

	resp, _ := client.Do(req)
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)

	return result
}

func (vb *VoucherBot) setOnchainRecipient(bitcoinAddress string, message string, signature string, iban string) {
	vb.BitcoinAddress = bitcoinAddress
	vb.Message = message
	vb.Signature = signature
	vb.Iban = iban
}

func (vb *VoucherBot) createOnchainOrder() (*http.Response, error) {
	url := "https://api.gwoq.com/v1/order/create"
	payload := map[string]interface{}{
		"event": "order.create",
		"payload": map[string]string{
			"bitcoin_address": vb.BitcoinAddress,
			"currency":        internal.Configuration.Voucherbot.Currency,
			"email":           "nomail@nomail.com",
			"iban":            vb.Iban,
			"message":         vb.Message,
			"signature":       vb.Signature,
			"public_key":      "npubxx",
		},
	}
	jsonPayload, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", vb.APIKey)
	client := &http.Client{}
	return client.Do(req)
}

func (vb *VoucherBot) cancelOrder(orderid string) (*http.Response, error) {
	url := "https://api.gwoq.com/v1/order/cancel"
	payload := map[string]interface{}{
		"event": "order.cancel",
		"payload": map[string]string{
			"orderid": orderid,
		},
	}
	jsonPayload, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", vb.APIKey)
	client := &http.Client{}
	return client.Do(req)
}

func (vb *VoucherBot) notifyPayment(orderid string) (*http.Response, error) {
	url := "https://api.gwoq.com/v1/order/notify_payment"
	payload := map[string]interface{}{
		"event": "order.notify_payment",
		"payload": map[string]string{
			"orderid": orderid,
		},
	}
	jsonPayload, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", vb.APIKey)
	client := &http.Client{}
	return client.Do(req)
}

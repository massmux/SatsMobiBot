package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/LightningTipBot/LightningTipBot/internal"
	"io/ioutil"
	"net/http"
)

// todo: this part is to be finished

type VoucherBot struct {
	Amount           string `json:"amount"`
	LightningAddress string `json:"lightning_address"`
	Iban             string `json:"iban"`
	APIKey           string `json:"api_key"`
	BitcoinAddress   string `json:"bitcoin_address"`
	Message          string `json:"message"`
	Signature        string `json:"signature"`
}

func (v *VoucherBot) GetChallenge() map[string]interface{} {
	//url := "https://api.gwoq.com/v1/order/challenge"
	url := fmt.Sprintf("https://%s/v1/order/challenge", internal.Configuration.Bot.Username)
	//internal.Configuration.Voucherbot.Endpoint

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", v.APIKey)
	res, _ := http.DefaultClient.Do(req)
	data, _ := ioutil.ReadAll(res.Body)
	var result map[string]interface{}
	json.Unmarshal([]byte(data), &result)
	return result
}

func (v *VoucherBot) GetFee() map[string]interface{} {
	//url := "https://api.gwoq.com/v1/order/getfee"
	url := fmt.Sprintf("https://%s/v1/order/getfee", internal.Configuration.Bot.Username)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", v.APIKey)
	res, _ := http.DefaultClient.Do(req)
	data, _ := ioutil.ReadAll(res.Body)
	var result map[string]interface{}
	json.Unmarshal([]byte(data), &result)
	return result
}

func (v *VoucherBot) SetLightningRecipient(lightningAddress string, amount string, iban string) {
	v.Amount = amount
	v.LightningAddress = lightningAddress
	v.Iban = iban
}

func (v *VoucherBot) CreateLightningOrder() map[string]interface{} {
	//url := "https://api.gwoq.com/v1/order/create_lightning"
	url := fmt.Sprintf("https://%s/v1/order/create_lightning", internal.Configuration.Bot.Username)
	order := map[string]interface{}{
		"event": "order.create",
		"payload": map[string]interface{}{
			"currency":       "EUR",
			"email":          "",
			"iban":           v.Iban,
			"amount":         v.Amount,
			"recipient":      v.LightningAddress,
			"recipient_type": 2,
			"public_key":     "",
			"op_type":        "LA-D",
		},
	}
	jsonValue, _ := json.Marshal(order)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", v.APIKey)
	res, _ := http.DefaultClient.Do(req)
	data, _ := ioutil.ReadAll(res.Body)
	var result map[string]interface{}
	json.Unmarshal([]byte(data), &result)
	return result
}

func (v *VoucherBot) SetOnChainRecipient(bitcoinAddress string, message string, signature string, iban string) {
	v.BitcoinAddress = bitcoinAddress
	v.Message = message
	v.Signature = signature
	v.Iban = iban
}

func (v *VoucherBot) CreateOnChainOrder() map[string]interface{} {
	//url := "https://api.gwoq.com/v1/order/create"
	url := fmt.Sprintf("https://%s/v1/order/create", internal.Configuration.Bot.Username)
	order := map[string]interface{}{
		"event": "order.create",
		"payload": map[string]interface{}{
			"bitcoin_address": v.BitcoinAddress,
			"currency":        "EUR",
			"email":           "nomail@nomail.com",
			"iban":            v.Iban,
			"message":         v.Message,
			"signature":       v.Signature,
			"public_key":      "",
		},
	}
	jsonValue, _ := json.Marshal(order)

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", v.APIKey)
	res, _ := http.DefaultClient.Do(req)
	data, _ := ioutil.ReadAll(res.Body)
	var result map[string]interface{}
	json.Unmarshal([]byte(data), &result)
	return result
}

func (v *VoucherBot) CancelOrder(orderID string) map[string]interface{} {
	//url := "https://api.gwoq.com/v1/order/cancel"
	url := fmt.Sprintf("https://%s/v1/order/cancel", internal.Configuration.Bot.Username)
	order := map[string]interface{}{
		"event": "order.cancel",
		"payload": map[string]interface{}{
			"orderid": orderID,
		},
	}
	jsonValue, _ := json.Marshal(order)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", v.APIKey)
	res, _ := http.DefaultClient.Do(req)
	data, _ := ioutil.ReadAll(res.Body)
	var result map[string]interface{}
	json.Unmarshal([]byte(data), &result)
	return result
}

func (v *VoucherBot) NotifyPayment(orderid string) (*http.Response, error) {
	//url := "https://api.gwoq.com/v1/order/notify_payment"
	url := fmt.Sprintf("https://%s/v1/order/notify_payment", internal.Configuration.Bot.Username)

	payload := map[string]interface{}{
		"event": "order.notify_payment",
		"payload": map[string]interface{}{
			"orderid": orderid,
		},
	}

	jsonPayload, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", v.APIKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	return resp, err
}

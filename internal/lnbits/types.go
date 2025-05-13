package lnbits

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/massmux/SatsMobiBot/internal/satdress"

	"github.com/imroc/req"
	tb "gopkg.in/lightningtipbot/telebot.v3"
)

type Client struct {
	header     req.Header
	url        string
	AdminKey   string
	InvoiceKey string
}

type User struct {
	ID           string       `json:"id"`
	Name         string       `json:"name" gorm:"primaryKey"`
	Initialized  bool         `json:"initialized"`
	Telegram     *tb.User     `gorm:"embedded;embeddedPrefix:telegram_"`
	Wallet       *Wallet      `gorm:"embedded;embeddedPrefix:wallet_"`
	StateKey     UserStateKey `json:"stateKey"`
	StateData    string       `json:"stateData"`
	CreatedAt    time.Time    `json:"created"`
	UpdatedAt    time.Time    `json:"updated"`
	AnonID       string       `json:"anon_id"`
	AnonIDSha256 string       `json:"anon_id_sha256"`
	UUID         string       `json:"uuid"`
	Banned       bool         `json:"banned"`
	Settings     *Settings    `json:"settings" gorm:"foreignKey:id"`
}

type Settings struct {
	ID      string          `json:"id" gorm:"primarykey"`
	Display DisplaySettings `gorm:"embedded;embeddedPrefix:display_"`
	Node    NodeSettings    `gorm:"embedded;embeddedPrefix:node_"`
	Nostr   NostrSettings   `gorm:"embedded;embeddedPrefix:nostr_"`
}

type DisplaySettings struct {
	DisplayCurrency string `json:"displaycurrency"`
}
type NostrSettings struct {
	PubKey string `json:"pubkey"`
}
type NodeSettings struct {
	NodeType     string                 `json:"nodetype"`
	LNDParams    *satdress.LNDParams    `gorm:"embedded;embeddedPrefix:lndparams_"`
	LNbitsParams *satdress.LNBitsParams `gorm:"embedded;embeddedPrefix:lnbitsparams_"`
}

const (
	UserStateConfirmPayment = iota + 1
	UserStateConfirmSend
	UserStateLNURLEnterAmount
	UserStateConfirmLNURLPay
	UserEnterAmount
	UserHasEnteredAmount
	UserEnterUser
	UserHasEnteredUser
	UserEnterShopTitle
	UserStateShopItemSendPhoto
	UserStateShopItemSendTitle
	UserStateShopItemSendDescription
	UserStateShopItemSendPrice
	UserStateShopItemSendItemFile
	UserEnterShopsDescription
	UserEnterDallePrompt
)

type UserStateKey int

func (u *User) ResetState() {
	u.StateData = ""
	u.StateKey = 0
}

type InvoiceParams struct {
	Out                 bool   `json:"out"`                            // must be True if invoice is payed, False if invoice is received
	Amount              int64  `json:"amount"`                         // amount in Satoshi
	Memo                string `json:"memo,omitempty"`                 // the invoice memo.
	Webhook             string `json:"webhook,omitempty"`              // the webhook to fire back to when payment is received.
	DescriptionHash     string `json:"description_hash,omitempty"`     // the invoice description hash.
	UnhashedDescription string `json:"unhashed_description,omitempty"` // the unhashed invoice description.
}

type PaymentParams struct {
	Out    bool   `json:"out"`
	Bolt11 string `json:"bolt11"`
}
type PayParams struct {
	// the BOLT11 payment request you want to pay.
	PaymentRequest string `json:"payment_request"`

	// custom data you may want to associate with this invoice. optional.
	PassThru map[string]interface{} `json:"passThru"`
}

type TransferParams struct {
	Memo         string `json:"memo"`           // the transfer description.
	NumSatoshis  int64  `json:"num_satoshis"`   // the transfer amount.
	DestWalletId string `json:"dest_wallet_id"` // the key or id of the destination
}

type Error struct {
	Detail string `json:"detail"`
}

func (err Error) Error() string {
	return err.Detail
}

type Wallet struct {
	ID       string `json:"id" gorm:"id"`
	Adminkey string `json:"adminkey"`
	Inkey    string `json:"inkey"`
	Balance  int64  `json:"balance"`
	Name     string `json:"name"`
	User     string `json:"user"`
}

type Payment struct {
	CheckingID    string      `json:"checking_id"`
	Pending       bool        `json:"pending"`
	Amount        int64       `json:"amount"`
	Fee           int64       `json:"fee"`
	Memo          string      `json:"memo"`
	Time          string      `json:"time"`
	Bolt11        string      `json:"bolt11"`
	Preimage      string      `json:"preimage"`
	PaymentHash   string      `json:"payment_hash"`
	Extra         struct{}    `json:"extra"`
	WalletID      string      `json:"wallet_id"`
	Webhook       interface{} `json:"webhook"`
	WebhookStatus interface{} `json:"webhook_status"`
}

type LNbitsPayment struct {
	Paid     bool    `json:"paid"`
	Preimage string  `json:"preimage"`
	Details  Payment `json:"details,omitempty"`
}

type Payments []Payment

type Invoice struct {
	PaymentHash    string `json:"payment_hash"`
	PaymentRequest string `json:"bolt11"`
}

// from fiatjaf/lnurl-go
func (u User) LinkingKey(domain string) (*btcec.PrivateKey, *btcec.PublicKey) {
	seedHash := sha256.Sum256([]byte(
		fmt.Sprintf("lnurlkeyseed:%s:%s",
			domain, u.ID)))
	return btcec.PrivKeyFromBytes(seedHash[:])
}

func (u User) SignKeyAuth(domain string, k1hex string) (key string, sig string, err error) {
	// lnurl-auth: create a key based on the user id and sign with it
	sk, pk := u.LinkingKey(domain)

	k1, err := hex.DecodeString(k1hex)
	if err != nil {
		return "", "", fmt.Errorf("invalid k1 hex '%s': %w", k1hex, err)
	}

	signature := ecdsa.Sign(sk, k1)
	if err != nil {
		return "", "", fmt.Errorf("error signing k1: %w", err)
	}

	sig = hex.EncodeToString(signature.Serialize())
	key = hex.EncodeToString(pk.SerializeCompressed())

	return key, sig, nil
}

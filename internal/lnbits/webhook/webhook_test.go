// Regression test for the 2026-08-06 unauthenticated-webhook / no-dedup
// findings (high-crit/2026-08-06-satsmobibot-unauthenticated-lnbits-webhook-forged-callback
// and high-crit/2026-08-06-satsmobibot-webhook-no-dedup-replay). Adapted from
// the original PoC harness to exercise the fixed handler:
//  - a request without the correct path secret is rejected (404), no dispatch
//  - a wallet_id that doesn't own the InvoiceEvent is rejected (403), no dispatch
//  - LNbits must confirm settlement before any dispatch or notification happens
//  - a valid, settled event dispatches its callback exactly once, even when
//    the same webhook is replayed multiple times (ClaimOnce dedup)
//  - the fallback "invoice received" message always uses the LNbits-confirmed
//    amount, never the attacker-controlled POST body amount
package webhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/massmux/SatsMobiBot/internal"
	"github.com/massmux/SatsMobiBot/internal/lnbits"
	"github.com/massmux/SatsMobiBot/internal/storage"
	"github.com/massmux/SatsMobiBot/internal/telegram"

	tb "gopkg.in/lightningtipbot/telebot.v3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const testSecret = "test-webhook-secret"

// fakeTelegramAPI records every Bot API method invocation hitting it.
type fakeTelegramAPI struct {
	t       *testing.T
	server  *httptest.Server
	sendCnt int64
	last    atomic.Value
}

func newFakeTelegramAPI(t *testing.T) *fakeTelegramAPI {
	f := &fakeTelegramAPI{t: t}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		f.last.Store(string(body))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "sendMessage"):
			atomic.AddInt64(&f.sendCnt, 1)
			fmt.Fprintf(w, `{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"}}}`)
		default:
			fmt.Fprintf(w, `{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"}}}`)
		}
	}))
	return f
}

// fakeLNbitsAPI answers GET /api/v1/payments/<hash> the way LNbits would --
// this is the source of truth the fixed handler must consult instead of
// trusting the webhook POST body.
type fakeLNbitsAPI struct {
	server   *httptest.Server
	mu       sync.Mutex
	payments map[string]lnbits.LNbitsPayment
}

func newFakeLNbitsAPI() *fakeLNbitsAPI {
	f := &fakeLNbitsAPI{payments: map[string]lnbits.LNbitsPayment{}}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		hash := parts[len(parts)-1]
		f.mu.Lock()
		p, ok := f.payments[hash]
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"detail":"not found"}`)
			return
		}
		b, _ := json.Marshal(p)
		w.Write(b)
	}))
	return f
}

func (f *fakeLNbitsAPI) setPaid(hash string, amountMsat int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.payments[hash] = lnbits.LNbitsPayment{Paid: true, Details: lnbits.Payment{Amount: amountMsat, PaymentHash: hash}}
}

func (f *fakeLNbitsAPI) setUnpaid(hash string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.payments[hash] = lnbits.LNbitsPayment{Paid: false, Details: lnbits.Payment{PaymentHash: hash}}
}

func mustJSON(t *testing.T, v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// testEnv bundles everything setupEnv wires up for one test.
type testEnv struct {
	srv    *Server
	fakeTG *fakeTelegramAPI
	fakeLN *fakeLNbitsAPI
	bunt   *storage.DB
	base   string // httptest base URL for the webhook listener
}

func setupEnv(t *testing.T) *testEnv {
	internal.Configuration.Lnbits.WebhookSecret = testSecret

	dsn := fmt.Sprintf("file:%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&lnbits.User{}, &lnbits.Settings{}); err != nil {
		t.Fatal(err)
	}
	victim := &lnbits.User{
		ID:   "victim-lnbits-user-id",
		Name: "victim",
		Telegram: &tb.User{
			ID:           111111,
			Username:     "victimuser",
			LanguageCode: "en",
		},
		Wallet: &lnbits.Wallet{ID: "victim-wallet-uuid", Inkey: "victim-inkey", Adminkey: "victim-adminkey"},
	}
	if err := db.Create(victim).Error; err != nil {
		t.Fatal(err)
	}
	attacker := &lnbits.User{
		ID:   "attacker-lnbits-user-id",
		Name: "attacker",
		Telegram: &tb.User{
			ID:           222222,
			Username:     "attackeruser",
			LanguageCode: "en",
		},
		Wallet: &lnbits.Wallet{ID: "attacker-wallet-uuid", Inkey: "attacker-inkey", Adminkey: "attacker-adminkey"},
	}
	if err := db.Create(attacker).Error; err != nil {
		t.Fatal(err)
	}

	bunt := storage.NewBunt(":memory:")
	fakeTG := newFakeTelegramAPI(t)
	fakeLN := newFakeLNbitsAPI()
	t.Cleanup(fakeTG.server.Close)
	t.Cleanup(fakeLN.server.Close)

	tgBot, err := tb.NewBot(tb.Settings{
		Token:   "12345:fake",
		URL:     fakeTG.server.URL,
		Offline: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	bot := &telegram.TipBot{
		DB:       &telegram.Databases{Users: db},
		Bunt:     bunt,
		Telegram: tgBot,
		Client:   lnbits.NewClient("dummy", fakeLN.server.URL),
	}

	internal.Configuration.Lnbits.WebhookServerUrl = &url.URL{Scheme: "http", Host: "127.0.0.1:0"}
	srv := NewServer(bot)
	ts := httptest.NewServer(srv.newRouter())
	t.Cleanup(ts.Close)

	return &testEnv{srv: srv, fakeTG: fakeTG, fakeLN: fakeLN, bunt: bunt, base: ts.URL}
}

func (e *testEnv) post(t *testing.T, secret string, w Webhook) *http.Response {
	resp, err := http.Post(e.base+"/"+secret, "application/json", bytes.NewReader(mustJSON(t, w)))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func registerCounterCallback(t *testing.T, id int, counter *int64) {
	orig := telegram.InvoiceCallback
	t.Cleanup(func() { telegram.InvoiceCallback = orig })
	telegram.InvoiceCallback = telegram.InvoiceEventCallback{}
	telegram.InvoiceCallback[id] = telegram.EventHandler{
		Function: func(event telegram.Event) { atomic.AddInt64(counter, 1) },
		Type:     telegram.EventTypeInvoice,
	}
}

// TestWrongSecretRejected: no/incorrect path secret must be rejected before
// any DB lookup or dispatch happens.
func TestWrongSecretRejected(t *testing.T) {
	env := setupEnv(t)
	var dispatched int64
	registerCounterCallback(t, 1, &dispatched)

	ev := &telegram.InvoiceEvent{
		Base:     storage.New(storage.ID("invoice:hash-wrongsecret")),
		Invoice:  &telegram.Invoice{PaymentHash: "hash-wrongsecret", Amount: 21000},
		User:     &lnbits.User{Name: "victim", Wallet: &lnbits.Wallet{ID: "victim-wallet-uuid"}},
		Callback: 1,
	}
	if err := env.bunt.Set(ev); err != nil {
		t.Fatal(err)
	}
	env.fakeLN.setPaid("hash-wrongsecret", 21000000)

	resp := env.post(t, "not-the-secret", Webhook{WalletID: "victim-wallet-uuid", PaymentHash: "hash-wrongsecret", Amount: 21000000})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for wrong secret, got %d", resp.StatusCode)
	}
	if atomic.LoadInt64(&dispatched) != 0 {
		t.Fatalf("callback dispatched despite wrong secret")
	}
}

// TestWalletMismatchRejected: a wallet_id that isn't the event's owner is
// rejected even with the correct secret.
func TestWalletMismatchRejected(t *testing.T) {
	env := setupEnv(t)
	var dispatched int64
	registerCounterCallback(t, 1, &dispatched)

	ev := &telegram.InvoiceEvent{
		Base:     storage.New(storage.ID("invoice:hash-mismatch")),
		Invoice:  &telegram.Invoice{PaymentHash: "hash-mismatch", Amount: 21000},
		User:     &lnbits.User{Name: "victim", Wallet: &lnbits.Wallet{ID: "victim-wallet-uuid"}},
		Callback: 1,
	}
	if err := env.bunt.Set(ev); err != nil {
		t.Fatal(err)
	}
	env.fakeLN.setPaid("hash-mismatch", 21000000)

	resp := env.post(t, testSecret, Webhook{WalletID: "attacker-wallet-uuid", PaymentHash: "hash-mismatch", Amount: 999999000})
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for wallet mismatch, got %d", resp.StatusCode)
	}
	if atomic.LoadInt64(&dispatched) != 0 {
		t.Fatalf("callback dispatched despite wallet_id/event owner mismatch")
	}
}

// TestUnpaidRejected: LNbits must confirm settlement; a body claiming an
// unpaid/unknown payment must not dispatch anything.
func TestUnpaidRejected(t *testing.T) {
	env := setupEnv(t)
	var dispatched int64
	registerCounterCallback(t, 1, &dispatched)

	ev := &telegram.InvoiceEvent{
		Base:     storage.New(storage.ID("invoice:hash-unpaid")),
		Invoice:  &telegram.Invoice{PaymentHash: "hash-unpaid", Amount: 21000},
		User:     &lnbits.User{Name: "victim", Wallet: &lnbits.Wallet{ID: "victim-wallet-uuid"}},
		Callback: 1,
	}
	if err := env.bunt.Set(ev); err != nil {
		t.Fatal(err)
	}
	env.fakeLN.setUnpaid("hash-unpaid")

	resp := env.post(t, testSecret, Webhook{WalletID: "victim-wallet-uuid", PaymentHash: "hash-unpaid", Amount: 21000000})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for unpaid/unconfirmed payment, got %d", resp.StatusCode)
	}
	if atomic.LoadInt64(&dispatched) != 0 {
		t.Fatalf("callback dispatched despite LNbits not confirming payment")
	}
}

// TestValidPaymentDispatchesAndDedupes: a correctly authenticated, owned,
// LNbits-confirmed webhook dispatches its callback exactly once even when the
// identical POST is replayed multiple times.
func TestValidPaymentDispatchesAndDedupes(t *testing.T) {
	env := setupEnv(t)
	var dispatched int64
	registerCounterCallback(t, 1, &dispatched)

	ev := &telegram.InvoiceEvent{
		Base:    storage.New(storage.ID("invoice:hash-valid")),
		Invoice: &telegram.Invoice{PaymentHash: "hash-valid", Amount: 21000},
		User: &lnbits.User{
			Name:     "victim",
			Telegram: &tb.User{ID: 111111, LanguageCode: "en"},
			Wallet:   &lnbits.Wallet{ID: "victim-wallet-uuid", Inkey: "victim-inkey"},
		},
		Callback: 1,
	}
	if err := env.bunt.Set(ev); err != nil {
		t.Fatal(err)
	}
	env.fakeLN.setPaid("hash-valid", 21000000)

	const N = 5
	for i := 0; i < N; i++ {
		resp := env.post(t, testSecret, Webhook{WalletID: "victim-wallet-uuid", PaymentHash: "hash-valid", Amount: 21000000})
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("delivery %d: expected 200, got %d", i, resp.StatusCode)
		}
	}
	if got := atomic.LoadInt64(&dispatched); got != 1 {
		t.Fatalf("expected exactly 1 dispatch across %d deliveries (dedup), got %d", N, got)
	}
}

// TestFallbackUsesConfirmedAmountNotForgedAmount: with no registered event,
// the notification amount must come from LNbits's confirmed payment, not the
// attacker-controlled POST body.
func TestFallbackUsesConfirmedAmountNotForgedAmount(t *testing.T) {
	env := setupEnv(t)

	env.fakeLN.setPaid("hash-nofollowup", 5000000) // LNbits says 5000 sat really arrived
	resp := env.post(t, testSecret, Webhook{WalletID: "victim-wallet-uuid", PaymentHash: "hash-nofollowup", Amount: 999999000}) // attacker claims ~1M sat
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt64(&env.fakeTG.sendCnt) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if atomic.LoadInt64(&env.fakeTG.sendCnt) != 1 {
		t.Fatalf("expected exactly one notification, got %d", atomic.LoadInt64(&env.fakeTG.sendCnt))
	}
	body := env.fakeTG.last.Load().(string)
	if strings.Contains(body, "999999") {
		t.Fatalf("notification used attacker-forged amount, not the LNbits-confirmed one: %s", body)
	}
	if !strings.Contains(body, "5000") {
		t.Fatalf("notification did not use LNbits-confirmed amount: %s", body)
	}
}

// TestUnknownWalletRejected: unknown wallet_id -> 400, no message, no dispatch.
func TestUnknownWalletRejected(t *testing.T) {
	env := setupEnv(t)
	resp := env.post(t, testSecret, Webhook{WalletID: "no-such-wallet", PaymentHash: "whatever", Amount: 1000})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown wallet_id, got %d", resp.StatusCode)
	}
	if atomic.LoadInt64(&env.fakeTG.sendCnt) != 0 {
		t.Fatalf("no message expected for unknown wallet")
	}
}

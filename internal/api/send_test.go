package api

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/mohsenm4/blockchain-insight/internal/ledger"
	"github.com/mohsenm4/blockchain-insight/internal/sender"
)

type fakeSender struct {
	signErr   error
	sendErrs  []error // one per Broadcast call; nil or missing = success
	state     sender.TxState
	checkErr  error
	status    string
	statusErr error

	signs, sends, checks, resets int
	to                           common.Address
	amount                       *big.Int
	tx                           *types.Transaction
}

func (f *fakeSender) Sign(ctx context.Context, to common.Address, amount *big.Int) (*types.Transaction, error) {
	f.signs++
	f.to = to
	f.amount = amount
	if f.signErr != nil {
		return nil, f.signErr
	}
	f.tx = types.NewTx(&types.LegacyTx{Nonce: 7, To: &to, Gas: 60_000, GasPrice: big.NewInt(1), Data: amount.Bytes()})
	return f.tx, nil
}

func (f *fakeSender) Broadcast(ctx context.Context, tx *types.Transaction) error {
	f.sends++
	if f.sends <= len(f.sendErrs) {
		return f.sendErrs[f.sends-1]
	}
	return nil
}

func (f *fakeSender) Check(ctx context.Context, tx *types.Transaction) (sender.TxState, error) {
	f.checks++
	return f.state, f.checkErr
}

func (f *fakeSender) ResetNonce() { f.resets++ }

func (f *fakeSender) Status(ctx context.Context, hash common.Hash) (string, error) {
	return f.status, f.statusErr
}

func newSendTestServer(f *fakeSender) (*Server, *fakeLedger) {
	lg := newFakeLedger()
	s := &Server{sender: f, ledger: lg}
	s.setupRouter()
	return s, lg
}

func postJSON(r http.Handler, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

const sendOK = `{"user_id":1,"idempotency_key":"k1","to":"0x0000000000000000000000000000000000000000","amount":"1000"}`

func TestPostSendInvalidAddress(t *testing.T) {
	f := &fakeSender{}
	s, _ := newSendTestServer(f)

	w := postJSON(s.router, "/send", `{"user_id":1,"idempotency_key":"k1","to":"invalid_address","amount":"1000"}`)

	if w.Code != 400 {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	if f.signs != 0 {
		t.Errorf("expected sender not to be called")
	}
}

func TestPostSendInvalidAmount(t *testing.T) {
	f := &fakeSender{}
	s, _ := newSendTestServer(f)

	w := postJSON(s.router, "/send", `{"user_id":1,"idempotency_key":"k1","to":"0x0000000000000000000000000000000000000000","amount":"invalid_amount"}`)

	if w.Code != 400 {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	if f.signs != 0 {
		t.Errorf("expected sender not to be called")
	}
}

func TestPostSendMissingKey(t *testing.T) {
	f := &fakeSender{}
	s, lg := newSendTestServer(f)

	w := postJSON(s.router, "/send", `{"user_id":1,"to":"0x0000000000000000000000000000000000000000","amount":"1000"}`)

	if w.Code != 400 {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	if f.signs != 0 || len(lg.withdrawals) != 0 {
		t.Errorf("expected no transfer and no row, got %d calls, %d rows", f.signs, len(lg.withdrawals))
	}
}

func TestPostSendOK(t *testing.T) {
	f := &fakeSender{}
	s, lg := newSendTestServer(f)

	w := postJSON(s.router, "/send", sendOK)

	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if f.signs != 1 || f.sends != 1 {
		t.Errorf("signs, sends = %d, %d, want 1, 1", f.signs, f.sends)
	}
	hash := f.tx.Hash().Hex()
	if !strings.Contains(w.Body.String(), hash) {
		t.Errorf("expected tx_hash in body, got %s", w.Body.String())
	}
	if f.amount.String() != "1000" {
		t.Errorf("expected amount 1000, got %s", f.amount)
	}

	if len(lg.withdrawals) != 1 {
		t.Fatalf("rows = %d, want 1", len(lg.withdrawals))
	}
	if got := lg.withdrawals[0]; got.Status != "broadcast" || got.TxHash != hash || got.RawTx == "" {
		t.Errorf("row = %s/%s raw=%q, want broadcast/%s with raw tx", got.Status, got.TxHash, got.RawTx, hash)
	}
}

func TestPostSendSameKeyTwice(t *testing.T) {
	f := &fakeSender{}
	s, lg := newSendTestServer(f)

	first := postJSON(s.router, "/send", sendOK)
	second := postJSON(s.router, "/send", sendOK)

	if first.Code != 200 || second.Code != 200 {
		t.Fatalf("codes = %d, %d, want 200, 200", first.Code, second.Code)
	}
	if f.signs != 1 || f.sends != 1 {
		t.Errorf("signs, sends = %d, %d, want 1, 1", f.signs, f.sends)
	}
	if len(lg.withdrawals) != 1 {
		t.Errorf("rows = %d, want 1", len(lg.withdrawals))
	}
	if first.Body.String() != second.Body.String() {
		t.Errorf("replay body = %s, want %s", second.Body.String(), first.Body.String())
	}
}

func TestPostSendSameKeyInProgress(t *testing.T) {
	f := &fakeSender{}
	s, lg := newSendTestServer(f)

	// The first request has inserted its row but not signed yet.
	lg.withdrawals = []ledger.Withdrawal{{
		ID: 1, UserID: 1, IdempotencyKey: "k1",
		To: ledger.AddressKey(common.Address{}), Amount: big.NewInt(1000), Status: "queued",
	}}

	w := postJSON(s.router, "/send", sendOK)

	if w.Code != http.StatusConflict {
		t.Errorf("expected status 409, got %d", w.Code)
	}
	if f.signs != 0 || f.sends != 0 {
		t.Errorf("signs, sends = %d, %d, want 0, 0", f.signs, f.sends)
	}
}

func TestPostSendSameKeyOtherAmount(t *testing.T) {
	f := &fakeSender{}
	s, _ := newSendTestServer(f)

	postJSON(s.router, "/send", sendOK)
	w := postJSON(s.router, "/send", `{"user_id":1,"idempotency_key":"k1","to":"0x0000000000000000000000000000000000000000","amount":"2000"}`)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected status 422, got %d", w.Code)
	}
	if f.signs != 1 || f.sends != 1 {
		t.Errorf("signs, sends = %d, %d, want 1, 1", f.signs, f.sends)
	}
}

// Case A for sure: signing failed, so no tx exists.
func TestPostSendSignError(t *testing.T) {
	f := &fakeSender{signErr: errors.New("next nonce: dial tcp: no such host")}
	s, lg := newSendTestServer(f)

	w := postJSON(s.router, "/send", sendOK)

	if w.Code != 500 {
		t.Errorf("expected status 500, got %d", w.Code)
	}
	if f.sends != 0 {
		t.Errorf("sends = %d, want 0", f.sends)
	}
	if len(lg.withdrawals) != 1 || lg.withdrawals[0].Status != "failed" {
		t.Fatalf("withdrawals = %+v, want one failed row", lg.withdrawals)
	}
	retry := postJSON(s.router, "/send", sendOK)
	if retry.Code != 200 || !strings.Contains(retry.Body.String(), `"status":"failed"`) {
		t.Errorf("retry = %d %s, want 200 failed", retry.Code, retry.Body.String())
	}
	if f.signs != 1 {
		t.Errorf("retry signed again: signs = %d, want 1", f.signs)
	}
}

// Case B: the broadcast errored but the node had taken the tx.
func TestPostSendBroadcastErrorButSent(t *testing.T) {
	f := &fakeSender{sendErrs: []error{errors.New("timeout")}, state: sender.TxSent}
	s, lg := newSendTestServer(f)

	w := postJSON(s.router, "/send", sendOK)

	if w.Code != 200 {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	if f.sends != 1 || f.checks != 1 {
		t.Errorf("sends, checks = %d, %d, want 1, 1", f.sends, f.checks)
	}
	if got := lg.withdrawals[0]; got.Status != "broadcast" || got.TxHash != f.tx.Hash().Hex() {
		t.Errorf("row = %s/%s, want broadcast/%s", got.Status, got.TxHash, f.tx.Hash().Hex())
	}
}

// The chain cannot be asked: the row stays signed, and a retry with the
// same key resends the same tx, not a new one.
func TestPostSendStateUnknownThenRetry(t *testing.T) {
	f := &fakeSender{sendErrs: []error{errors.New("no such host")}, checkErr: errors.New("no such host")}
	s, lg := newSendTestServer(f)

	w := postJSON(s.router, "/send", sendOK)

	if w.Code != 500 || !strings.Contains(w.Body.String(), `"status":"signed"`) {
		t.Fatalf("first = %d %s, want 500 signed", w.Code, w.Body.String())
	}
	if f.resets != 1 {
		t.Errorf("resets = %d, want 1", f.resets)
	}
	if got := lg.withdrawals[0]; got.Status != "signed" {
		t.Fatalf("row status = %s, want signed", got.Status)
	}

	// Network back; the chain has not seen the tx.
	f.checkErr, f.state = nil, sender.TxNotSeen
	retry := postJSON(s.router, "/send", sendOK)

	if retry.Code != 200 || !strings.Contains(retry.Body.String(), f.tx.Hash().Hex()) {
		t.Fatalf("retry = %d %s, want 200 with first tx hash", retry.Code, retry.Body.String())
	}
	if f.signs != 1 || f.sends != 2 {
		t.Errorf("signs, sends = %d, %d, want 1, 2 (same tx resent)", f.signs, f.sends)
	}
	if got := lg.withdrawals[0]; got.Status != "broadcast" {
		t.Errorf("row status = %s, want broadcast", got.Status)
	}
}

// Case A, found later: another tx took the nonce, so this one can never
// land.
func TestPostSendDroppedOnRetry(t *testing.T) {
	f := &fakeSender{sendErrs: []error{errors.New("no such host")}, checkErr: errors.New("no such host")}
	s, lg := newSendTestServer(f)

	postJSON(s.router, "/send", sendOK)

	f.checkErr, f.state = nil, sender.TxDropped
	retry := postJSON(s.router, "/send", sendOK)

	if retry.Code != 200 || !strings.Contains(retry.Body.String(), `"status":"failed"`) {
		t.Fatalf("retry = %d %s, want 200 failed", retry.Code, retry.Body.String())
	}
	if f.sends != 1 {
		t.Errorf("sends = %d, want 1 (a dropped tx is not resent)", f.sends)
	}
	if got := lg.withdrawals[0]; got.Status != "failed" {
		t.Errorf("row status = %s, want failed", got.Status)
	}
}

func TestPostSendLedgerError(t *testing.T) {
	f := &fakeSender{}
	s, lg := newSendTestServer(f)
	lg.err = errors.New("db down")

	w := postJSON(s.router, "/send", sendOK)

	if w.Code != 500 {
		t.Errorf("expected status 500, got %d", w.Code)
	}
	if f.signs != 0 {
		t.Errorf("expected sender not to be called")
	}
}

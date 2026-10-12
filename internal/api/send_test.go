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
	"github.com/mohsenm4/blockchain-insight/internal/ledger"
)

type fakeSender struct {
	hash   common.Hash
	err    error
	calls  int
	to     common.Address
	amount *big.Int
	status string
}

func (f *fakeSender) Transfer(ctx context.Context, to common.Address, amount *big.Int) (common.Hash, error) {
	f.calls++
	f.to = to
	f.amount = amount
	return f.hash, f.err
}

func (f *fakeSender) Status(ctx context.Context, hash common.Hash) (string, error) {
	return f.status, f.err
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
	if f.calls != 0 {
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
	if f.calls != 0 {
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
	if f.calls != 0 || len(lg.withdrawals) != 0 {
		t.Errorf("expected no transfer and no row, got %d calls, %d rows", f.calls, len(lg.withdrawals))
	}
}

func TestPostSendOK(t *testing.T) {
	f := &fakeSender{hash: common.HexToHash("0xabc")}
	s, lg := newSendTestServer(f)

	w := postJSON(s.router, "/send", sendOK)

	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if f.calls != 1 {
		t.Errorf("expected sender to be called once, got %d", f.calls)
	}
	if !strings.Contains(w.Body.String(), f.hash.Hex()) {
		t.Errorf("expected tx_hash in body, got %s", w.Body.String())
	}
	if f.amount.String() != "1000" {
		t.Errorf("expected amount 1000, got %s", f.amount)
	}

	if len(lg.withdrawals) != 1 {
		t.Fatalf("rows = %d, want 1", len(lg.withdrawals))
	}
	if got := lg.withdrawals[0]; got.Status != "broadcast" || got.TxHash != f.hash.Hex() {
		t.Errorf("row = %s/%s, want broadcast/%s", got.Status, got.TxHash, f.hash.Hex())
	}
}

func TestPostSendSameKeyTwice(t *testing.T) {
	f := &fakeSender{hash: common.HexToHash("0xabc")}
	s, lg := newSendTestServer(f)

	first := postJSON(s.router, "/send", sendOK)
	second := postJSON(s.router, "/send", sendOK)

	if first.Code != 200 || second.Code != 200 {
		t.Fatalf("codes = %d, %d, want 200, 200", first.Code, second.Code)
	}
	if f.calls != 1 {
		t.Errorf("Transfer called %d times, want 1", f.calls)
	}
	if len(lg.withdrawals) != 1 {
		t.Errorf("rows = %d, want 1", len(lg.withdrawals))
	}
	if first.Body.String() != second.Body.String() {
		t.Errorf("replay body = %s, want %s", second.Body.String(), first.Body.String())
	}
}

func TestPostSendSameKeyInProgress(t *testing.T) {
	f := &fakeSender{hash: common.HexToHash("0xabc")}
	s, lg := newSendTestServer(f)

	// The first request has inserted its row but not broadcast yet.
	lg.withdrawals = []ledger.Withdrawal{{
		ID: 1, UserID: 1, IdempotencyKey: "k1",
		To: ledger.AddressKey(common.Address{}), Amount: big.NewInt(1000), Status: "queued",
	}}

	w := postJSON(s.router, "/send", sendOK)

	if w.Code != http.StatusConflict {
		t.Errorf("expected status 409, got %d", w.Code)
	}
	if f.calls != 0 {
		t.Errorf("Transfer called %d times, want 0", f.calls)
	}
}

func TestPostSendSameKeyOtherAmount(t *testing.T) {
	f := &fakeSender{hash: common.HexToHash("0xabc")}
	s, _ := newSendTestServer(f)

	postJSON(s.router, "/send", sendOK)
	w := postJSON(s.router, "/send", `{"user_id":1,"idempotency_key":"k1","to":"0x0000000000000000000000000000000000000000","amount":"2000"}`)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected status 422, got %d", w.Code)
	}
	if f.calls != 1 {
		t.Errorf("Transfer called %d times, want 1", f.calls)
	}
}

func TestPostSendSenderError(t *testing.T) {
	f := &fakeSender{
		err: errors.New("sender error"),
	}
	s, lg := newSendTestServer(f)

	w := postJSON(s.router, "/send", sendOK)

	if w.Code != 500 {
		t.Errorf("expected status 500, got %d", w.Code)
	}
	if f.calls != 1 {
		t.Errorf("expected sender to be called once, got %d", f.calls)
	}
	// The tx may have left anyway, so the row stays queued and a retry
	// must not send again.
	if len(lg.withdrawals) != 1 || lg.withdrawals[0].Status != "queued" {
		t.Fatalf("withdrawals = %+v, want one queued row", lg.withdrawals)
	}
	if w := postJSON(s.router, "/send", sendOK); w.Code != http.StatusConflict {
		t.Errorf("retry: expected status 409, got %d", w.Code)
	}
	if f.calls != 1 {
		t.Errorf("retry: Transfer called %d times, want 1", f.calls)
	}
}

func TestPostSendLedgerError(t *testing.T) {
	f := &fakeSender{hash: common.HexToHash("0xabc")}
	s, lg := newSendTestServer(f)
	lg.err = errors.New("db down")

	w := postJSON(s.router, "/send", sendOK)

	if w.Code != 500 {
		t.Errorf("expected status 500, got %d", w.Code)
	}
	if f.calls != 0 {
		t.Errorf("expected sender not to be called")
	}
}

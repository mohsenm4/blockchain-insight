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
)

type fakeSender struct {
	hash   common.Hash
	err    error
	called bool
	to     common.Address
	amount *big.Int
	status string
}

func (f *fakeSender) Transfer(ctx context.Context, to common.Address, amount *big.Int) (common.Hash, error) {
	f.called = true
	f.to = to
	f.amount = amount
	return f.hash, f.err
}

func (f *fakeSender) Status(ctx context.Context, hash common.Hash) (string, error) {
	return f.status, f.err
}

func newSendTestServer(f *fakeSender) *Server {
	s := &Server{sender: f}
	s.setupRouter()
	return s
}

func postJSON(r http.Handler, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestPostSendInvalidAddress(t *testing.T) {
	f := &fakeSender{}
	s := newSendTestServer(f)

	// Simulate a POST request with an invalid address
	w := postJSON(s.router, "/send", `{"to":"invalid_address","amount":"1000"}`)

	if w.Code != 400 {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	if f.called {
		t.Errorf("expected sender not to be called")
	}
}

func TestPostSendInvalidAmount(t *testing.T) {
	f := &fakeSender{}
	s := newSendTestServer(f)

	// Simulate a POST request with an invalid amount
	w := postJSON(s.router, "/send", `{"to":"0x0000000000000000000000000000000000000000","amount":"invalid_amount"}`)

	if w.Code != 400 {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	if f.called {
		t.Errorf("expected sender not to be called")
	}
}

func TestPostSendOK(t *testing.T) {
	f := &fakeSender{hash: common.HexToHash("0xabc")}
	s := newSendTestServer(f)

	// Simulate a POST request with a valid address and amount
	w := postJSON(s.router, "/send", `{"to":"0x0000000000000000000000000000000000000000","amount":"1000"}`)

	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if !f.called {
		t.Errorf("expected sender to be called")
	}

	if !strings.Contains(w.Body.String(), f.hash.Hex()) {
		t.Errorf("expected tx_hash in body, got %s", w.Body.String())
	}
	if f.amount.String() != "1000" {
		t.Errorf("expected amount 1000, got %s", f.amount)
	}

}

func TestPostSendSenderError(t *testing.T) {
	f := &fakeSender{
		err: errors.New("sender error"),
	}
	s := newSendTestServer(f)

	// Simulate a POST request with a valid address and amount
	w := postJSON(s.router, "/send", `{"to":"0x0000000000000000000000000000000000000000","amount":"1000"}`)

	if w.Code != 500 {
		t.Errorf("expected status 500, got %d", w.Code)
	}
	if !f.called {
		t.Errorf("expected sender to be called")
	}
}

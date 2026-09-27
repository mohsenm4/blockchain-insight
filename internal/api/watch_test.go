package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/mohsenm4/blockchain-insight/internal/watch"
)

func newTestServer() *Server {
	s := &Server{
		store: watch.NewStore(),
	}

	s.setupRouter()
	return s
}

func TestPostWatchEmptyBody(t *testing.T) {
	s := newTestServer()

	req := httptest.NewRequest(
		http.MethodPost,
		"/watch",
		strings.NewReader(`{}`),
	)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()

	s.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestPostWatchInvalidAddress(t *testing.T) {
	s := newTestServer()

	req := httptest.NewRequest(
		http.MethodPost,
		"/watch",
		strings.NewReader(`{"address":"salam"}`),
	)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()

	s.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestPostWatchValidAddress(t *testing.T) {
	s := newTestServer()

	address := "0x0000000000000000000000000000000000000001"

	req := httptest.NewRequest(
		http.MethodPost,
		"/watch",
		strings.NewReader(`{"address":"`+address+`"}`),
	)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()

	s.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestGetWatchTransfersInvalidAddress(t *testing.T) {
	s := newTestServer()

	req := httptest.NewRequest(
		http.MethodGet,
		"/watch/salam/transfers",
		nil,
	)

	rec := httptest.NewRecorder()

	s.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestGetWatchTransfers(t *testing.T) {
	s := newTestServer()

	address := "0x0000000000000000000000000000000000000001"

	// First, watch the address.
	req := httptest.NewRequest(
		http.MethodPost,
		"/watch",
		strings.NewReader(`{"address":"`+address+`"}`),
	)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected POST status code %d, got %d", http.StatusOK, rec.Code)
	}

	// Add a transfer manually to the store.
	tr := watch.Transfer{
		Block: 123,
		Index: 0,
		From:  common.HexToAddress("0x0000000000000000000000000000000000000002"),
		To:    common.HexToAddress(address),
		Value: big.NewInt(100),
		Tx:    common.HexToHash("0x1234"),
	}

	s.store.Add(tr)

	// Then fetch the transfers.
	req = httptest.NewRequest(
		http.MethodGet,
		"/watch/"+address+"/transfers",
		nil,
	)

	rec = httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected GET status code %d, got %d", http.StatusOK, rec.Code)
	}

	body := rec.Body.String()

	if !strings.Contains(body, `"block":123`) {
		t.Errorf("expected response to contain block 123, got %s", body)
	}

	if !strings.Contains(body, `"value":"100"`) {
		t.Errorf("expected response to contain value 100, got %s", body)
	}
}

func TestGetWatchTransfersEmpty(t *testing.T) {
	s := newTestServer()

	address := "0x0000000000000000000000000000000000000001"

	req := httptest.NewRequest(
		http.MethodGet,
		"/watch/"+address+"/transfers",
		nil,
	)

	rec := httptest.NewRecorder()

	s.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status code %d, got %d", http.StatusOK, rec.Code)
	}

	expected := `{"transfers":[]}`
	if strings.TrimSpace(rec.Body.String()) != expected {
		t.Errorf("expected body %s, got %s", expected, rec.Body.String())
	}
}

func TestPostWatchValidAddressResponse(t *testing.T) {
	s := newTestServer()

	address := "0x0000000000000000000000000000000000000001"

	req := httptest.NewRequest(
		http.MethodPost,
		"/watch",
		strings.NewReader(`{"address":"`+address+`"}`),
	)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()

	s.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status code %d, got %d", http.StatusOK, rec.Code)
	}

	expected := `{"watching":"` + address + `"}`
	if strings.TrimSpace(rec.Body.String()) != expected {
		t.Errorf("expected body %s, got %s", expected, rec.Body.String())
	}
}

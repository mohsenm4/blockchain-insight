package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Mohsen20031203/blockchain-insight/internal/enth"
)

func TestGetAddressBalance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1bc16d674ec80000"}`))
	}))

	defer server.Close()

	client, err := enth.NewClient(server.URL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	s := Server{
		client: client,
	}
	s.setupRouter()

	reco := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/balance/0xa1e4380a3b1f749673e270229993ee55f35663b4", nil)

	s.router.ServeHTTP(reco, req)

	if reco.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d. Body: %s", http.StatusOK, reco.Code, reco.Body.String())
	}

	expectedBody := `{"balance":"2000000000000000000"}`
	if reco.Body.String() != expectedBody {
		t.Errorf("Expected body %s, got %s", expectedBody, reco.Body.String())
	}
}

func TestGetAddressBalance_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := enth.NewClient(server.URL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	s := Server{
		client: client,
	}
	s.setupRouter()

	reco := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/balance/0xa1e4380a3b1f749673e270229993ee55f35663b4", nil)

	s.router.ServeHTTP(reco, req)

	if reco.Code != http.StatusInternalServerError {
		t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, reco.Code)
	}
}

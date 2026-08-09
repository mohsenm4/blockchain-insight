package enth

import (
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetBalance(t *testing.T) {
	// 0x1bc16d674ec80000 = 2 * 10^18 wei = 2 ether
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1bc16d674ec80000"}`)); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	got, err := client.GetBalance("0xa1e4380a3b1f749673e270229993ee55f35663b4")
	if err != nil {
		t.Fatalf("GetBalance failed: %v", err)
	}

	want := new(big.Int)
	want.SetString("2000000000000000000", 10)
	if got.Cmp(want) != 0 {
		t.Errorf("GetBalance() = %s, want %s", got.String(), want.String())
	}
}

func TestGetBalance_Error(t *testing.T) {

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	balance, err := client.GetBalance("0xa1e4380a3b1f749673e270229993ee55f35663b4")
	if err == nil {
		t.Error("expected error, got nil")
	}
	if balance != nil {
		t.Errorf("expected nil balance, got %v", balance)
	}
}
func TestNewClient_InvalidURL(t *testing.T) {
	_, err := NewClient("this-is-not-a-valid-url")
	if err == nil {
		t.Error("expected error for invalid URL, got nil")
	}
}

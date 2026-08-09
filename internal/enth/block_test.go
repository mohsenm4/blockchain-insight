package enth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetLastBlockNumber(t *testing.T) {

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x5daf3b"}`)); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))

	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	blockNumber, err := client.GetLastBlockNumber()
	if err != nil {
		t.Fatalf("GetLastBlockNumber() error = %v", err)
	}

	wantBlockNumber := uint64(0x5daf3b) // 0x5daf3b in decimal
	if blockNumber != wantBlockNumber {
		t.Errorf("GetLastBlockNumber() = %d, want %d", blockNumber, wantBlockNumber)
	}
}

func TestGetLastBlockNumber_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	blockNumber, err := client.GetLastBlockNumber()
	if err == nil {
		t.Error("expected error, got nil")
	}
	if blockNumber != 0 {
		t.Errorf("expected 0 block number, got %d", blockNumber)
	}
}

func TestGetBlockByNumber_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	block, err := client.GetBlockByNumber(6139395)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if block != nil {
		t.Errorf("expected nil block, got %v", block)
	}
}

func TestGetBlockByNumber(t *testing.T) {
	t.Skip("requires real Ethereum node — ethclient.BlockByNumber validates block hash against header")
}

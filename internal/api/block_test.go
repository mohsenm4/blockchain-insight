package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Mohsen20031203/blockchain-insight/internal/enth"
	"github.com/patrickmn/go-cache"
)

func TestGetLastBlock_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := enth.NewClient(server.URL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	s := Server{
		cach:   cache.New(cache.NoExpiration, 1*time.Hour),
		client: client,
	}
	s.setupRouter()

	reco := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/last/block", nil)

	s.router.ServeHTTP(reco, req)

	if reco.Code != http.StatusInternalServerError {
		t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, reco.Code)
	}
}

func TestGetLastBlock(t *testing.T) {
	t.Skip("requires real Ethereum node — ethclient.BlockByNumber validates block hash against header")
}

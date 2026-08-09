package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Mohsen20031203/blockchain-insight/internal/enth"
	"github.com/Mohsen20031203/blockchain-insight/internal/models"
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
	expectedBlock := &models.Block{Number: 200, Hash: "0xdef"}
	fake := &fakeClient{block: expectedBlock}

	s := Server{
		cach:   cache.New(cache.NoExpiration, 1*time.Hour),
		client: fake,
	}
	s.setupRouter()

	reco := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/last/block", nil)

	s.router.ServeHTTP(reco, req)

	if reco.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, reco.Code)
	}

	var got models.Block
	if err := json.Unmarshal(reco.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if got.Number != expectedBlock.Number {
		t.Errorf("expected Number %d, got %d", expectedBlock.Number, got.Number)
	}
	if got.Hash != expectedBlock.Hash {
		t.Errorf("expected Hash %s, got %s", expectedBlock.Hash, got.Hash)
	}
}

func TestGetBlockById_InvalidNumber(t *testing.T) {
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
	req := httptest.NewRequest("GET", "/block/invalid-number", nil)

	s.router.ServeHTTP(reco, req)

	if reco.Code != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, reco.Code)
	}
	expectedBody := `{"error":"invalid block number"}`
	if reco.Body.String() != expectedBody {
		t.Errorf("Expected body %s, got %s", expectedBody, reco.Body.String())
	}
}

func TestGetBlockById_Error(t *testing.T) {
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
	req := httptest.NewRequest("GET", "/block/123", nil)

	s.router.ServeHTTP(reco, req)

	if reco.Code != http.StatusInternalServerError {
		t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, reco.Code)
	}
}

func TestGetBlockById(t *testing.T) {
	expectedBlock := &models.Block{Number: 100, Hash: "0xabc"}
	fake := &fakeClient{block: expectedBlock}

	s := Server{
		cach:   cache.New(cache.NoExpiration, 1*time.Hour),
		client: fake,
	}
	s.setupRouter()

	reco := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/block/100", nil)

	s.router.ServeHTTP(reco, req)

	if reco.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, reco.Code)
	}

	var got models.Block
	if err := json.Unmarshal(reco.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if got.Number != expectedBlock.Number {
		t.Errorf("expected Number %d, got %d", expectedBlock.Number, got.Number)
	}
	if got.Hash != expectedBlock.Hash {
		t.Errorf("expected Hash %s, got %s", expectedBlock.Hash, got.Hash)
	}

}

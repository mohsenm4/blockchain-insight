package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Mohsen20031203/blockchain-insight/internal/models"
	"github.com/patrickmn/go-cache"
)

func TestCache_HIT(t *testing.T) {
	expectedBlock := &models.Block{
		Number: 42,
		Hash:   "0xdeadbeef",
	}

	s := Server{
		client: &fakeClient{},
		cach:   cache.New(cache.NoExpiration, 1*time.Hour),
	}
	s.setupRouter()

	s.cach.Set(LastBlock, expectedBlock, cache.DefaultExpiration)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/last/block", nil)
	s.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var got models.Block
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.Number != expectedBlock.Number {
		t.Errorf("expected Number %d, got %d", expectedBlock.Number, got.Number)
	}
	if got.Hash != expectedBlock.Hash {
		t.Errorf("expected Hash %s, got %s", expectedBlock.Hash, got.Hash)
	}

}

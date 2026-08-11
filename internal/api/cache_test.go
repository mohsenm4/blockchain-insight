package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
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

func TestSingleflight_Dedupes_Concurrent_Misses(t *testing.T) {
	fake := &fakeClient{
		block:           &models.Block{Number: 42, Hash: "0xabc"},
		lastBlockNumber: 42,
		latency:         10 * time.Millisecond,
	}

	s := Server{
		client: fake,
		cach:   cache.New(cache.NoExpiration, 1*time.Hour),
	}
	s.setupRouter()

	const N = 20
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(N)

	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			<-start

			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/last/block", nil)
			s.router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("expected 200, got %d", rec.Code)
			}
		}()
	}

	close(start)
	wg.Wait()

	if got := fake.getLastBlockCount.Load(); got != 1 {
		t.Errorf("stampede on GetLastBlockNumber! expected 1 call, got %d", got)
	}
	if got := fake.getBlockCount.Load(); got != 1 {
		t.Errorf("stampede on GetBlockByNumber! expected 1 call, got %d", got)
	}

}

package api

import (
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Mohsen20031203/blockchain-insight/internal/models"
	"github.com/patrickmn/go-cache"
)

func TestServerCacheConcurrentAccess(t *testing.T) {
	s := &Server{
		cach: cache.New(cache.NoExpiration, 1*time.Hour),
	}

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()

			if cached, ok := s.cach.Get(LastBlock); ok {
				blk := cached.(*models.Block)
				_ = blk.Number
			}

			block := &models.Block{Number: uint64(id)}
			s.cach.Set(LastBlock, block, 10*time.Second)
		}(i)
	}

	wg.Wait()
}

func TestCacheMiddleware_Hit(t *testing.T) {
	server := &Server{
		cach: cache.New(cache.NoExpiration, 1*time.Hour),
	}

	block := &models.Block{Number: 123}
	server.cach.Set(LastBlock, block, 10*time.Second)

	server.setupRouter()

	reco := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/last/block", nil)

	server.router.ServeHTTP(reco, req)

	if reco.Code != 200 {
		t.Errorf("Expected status code %d, got %d", 200, reco.Code)
	}
	if !strings.Contains(reco.Body.String(), `"number":123`) {
		t.Errorf("Expected body to contain cached block, got %s", reco.Body.String())
	}
}

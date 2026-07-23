package api

import (
	"sync"
	"testing"
	"time"

	"github.com/Mohsen20031203/blockchain-insight/internal/models"
	"github.com/patrickmn/go-cache"
)

// TestServerCacheConcurrentAccess mirrors the real access pattern under
// concurrent HTTP requests to /last/block: the Cache() middleware reads
// s.cach, and GetLastBlock writes to s.cach with s.cach.Set. This test
// fires many goroutines doing both, so the race detector can observe
// whether the pattern is safe.
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

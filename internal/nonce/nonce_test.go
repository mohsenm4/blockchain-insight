package nonce

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

type fakeNonceProvider struct {
	calls atomic.Int32
}

func (f *fakeNonceProvider) PendingNonceAt(
	ctx context.Context,
	addr common.Address,
) (uint64, error) {
	f.calls.Add(1)
	return 5, nil
}

func TestNonceManager_Concurrent(t *testing.T) {
	fake := &fakeNonceProvider{}
	manager := NewNonceManager(common.Address{}, fake)

	const n = 10

	results := make([]uint64, n)

	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()

			nonce, err := manager.NextNonce(context.Background())
			if err != nil {
				t.Errorf("NextNonce failed: %v", err)
				return
			}

			results[i] = nonce
		}(i)
	}

	wg.Wait()

	seen := make(map[uint64]bool)

	for _, nonce := range results {
		if seen[nonce] {
			t.Errorf("duplicate nonce: %d", nonce)
		}

		seen[nonce] = true
	}

	for nonce := uint64(5); nonce <= 14; nonce++ {
		if !seen[nonce] {
			t.Errorf("missing nonce: %d", nonce)
		}
	}

	if calls := fake.calls.Load(); calls != 1 {
		t.Errorf("provider called %d times, want 1", calls)
	}
}

func TestNonceManager_Reset(t *testing.T) {
	fake := &fakeNonceProvider{}
	manager := NewNonceManager(common.Address{}, fake)

	nonce, err := manager.NextNonce(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if nonce != 5 {
		t.Errorf("got nonce %d, want 5", nonce)
	}

	manager.Reset()

	nonce, err = manager.NextNonce(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if nonce != 5 {
		t.Errorf("got nonce %d, want 5", nonce)
	}

	if calls := fake.calls.Load(); calls != 2 {
		t.Errorf("provider called %d times, want 2", calls)
	}
}

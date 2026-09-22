package watch

import (
	"context"
	"errors"
	"math/big"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type fakeSource struct {
	mu    sync.Mutex
	block uint64
}

func (f *fakeSource) BlockNumber(ctx context.Context) (uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.block++
	return f.block, nil
}

func (f *fakeSource) Transfers(ctx context.Context, from, to uint64) ([]Transfer, error) {
	return []Transfer{
		{
			Block: from,
			Index: 0,
			From:  common.Address{},
			To:    common.Address{},
			Value: big.NewInt(1),
			Tx:    common.Hash{},
		},
	}, nil
}

func TestRunNoLeak(t *testing.T) {
	src := &fakeSource{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out := make(chan Transfer)

	before := runtime.NumGoroutine()

	var received atomic.Int64
	readerDone := make(chan struct{})

	go func() {
		defer close(readerDone)

		for range out {
			received.Add(1)
		}
	}()

	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, src, 5*time.Millisecond, out)
	}()

	// Give Run time to poll and produce at least one transfer.
	time.Sleep(20 * time.Millisecond)

	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after cancellation")
	}

	if received.Load() == 0 {
		t.Fatal("Run produced no transfers")
	}

	// Run never closes out; the owner of the channel does.
	close(out)

	select {
	case <-readerDone:
	case <-time.After(time.Second):
		t.Fatal("reader goroutine did not stop")
	}

	time.Sleep(50 * time.Millisecond)

	after := runtime.NumGoroutine()
	if after > before {
		t.Fatalf("goroutine leak: before=%d, after=%d", before, after)
	}
}

func TestRunReturnsWhenNobodyReads(t *testing.T) {
	src := &fakeSource{}
	ctx, cancel := context.WithCancel(context.Background())

	out := make(chan Transfer)

	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, src, 5*time.Millisecond, out)
	}()

	// Wait until Run has had a chance to reach the send and block.
	time.Sleep(20 * time.Millisecond)

	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run is stuck sending to out after cancellation")
	}
}

func TestRunReturnsPromptlyAfterCancel(t *testing.T) {
	src := &fakeSource{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out := make(chan Transfer)
	go func() {
		for range out {
		}
	}()

	done := make(chan error, 1)
	go func() { done <- Run(ctx, src, time.Second, out) }()

	time.Sleep(20 * time.Millisecond) // بذار وارد انتظار بشه
	start := time.Now()
	cancel()

	select {
	case <-done:
		if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
			t.Fatalf("Run took %v to return after cancel, want < 200ms", elapsed)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

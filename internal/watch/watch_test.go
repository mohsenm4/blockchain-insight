package watch

import (
	"context"
	"errors"
	"math/big"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// fakeSource reports a head that grows by step on every BlockNumber call,
// and one transfer per Transfers call.
type fakeSource struct {
	mu   sync.Mutex
	head uint64
	step uint64
}

func (f *fakeSource) BlockNumber(ctx context.Context) (uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.head += f.step
	return f.head, nil
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

// batchRecorder is a Handler that keeps every batch it was given.
type batchRecorder struct {
	mu      sync.Mutex
	batches []Batch
}

func (r *batchRecorder) handle(ctx context.Context, b Batch) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.batches = append(r.batches, b)
	return nil
}

func (r *batchRecorder) snapshot() []Batch {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Batch(nil), r.batches...)
}

// runFor runs Run in the background and stops it after d.
func runFor(t *testing.T, d time.Duration, src Source, last uint64, interval time.Duration, h Handler) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, src, last, interval, h) }()

	time.Sleep(d)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

func TestRunHandsOverContiguousRanges(t *testing.T) {
	src := &fakeSource{head: 100, step: 1}
	rec := &batchRecorder{}

	runFor(t, 50*time.Millisecond, src, 100, 5*time.Millisecond, rec.handle)

	batches := rec.snapshot()
	if len(batches) < 2 {
		t.Fatalf("got %d batches, want at least 2", len(batches))
	}
	next := uint64(101) // first block after last
	for i, b := range batches {
		if b.From != next || b.To < b.From {
			t.Fatalf("batch %d = %d..%d, want it to start at %d", i, b.From, b.To, next)
		}
		next = b.To + 1
	}
}

func TestRunRetriesFailedBatch(t *testing.T) {
	src := &fakeSource{head: 10, step: 0}

	var mu sync.Mutex
	var froms []uint64
	handle := func(ctx context.Context, b Batch) error {
		mu.Lock()
		defer mu.Unlock()
		froms = append(froms, b.From)
		if len(froms) == 1 {
			return errors.New("db down")
		}
		return nil
	}

	runFor(t, 50*time.Millisecond, src, 5, 5*time.Millisecond, handle)

	mu.Lock()
	defer mu.Unlock()
	if len(froms) < 2 || froms[0] != 6 || froms[1] != 6 {
		t.Fatalf("batch starts = %v, want the failed batch (6) retried", froms)
	}
}

func TestRunCatchesUpInCappedRanges(t *testing.T) {
	src := &fakeSource{head: 1200, step: 0}
	rec := &batchRecorder{}

	// A long interval: catching up must not wait between ranges.
	runFor(t, 50*time.Millisecond, src, 0, time.Hour, rec.handle)

	want := []Batch{{From: 1, To: 500}, {From: 501, To: 1000}, {From: 1001, To: 1200}}
	got := rec.snapshot()
	if len(got) != len(want) {
		t.Fatalf("got %d batches, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].From != want[i].From || got[i].To != want[i].To {
			t.Fatalf("batch %d = %d..%d, want %d..%d", i, got[i].From, got[i].To, want[i].From, want[i].To)
		}
	}
}

func TestRunNoLeak(t *testing.T) {
	before := runtime.NumGoroutine()

	rec := &batchRecorder{}
	runFor(t, 20*time.Millisecond, &fakeSource{step: 1}, 0, 5*time.Millisecond, rec.handle)

	if len(rec.snapshot()) == 0 {
		t.Fatal("Run produced no batches")
	}

	time.Sleep(50 * time.Millisecond)

	after := runtime.NumGoroutine()
	if after > before {
		t.Fatalf("goroutine leak: before=%d, after=%d", before, after)
	}
}

func TestRunReturnsWhenHandlerBlocks(t *testing.T) {
	// A handler stuck until ctx is done, like a slow DB write.
	handle := func(ctx context.Context, b Batch) error {
		<-ctx.Done()
		return ctx.Err()
	}

	runFor(t, 20*time.Millisecond, &fakeSource{step: 1}, 0, 5*time.Millisecond, handle)
}

func TestRunReturnsPromptlyAfterCancel(t *testing.T) {
	src := &fakeSource{step: 1}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	noop := func(ctx context.Context, b Batch) error { return nil }

	done := make(chan error, 1)
	go func() { done <- Run(ctx, src, 0, time.Second, noop) }()

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

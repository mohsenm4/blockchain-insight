package watch

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mohsenm4/blockchain-insight/internal/contracts/erc20"
	"github.com/mohsenm4/blockchain-insight/internal/enth"
)

// hangFirst holds the first request until the caller gives up, and hands
// every later one to next: a node that hangs once and comes back.
type hangFirst struct {
	hung atomic.Bool
	next http.Handler
}

func (h *hangFirst) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.hung.CompareAndSwap(false, true) {
		// Read the body first: only then does the server watch the
		// connection and cancel r.Context() when the caller gives up.
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
		return
	}
	h.next.ServeHTTP(w, r)
}

// One hung call must not stop the watcher for good: it gives up on that
// call and reads the next blocks once the node answers again. Without a
// timeout Run waited on the first call forever, without a log line.
func TestRunGoesOnAfterNodeHangs(t *testing.T) {
	srv := httptest.NewServer(&hangFirst{next: &fakeNode{head: 1000, logs: []map[string]any{}}})
	t.Cleanup(srv.Close)

	client, err := enth.Dial(context.Background(), srv.URL, 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	token, err := erc20.NewERC20(testToken, client)
	if err != nil {
		t.Fatal(err)
	}
	src := NewERC20Source(client, token)

	want := uint64(1000 - confirmations)
	got := make(chan Batch, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, src, want-1, 10*time.Millisecond, func(_ context.Context, b Batch) error {
			select {
			case got <- b:
			default:
			}
			return nil
		})
	}()
	defer func() {
		cancel()
		<-done
	}()

	select {
	case b := <-got:
		if b.From != want || b.To != want {
			t.Fatalf("batch %d..%d, want %d..%d", b.From, b.To, want, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no batch after 5s: Run is stuck on the hung call")
	}
}

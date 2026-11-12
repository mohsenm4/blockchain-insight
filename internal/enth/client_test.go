package enth

import (
	"context"
	"net"
	"testing"
	"time"
)

// hungNode returns the URL of a node that takes connections and never
// answers. The kernel completes the connect without Accept; nothing ever
// reads or replies.
func hungNode(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return "http://" + ln.Addr().String()
}

// A node that never answers must give an error after the timeout, also to
// a caller whose context has no deadline, like the watcher and the trackers.
func TestDialHungNodeTimesOut(t *testing.T) {
	const timeout = 200 * time.Millisecond
	client, err := Dial(context.Background(), hungNode(t), timeout)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)

	// Only the test's own limit: without it a broken Dial would hang the
	// test binary instead of failing here.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	_, err = client.BlockNumber(ctx)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("BlockNumber on a hung node: no error")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("BlockNumber took %v, want about %v", elapsed, timeout)
	}
}

func TestDialNeedsATimeout(t *testing.T) {
	if _, err := Dial(context.Background(), "http://127.0.0.1:8545", 0); err == nil {
		t.Fatal("Dial with no timeout: no error")
	}
}

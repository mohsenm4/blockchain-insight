package ratelimit

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// testRedis connects to TEST_REDIS_URL, or skips the test when it is unset.
func testRedis(t *testing.T) *redis.Client {
	t.Helper()
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL not set") // CI has no redis
	}
	rdb, err := NewClient(url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rdb.Close() })
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	return rdb
}

// testPrefix keeps each test's keys apart, also across runs.
func testPrefix(t *testing.T) string {
	return fmt.Sprintf("test:%s:%d:", t.Name(), time.Now().UnixNano())
}

func TestAllowUpToLimit(t *testing.T) {
	rdb := testRedis(t)
	l := New(rdb, testPrefix(t), 5, time.Minute)
	ctx := context.Background()

	for i := 1; i <= 5; i++ {
		ok, _, err := l.Allow(ctx, "1")
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("request %d refused, want allowed", i)
		}
	}

	ok, retryAfter, err := l.Allow(ctx, "1")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("request 6 allowed, want refused")
	}
	if retryAfter <= 0 || retryAfter > time.Minute {
		t.Fatalf("retryAfter = %v, want in (0, 1m]", retryAfter)
	}
}

func TestAllowKeysAreSeparate(t *testing.T) {
	rdb := testRedis(t)
	l := New(rdb, testPrefix(t), 1, time.Minute)
	ctx := context.Background()

	if ok, _, err := l.Allow(ctx, "1"); err != nil || !ok {
		t.Fatalf("user 1 first request: ok=%v err=%v", ok, err)
	}
	if ok, _, err := l.Allow(ctx, "2"); err != nil || !ok {
		t.Fatalf("user 2 first request: ok=%v err=%v, want allowed", ok, err)
	}
}

func TestAllowWindowEnds(t *testing.T) {
	rdb := testRedis(t)
	l := New(rdb, testPrefix(t), 1, time.Second)
	ctx := context.Background()

	if ok, _, _ := l.Allow(ctx, "1"); !ok {
		t.Fatal("first request refused")
	}
	if ok, _, _ := l.Allow(ctx, "1"); ok {
		t.Fatal("second request allowed in the same window")
	}

	time.Sleep(1100 * time.Millisecond)
	if ok, _, err := l.Allow(ctx, "1"); err != nil || !ok {
		t.Fatalf("after the window: ok=%v err=%v, want allowed", ok, err)
	}
}

// The counter must always have a TTL; one without would block the user
// forever.
func TestCounterHasTTL(t *testing.T) {
	rdb := testRedis(t)
	prefix := testPrefix(t)
	l := New(rdb, prefix, 5, time.Minute)
	ctx := context.Background()

	for range 3 {
		if _, _, err := l.Allow(ctx, "1"); err != nil {
			t.Fatal(err)
		}
	}
	ttl, err := rdb.PTTL(ctx, prefix+"1").Result()
	if err != nil {
		t.Fatal(err)
	}
	if ttl <= 0 || ttl > time.Minute {
		t.Fatalf("TTL = %v, want in (0, 1m]", ttl)
	}
}

// Later requests must not push the window's end back (EXPIRE NX).
func TestLaterRequestsKeepTheWindowEnd(t *testing.T) {
	rdb := testRedis(t)
	l := New(rdb, testPrefix(t), 1, 2*time.Second)
	ctx := context.Background()

	if ok, _, _ := l.Allow(ctx, "1"); !ok {
		t.Fatal("first request refused")
	}
	time.Sleep(1100 * time.Millisecond)
	_, retryAfter, err := l.Allow(ctx, "1")
	if err != nil {
		t.Fatal(err)
	}
	if retryAfter > time.Second {
		t.Fatalf("retryAfter = %v, want <= 1s: the window was extended", retryAfter)
	}
}

// refusedAddr returns a local address where nothing listens, so a connect
// is refused: a stopped Redis.
func refusedAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

// hangingAddr returns a local address that takes connections and never
// answers: a Redis that is frozen, or a broken network path. The kernel
// completes the connect without Accept; nothing ever reads or replies.
func hangingAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln.Addr().String()
}

// Redis down must give an error in about Timeout. Without it, go-redis
// retries a refused connect for ~3s and waits ReadTimeout (5s) on a hang.
func TestAllowRedisDown(t *testing.T) {
	for _, tc := range []struct {
		name string
		addr string
	}{
		{"refused", refusedAddr(t)},
		{"hangs", hangingAddr(t)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rdb, err := NewClient("redis://" + tc.addr)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = rdb.Close() })
			l := New(rdb, "test:", 5, time.Minute)

			start := time.Now()
			ok, _, err := l.Allow(context.Background(), "1")
			elapsed := time.Since(start)
			if err == nil || ok {
				t.Fatalf("ok=%v err=%v, want an error", ok, err)
			}
			if elapsed > time.Second {
				t.Fatalf("Allow took %v, want about %v", elapsed, Timeout)
			}
		})
	}
}

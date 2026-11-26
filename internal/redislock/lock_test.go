package redislock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func testRedisClient(t *testing.T) *redis.Client {
	t.Helper()

	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL not set")
	}

	opt, err := redis.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}

	opt.ContextTimeoutEnabled = true
	rdb := redis.NewClient(opt)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		2*time.Second,
	)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = rdb.Close()
	})

	return rdb
}

func testLockKey(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf(
		"test:redislock:%s:%d",
		t.Name(),
		time.Now().UnixNano(),
	)
}

func TestAcquireAllowsOnlyOneConcurrentOwner(t *testing.T) {
	rdb := testRedisClient(t)

	lock, err := New(
		rdb,
		testLockKey(t),
		5*time.Second,
		time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}

	const contenders = 16

	start := make(chan struct{})
	leases := make(chan *Lease, contenders)

	var wg sync.WaitGroup

	for i := 0; i < contenders; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			<-start

			lease, acquired, err := lock.Acquire(
				context.Background(),
			)
			if err != nil {
				t.Errorf("Acquire: %v", err)
				return
			}

			if acquired {
				leases <- lease
			}
		}()
	}

	close(start)
	wg.Wait()

	if got := len(leases); got != 1 {
		for len(leases) > 0 {
			lease := <-leases
			_ = lease.Release(context.Background())
		}

		t.Fatalf("acquired leases = %d, want 1", got)
	}

	lease := <-leases

	t.Cleanup(func() {
		_ = lease.Release(context.Background())
	})
}

func TestReleaseAllowsNextOwner(t *testing.T) {
	rdb := testRedisClient(t)

	lock, err := New(
		rdb,
		testLockKey(t),
		2*time.Second,
		500*time.Millisecond,
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	first, ok, err := lock.Acquire(ctx)
	if err != nil || !ok {
		t.Fatalf("first Acquire: ok=%v err=%v", ok, err)
	}

	t.Cleanup(func() {
		_ = first.Release(context.Background())
	})

	if _, ok, err := lock.Acquire(ctx); err != nil || ok {
		t.Fatalf("second Acquire: ok=%v err=%v; want unavailable", ok, err)
	}

	if err := first.Release(ctx); err != nil {
		t.Fatal(err)
	}

	second, ok, err := lock.Acquire(ctx)
	if err != nil || !ok {
		t.Fatalf("Acquire after release: ok=%v err=%v", ok, err)
	}

	t.Cleanup(func() {
		_ = second.Release(context.Background())
	})
}

func TestRenewalKeepsLockAlive(t *testing.T) {
	rdb := testRedisClient(t)

	lock, err := New(
		rdb,
		testLockKey(t),
		300*time.Millisecond,
		75*time.Millisecond,
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	lease, ok, err := lock.Acquire(ctx)
	if err != nil || !ok {
		t.Fatalf("Acquire: ok=%v err=%v", ok, err)
	}

	t.Cleanup(func() {
		_ = lease.Release(context.Background())
	})

	time.Sleep(700 * time.Millisecond)

	if lease.Context().Err() != nil {
		t.Fatalf("lease context unexpectedly canceled: %v", lease.Context().Err())
	}

	_, ok, err = lock.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if ok {
		t.Fatal("lock expired despite automatic renewal")
	}
}

func TestOldOwnerCannotReleaseNewOwnerLock(t *testing.T) {
	rdb := testRedisClient(t)

	key := testLockKey(t)

	lock, err := New(
		rdb,
		key,
		2*time.Second,
		500*time.Millisecond,
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	first, ok, err := lock.Acquire(ctx)
	if err != nil || !ok {
		t.Fatalf("first Acquire: ok=%v err=%v", ok, err)
	}

	t.Cleanup(func() {
		_ = first.Release(context.Background())
	})

	// Simulate expiration or ownership replacement.
	if err := rdb.Del(ctx, key).Err(); err != nil {
		t.Fatal(err)
	}

	second, ok, err := lock.Acquire(ctx)
	if err != nil || !ok {
		t.Fatalf("second Acquire: ok=%v err=%v", ok, err)
	}

	t.Cleanup(func() {
		_ = second.Release(context.Background())
	})

	err = first.Release(ctx)
	if !errors.Is(err, ErrNotOwner) {
		t.Fatalf("old owner Release error = %v, want ErrNotOwner", err)
	}

	got, err := rdb.Get(ctx, key).Result()
	if err != nil {
		t.Fatal(err)
	}

	if got != second.token {
		t.Fatal("old owner deleted or changed the new owner's lock")
	}
}

func TestLostOwnershipCancelsContext(t *testing.T) {
	rdb := testRedisClient(t)

	key := testLockKey(t)

	lock, err := New(
		rdb,
		key,
		2*time.Second,
		50*time.Millisecond,
	)
	if err != nil {
		t.Fatal(err)
	}

	lease, ok, err := lock.Acquire(context.Background())
	if err != nil || !ok {
		t.Fatalf("Acquire: ok=%v err=%v", ok, err)
	}

	t.Cleanup(func() {
		_ = lease.Release(context.Background())
	})

	if err := rdb.Del(context.Background(), key).Err(); err != nil {
		t.Fatal(err)
	}

	select {
	case <-lease.Lost():
		// Ownership loss detected.

	case <-time.After(time.Second):
		t.Fatal("ownership loss was not detected")
	}

	if lease.Context().Err() == nil {
		t.Fatal("lease context was not canceled after ownership loss")
	}
}

func TestDoSkipsWhenLockUnavailable(t *testing.T) {
	rdb := testRedisClient(t)

	lock, err := New(rdb, testLockKey(t), 2*time.Second, 500*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	first, ok, err := lock.Acquire(ctx)
	if err != nil || !ok {
		t.Fatalf("first Acquire: ok=%v err=%v", ok, err)
	}
	t.Cleanup(func() { _ = first.Release(context.Background()) })

	called := false
	acquired, err := lock.Do(ctx, func(context.Context) error {
		called = true
		return nil
	})

	if err != nil {
		t.Fatal(err)
	}
	if acquired {
		t.Fatal("Do acquired an already-owned lock")
	}
	if called {
		t.Fatal("callback ran without acquiring the lock")
	}

	if err := first.Release(ctx); err != nil {
		t.Fatal(err)
	}

	acquired, err = lock.Do(ctx, func(context.Context) error {
		called = true
		return nil
	})

	if err != nil || !acquired || !called {
		t.Fatalf("Do after release: acquired=%v called=%v err=%v",
			acquired, called, err)
	}
}

func TestDoReturnsLeaseLost(t *testing.T) {
	rdb := testRedisClient(t)
	key := testLockKey(t)

	lock, err := New(rdb, key, 2*time.Second, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	acquired, err := lock.Do(context.Background(), func(ctx context.Context) error {
		if err := rdb.Del(context.Background(), key).Err(); err != nil {
			return err
		}

		<-ctx.Done()
		return ctx.Err()
	})

	if !acquired {
		t.Fatal("Do did not acquire the lock")
	}
	if !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("Do error = %v, want ErrLeaseLost", err)
	}
}

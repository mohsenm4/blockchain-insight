package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"
)

// fakeLimiter allows limit requests per key, with no window.
type fakeLimiter struct {
	limit int
	err   error
	calls map[string]int
}

func (f *fakeLimiter) Allow(ctx context.Context, key string) (bool, time.Duration, error) {
	if f.err != nil {
		return false, 0, f.err
	}
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[key]++
	if f.calls[key] > f.limit {
		return false, 30 * time.Second, nil
	}
	return true, 0, nil
}

func sendBody(userID int64, key string) string {
	return `{"user_id":` + strconv.FormatInt(userID, 10) + `,"idempotency_key":"` + key +
		`","to":"0x0000000000000000000000000000000000000000","amount":"1000"}`
}

func TestPostSendRateLimited(t *testing.T) {
	f := &fakeSender{}
	s, lg := newSendTestServer(f)
	s.limiter = &fakeLimiter{limit: 2}

	for i, key := range []string{"k1", "k2"} {
		if w := postSend(s.router, sendBody(1, key)); w.Code != http.StatusOK {
			t.Fatalf("request %d: code = %d, want 200: %s", i+1, w.Code, w.Body)
		}
	}

	w := postSend(s.router, sendBody(1, "k3"))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("request 3: code = %d, want 429: %s", w.Code, w.Body)
	}
	if got := w.Header().Get("Retry-After"); got != "30" {
		t.Errorf("Retry-After = %q, want 30", got)
	}
	if f.signs != 2 || len(lg.withdrawals) != 2 {
		t.Errorf("signs, rows = %d, %d, want 2, 2: a refused request must not create a withdrawal", f.signs, len(lg.withdrawals))
	}
}

func TestPostSendRateLimitPerUser(t *testing.T) {
	f := &fakeSender{}
	s, _ := newSendTestServer(f)
	s.limiter = &fakeLimiter{limit: 1}

	if w := postSend(s.router, sendBody(1, "k1")); w.Code != http.StatusOK {
		t.Fatalf("user 1: code = %d, want 200: %s", w.Code, w.Body)
	}
	if w := postSend(s.router, sendBody(2, "k1")); w.Code != http.StatusOK {
		t.Fatalf("user 2: code = %d, want 200 (own limit): %s", w.Code, w.Body)
	}
}

func TestPostSendRateLimiterError(t *testing.T) {
	f := &fakeSender{}
	s, lg := newSendTestServer(f)
	s.limiter = &fakeLimiter{err: errors.New("connection refused")}

	w := postSend(s.router, sendOK)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503: %s", w.Code, w.Body)
	}
	if f.signs != 0 || len(lg.withdrawals) != 0 {
		t.Errorf("signs, rows = %d, %d, want 0, 0", f.signs, len(lg.withdrawals))
	}
}

func TestPostSendBadRequestNotCounted(t *testing.T) {
	f := &fakeSender{}
	s, _ := newSendTestServer(f)
	lim := &fakeLimiter{limit: 1}
	s.limiter = lim

	// No user_id to count it against: 400 before the limiter.
	if w := postSend(s.router, `{"to":"0x0"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", w.Code)
	}
	if len(lim.calls) != 0 {
		t.Errorf("limiter calls = %v, want none", lim.calls)
	}
}

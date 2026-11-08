// Package ratelimit counts requests per key in Redis, in fixed windows.
package ratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limiter allows at most limit requests per key in each window.
type Limiter struct {
	rdb    *redis.Client
	prefix string
	limit  int64
	window time.Duration
}

// New returns a Limiter whose keys start with prefix. window is rounded
// down to whole seconds (EXPIRE works in seconds), so it must be >= 1s.
func New(rdb *redis.Client, prefix string, limit int64, window time.Duration) *Limiter {
	return &Limiter{rdb: rdb, prefix: prefix, limit: limit, window: window}
}

// Allow counts one request for key. When the limit is reached, it returns
// false and how long until the window ends.
//
// The window starts at the first request: INCR creates the counter and
// EXPIRE NX gives it a TTL only if it has none. Both run in one MULTI, so a
// crash between them cannot leave a counter that never expires (that user
// would be blocked forever).
func (l *Limiter) Allow(ctx context.Context, key string) (ok bool, retryAfter time.Duration, err error) {
	k := l.prefix + key

	var incr *redis.IntCmd
	var ttl *redis.DurationCmd
	_, err = l.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		incr = p.Incr(ctx, k)
		p.ExpireNX(ctx, k, l.window)
		ttl = p.PTTL(ctx, k)
		return nil
	})
	if err != nil {
		return false, 0, err
	}

	if incr.Val() > l.limit {
		return false, ttl.Val(), nil
	}
	return true, 0, nil
}

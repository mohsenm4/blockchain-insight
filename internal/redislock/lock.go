package redislock

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const operationTimeout = time.Second

var ErrNotOwner = errors.New("redis lock is no longer owned")
var ErrLeaseLost = errors.New("redis lock lease lost")

const renewScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0
`

const releaseScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
end
return 0
`

type Lock struct {
	client     *redis.Client
	key        string
	ttl        time.Duration
	renewEvery time.Duration
}

type Lease struct {
	lock  *Lock
	token string

	ctx    context.Context
	cancel context.CancelFunc

	done chan struct{}
	lost chan struct{}

	releaseOnce sync.Once
	releaseErr  error
}

func New(
	client *redis.Client,
	key string,
	ttl time.Duration,
	renewEvery time.Duration,
) (*Lock, error) {
	if client == nil {
		return nil, errors.New("redis client is nil")
	}

	if key == "" {
		return nil, errors.New("redis lock key is empty")
	}

	if ttl < 3*time.Millisecond {
		return nil, errors.New("redis lock TTL must be at least 3ms")
	}

	if renewEvery <= 0 || renewEvery*2 >= ttl {
		return nil, errors.New("renew interval must be positive and less than half the TTL")
	}

	return &Lock{
		client:     client,
		key:        key,
		ttl:        ttl,
		renewEvery: renewEvery,
	}, nil
}

func (l *Lock) Acquire(ctx context.Context) (*Lease, bool, error) {
	token, err := newToken()
	if err != nil {
		return nil, false, fmt.Errorf("generate lock token: %w", err)
	}

	acquireCtx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()

	acquired, err := l.client.SetNX(
		acquireCtx,
		l.key,
		token,
		l.ttl,
	).Result()

	if err != nil {
		return nil, false, fmt.Errorf("acquire redis lock: %w", err)
	}

	if !acquired {
		return nil, false, nil
	}

	leaseCtx, leaseCancel := context.WithCancel(ctx)

	lease := &Lease{
		lock:   l,
		token:  token,
		ctx:    leaseCtx,
		cancel: leaseCancel,
		done:   make(chan struct{}),
		lost:   make(chan struct{}),
	}

	go lease.renewLoop()

	return lease, true, nil
}

func (l *Lease) renewLoop() {
	defer close(l.done)

	ticker := time.NewTicker(l.lock.renewEvery)
	defer ticker.Stop()

	for {
		select {
		case <-l.ctx.Done():
			return

		case <-ticker.C:
			ctx, cancel := context.WithTimeout(
				l.ctx,
				operationTimeout,
			)

			renewed, err := l.lock.renew(ctx, l.token)
			cancel()

			if l.ctx.Err() != nil {
				return
			}

			if err != nil || !renewed {
				close(l.lost)
				l.cancel()
				return
			}
		}
	}
}

func (l *Lock) renew(ctx context.Context, token string) (bool, error) {
	result, err := l.client.Eval(
		ctx,
		renewScript,
		[]string{l.key},
		token,
		l.ttl.Milliseconds(),
	).Int()

	if err != nil {
		return false, err
	}

	return result == 1, nil
}

func (l *Lease) Context() context.Context {
	return l.ctx
}

func (l *Lease) Lost() <-chan struct{} {
	return l.lost
}

func (l *Lease) Release(ctx context.Context) error {
	l.releaseOnce.Do(func() {
		l.cancel()
		<-l.done

		releaseCtx, cancel := context.WithTimeout(
			ctx,
			operationTimeout,
		)
		defer cancel()

		result, err := l.lock.client.Eval(
			releaseCtx,
			releaseScript,
			[]string{l.lock.key},
			l.token,
		).Int()

		if err != nil {
			l.releaseErr = fmt.Errorf("release redis lock: %w", err)
			return
		}

		if result != 1 {
			l.releaseErr = ErrNotOwner
		}
	})

	return l.releaseErr
}

func newToken() (string, error) {
	var b [16]byte

	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}

	return hex.EncodeToString(b[:]), nil
}

// Do acquires the lock, executes fn while holding the lease,
// and releases the lock when fn returns.
func (l *Lock) Do(
	ctx context.Context,
	fn func(context.Context) error,
) (bool, error) {
	if fn == nil {
		return false, errors.New("lock callback is nil")
	}

	lease, acquired, err := l.Acquire(ctx)
	if err != nil || !acquired {
		return acquired, err
	}

	defer func() {
		_ = lease.Release(context.Background())
	}()

	workErr := fn(lease.Context())

	lost := false
	select {
	case <-lease.Lost():
		lost = true
	default:
	}

	releaseErr := lease.Release(context.Background())

	if ctx.Err() != nil {
		if workErr != nil {
			return true, workErr
		}
		return true, ctx.Err()
	}

	if lost || errors.Is(releaseErr, ErrNotOwner) {
		if workErr != nil {
			return true, fmt.Errorf("%w (work also failed: %v)", ErrLeaseLost, workErr)
		}
		return true, ErrLeaseLost
	}

	if workErr != nil {
		return true, workErr
	}

	if releaseErr != nil {
		return true, releaseErr
	}

	return true, nil
}

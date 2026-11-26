package confirm

import (
	"context"
	"errors"
	"testing"
	"time"
)

type testWithdrawalLocker struct {
	acquired      bool
	err           error
	calls         int
	callbackCalls int
	cancel        context.CancelFunc
}

func (l *testWithdrawalLocker) Do(
	ctx context.Context,
	fn func(context.Context) error,
) (bool, error) {
	l.calls++

	if l.err != nil {
		if l.cancel != nil {
			l.cancel()
		}
		return false, l.err
	}

	if !l.acquired {
		if l.cancel != nil {
			l.cancel()
		}
		return false, nil
	}

	l.callbackCalls++
	err := fn(ctx)

	if l.cancel != nil {
		l.cancel()
	}

	return true, err
}

func TestRunWithdrawalsRequiresLocker(t *testing.T) {
	err := RunWithdrawals(
		context.Background(),
		&fakeChain{head: 100},
		&fakeWithdrawals{},
		nil,
		12,
		time.Second,
	)

	if err == nil {
		t.Fatal("RunWithdrawals without locker should fail")
	}
}

func TestRunWithdrawalsProcessesInsideLock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	locker := &testWithdrawalLocker{
		acquired: true,
		cancel:   cancel,
	}

	err := RunWithdrawals(
		ctx,
		&fakeChain{head: 100},
		&fakeWithdrawals{},
		locker,
		12,
		time.Hour,
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunWithdrawals error = %v, want context.Canceled", err)
	}

	if locker.calls != 1 {
		t.Fatalf("lock calls = %d, want 1", locker.calls)
	}

	if locker.callbackCalls != 1 {
		t.Fatalf("callback calls = %d, want 1", locker.callbackCalls)
	}
}

func TestRunWithdrawalsSkipsWhenLockUnavailable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	locker := &testWithdrawalLocker{
		acquired: false,
		cancel:   cancel,
	}

	err := RunWithdrawals(
		ctx,
		&fakeChain{head: 100},
		&fakeWithdrawals{},
		locker,
		12,
		time.Hour,
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunWithdrawals error = %v, want context.Canceled", err)
	}

	if locker.calls != 1 {
		t.Fatalf("lock calls = %d, want 1", locker.calls)
	}

	if locker.callbackCalls != 0 {
		t.Fatalf("callback calls = %d, want 0", locker.callbackCalls)
	}
}

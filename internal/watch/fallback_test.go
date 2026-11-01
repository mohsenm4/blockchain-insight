package watch

import (
	"context"
	"errors"
	"testing"
)

// stubSource returns fixed transfers or a fixed error, and counts calls.
type stubSource struct {
	trs   []Transfer
	err   error
	calls int
}

func (s *stubSource) BlockNumber(ctx context.Context) (uint64, error) { return 0, s.err }

func (s *stubSource) Transfers(ctx context.Context, from, to uint64) ([]Transfer, error) {
	s.calls++
	return s.trs, s.err
}

func TestFallbackPrimaryOK(t *testing.T) {
	primary := &stubSource{trs: []Transfer{{}}}
	archive := &stubSource{}
	fallback := NewFallbackSource(primary, archive)

	res, err := fallback.Transfers(context.Background(), 0, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 transfer, got %d", len(res))
	}
	if primary.calls != 1 {
		t.Fatalf("expected primary to be called once, got %d", primary.calls)
	}
	if archive.calls != 0 {
		t.Fatalf("expected archive not to be called, got %d", archive.calls)
	}
}

func TestFallbackArchiveWhenPrimaryFails(t *testing.T) {
	primary := &stubSource{err: errors.New("primary error")}
	archive := &stubSource{trs: []Transfer{{}}}
	fallback := NewFallbackSource(primary, archive)

	res, err := fallback.Transfers(context.Background(), 0, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 transfer, got %d", len(res))
	}
	if primary.calls != 1 {
		t.Fatalf("expected primary to be called once, got %d", primary.calls)
	}
	if archive.calls != 1 {
		t.Fatalf("expected archive to be called once, got %d", archive.calls)
	}
}

func TestFallbackBothFail(t *testing.T) {
	errA := errors.New("primary error")
	errB := errors.New("archive error")

	primary := &stubSource{err: errA}
	archive := &stubSource{err: errB}
	fallback := NewFallbackSource(primary, archive)

	_, err := fallback.Transfers(context.Background(), 0, 10)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if primary.calls != 1 {
		t.Fatalf("expected primary to be called once, got %d", primary.calls)
	}
	if archive.calls != 1 {
		t.Fatalf("expected archive to be called once, got %d", archive.calls)
	}

	if !errors.Is(err, errA) {
		t.Fatalf("primary error lost: %v", err)
	}
	if !errors.Is(err, errB) {
		t.Fatalf("archive error lost: %v", err)
	}

}

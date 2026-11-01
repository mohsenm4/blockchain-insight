package watch

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// FallbackSource reads from primary, and asks archive when primary fails
// (for example: a public node that pruned old blocks).
type FallbackSource struct {
	primary, archive Source
}

func NewFallbackSource(primary, archive Source) FallbackSource {
	return FallbackSource{primary: primary, archive: archive}
}

func (s FallbackSource) BlockNumber(ctx context.Context) (uint64, error) {
	return s.primary.BlockNumber(ctx)
}

func (s FallbackSource) Transfers(ctx context.Context, from, to uint64) ([]Transfer, error) {
	res, err := s.primary.Transfers(ctx, from, to)
	if err == nil {
		return res, nil
	}
	slog.Warn("primary failed", "from", from, "to", to, "err", err)

	archiveRes, archiveErr := s.archive.Transfers(ctx, from, to)
	if archiveErr != nil {
		return nil, errors.Join(
			fmt.Errorf("primary: %w", err),
			fmt.Errorf("archive: %w", archiveErr),
		)
	}
	return archiveRes, nil
}

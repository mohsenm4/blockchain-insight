package watch

import (
	"context"
	"log/slog"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type Source interface {
	BlockNumber(ctx context.Context) (uint64, error)
	Transfers(ctx context.Context, from, to uint64) ([]Transfer, error)
}

type Transfer struct {
	Block     uint64
	BlockHash common.Hash // lets the ledger detect reorgs
	Index     uint
	From, To  common.Address
	Value     *big.Int
	Tx        common.Hash
}

// Batch is what Run found in blocks From..To (inclusive).
type Batch struct {
	From, To  uint64
	Transfers []Transfer
}

// Handler processes one batch. Run moves past a batch only when its handler
// returns nil; on error the same blocks are retried on the next poll.
type Handler func(ctx context.Context, b Batch) error

// maxBlockRange caps the blocks per Transfers call: RPC nodes reject large
// eth_getLogs ranges, which happens when catching up after downtime.
const maxBlockRange = 500

// Run polls src every interval and hands every block after last to handle,
// in order, until ctx is cancelled. It returns ctx.Err() on cancellation.
func Run(ctx context.Context, src Source, last uint64, interval time.Duration, handle Handler) error {
	for {
		head, err := src.BlockNumber(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.Warn("get block number", "err", err)
		} else if head > last {
			to := min(head, last+maxBlockRange)
			if err := process(ctx, src, last+1, to, handle); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				slog.Warn("process blocks", "from", last+1, "to", to, "err", err)
			} else {
				last = to
				if last < head {
					continue // still catching up: don't wait
				}
			}
		}

		select {
		case <-time.After(interval):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func process(ctx context.Context, src Source, from, to uint64, handle Handler) error {
	trs, err := src.Transfers(ctx, from, to)
	if err != nil {
		return err
	}
	return handle(ctx, Batch{From: from, To: to, Transfers: trs})
}

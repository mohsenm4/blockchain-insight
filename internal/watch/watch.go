package watch

import (
	"context"
	"log"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type Source interface {
	BlockNumber(ctx context.Context) (uint64, error)
	Transfers(ctx context.Context, from, to uint64) ([]Transfer, error)
}

type Transfer struct {
	Block    uint64
	Index    uint
	From, To common.Address
	Value    *big.Int
	Tx       common.Hash
}

// Run polls src every interval and sends new transfers to out until ctx is
// cancelled. It returns ctx.Err() on cancellation. Run never closes out;
// the owner of the channel does.
func Run(ctx context.Context, src Source, interval time.Duration, out chan<- Transfer) error {
	last, err := src.BlockNumber(ctx)
	if err != nil {
		return err
	}

	for {
		head, err := src.BlockNumber(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Println("error getting block number:", err)
		} else if head > last {
			trs, err := src.Transfers(ctx, last+1, head)
			if err != nil {
				log.Println("error filtering transfers:", err)
			} else {
				for _, tr := range trs {
					select {
					case out <- tr:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				last = head
			}
		}

		select {
		case <-time.After(interval):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

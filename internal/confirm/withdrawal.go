package confirm

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/mohsenm4/blockchain-insight/internal/ledger"
)

// WithdrawalStore is the part of ledger.Ledger the withdrawal tracker needs.
type WithdrawalStore interface {
	BroadcastWithdrawals(ctx context.Context) ([]ledger.Withdrawal, error)
	MarkWithdrawalConfirmed(ctx context.Context, id int64, txHash string) error
	MarkBroadcastFailed(ctx context.Context, id int64, txHash string) error
}

// RunWithdrawals calls StepWithdrawals every interval until ctx is
// cancelled. It returns ctx.Err() on cancellation.
func RunWithdrawals(ctx context.Context, chain Chain, store WithdrawalStore, confirmations uint64, interval time.Duration) error {
	for {
		if _, err := StepWithdrawals(ctx, chain, store, confirmations); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.Warn("confirm withdrawals", "err", err)
		}

		select {
		case <-time.After(interval):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// StepWithdrawals asks the node for the receipt of every broadcast
// withdrawal. No receipt: the tx is in no block yet, so it waits. A receipt
// with confirmations blocks on top of it (head − block >= confirmations)
// settles the withdrawal: status 1 confirms it, status 0 fails it and gives
// the amount back (the tokens did not move).
//
// A failed tx waits for confirmations too: after a reorg the same tx can
// land again and succeed, and then the amount would be paid twice.
func StepWithdrawals(ctx context.Context, chain Chain, store WithdrawalStore, confirmations uint64) (settled int, err error) {
	head, err := chain.BlockNumber(ctx)
	if err != nil {
		return 0, err
	}
	ws, err := store.BroadcastWithdrawals(ctx)
	if err != nil {
		return 0, err
	}

	for _, w := range ws {
		r, err := chain.Receipt(ctx, common.HexToHash(w.TxHash))
		if errors.Is(err, ethereum.NotFound) {
			continue // not in a block yet
		}
		if err != nil {
			return settled, err
		}
		block := r.BlockNumber.Uint64()
		if head < block || head-block < confirmations {
			continue // not deep enough
		}

		if r.Status == types.ReceiptStatusSuccessful {
			err = store.MarkWithdrawalConfirmed(ctx, w.ID, w.TxHash)
		} else {
			err = store.MarkBroadcastFailed(ctx, w.ID, w.TxHash)
		}
		if err != nil {
			return settled, err
		}
		settled++
		slog.Info("withdrawal settled", "withdrawal", w.ID, "tx", w.TxHash, "block", block,
			"success", r.Status == types.ReceiptStatusSuccessful)
	}
	return settled, nil
}

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

// ReversalStore is the part of ledger.Ledger the reversal tracker needs.
type ReversalStore interface {
	CreditedDeposits(ctx context.Context, minBlock uint64) ([]ledger.Deposit, error)
	ReverseDeposit(ctx context.Context, id int64, blockHash, reason string) (status string, err error)
}

// RunReversals calls StepReversals every interval until ctx is cancelled.
// It returns ctx.Err() on cancellation.
func RunReversals(ctx context.Context, chain Chain, store ReversalStore, finality uint64, interval time.Duration) error {
	for {
		if _, err := StepReversals(ctx, chain, store, finality); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.Warn("check credited deposits", "err", err)
		}

		select {
		case <-time.After(interval):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// StepReversals checks again every credited deposit in the last finality
// blocks, which a reorg can still remove. A deposit whose block hash changed
// is looked up by its tx receipt, like in reorg: if the transfer is in the
// new block, the money did arrive and nothing changes. If the tx is in no
// block, failed, or the transfer is not there, the credit is reversed.
//
// A deposit in a replaced block whose tx moved is checked by its receipt on
// every Step, so a second reorg that removes the new block reverses it too.
func StepReversals(ctx context.Context, chain Chain, store ReversalStore, finality uint64) (reversed int, err error) {
	head, err := chain.BlockNumber(ctx)
	if err != nil {
		return 0, err
	}
	var minBlock uint64
	if head > finality {
		minBlock = head - finality
	}
	deps, err := store.CreditedDeposits(ctx, minBlock)
	if err != nil {
		return 0, err
	}

	hashes := map[uint64]common.Hash{}
	var replaced []ledger.Deposit
	for _, d := range deps {
		h, ok := hashes[d.BlockNumber]
		if !ok {
			if h, err = chain.BlockHash(ctx, d.BlockNumber); err != nil {
				return reversed, err
			}
			hashes[d.BlockNumber] = h
		}
		if h.Hex() != d.BlockHash {
			replaced = append(replaced, d)
		}
	}

	for _, txDeps := range byTx(replaced) {
		n, err := reverseGone(ctx, chain, store, txDeps)
		reversed += n
		if err != nil {
			return reversed, err
		}
	}
	return reversed, nil
}

// reverseGone reverses the credited deposits of one tx, in a replaced block,
// whose transfer is no longer on the chain.
func reverseGone(ctx context.Context, chain Chain, store ReversalStore, deps []ledger.Deposit) (reversed int, err error) {
	old := deps[0]
	r, err := chain.Receipt(ctx, common.HexToHash(old.TxHash))

	var moved []ledger.Deposit
	reason := "tx is in no block"
	switch {
	case errors.Is(err, ethereum.NotFound):
	case err != nil:
		return 0, err
	case r.BlockHash.Hex() == old.BlockHash:
		return 0, nil // this node still has the old block: ask again next Step
	case r.Status != types.ReceiptStatusSuccessful:
		reason = "tx failed in its new block"
	default:
		moved = follow(deps, r)
		reason = "transfer is not in the new block"
	}

	arrived := map[int64]bool{} // follow keeps each deposit's ID
	for _, m := range moved {
		arrived[m.ID] = true
	}
	for _, d := range deps {
		if arrived[d.ID] {
			continue // the money did arrive, in the new block; the watcher won't record it again (see ledger.InsertDeposit)
		}
		status, err := store.ReverseDeposit(ctx, d.ID, d.BlockHash, reason)
		if err != nil {
			return reversed, err
		}
		reversed++
		if status == "needs_review" {
			slog.Error("credited deposit reverted, balance too low to take it back: needs review",
				"deposit", d.ID, "user", d.UserID, "tx", d.TxHash, "amount", d.Amount, "reason", reason)
			continue
		}
		slog.Warn("credited deposit reversed", "deposit", d.ID, "user", d.UserID,
			"tx", d.TxHash, "block", d.BlockNumber, "reason", reason)
	}
	return reversed, nil
}

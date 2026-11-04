// Package confirm moves deposits from pending to confirmed once they are
// deep enough in the chain.
package confirm

import (
	"context"
	"log/slog"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/mohsenm4/blockchain-insight/internal/ledger"
)

// Chain is what the tracker reads from the node.
type Chain interface {
	BlockNumber(ctx context.Context) (uint64, error)
	BlockHash(ctx context.Context, number uint64) (common.Hash, error)
}

// Store is the part of ledger.Ledger the tracker needs.
type Store interface {
	PendingDeposits(ctx context.Context, maxBlock uint64) ([]ledger.Deposit, error)
	MarkConfirmed(ctx context.Context, id int64, blockHash string) error
	ConfirmedDeposits(ctx context.Context) ([]ledger.Deposit, error)
	MarkCredited(ctx context.Context, id int64, head uint64) error
}

// Run calls Step every interval until ctx is cancelled. It returns
// ctx.Err() on cancellation.
func Run(ctx context.Context, chain Chain, store Store, confirmations uint64, interval time.Duration) error {
	for {
		if _, err := Step(ctx, chain, store, confirmations); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.Warn("confirm deposits", "err", err)
		}

		select {
		case <-time.After(interval):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Step first credits every confirmed deposit, then confirms every pending
// deposit with at least confirmations blocks on top of it
// (head − block_number >= confirmations) whose block is still on the chain
// with the same hash. A deposit whose block hash changed stays pending: the
// block was replaced (reorg).
//
// Crediting first means a deposit confirmed now is credited on the next
// Step, and one left confirmed by a crash is never stuck.
func Step(ctx context.Context, chain Chain, store Store, confirmations uint64) (confirmed int, err error) {
	head, err := chain.BlockNumber(ctx)
	if err != nil {
		return 0, err
	}
	if err := credit(ctx, store, head); err != nil {
		return 0, err
	}
	if head < confirmations {
		return 0, nil // young chain: nothing can be deep enough
	}

	deps, err := store.PendingDeposits(ctx, head-confirmations)
	if err != nil {
		return 0, err
	}

	// Many deposits share a block: ask the node once per block.
	hashes := map[uint64]common.Hash{}
	for _, d := range deps {
		h, ok := hashes[d.BlockNumber]
		if !ok {
			if h, err = chain.BlockHash(ctx, d.BlockNumber); err != nil {
				return confirmed, err
			}
			hashes[d.BlockNumber] = h
		}
		if h.Hex() != d.BlockHash {
			slog.Warn("deposit block was replaced", "deposit", d.ID, "tx", d.TxHash,
				"block", d.BlockNumber, "saved_hash", d.BlockHash, "chain_hash", h.Hex())
			continue
		}
		if err := store.MarkConfirmed(ctx, d.ID, d.BlockHash); err != nil {
			return confirmed, err
		}
		confirmed++
		slog.Info("deposit confirmed", "deposit", d.ID, "tx", d.TxHash, "block", d.BlockNumber)
	}
	return confirmed, nil
}

// credit adds every confirmed deposit to its user's balance. head is saved
// as credited_at_block.
func credit(ctx context.Context, store Store, head uint64) error {
	deps, err := store.ConfirmedDeposits(ctx)
	if err != nil {
		return err
	}
	for _, d := range deps {
		if err := store.MarkCredited(ctx, d.ID, head); err != nil {
			return err
		}
		slog.Info("deposit credited", "deposit", d.ID, "tx", d.TxHash, "block", d.BlockNumber)
	}
	return nil
}

// EthChain adapts an ethclient.Client to Chain.
type EthChain struct {
	client *ethclient.Client
}

func NewEthChain(client *ethclient.Client) EthChain {
	return EthChain{client: client}
}

// BlockNumber is the real head, not the lagged one the watcher uses.
func (c EthChain) BlockNumber(ctx context.Context) (uint64, error) {
	return c.client.BlockNumber(ctx)
}

func (c EthChain) BlockHash(ctx context.Context, number uint64) (common.Hash, error) {
	h, err := c.client.HeaderByNumber(ctx, new(big.Int).SetUint64(number))
	if err != nil {
		return common.Hash{}, err
	}
	return h.Hash(), nil
}

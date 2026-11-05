// Package confirm moves deposits from pending to confirmed once they are
// deep enough in the chain, and follows them when their block is replaced.
package confirm

import (
	"context"
	"errors"
	"log/slog"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/mohsenm4/blockchain-insight/internal/ledger"
)

// Chain is what the tracker reads from the node.
type Chain interface {
	BlockNumber(ctx context.Context) (uint64, error)
	BlockHash(ctx context.Context, number uint64) (common.Hash, error)
	// Receipt returns ethereum.NotFound when the tx is in no block.
	Receipt(ctx context.Context, tx common.Hash) (*types.Receipt, error)
}

// Store is the part of ledger.Ledger the tracker needs.
type Store interface {
	PendingDeposits(ctx context.Context, maxBlock uint64) ([]ledger.Deposit, error)
	MarkConfirmed(ctx context.Context, id int64, blockHash string) error
	ConfirmedDeposits(ctx context.Context) ([]ledger.Deposit, error)
	MarkCredited(ctx context.Context, id int64, head uint64) error
	RevertDeposits(ctx context.Context, txHash, blockHash string, moved []ledger.Deposit) error
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
// with the same hash. A deposit whose block hash changed is in a block that
// was replaced (reorg): see reorg.
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
	var replaced []ledger.Deposit
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
			replaced = append(replaced, d)
			continue
		}
		if err := store.MarkConfirmed(ctx, d.ID, d.BlockHash); err != nil {
			return confirmed, err
		}
		confirmed++
		slog.Info("deposit confirmed", "deposit", d.ID, "tx", d.TxHash, "block", d.BlockNumber)
	}

	// The deposits of one tx move together: their log indexes can shift
	// in the new block.
	for _, txDeps := range byTx(replaced) {
		if err := reorg(ctx, chain, store, txDeps); err != nil {
			return confirmed, err
		}
	}
	return confirmed, nil
}

// transferTopic is topic 0 of an ERC20 Transfer log.
var transferTopic = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))

// reorg handles the pending deposits of one tx whose block was replaced.
// It asks the node where the tx is now. In another block, the deposits are
// recorded there and count their confirmations again from that block. In
// no block, or failed there, they are reverted; if the tx comes back later,
// the watcher records it again.
func reorg(ctx context.Context, chain Chain, store Store, deps []ledger.Deposit) error {
	old := deps[0]
	r, err := chain.Receipt(ctx, common.HexToHash(old.TxHash))

	var moved []ledger.Deposit
	reason := "tx is in no block"
	switch {
	case errors.Is(err, ethereum.NotFound):
		// maybe back in the mempool
	case err != nil:
		return err
	case r.BlockHash.Hex() == old.BlockHash:
		return nil // this node still has the old block: ask again next Step
	case r.Status != types.ReceiptStatusSuccessful:
		reason = "tx failed in its new block"
	default:
		moved = follow(deps, r)
		reason = "transfer is not in the new block"
	}

	if err := store.RevertDeposits(ctx, old.TxHash, old.BlockHash, moved); err != nil {
		return err
	}
	if len(moved) > 0 {
		slog.Warn("deposit moved to a new block", "tx", old.TxHash,
			"from", old.BlockNumber, "to", moved[0].BlockNumber, "deposits", len(moved))
	}
	if len(moved) < len(deps) {
		slog.Warn("deposit reverted", "tx", old.TxHash, "block", old.BlockNumber,
			"deposits", len(deps)-len(moved), "reason", reason)
	}
	return nil
}

// follow finds the transfer of each deposit in r, the receipt of their tx in
// its new block, and returns the deposits as they are there. A deposit whose
// transfer is not in r is left out. Equal transfers in one tx are matched in
// order, each log once.
func follow(deps []ledger.Deposit, r *types.Receipt) []ledger.Deposit {
	used := map[uint]bool{}
	var moved []ledger.Deposit
	for _, d := range deps {
		for _, lg := range r.Logs {
			if used[lg.Index] || !isTransfer(lg, d) {
				continue
			}
			used[lg.Index] = true
			d.BlockNumber = r.BlockNumber.Uint64()
			d.BlockHash = r.BlockHash.Hex()
			d.LogIndex = lg.Index
			moved = append(moved, d)
			break
		}
	}
	return moved
}

// isTransfer reports whether lg is the ERC20 Transfer that d records.
func isTransfer(lg *types.Log, d ledger.Deposit) bool {
	return len(lg.Topics) == 3 && lg.Topics[0] == transferTopic &&
		ledger.AddressKey(lg.Address) == d.Token &&
		ledger.AddressKey(common.BytesToAddress(lg.Topics[2].Bytes())) == d.Address &&
		new(big.Int).SetBytes(lg.Data).Cmp(d.Amount) == 0
}

// byTx groups deps by tx hash, in the order each tx first appears.
func byTx(deps []ledger.Deposit) [][]ledger.Deposit {
	var groups [][]ledger.Deposit
	at := map[string]int{}
	for _, d := range deps {
		i, ok := at[d.TxHash]
		if !ok {
			i = len(groups)
			at[d.TxHash] = i
			groups = append(groups, nil)
		}
		groups[i] = append(groups[i], d)
	}
	return groups
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

func (c EthChain) Receipt(ctx context.Context, tx common.Hash) (*types.Receipt, error) {
	return c.client.TransactionReceipt(ctx, tx)
}

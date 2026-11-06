package confirm

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/mohsenm4/blockchain-insight/internal/ledger"
)

// fakeReversals keeps credited deposits and their reversals in memory.
type fakeReversals struct {
	deps     []ledger.Deposit
	minBlock uint64           // last CreditedDeposits argument
	reversed map[int64]string // deposit ID → reason
}

func (f *fakeReversals) CreditedDeposits(ctx context.Context, minBlock uint64) ([]ledger.Deposit, error) {
	f.minBlock = minBlock
	var out []ledger.Deposit
	for _, d := range f.deps {
		if _, ok := f.reversed[d.ID]; !ok && d.BlockNumber >= minBlock {
			out = append(out, d)
		}
	}
	return out, nil
}

func (f *fakeReversals) ReverseDeposit(ctx context.Context, id int64, blockHash, reason string) (string, error) {
	if _, ok := f.reversed[id]; ok {
		return "", ledger.ErrNotCredited
	}
	f.reversed[id] = reason
	return "reversed", nil
}

func credited(id int64, logIndex uint, amount int64) ledger.Deposit {
	d := transfer(id, hashA, logIndex, amount)
	d.Status = "credited"
	return d
}

func TestStepReversals(t *testing.T) {
	for _, tc := range []struct {
		name     string
		hash     common.Hash // block 1000 on the chain now
		receipts map[common.Hash]*types.Receipt
		want     map[int64]string // reversed deposits
	}{
		{"same block", hashA, nil, map[int64]string{}},
		{"tx in no block", hashB, nil, map[int64]string{1: "tx is in no block", 2: "tx is in no block"}},
		{"tx failed", hashB, receipt(1001, hashC, types.ReceiptStatusFailed),
			map[int64]string{1: "tx failed in its new block", 2: "tx failed in its new block"}},
		{"both moved", hashB, receipt(1001, hashC, types.ReceiptStatusSuccessful, transferLog(3, 10), transferLog(4, 20)),
			map[int64]string{}},
		{"one moved", hashB, receipt(1001, hashC, types.ReceiptStatusSuccessful, transferLog(3, 20)),
			map[int64]string{1: "transfer is not in the new block"}},
		{"node has old block", hashB, receipt(1000, hashA, types.ReceiptStatusSuccessful), map[int64]string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chain := &fakeChain{head: 1050, hashes: map[uint64]common.Hash{1000: tc.hash},
				asked: map[uint64]int{}, receipts: tc.receipts}
			store := &fakeReversals{deps: []ledger.Deposit{credited(1, 0, 10), credited(2, 1, 20)},
				reversed: map[int64]string{}}

			n, err := StepReversals(context.Background(), chain, store, 64)
			if err != nil {
				t.Fatal(err)
			}
			if n != len(tc.want) || len(store.reversed) != len(tc.want) {
				t.Fatalf("reversed = %d (%v), want %v", n, store.reversed, tc.want)
			}
			for id, reason := range tc.want {
				if store.reversed[id] != reason {
					t.Errorf("deposit %d: reason = %q, want %q", id, store.reversed[id], reason)
				}
			}
			if chain.asked[1000] != 1 {
				t.Errorf("block 1000 asked %d times, want 1", chain.asked[1000])
			}
		})
	}
}

// Only the last finality blocks are checked.
func TestStepReversalsWindow(t *testing.T) {
	chain := &fakeChain{head: 1100, asked: map[uint64]int{}}
	store := &fakeReversals{reversed: map[int64]string{}}
	if _, err := StepReversals(context.Background(), chain, store, 64); err != nil {
		t.Fatal(err)
	}
	if store.minBlock != 1036 {
		t.Fatalf("minBlock = %d, want 1036", store.minBlock)
	}

	chain.head = 10 // young chain
	if _, err := StepReversals(context.Background(), chain, store, 64); err != nil {
		t.Fatal(err)
	}
	if store.minBlock != 0 {
		t.Fatalf("young chain: minBlock = %d, want 0", store.minBlock)
	}
}

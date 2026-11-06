package confirm

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/mohsenm4/blockchain-insight/internal/ledger"
)

// fakeWithdrawals keeps withdrawal statuses in memory, by tx hash.
type fakeWithdrawals struct {
	ws []ledger.Withdrawal
}

func (f *fakeWithdrawals) BroadcastWithdrawals(ctx context.Context) ([]ledger.Withdrawal, error) {
	var out []ledger.Withdrawal
	for _, w := range f.ws {
		if w.Status == "broadcast" {
			out = append(out, w)
		}
	}
	return out, nil
}

func (f *fakeWithdrawals) set(id int64, txHash, status string) error {
	for i := range f.ws {
		w := &f.ws[i]
		if w.ID == id && w.TxHash == txHash && w.Status == "broadcast" {
			w.Status = status
			return nil
		}
	}
	return ledger.ErrNotBroadcast
}

func (f *fakeWithdrawals) MarkWithdrawalConfirmed(ctx context.Context, id int64, txHash string) error {
	return f.set(id, txHash, "confirmed")
}

func (f *fakeWithdrawals) MarkBroadcastFailed(ctx context.Context, id int64, txHash string) error {
	return f.set(id, txHash, "failed")
}

func TestStepWithdrawals(t *testing.T) {
	txOK := common.HexToHash("0x1")      // succeeded, deep enough
	txFail := common.HexToHash("0x2")    // failed, deep enough
	txShallow := common.HexToHash("0x3") // succeeded, 11 blocks on top
	txMempool := common.HexToHash("0x4") // no receipt yet

	store := &fakeWithdrawals{ws: []ledger.Withdrawal{
		{ID: 1, TxHash: txOK.Hex(), Status: "broadcast"},
		{ID: 2, TxHash: txFail.Hex(), Status: "broadcast"},
		{ID: 3, TxHash: txShallow.Hex(), Status: "broadcast"},
		{ID: 4, TxHash: txMempool.Hex(), Status: "broadcast"},
	}}
	r := func(block, status uint64) *types.Receipt {
		return &types.Receipt{BlockNumber: new(big.Int).SetUint64(block), Status: status}
	}
	chain := &fakeChain{head: 100, receipts: map[common.Hash]*types.Receipt{
		txOK:      r(88, types.ReceiptStatusSuccessful),
		txFail:    r(80, types.ReceiptStatusFailed),
		txShallow: r(89, types.ReceiptStatusSuccessful),
	}}

	settled, err := StepWithdrawals(context.Background(), chain, store, 12)
	if err != nil {
		t.Fatal(err)
	}
	if settled != 2 {
		t.Fatalf("settled = %d, want 2", settled)
	}
	want := []string{"confirmed", "failed", "broadcast", "broadcast"}
	for i, w := range store.ws {
		if w.Status != want[i] {
			t.Errorf("withdrawal %d: status = %s, want %s", w.ID, w.Status, want[i])
		}
	}

	// A second Step changes nothing that is settled.
	if settled, err = StepWithdrawals(context.Background(), chain, store, 12); err != nil || settled != 0 {
		t.Fatalf("2nd step: settled = %d, err = %v; want 0, nil", settled, err)
	}
}

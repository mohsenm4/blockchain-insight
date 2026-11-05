package confirm

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/mohsenm4/blockchain-insight/internal/ledger"
)

type fakeChain struct {
	head     uint64
	hashes   map[uint64]common.Hash
	asked    map[uint64]int // BlockHash calls per block
	receipts map[common.Hash]*types.Receipt
}

func (f *fakeChain) BlockNumber(ctx context.Context) (uint64, error) { return f.head, nil }

func (f *fakeChain) BlockHash(ctx context.Context, number uint64) (common.Hash, error) {
	f.asked[number]++
	return f.hashes[number], nil
}

func (f *fakeChain) Receipt(ctx context.Context, tx common.Hash) (*types.Receipt, error) {
	if r, ok := f.receipts[tx]; ok {
		return r, nil
	}
	return nil, ethereum.NotFound
}

// fakeStore keeps deposits in memory and filters like the SQL does.
type fakeStore struct {
	deps     []ledger.Deposit
	maxBlock uint64 // last PendingDeposits argument
}

func (f *fakeStore) PendingDeposits(ctx context.Context, maxBlock uint64) ([]ledger.Deposit, error) {
	f.maxBlock = maxBlock
	var out []ledger.Deposit
	for _, d := range f.deps {
		if d.Status == "pending" && d.BlockNumber <= maxBlock {
			out = append(out, d)
		}
	}
	return out, nil
}

func (f *fakeStore) MarkConfirmed(ctx context.Context, id int64, blockHash string) error {
	for i := range f.deps {
		d := &f.deps[i]
		if d.ID == id && d.Status == "pending" && d.BlockHash == blockHash {
			d.Status = "confirmed"
			return nil
		}
	}
	return ledger.ErrNotPending
}

func (f *fakeStore) ConfirmedDeposits(ctx context.Context) ([]ledger.Deposit, error) {
	var out []ledger.Deposit
	for _, d := range f.deps {
		if d.Status == "confirmed" {
			out = append(out, d)
		}
	}
	return out, nil
}

func (f *fakeStore) MarkCredited(ctx context.Context, id int64, head uint64) error {
	for i := range f.deps {
		d := &f.deps[i]
		if d.ID == id && d.Status == "confirmed" {
			d.Status = "credited"
			return nil
		}
	}
	return ledger.ErrNotConfirmed
}

// RevertDeposits does what the SQL does, also using a reverted row again.
func (f *fakeStore) RevertDeposits(ctx context.Context, txHash, blockHash string, moved []ledger.Deposit) error {
	reverted := 0
	for i := range f.deps {
		d := &f.deps[i]
		if d.TxHash == txHash && d.BlockHash == blockHash && d.Status == "pending" {
			d.Status = "reverted"
			reverted++
		}
	}
	if reverted == 0 {
		return ledger.ErrNotPending
	}
next:
	for _, m := range moved {
		m.Status = "pending"
		for i := range f.deps {
			d := &f.deps[i]
			if d.TxHash == m.TxHash && d.LogIndex == m.LogIndex {
				if d.Status == "reverted" {
					m.ID = d.ID
					*d = m
				}
				continue next
			}
		}
		m.ID = int64(len(f.deps) + 1)
		f.deps = append(f.deps, m)
	}
	return nil
}

var (
	hashA = common.HexToHash("0xa")
	hashB = common.HexToHash("0xb")
	hashC = common.HexToHash("0xc")

	token = common.HexToAddress("0x1c7d4b196cb0c7b01d743fbc6116a902379c7238")
	user  = common.HexToAddress("0xdead")
	txA   = common.HexToHash("0xaa")
)

func pending(id int64, block uint64, hash common.Hash) ledger.Deposit {
	return ledger.Deposit{ID: id, BlockNumber: block, BlockHash: hash.Hex(), Status: "pending"}
}

func TestStepConfirmsOnlyDeepEnough(t *testing.T) {
	chain := &fakeChain{head: 1012, hashes: map[uint64]common.Hash{1000: hashA, 1001: hashA}, asked: map[uint64]int{}}
	store := &fakeStore{deps: []ledger.Deposit{
		pending(1, 1000, hashA), // 1012 − 1000 = 12 → confirmed
		pending(2, 1001, hashA), // 11 → still pending
	}}

	n, err := Step(context.Background(), chain, store, 12)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || store.maxBlock != 1000 {
		t.Fatalf("confirmed = %d, maxBlock = %d; want 1, 1000", n, store.maxBlock)
	}
	if store.deps[0].Status != "confirmed" || store.deps[1].Status != "pending" {
		t.Fatalf("statuses = %s, %s; want confirmed, pending", store.deps[0].Status, store.deps[1].Status)
	}
}

// transfer is a pending deposit of amount tokens to user in tx txA, block 1000.
func transfer(id int64, hash common.Hash, logIndex uint, amount int64) ledger.Deposit {
	d := pending(id, 1000, hash)
	d.TxHash, d.LogIndex = txA.Hex(), logIndex
	d.Address, d.Token = ledger.AddressKey(user), ledger.AddressKey(token)
	d.Amount = big.NewInt(amount)
	return d
}

// transferLog is the Transfer log of amount tokens to user.
func transferLog(index uint, amount int64) *types.Log {
	return &types.Log{
		Address: token,
		Topics:  []common.Hash{transferTopic, {}, common.BytesToHash(user.Bytes())},
		Data:    common.LeftPadBytes(big.NewInt(amount).Bytes(), 32),
		Index:   index,
	}
}

func receipt(block uint64, hash common.Hash, status uint64, logs ...*types.Log) map[common.Hash]*types.Receipt {
	return map[common.Hash]*types.Receipt{txA: {
		Status: status, BlockNumber: new(big.Int).SetUint64(block), BlockHash: hash, Logs: logs,
	}}
}

func statuses(store *fakeStore) []string {
	var out []string
	for _, d := range store.deps {
		out = append(out, d.Status)
	}
	return out
}

// Block 1000 was replaced and the tx is now in block 1003: the deposit
// follows it there, and is confirmed once 1003 is deep enough.
func TestStepMovesReplacedDeposit(t *testing.T) {
	chain := &fakeChain{head: 2000, hashes: map[uint64]common.Hash{1000: hashB, 1003: hashC}, asked: map[uint64]int{},
		receipts: receipt(1003, hashC, types.ReceiptStatusSuccessful, transferLog(7, 1000))}
	store := &fakeStore{deps: []ledger.Deposit{transfer(1, hashA, 3, 1000)}}

	n, err := Step(context.Background(), chain, store, 12)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 || len(store.deps) != 2 {
		t.Fatalf("confirmed = %d, rows = %d; want 0, 2", n, len(store.deps))
	}
	moved := store.deps[1]
	if store.deps[0].Status != "reverted" || moved.Status != "pending" ||
		moved.BlockNumber != 1003 || moved.BlockHash != hashC.Hex() || moved.LogIndex != 7 {
		t.Fatalf("after reorg: old %s, new %s in %d/%s log %d; want reverted, pending in 1003/%s log 7",
			store.deps[0].Status, moved.Status, moved.BlockNumber, moved.BlockHash, moved.LogIndex, hashC.Hex())
	}

	if n, err = Step(context.Background(), chain, store, 12); err != nil || n != 1 {
		t.Fatalf("next step: confirmed = %d, err = %v; want 1, nil", n, err)
	}
}

// The tx has two equal transfers, and in the new block every log index is
// one higher: the first transfer gets the old index of the second one.
func TestStepMovesTwoTransfersOfOneTx(t *testing.T) {
	chain := &fakeChain{head: 2000, hashes: map[uint64]common.Hash{1000: hashB}, asked: map[uint64]int{},
		receipts: receipt(1000, hashB, types.ReceiptStatusSuccessful, transferLog(4, 500), transferLog(5, 500))}
	store := &fakeStore{deps: []ledger.Deposit{transfer(1, hashA, 3, 500), transfer(2, hashA, 4, 500)}}

	if _, err := Step(context.Background(), chain, store, 12); err != nil {
		t.Fatal(err)
	}
	got := map[uint]string{}
	for _, d := range store.deps {
		got[d.LogIndex] = d.Status + " " + d.BlockHash
	}
	pendingB := "pending " + hashB.Hex()
	if len(got) != 3 || got[3] != "reverted "+hashA.Hex() || got[4] != pendingB || got[5] != pendingB {
		t.Fatalf("rows by log index = %v; want 3 reverted, 4 and 5 pending in the new block", got)
	}
}

func TestStepRevertsReplacedDeposit(t *testing.T) {
	for _, tc := range []struct {
		name     string
		receipts map[common.Hash]*types.Receipt
	}{
		{"tx in no block", nil},
		{"tx failed", receipt(1003, hashC, types.ReceiptStatusFailed)},
		{"transfer not in the tx", receipt(1003, hashC, types.ReceiptStatusSuccessful, transferLog(7, 999))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chain := &fakeChain{head: 2000, hashes: map[uint64]common.Hash{1000: hashB}, asked: map[uint64]int{},
				receipts: tc.receipts}
			store := &fakeStore{deps: []ledger.Deposit{transfer(1, hashA, 3, 1000)}}

			if _, err := Step(context.Background(), chain, store, 12); err != nil {
				t.Fatal(err)
			}
			if got := statuses(store); len(got) != 1 || got[0] != "reverted" {
				t.Fatalf("statuses = %v; want [reverted]", got)
			}
		})
	}
}

// The header came from a node with the new block, the receipt from one
// that still has the old block: wait for the next Step.
func TestStepWaitsWhenNodesDisagree(t *testing.T) {
	chain := &fakeChain{head: 2000, hashes: map[uint64]common.Hash{1000: hashB}, asked: map[uint64]int{},
		receipts: receipt(1000, hashA, types.ReceiptStatusSuccessful, transferLog(3, 1000))}
	store := &fakeStore{deps: []ledger.Deposit{transfer(1, hashA, 3, 1000)}}

	n, err := Step(context.Background(), chain, store, 12)
	if err != nil {
		t.Fatal(err)
	}
	if got := statuses(store); n != 0 || len(got) != 1 || got[0] != "pending" {
		t.Fatalf("confirmed = %d, statuses = %v; want 0, [pending]", n, got)
	}
}

func TestStepAsksOncePerBlock(t *testing.T) {
	chain := &fakeChain{head: 2000, hashes: map[uint64]common.Hash{1000: hashA}, asked: map[uint64]int{}}
	store := &fakeStore{deps: []ledger.Deposit{pending(1, 1000, hashA), pending(2, 1000, hashA)}}

	n, err := Step(context.Background(), chain, store, 12)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 || chain.asked[1000] != 1 {
		t.Fatalf("confirmed = %d, BlockHash calls = %d; want 2, 1", n, chain.asked[1000])
	}
}

func TestStepYoungChain(t *testing.T) {
	chain := &fakeChain{head: 5, asked: map[uint64]int{}}
	store := &fakeStore{deps: []ledger.Deposit{pending(1, 0, hashA)}}

	n, err := Step(context.Background(), chain, store, 12)
	if err != nil || n != 0 {
		t.Fatalf("confirmed = %d, err = %v; want 0, nil", n, err)
	}
}

// A deposit left confirmed (for example by a crash after MarkConfirmed) is
// credited by the next Step; one confirmed in this Step waits for the next.
func TestStepCreditsConfirmed(t *testing.T) {
	chain := &fakeChain{head: 2000, hashes: map[uint64]common.Hash{1000: hashA}, asked: map[uint64]int{}}
	left := pending(1, 900, hashA)
	left.Status = "confirmed"
	store := &fakeStore{deps: []ledger.Deposit{left, pending(2, 1000, hashA)}}

	if _, err := Step(context.Background(), chain, store, 12); err != nil {
		t.Fatal(err)
	}
	if store.deps[0].Status != "credited" || store.deps[1].Status != "confirmed" {
		t.Fatalf("after step 1: %s, %s; want credited, confirmed", store.deps[0].Status, store.deps[1].Status)
	}

	if _, err := Step(context.Background(), chain, store, 12); err != nil {
		t.Fatal(err)
	}
	if store.deps[1].Status != "credited" {
		t.Fatalf("after step 2: %s; want credited", store.deps[1].Status)
	}
}

package confirm

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/mohsenm4/blockchain-insight/internal/ledger"
)

type fakeChain struct {
	head   uint64
	hashes map[uint64]common.Hash
	asked  map[uint64]int // BlockHash calls per block
}

func (f *fakeChain) BlockNumber(ctx context.Context) (uint64, error) { return f.head, nil }

func (f *fakeChain) BlockHash(ctx context.Context, number uint64) (common.Hash, error) {
	f.asked[number]++
	return f.hashes[number], nil
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

var (
	hashA = common.HexToHash("0xa")
	hashB = common.HexToHash("0xb")
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

func TestStepKeepsReplacedBlockPending(t *testing.T) {
	chain := &fakeChain{head: 2000, hashes: map[uint64]common.Hash{1000: hashB}, asked: map[uint64]int{}}
	store := &fakeStore{deps: []ledger.Deposit{pending(1, 1000, hashA)}}

	n, err := Step(context.Background(), chain, store, 12)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 || store.deps[0].Status != "pending" {
		t.Fatalf("confirmed = %d, status = %s; want 0, pending", n, store.deps[0].Status)
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

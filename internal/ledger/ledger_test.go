package ledger

import (
	"context"
	"math/big"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool connects to TEST_DATABASE_URL, or skips the test when it is unset.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set") // CI has no postgres
	}

	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestInsertDepositTwice(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)

	l := New(pool)
	d := Deposit{
		UserID: 1, Address: "0xabc", TxHash: "0xtest-insert-twice", LogIndex: 0,
		Token: "USDC", Amount: big.NewInt(1_000_000), BlockNumber: 100, BlockHash: "0xblock",
	}

	// Remove leftovers from a failed run, and clean up after this one.
	// Cleanups run LIFO, so this delete runs before pool.Close.
	deleteRow := func() {
		if _, err := pool.Exec(ctx, `DELETE FROM deposits WHERE tx_hash = $1`, d.TxHash); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	}
	deleteRow()
	t.Cleanup(deleteRow)

	inserted, err := l.InsertDeposit(ctx, d)
	if err != nil {
		t.Fatalf("1st insert: %v", err)
	}
	if !inserted {
		t.Fatal("1st insert: inserted = false, want true")
	}

	inserted, err = l.InsertDeposit(ctx, d)
	if err != nil {
		t.Fatalf("2nd insert: %v", err)
	}
	if inserted {
		t.Fatal("2nd insert: inserted = true, want false")
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM deposits WHERE tx_hash = $1`, d.TxHash).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("rows = %d, want 1", n)
	}
}

func TestUserByAddress(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)

	l := New(pool)
	addr := common.HexToAddress("0x00000000000000000000000000000000DeAdBeEf")
	unknown := common.HexToAddress("0x00000000000000000000000000000000000000Ff")

	// Same leftover/cleanup pattern as TestInsertDepositTwice.
	deleteRow := func() {
		if _, err := pool.Exec(ctx, `DELETE FROM deposit_addresses WHERE address = $1`, addressKey(addr)); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	}
	deleteRow()
	t.Cleanup(deleteRow)

	if err := l.AssignAddress(ctx, 42, addr); err != nil {
		t.Fatalf("assign: %v", err)
	}

	userID, found, err := l.UserByAddress(ctx, addr)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if !found || userID != 42 {
		t.Fatalf("lookup = (%d, %v), want (42, true)", userID, found)
	}

	// Not one of our addresses: not an error, just not a deposit.
	if _, found, err := l.UserByAddress(ctx, unknown); err != nil || found {
		t.Fatalf("unknown lookup: found = %v, err = %v; want false, nil", found, err)
	}

	// An address must never move to another user.
	if err := l.AssignAddress(ctx, 7, addr); err == nil {
		t.Fatal("re-assign to another user: err = nil, want error")
	}
}

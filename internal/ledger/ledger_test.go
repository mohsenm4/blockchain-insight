package ledger

import (
	"context"
	"math/big"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestInsertDepositTwice(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set") // CI has no postgres
	}
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

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

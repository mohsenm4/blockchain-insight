package ledger

import (
	"context"
	"errors"
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
		if _, err := pool.Exec(ctx, `DELETE FROM deposit_addresses WHERE address = $1`, AddressKey(addr)); err != nil {
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
	if err := l.AssignAddress(ctx, 7, addr); !errors.Is(err, ErrAddressTaken) {
		t.Fatalf("re-assign to another user: err = %v, want ErrAddressTaken", err)
	}
}

func TestRecordBatch(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	l := New(pool)

	const cursor = "test-record-batch"
	addr := common.HexToAddress("0x00000000000000000000000000000000000000B1")
	big1, _ := new(big.Int).SetString("123456789012345678901234567890", 10)
	deps := []Deposit{
		{UserID: 1, Address: AddressKey(addr), TxHash: "0xtest-batch", LogIndex: 0,
			Token: "USDC", Amount: big1, BlockNumber: 10, BlockHash: "0xb10"},
		{UserID: 1, Address: AddressKey(addr), TxHash: "0xtest-batch", LogIndex: 1,
			Token: "USDC", Amount: big.NewInt(5), BlockNumber: 10, BlockHash: "0xb10"},
	}
	cleanupBatch(t, pool, cursor, "0xtest-batch")

	if _, found, err := l.Cursor(ctx, cursor); err != nil || found {
		t.Fatalf("cursor before first batch: found = %v, err = %v; want false, nil", found, err)
	}

	inserted, err := l.RecordBatch(ctx, cursor, 10, deps)
	if err != nil {
		t.Fatalf("1st batch: %v", err)
	}
	if inserted != 2 {
		t.Fatalf("1st batch: inserted = %d, want 2", inserted)
	}

	// Same deposits again (e.g. after a restart): nothing new, cursor still moves.
	inserted, err = l.RecordBatch(ctx, cursor, 11, deps)
	if err != nil {
		t.Fatalf("2nd batch: %v", err)
	}
	if inserted != 0 {
		t.Fatalf("2nd batch: inserted = %d, want 0", inserted)
	}

	block, found, err := l.Cursor(ctx, cursor)
	if err != nil || !found || block != 11 {
		t.Fatalf("cursor = (%d, %v, %v), want (11, true, nil)", block, found, err)
	}

	got, err := l.DepositsByAddress(ctx, addr)
	if err != nil {
		t.Fatalf("deposits by address: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("deposits = %d, want 2", len(got))
	}
	if got[0].Amount.Cmp(big1) != 0 || got[0].Status != "pending" {
		t.Fatalf("deposit 0: amount = %s, status = %q; want %s, pending", got[0].Amount, got[0].Status, big1)
	}
}

func TestRecordBatchRollsBack(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	l := New(pool)

	const cursor = "test-record-batch-rollback"
	ok := Deposit{UserID: 1, Address: "0xb2", TxHash: "0xtest-rollback", LogIndex: 0,
		Token: "USDC", Amount: big.NewInt(1), BlockNumber: 20, BlockHash: "0xb20"}
	bad := ok
	bad.LogIndex, bad.Amount = 1, nil // violates amount NOT NULL
	cleanupBatch(t, pool, cursor, "0xtest-rollback")

	if _, err := l.RecordBatch(ctx, cursor, 20, []Deposit{ok, bad}); err == nil {
		t.Fatal("batch with a bad deposit: err = nil, want error")
	}

	// Nothing from the failed batch may stick: not the good deposit, not the cursor.
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM deposits WHERE tx_hash = $1`, ok.TxHash).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("rows = %d, want 0", n)
	}
	if _, found, err := l.Cursor(ctx, cursor); err != nil || found {
		t.Fatalf("cursor after failed batch: found = %v, err = %v; want false, nil", found, err)
	}
}

// cleanupBatch removes a test's deposits and cursor before and after it runs.
func cleanupBatch(t *testing.T, pool *pgxpool.Pool, cursor, txHash string) {
	t.Helper()
	ctx := context.Background()
	del := func() {
		if _, err := pool.Exec(ctx, `DELETE FROM deposits WHERE tx_hash = $1`, txHash); err != nil {
			t.Errorf("cleanup deposits: %v", err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM watcher_cursor WHERE name = $1`, cursor); err != nil {
			t.Errorf("cleanup cursor: %v", err)
		}
	}
	del()
	t.Cleanup(del)
}

func TestCreateWithdrawalTwice(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)

	l := New(pool)
	w := Withdrawal{
		UserID: 1, IdempotencyKey: "test-create-withdrawal-twice",
		To: "0xabc", Token: "0xtoken", Amount: big.NewInt(1_000_000),
	}

	deleteRow := func() {
		if _, err := pool.Exec(ctx, `DELETE FROM withdrawals WHERE idempotency_key = $1`, w.IdempotencyKey); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	}
	deleteRow()
	t.Cleanup(deleteRow)

	id, created, err := l.CreateWithdrawal(ctx, w)
	if err != nil {
		t.Fatalf("1st create: %v", err)
	}
	if !created || id == 0 {
		t.Fatalf("1st create: id=%d created=%v, want new row", id, created)
	}

	_, created, err = l.CreateWithdrawal(ctx, w)
	if err != nil {
		t.Fatalf("2nd create: %v", err)
	}
	if created {
		t.Fatal("2nd create: created = true, want false")
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM withdrawals WHERE idempotency_key = $1`, w.IdempotencyKey).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("rows = %d, want 1", n)
	}

	got, found, err := l.WithdrawalByKey(ctx, w.UserID, w.IdempotencyKey)
	if err != nil || !found {
		t.Fatalf("by key: found=%v err=%v", found, err)
	}
	if got.ID != id || got.Status != "queued" || got.TxHash != "" || got.Amount.Cmp(w.Amount) != 0 {
		t.Fatalf("by key = %+v, want queued row %d", got, id)
	}

	if err := l.MarkBroadcast(ctx, id, "0xtest-withdrawal-tx"); !errors.Is(err, ErrNotSigned) {
		t.Fatalf("broadcast before sign: err = %v, want ErrNotSigned", err)
	}
	if err := l.MarkSigned(ctx, id, 7, "0xtest-withdrawal-tx", "0xraw"); err != nil {
		t.Fatalf("mark signed: %v", err)
	}
	if err := l.MarkSigned(ctx, id, 8, "0xother", "0xraw2"); !errors.Is(err, ErrNotQueued) {
		t.Fatalf("2nd mark signed: err = %v, want ErrNotQueued", err)
	}

	got, _, err = l.WithdrawalByKey(ctx, w.UserID, w.IdempotencyKey)
	if err != nil {
		t.Fatalf("by key after sign: %v", err)
	}
	if got.Status != "signed" || got.TxHash != "0xtest-withdrawal-tx" || got.RawTx != "0xraw" {
		t.Fatalf("after sign = %+v, want signed 0xtest-withdrawal-tx/0xraw", got)
	}

	if err := l.MarkBroadcast(ctx, id, "0xother"); !errors.Is(err, ErrNotSigned) {
		t.Fatalf("broadcast other hash: err = %v, want ErrNotSigned", err)
	}
	if err := l.MarkBroadcast(ctx, id, "0xtest-withdrawal-tx"); err != nil {
		t.Fatalf("mark broadcast: %v", err)
	}

	got, _, err = l.WithdrawalByKey(ctx, w.UserID, w.IdempotencyKey)
	if err != nil {
		t.Fatalf("by key after broadcast: %v", err)
	}
	if got.Status != "broadcast" || got.TxHash != "0xtest-withdrawal-tx" {
		t.Fatalf("after broadcast = %s/%s, want broadcast/0xtest-withdrawal-tx", got.Status, got.TxHash)
	}
}

func TestMarkConfirmed(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)

	l := New(pool)
	d := Deposit{
		UserID: 1, Address: "0xabc", TxHash: "0xtest-mark-confirmed", LogIndex: 0,
		Token: "USDC", Amount: big.NewInt(1), BlockNumber: 100, BlockHash: "0xblock",
	}
	deleteRow := func() {
		if _, err := pool.Exec(ctx, `DELETE FROM deposits WHERE tx_hash = $1`, d.TxHash); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	}
	deleteRow()
	t.Cleanup(deleteRow)

	if _, err := l.InsertDeposit(ctx, d); err != nil {
		t.Fatalf("insert: %v", err)
	}

	find := func(maxBlock uint64) (Deposit, bool) {
		deps, err := l.PendingDeposits(ctx, maxBlock)
		if err != nil {
			t.Fatalf("pending deposits: %v", err)
		}
		for _, p := range deps {
			if p.TxHash == d.TxHash {
				return p, true
			}
		}
		return Deposit{}, false
	}
	if _, ok := find(99); ok {
		t.Fatal("block 100 returned for maxBlock 99")
	}
	p, ok := find(100)
	if !ok {
		t.Fatal("pending deposit not returned for maxBlock 100")
	}

	if err := l.MarkConfirmed(ctx, p.ID, "0xother"); !errors.Is(err, ErrNotPending) {
		t.Fatalf("wrong hash: err = %v, want ErrNotPending", err)
	}
	if err := l.MarkConfirmed(ctx, p.ID, d.BlockHash); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if err := l.MarkConfirmed(ctx, p.ID, d.BlockHash); !errors.Is(err, ErrNotPending) {
		t.Fatalf("2nd confirm: err = %v, want ErrNotPending", err)
	}
	if _, ok := find(100); ok {
		t.Fatal("confirmed deposit still returned as pending")
	}
}

func TestMarkCreditedOnce(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)

	l := New(pool)
	d := Deposit{
		UserID: 999001, Address: "0xabc", TxHash: "0xtest-mark-credited", LogIndex: 0,
		Token: "TESTCREDIT", Amount: big.NewInt(250), BlockNumber: 100, BlockHash: "0xblock",
	}
	cleanup := func() {
		if _, err := pool.Exec(ctx, `DELETE FROM deposits WHERE tx_hash = $1`, d.TxHash); err != nil {
			t.Errorf("cleanup deposits: %v", err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM balances WHERE user_id = $1`, d.UserID); err != nil {
			t.Errorf("cleanup balances: %v", err)
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	if _, err := l.InsertDeposit(ctx, d); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var id int64
	if err := pool.QueryRow(ctx, `SELECT id FROM deposits WHERE tx_hash = $1`, d.TxHash).Scan(&id); err != nil {
		t.Fatalf("find id: %v", err)
	}

	if err := l.MarkCredited(ctx, id, 112); !errors.Is(err, ErrNotConfirmed) {
		t.Fatalf("credit pending: err = %v, want ErrNotConfirmed", err)
	}
	if err := l.MarkConfirmed(ctx, id, d.BlockHash); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if err := l.MarkCredited(ctx, id, 112); err != nil {
		t.Fatalf("credit: %v", err)
	}
	if err := l.MarkCredited(ctx, id, 113); !errors.Is(err, ErrNotConfirmed) {
		t.Fatalf("2nd credit: err = %v, want ErrNotConfirmed", err)
	}

	var balance string
	if err := pool.QueryRow(ctx,
		`SELECT amount::text FROM balances WHERE user_id = $1 AND token = $2`,
		d.UserID, d.Token).Scan(&balance); err != nil {
		t.Fatalf("read balance: %v", err)
	}
	if balance != "250" {
		t.Fatalf("balance = %s, want 250 (credited once)", balance)
	}

	var status string
	var atBlock uint64
	if err := pool.QueryRow(ctx,
		`SELECT status, credited_at_block FROM deposits WHERE id = $1`, id).Scan(&status, &atBlock); err != nil {
		t.Fatalf("read deposit: %v", err)
	}
	if status != "credited" || atBlock != 112 {
		t.Fatalf("deposit = %s at %d, want credited at 112", status, atBlock)
	}
}

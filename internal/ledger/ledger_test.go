package ledger

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"os"
	"sync"
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
		UserID: 999003, IdempotencyKey: "test-create-withdrawal-twice",
		To: "0xabc", Token: testToken, Amount: big.NewInt(1_000_000),
	}
	cleanupWithdrawals(t, pool, w.UserID)
	setBalance(t, pool, w.UserID, 1_000_000)

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
	if p.UserID != d.UserID || p.Address != d.Address || p.Token != d.Token || p.Amount.Cmp(d.Amount) != 0 {
		t.Fatalf("pending deposit = %+v, want the fields of %+v", p, d)
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

// depositRow is what the reorg tests read back.
type depositRow struct {
	logIndex uint
	status   string
	block    uint64
	hash     string
	amount   string
}

func depositRows(t *testing.T, pool *pgxpool.Pool, txHash string) []depositRow {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT log_index, status, block_number, block_hash, amount::text
		FROM deposits WHERE tx_hash = $1 ORDER BY log_index`, txHash)
	if err != nil {
		t.Fatalf("read deposits: %v", err)
	}
	defer rows.Close()

	var out []depositRow
	for rows.Next() {
		var r depositRow
		if err := rows.Scan(&r.logIndex, &r.status, &r.block, &r.hash, &r.amount); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read deposits: %v", err)
	}
	return out
}

// Two transfers of one tx were in a replaced block at log indexes 3 and 4.
// In the new block they are at 4 and 5, so the first one takes the old row
// of the second.
func TestRevertDeposits(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)

	l := New(pool)
	const tx = "0xtest-revert-deposits"
	deleteRows := func() {
		if _, err := pool.Exec(ctx, `DELETE FROM deposits WHERE tx_hash = $1`, tx); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	}
	deleteRows()
	t.Cleanup(deleteRows)

	first := Deposit{UserID: 1, Address: "0xabc", TxHash: tx, LogIndex: 3, Token: "USDC",
		Amount: big.NewInt(10), BlockNumber: 100, BlockHash: "0xold"}
	second := first
	second.LogIndex, second.Amount = 4, big.NewInt(20)
	for _, d := range []Deposit{first, second} {
		if _, err := l.InsertDeposit(ctx, d); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	moved := []Deposit{first, second}
	for i := range moved {
		moved[i].LogIndex++
		moved[i].BlockNumber, moved[i].BlockHash = 101, "0xnew"
	}
	if err := l.RevertDeposits(ctx, tx, "0xold", moved); err != nil {
		t.Fatalf("revert: %v", err)
	}

	want := []depositRow{
		{3, "reverted", 100, "0xold", "10"},
		{4, "pending", 101, "0xnew", "10"},
		{5, "pending", 101, "0xnew", "20"},
	}
	got := depositRows(t, pool, tx)
	if len(got) != len(want) {
		t.Fatalf("rows = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rows = %+v, want %+v", got, want)
		}
	}

	if err := l.RevertDeposits(ctx, tx, "0xold", moved); !errors.Is(err, ErrNotPending) {
		t.Fatalf("2nd revert: err = %v, want ErrNotPending", err)
	}

	// The watcher sees the new block later: no second row.
	if inserted, err := l.InsertDeposit(ctx, moved[1]); err != nil || inserted {
		t.Fatalf("watcher insert: inserted = %v, err = %v; want false, nil", inserted, err)
	}
}

// The tx was in no block when its deposit was reverted. When the watcher
// sees it again at the same log index, the reverted row is pending again.
func TestRevertedDepositComesBack(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)

	l := New(pool)
	d := Deposit{UserID: 1, Address: "0xabc", TxHash: "0xtest-comes-back", LogIndex: 0, Token: "USDC",
		Amount: big.NewInt(10), BlockNumber: 100, BlockHash: "0xold"}
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
	if err := l.RevertDeposits(ctx, d.TxHash, d.BlockHash, nil); err != nil {
		t.Fatalf("revert: %v", err)
	}
	if got := depositRows(t, pool, d.TxHash); len(got) != 1 || got[0].status != "reverted" {
		t.Fatalf("after revert = %+v, want one reverted row", got)
	}

	d.BlockNumber, d.BlockHash = 105, "0xnew"
	inserted, err := l.InsertDeposit(ctx, d)
	if err != nil || !inserted {
		t.Fatalf("insert again: inserted = %v, err = %v; want true, nil", inserted, err)
	}
	want := depositRow{0, "pending", 105, "0xnew", "10"}
	if got := depositRows(t, pool, d.TxHash); len(got) != 1 || got[0] != want {
		t.Fatalf("after insert again = %+v, want [%+v]", got, want)
	}
}

// testToken is the token of the withdrawal tests' balances.
const testToken = "0xtoken"

// cleanupWithdrawals deletes the withdrawals and balances of a test user,
// now and after the test.
func cleanupWithdrawals(t *testing.T, pool *pgxpool.Pool, userID int64) {
	t.Helper()
	clean := func() {
		for _, q := range []string{
			`DELETE FROM withdrawals WHERE user_id = $1`,
			`DELETE FROM balances WHERE user_id = $1`,
		} {
			if _, err := pool.Exec(context.Background(), q, userID); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}
	}
	clean()
	t.Cleanup(clean)
}

func setBalance(t *testing.T, pool *pgxpool.Pool, userID int64, amount int64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO balances (user_id, token, amount) VALUES ($1, $2, $3)`, userID, testToken, amount); err != nil {
		t.Fatalf("set balance: %v", err)
	}
}

func balanceOf(t *testing.T, pool *pgxpool.Pool, userID int64) string {
	t.Helper()
	var amount string
	if err := pool.QueryRow(context.Background(),
		`SELECT amount::text FROM balances WHERE user_id = $1 AND token = $2`, userID, testToken).Scan(&amount); err != nil {
		t.Fatalf("read balance: %v", err)
	}
	return amount
}

// A withdrawal takes its amount once, a too-big one writes nothing, and a
// failed one gives the amount back once.
func TestWithdrawalBalance(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)

	l := New(pool)
	const user = 999004
	cleanupWithdrawals(t, pool, user)
	setBalance(t, pool, user, 1500)

	first := Withdrawal{UserID: user, IdempotencyKey: "k1", To: "0xabc", Token: testToken, Amount: big.NewInt(1000)}
	id, created, err := l.CreateWithdrawal(ctx, first)
	if err != nil || !created {
		t.Fatalf("1st create: created = %v, err = %v", created, err)
	}
	if got := balanceOf(t, pool, user); got != "500" {
		t.Fatalf("after 1st create: balance = %s, want 500", got)
	}

	// The same key again: the first request already took the amount.
	if _, created, err := l.CreateWithdrawal(ctx, first); err != nil || created {
		t.Fatalf("same key: created = %v, err = %v; want false, nil", created, err)
	}
	if got := balanceOf(t, pool, user); got != "500" {
		t.Fatalf("after same key: balance = %s, want 500", got)
	}

	second := first
	second.IdempotencyKey = "k2"
	if _, _, err := l.CreateWithdrawal(ctx, second); !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("too big: err = %v, want ErrInsufficientBalance", err)
	}
	if _, found, err := l.WithdrawalByKey(ctx, user, "k2"); err != nil || found {
		t.Fatalf("too big: found = %v, err = %v; want no row", found, err)
	}
	if got := balanceOf(t, pool, user); got != "500" {
		t.Fatalf("after too big: balance = %s, want 500", got)
	}

	if err := l.MarkFailed(ctx, id); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	if got := balanceOf(t, pool, user); got != "1500" {
		t.Fatalf("after failed: balance = %s, want 1500", got)
	}
	if err := l.MarkFailed(ctx, id); !errors.Is(err, ErrNotQueued) {
		t.Fatalf("2nd mark failed: err = %v, want ErrNotQueued", err)
	}
	if got := balanceOf(t, pool, user); got != "1500" {
		t.Fatalf("after 2nd failed: balance = %s, want 1500 (given back once)", got)
	}
}

// Many withdrawals with different keys, at the same time, and money for
// only one of them: exactly one is created.
func TestWithdrawalsRaceForOneBalance(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)

	l := New(pool)
	const user = 999005
	cleanupWithdrawals(t, pool, user)
	setBalance(t, pool, user, 1000)

	// Open the connections first: a goroutine that has to dial a new one
	// starts too late to race.
	n := min(int(pool.Config().MaxConns), 8)
	conns := make([]*pgxpool.Conn, n)
	for i := range conns {
		c, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatalf("acquire: %v", err)
		}
		conns[i] = c
	}
	for _, c := range conns {
		c.Release()
	}

	start := make(chan struct{})
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			w := Withdrawal{UserID: user, IdempotencyKey: fmt.Sprintf("race-%d", i), To: "0xabc",
				Token: testToken, Amount: big.NewInt(1000)}
			_, _, errs[i] = l.CreateWithdrawal(ctx, w)
		}()
	}
	close(start)
	wg.Wait()

	created, short := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			created++
		case errors.Is(err, ErrInsufficientBalance):
			short++
		default:
			t.Fatalf("create: %v", err)
		}
	}
	if created != 1 || short != n-1 {
		t.Fatalf("created = %d, insufficient = %d; want 1, %d", created, short, n-1)
	}
	if got := balanceOf(t, pool, user); got != "0" {
		t.Fatalf("balance = %s, want 0", got)
	}
}

// A broadcast withdrawal is confirmed or failed on chain once; failing it
// gives the amount back once, and only for its own tx hash.
func TestSettleBroadcastWithdrawal(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)

	l := New(pool)
	const user = 999006
	cleanupWithdrawals(t, pool, user)
	setBalance(t, pool, user, 3000)

	broadcast := func(key, hash string) int64 {
		t.Helper()
		w := Withdrawal{UserID: user, IdempotencyKey: key, To: "0xabc", Token: testToken, Amount: big.NewInt(1000)}
		id, _, err := l.CreateWithdrawal(ctx, w)
		if err != nil {
			t.Fatalf("create %s: %v", key, err)
		}
		if err := l.MarkSigned(ctx, id, uint64(id), hash, "0x00"); err != nil {
			t.Fatalf("sign %s: %v", key, err)
		}
		if err := l.MarkBroadcast(ctx, id, hash); err != nil {
			t.Fatalf("broadcast %s: %v", key, err)
		}
		return id
	}
	ok := broadcast("ok", "0xtest-settle-ok")
	bad := broadcast("bad", "0xtest-settle-bad")

	ws, err := l.BroadcastWithdrawals(ctx)
	if err != nil {
		t.Fatalf("broadcast withdrawals: %v", err)
	}
	var mine int
	for _, w := range ws {
		if w.ID == ok || w.ID == bad {
			mine++
		}
	}
	if mine != 2 {
		t.Fatalf("broadcast withdrawals: found %d of ours, want 2", mine)
	}

	if err := l.MarkWithdrawalConfirmed(ctx, ok, "0xother"); !errors.Is(err, ErrNotBroadcast) {
		t.Fatalf("confirm with wrong hash: err = %v, want ErrNotBroadcast", err)
	}
	if err := l.MarkWithdrawalConfirmed(ctx, ok, "0xtest-settle-ok"); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if err := l.MarkBroadcastFailed(ctx, ok, "0xtest-settle-ok"); !errors.Is(err, ErrNotBroadcast) {
		t.Fatalf("fail a confirmed one: err = %v, want ErrNotBroadcast", err)
	}
	if got := balanceOf(t, pool, user); got != "1000" {
		t.Fatalf("after confirm: balance = %s, want 1000", got)
	}

	if err := l.MarkBroadcastFailed(ctx, bad, "0xtest-settle-bad"); err != nil {
		t.Fatalf("fail: %v", err)
	}
	if err := l.MarkBroadcastFailed(ctx, bad, "0xtest-settle-bad"); !errors.Is(err, ErrNotBroadcast) {
		t.Fatalf("2nd fail: err = %v, want ErrNotBroadcast", err)
	}
	if got := balanceOf(t, pool, user); got != "2000" {
		t.Fatalf("after fail: balance = %s, want 2000 (given back once)", got)
	}
}

// A reversal takes the amount back once and never edits the credited row;
// with too little balance it takes nothing and needs review.
func TestReverseDeposit(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)

	l := New(pool)
	const user = 999007
	cleanupWithdrawals(t, pool, user)
	clean := func() {
		for _, q := range []string{
			`DELETE FROM deposit_reversals WHERE user_id = $1`,
			`DELETE FROM deposits WHERE user_id = $1`,
		} {
			if _, err := pool.Exec(ctx, q, user); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}
	}
	clean()
	t.Cleanup(clean) // registered last, so it runs before the balances cleanup

	credit := func(txHash string, amount int64) int64 {
		t.Helper()
		d := Deposit{UserID: user, Address: "0xabc", TxHash: txHash, Token: testToken,
			Amount: big.NewInt(amount), BlockNumber: 5000, BlockHash: "0xold"}
		if _, err := l.InsertDeposit(ctx, d); err != nil {
			t.Fatalf("insert %s: %v", txHash, err)
		}
		var id int64
		if err := pool.QueryRow(ctx, `SELECT id FROM deposits WHERE tx_hash = $1`, txHash).Scan(&id); err != nil {
			t.Fatalf("id of %s: %v", txHash, err)
		}
		if err := l.MarkConfirmed(ctx, id, "0xold"); err != nil {
			t.Fatalf("confirm %s: %v", txHash, err)
		}
		if err := l.MarkCredited(ctx, id, 5012); err != nil {
			t.Fatalf("credit %s: %v", txHash, err)
		}
		return id
	}
	a := credit("0xtest-reverse-a", 1000)
	b := credit("0xtest-reverse-b", 500)
	if got := balanceOf(t, pool, user); got != "1500" {
		t.Fatalf("after credit: balance = %s, want 1500", got)
	}

	deps, err := l.CreditedDeposits(ctx, 5000)
	if err != nil {
		t.Fatalf("credited deposits: %v", err)
	}
	var mine int
	for _, d := range deps {
		if d.UserID == user {
			mine++
		}
	}
	if mine != 2 {
		t.Fatalf("credited deposits: %d of ours, want 2", mine)
	}

	if _, err := l.ReverseDeposit(ctx, a, "0xnew", "test"); !errors.Is(err, ErrNotCredited) {
		t.Fatalf("wrong block hash: err = %v, want ErrNotCredited", err)
	}
	if status, err := l.ReverseDeposit(ctx, a, "0xold", "test"); err != nil || status != "reversed" {
		t.Fatalf("reverse a: status = %q, err = %v; want reversed", status, err)
	}
	if _, err := l.ReverseDeposit(ctx, a, "0xold", "test"); !errors.Is(err, ErrNotCredited) {
		t.Fatalf("2nd reverse a: err = %v, want ErrNotCredited", err)
	}
	if got := balanceOf(t, pool, user); got != "500" {
		t.Fatalf("after reverse a: balance = %s, want 500 (taken once)", got)
	}

	// The user spends it, then b's block goes away.
	if _, err := pool.Exec(ctx, `UPDATE balances SET amount = 100 WHERE user_id = $1`, user); err != nil {
		t.Fatalf("spend: %v", err)
	}
	if status, err := l.ReverseDeposit(ctx, b, "0xold", "test"); err != nil || status != "needs_review" {
		t.Fatalf("reverse b: status = %q, err = %v; want needs_review", status, err)
	}
	if got := balanceOf(t, pool, user); got != "100" {
		t.Fatalf("after reverse b: balance = %s, want 100 (nothing taken)", got)
	}

	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM deposits WHERE id = $1`, a).Scan(&status); err != nil || status != "credited" {
		t.Fatalf("deposit a: status = %q, err = %v; want credited (never edited)", status, err)
	}
	if deps, err = l.CreditedDeposits(ctx, 5000); err != nil {
		t.Fatalf("credited deposits: %v", err)
	}
	for _, d := range deps {
		if d.UserID == user {
			t.Fatalf("deposit %d still listed after its reversal", d.ID)
		}
	}
}

// cleanupDeposits deletes the deposits, reversals and balances of a test
// user, now and after the test.
func cleanupDeposits(t *testing.T, pool *pgxpool.Pool, userID int64) {
	t.Helper()
	clean := func() {
		for _, q := range []string{
			`DELETE FROM deposit_reversals WHERE user_id = $1`,
			`DELETE FROM deposits WHERE user_id = $1`,
			`DELETE FROM balances WHERE user_id = $1`,
		} {
			if _, err := pool.Exec(context.Background(), q, userID); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}
	}
	clean()
	t.Cleanup(clean)
}

// creditDeposit inserts d, confirms it and credits it, and returns its id.
func creditDeposit(t *testing.T, l *Ledger, pool *pgxpool.Pool, d Deposit) int64 {
	t.Helper()
	ctx := context.Background()
	if _, err := l.InsertDeposit(ctx, d); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var id int64
	if err := pool.QueryRow(ctx, `SELECT id FROM deposits WHERE tx_hash = $1 AND log_index = $2`,
		d.TxHash, d.LogIndex).Scan(&id); err != nil {
		t.Fatalf("find id: %v", err)
	}
	if err := l.MarkConfirmed(ctx, id, d.BlockHash); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if err := l.MarkCredited(ctx, id, d.BlockNumber+12); err != nil {
		t.Fatalf("credit: %v", err)
	}
	return id
}

// A credited deposit's tx moved to a new block, where its transfer has
// another log index. The watcher sees that block later: the copy must not
// be recorded (it would be credited twice), until the deposit is reversed.
func TestInsertDepositSkipsMovedCreditedTransfer(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)

	l := New(pool)
	const user = 999008
	cleanupDeposits(t, pool, user)

	d := Deposit{UserID: user, Address: "0xabc", TxHash: "0xtest-moved-credited", LogIndex: 3, Token: testToken,
		Amount: big.NewInt(1000), BlockNumber: 6000, BlockHash: "0xold"}
	id := creditDeposit(t, l, pool, d)

	moved := d
	moved.LogIndex, moved.BlockNumber, moved.BlockHash = 7, 6001, "0xnew"
	if inserted, err := l.InsertDeposit(ctx, moved); err != nil || inserted {
		t.Fatalf("moved copy: inserted = %v, err = %v; want false, nil", inserted, err)
	}

	// Another transfer of the same tx (another amount) is not a copy.
	other := moved
	other.LogIndex, other.Amount = 8, big.NewInt(50)
	if inserted, err := l.InsertDeposit(ctx, other); err != nil || !inserted {
		t.Fatalf("other transfer: inserted = %v, err = %v; want true, nil", inserted, err)
	}

	// After the credit is reversed, the transfer can be recorded again.
	if _, err := l.ReverseDeposit(ctx, id, "0xold", "test"); err != nil {
		t.Fatalf("reverse: %v", err)
	}
	if inserted, err := l.InsertDeposit(ctx, moved); err != nil || !inserted {
		t.Fatalf("after reversal: inserted = %v, err = %v; want true, nil", inserted, err)
	}
}

// The API shows a reversed deposit with its reversal's status.
func TestDepositsByAddressShowsReversal(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)

	l := New(pool)
	const user = 999009
	cleanupDeposits(t, pool, user)

	addr := common.HexToAddress("0x00000000000000000000000000000000009990")
	d := Deposit{UserID: user, Address: AddressKey(addr), TxHash: "0xtest-by-address-reversed", Token: testToken,
		Amount: big.NewInt(1000), BlockNumber: 7000, BlockHash: "0xold"}
	id := creditDeposit(t, l, pool, d)

	status := func() string {
		t.Helper()
		deps, err := l.DepositsByAddress(ctx, addr)
		if err != nil || len(deps) != 1 {
			t.Fatalf("deposits by address: %d rows, err = %v; want 1", len(deps), err)
		}
		return deps[0].Status
	}
	if got := status(); got != "credited" {
		t.Fatalf("before reversal: status = %s, want credited", got)
	}
	if _, err := l.ReverseDeposit(ctx, id, "0xold", "test"); err != nil {
		t.Fatalf("reverse: %v", err)
	}
	if got := status(); got != "reversed" {
		t.Fatalf("after reversal: status = %s, want reversed", got)
	}
}

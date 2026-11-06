package ledger

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Deposit struct {
	ID          int64 // set by the DB; ignored on insert
	UserID      int64
	Address     string
	TxHash      string
	LogIndex    uint
	Token       string
	Amount      *big.Int
	BlockNumber uint64
	BlockHash   string
	Status      string // set by the DB; ignored on insert
}

// Withdrawal is a user's request to send tokens out. To and Token are
// stored lowercase (see AddressKey).
type Withdrawal struct {
	ID             int64 // set by the DB; ignored on insert
	UserID         int64
	IdempotencyKey string
	To             string
	Token          string
	Amount         *big.Int
	TxHash         string // empty until signed
	RawTx          string // signed tx, hex; empty until signed
	Status         string // set by the DB; ignored on insert
}

// ErrAddressTaken means the address already belongs to a user.
var ErrAddressTaken = errors.New("address already assigned")

// ErrNotQueued means the withdrawal is missing or has already left 'queued'.
var ErrNotQueued = errors.New("withdrawal not queued")

// ErrNotSigned means the withdrawal is missing, has already left 'signed', or
// was signed as a different tx.
var ErrNotSigned = errors.New("withdrawal not signed")

// ErrNotPending means the deposit is missing, has already left 'pending', or
// is now in a block with a different hash.
var ErrNotPending = errors.New("deposit not pending")

// ErrNotConfirmed means the deposit is missing or has already left 'confirmed'
// (for example, it was already credited).
var ErrNotConfirmed = errors.New("deposit not confirmed")

// ErrInsufficientBalance means the user has less of the token than the
// withdrawal amount.
var ErrInsufficientBalance = errors.New("insufficient balance")

// uniqueViolation is the Postgres error code for a UNIQUE/PK conflict.
const uniqueViolation = "23505"

type Ledger struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Ledger {
	return &Ledger{pool: pool}
}

// dbtx is what pgxpool.Pool and pgx.Tx have in common, so a query can run
// inside or outside a transaction.
type dbtx interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// InsertDeposit stores d. It returns inserted=false when a row with the
// same (tx_hash, log_index) already exists and is not reverted. A reverted
// row is used again, as pending: after a reorg the transfer is back on the
// chain, maybe at a log index another transfer of the same tx had before.
func (l *Ledger) InsertDeposit(ctx context.Context, d Deposit) (inserted bool, err error) {
	return insertDeposit(ctx, l.pool, d)
}

func insertDeposit(ctx context.Context, db dbtx, d Deposit) (inserted bool, err error) {
	const q = `
		INSERT INTO deposits (user_id, address, tx_hash, log_index, token, amount, block_number, block_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (tx_hash, log_index) DO UPDATE
		SET user_id = EXCLUDED.user_id, address = EXCLUDED.address, token = EXCLUDED.token,
		    amount = EXCLUDED.amount, block_number = EXCLUDED.block_number,
		    block_hash = EXCLUDED.block_hash, status = 'pending', updated_at = now()
		WHERE deposits.status = 'reverted'`

	tag, err := db.Exec(ctx, q,
		d.UserID, d.Address, d.TxHash, d.LogIndex, d.Token, d.Amount, d.BlockNumber, d.BlockHash)
	if err != nil {
		return false, fmt.Errorf("insert deposit %s:%d: %w", d.TxHash, d.LogIndex, err)
	}

	// A conflict with a row that is not reverted changes nothing: 0 rows affected.
	return tag.RowsAffected() == 1, nil
}

// AddressKey is how an address is stored: lowercase hex, so lookups don't
// depend on checksum casing.
func AddressKey(a common.Address) string {
	return strings.ToLower(a.Hex())
}

// AssignAddress makes addr a deposit address of userID. It returns
// ErrAddressTaken if addr is already assigned, so an address never silently
// moves to another user.
func (l *Ledger) AssignAddress(ctx context.Context, userID int64, addr common.Address) error {
	const q = `INSERT INTO deposit_addresses (address, user_id) VALUES ($1, $2)`

	if _, err := l.pool.Exec(ctx, q, AddressKey(addr), userID); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return fmt.Errorf("assign address %s: %w", addr.Hex(), ErrAddressTaken)
		}
		return fmt.Errorf("assign address %s to user %d: %w", addr.Hex(), userID, err)
	}
	return nil
}

// UserByAddress returns the owner of a deposit address. found is false when
// addr is not one of ours, which means a transfer to it is not a deposit.
func (l *Ledger) UserByAddress(ctx context.Context, addr common.Address) (userID int64, found bool, err error) {
	const q = `SELECT user_id FROM deposit_addresses WHERE address = $1`

	err = l.pool.QueryRow(ctx, q, AddressKey(addr)).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("user by address %s: %w", addr.Hex(), err)
	}
	return userID, true, nil
}

// DepositsByAddress returns the deposits to addr, oldest first.
func (l *Ledger) DepositsByAddress(ctx context.Context, addr common.Address) ([]Deposit, error) {
	// amount as text: NUMERIC → *big.Int has no direct scan, and text is exact.
	const q = `
		SELECT user_id, address, tx_hash, log_index, token, amount::text, block_number, block_hash, status
		FROM deposits
		WHERE address = $1
		ORDER BY block_number, log_index`

	rows, err := l.pool.Query(ctx, q, AddressKey(addr))
	if err != nil {
		return nil, fmt.Errorf("deposits by address %s: %w", addr.Hex(), err)
	}
	defer rows.Close()

	var deps []Deposit
	for rows.Next() {
		var d Deposit
		var amount string
		if err := rows.Scan(&d.UserID, &d.Address, &d.TxHash, &d.LogIndex, &d.Token,
			&amount, &d.BlockNumber, &d.BlockHash, &d.Status); err != nil {
			return nil, fmt.Errorf("scan deposit: %w", err)
		}
		var ok bool
		if d.Amount, ok = new(big.Int).SetString(amount, 10); !ok {
			return nil, fmt.Errorf("deposit %s:%d: bad amount %q", d.TxHash, d.LogIndex, amount)
		}
		deps = append(deps, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("deposits by address %s: %w", addr.Hex(), err)
	}
	return deps, nil
}

// PendingDeposits returns the pending deposits in blocks up to maxBlock,
// oldest first. Every field except Status is set: after a reorg the tracker
// needs the transfer itself to find it again.
func (l *Ledger) PendingDeposits(ctx context.Context, maxBlock uint64) ([]Deposit, error) {
	// amount as text: see DepositsByAddress.
	const q = `
		SELECT id, user_id, address, tx_hash, log_index, token, amount::text, block_number, block_hash
		FROM deposits
		WHERE status = 'pending' AND block_number <= $1
		ORDER BY block_number, log_index`

	rows, err := l.pool.Query(ctx, q, maxBlock)
	if err != nil {
		return nil, fmt.Errorf("pending deposits up to %d: %w", maxBlock, err)
	}
	defer rows.Close()

	var deps []Deposit
	for rows.Next() {
		var d Deposit
		var amount string
		if err := rows.Scan(&d.ID, &d.UserID, &d.Address, &d.TxHash, &d.LogIndex, &d.Token,
			&amount, &d.BlockNumber, &d.BlockHash); err != nil {
			return nil, fmt.Errorf("scan deposit: %w", err)
		}
		var ok bool
		if d.Amount, ok = new(big.Int).SetString(amount, 10); !ok {
			return nil, fmt.Errorf("deposit %d: bad amount %q", d.ID, amount)
		}
		deps = append(deps, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pending deposits up to %d: %w", maxBlock, err)
	}
	return deps, nil
}

// ConfirmedDeposits returns the deposits that are confirmed but not credited
// yet, oldest first. Only ID, TxHash, LogIndex, BlockNumber and BlockHash are
// set.
func (l *Ledger) ConfirmedDeposits(ctx context.Context) ([]Deposit, error) {
	const q = `
		SELECT id, tx_hash, log_index, block_number, block_hash
		FROM deposits
		WHERE status = 'confirmed'
		ORDER BY block_number, log_index`

	rows, err := l.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("confirmed deposits: %w", err)
	}
	defer rows.Close()

	var deps []Deposit
	for rows.Next() {
		var d Deposit
		if err := rows.Scan(&d.ID, &d.TxHash, &d.LogIndex, &d.BlockNumber, &d.BlockHash); err != nil {
			return nil, fmt.Errorf("scan deposit: %w", err)
		}
		deps = append(deps, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("confirmed deposits: %w", err)
	}
	return deps, nil
}

// MarkConfirmed records that deposit id has enough confirmations. It moves
// only a pending row whose block hash is still blockHash, else it returns
// ErrNotPending: a block that was replaced must never confirm a deposit.
func (l *Ledger) MarkConfirmed(ctx context.Context, id int64, blockHash string) error {
	const q = `
		UPDATE deposits SET status = 'confirmed', updated_at = now()
		WHERE id = $1 AND status = 'pending' AND block_hash = $2`

	tag, err := l.pool.Exec(ctx, q, id, blockHash)
	if err != nil {
		return fmt.Errorf("mark deposit %d confirmed: %w", id, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("mark deposit %d confirmed: %w", id, ErrNotPending)
	}
	return nil
}

// RevertDeposits handles a reorg: block blockHash, which held tx txHash,
// was replaced. In one transaction it reverts the pending deposits of
// txHash in that block and records moved: the same transfers in the block
// the tx is in now (none when the tx is in no block or failed there). It
// returns ErrNotPending when nothing was reverted, for example because
// another worker did it first.
func (l *Ledger) RevertDeposits(ctx context.Context, txHash, blockHash string, moved []Deposit) error {
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after Commit

	const revert = `
		UPDATE deposits SET status = 'reverted', updated_at = now()
		WHERE tx_hash = $1 AND block_hash = $2 AND status = 'pending'`

	tag, err := tx.Exec(ctx, revert, txHash, blockHash)
	if err != nil {
		return fmt.Errorf("revert deposits of %s: %w", txHash, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("revert deposits of %s: %w", txHash, ErrNotPending)
	}

	// Revert first: a moved transfer may get a log index this tx had in
	// the old block, and insertDeposit uses a reverted row again.
	for _, d := range moved {
		if _, err := insertDeposit(ctx, tx, d); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// Cursor returns the last block the watcher called name has fully
// processed. found is false on its first run.
func (l *Ledger) Cursor(ctx context.Context, name string) (block uint64, found bool, err error) {
	const q = `SELECT block FROM watcher_cursor WHERE name = $1`

	err = l.pool.QueryRow(ctx, q, name).Scan(&block)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("cursor %s: %w", name, err)
	}
	return block, true, nil
}

// RecordBatch stores deps and moves the cursor called name to block, in one
// transaction: after a crash the cursor never points past a deposit that was
// not saved. It returns how many deposits were new.
func (l *Ledger) RecordBatch(ctx context.Context, name string, block uint64, deps []Deposit) (inserted int, err error) {
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after Commit

	for _, d := range deps {
		ok, err := insertDeposit(ctx, tx, d)
		if err != nil {
			return 0, err
		}
		if ok {
			inserted++
		}
	}

	const q = `
		INSERT INTO watcher_cursor (name, block) VALUES ($1, $2)
		ON CONFLICT (name) DO UPDATE SET block = EXCLUDED.block, updated_at = now()`

	if _, err := tx.Exec(ctx, q, name, block); err != nil {
		return 0, fmt.Errorf("save cursor %s: %w", name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return inserted, nil
}

// CreateWithdrawal queues w and takes its amount from the user's balance,
// in one transaction. created is false when userID already has a
// withdrawal with the same idempotency key: nothing is written and id is 0.
// It returns ErrInsufficientBalance, and writes nothing, when the balance
// is less than the amount.
func (l *Ledger) CreateWithdrawal(ctx context.Context, w Withdrawal) (id int64, created bool, err error) {
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after Commit

	const insert = `
		INSERT INTO withdrawals (user_id, idempotency_key, to_address, token, amount)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, idempotency_key) DO NOTHING
		RETURNING id`

	err = tx.QueryRow(ctx, insert, w.UserID, w.IdempotencyKey, w.To, w.Token, w.Amount).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		// DO NOTHING on conflict returns no row: the key was used before,
		// and that first request already took the amount.
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("create withdrawal %d/%s: %w", w.UserID, w.IdempotencyKey, err)
	}

	// One conditional UPDATE: two withdrawals at the same time can't both
	// pass the check. The second waits for the row lock, then checks the
	// new amount.
	const debit = `
		UPDATE balances SET amount = amount - $3, updated_at = now()
		WHERE user_id = $1 AND token = $2 AND amount >= $3`

	tag, err := tx.Exec(ctx, debit, w.UserID, w.Token, w.Amount)
	if err != nil {
		return 0, false, fmt.Errorf("debit withdrawal %d: %w", id, err)
	}
	if tag.RowsAffected() != 1 {
		return 0, false, fmt.Errorf("create withdrawal %d/%s: %w", w.UserID, w.IdempotencyKey, ErrInsufficientBalance)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, false, fmt.Errorf("commit: %w", err)
	}
	return id, true, nil
}

// WithdrawalByKey returns the withdrawal userID created with key. found is
// false when there is none.
func (l *Ledger) WithdrawalByKey(ctx context.Context, userID int64, key string) (w Withdrawal, found bool, err error) {
	// amount as text: see DepositsByAddress.
	const q = `
		SELECT id, user_id, idempotency_key, to_address, token, amount::text, COALESCE(tx_hash, ''), COALESCE(raw_tx, ''), status
		FROM withdrawals
		WHERE user_id = $1 AND idempotency_key = $2`

	var amount string
	err = l.pool.QueryRow(ctx, q, userID, key).Scan(&w.ID, &w.UserID, &w.IdempotencyKey,
		&w.To, &w.Token, &amount, &w.TxHash, &w.RawTx, &w.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Withdrawal{}, false, nil
	}
	if err != nil {
		return Withdrawal{}, false, fmt.Errorf("withdrawal %d/%s: %w", userID, key, err)
	}
	var ok bool
	if w.Amount, ok = new(big.Int).SetString(amount, 10); !ok {
		return Withdrawal{}, false, fmt.Errorf("withdrawal %d: bad amount %q", w.ID, amount)
	}
	return w, true, nil
}

// MarkSigned stores the signed tx of withdrawal id before it is broadcast,
// so after a failed or interrupted broadcast its hash can be looked up on
// the chain. It moves only a queued row, else it returns ErrNotQueued.
func (l *Ledger) MarkSigned(ctx context.Context, id int64, nonce uint64, txHash, rawTx string) error {
	const q = `
		UPDATE withdrawals SET status = 'signed', nonce = $2, tx_hash = $3, raw_tx = $4, updated_at = now()
		WHERE id = $1 AND status = 'queued'`

	tag, err := l.pool.Exec(ctx, q, id, nonce, txHash, rawTx)
	if err != nil {
		return fmt.Errorf("mark withdrawal %d signed: %w", id, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("mark withdrawal %d signed: %w", id, ErrNotQueued)
	}
	return nil
}

// MarkBroadcast records that the signed tx txHash of withdrawal id is on
// the network. It moves only a signed row with that hash, else it returns
// ErrNotSigned.
func (l *Ledger) MarkBroadcast(ctx context.Context, id int64, txHash string) error {
	const q = `
		UPDATE withdrawals SET status = 'broadcast', updated_at = now()
		WHERE id = $1 AND status = 'signed' AND tx_hash = $2`

	tag, err := l.pool.Exec(ctx, q, id, txHash)
	if err != nil {
		return fmt.Errorf("mark withdrawal %d broadcast: %w", id, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("mark withdrawal %d broadcast: %w", id, ErrNotSigned)
	}
	return nil
}

// MarkFailed records that withdrawal id never sent and never will: it was
// not signed, or its nonce went to another tx. It gives the amount back to
// the user's balance, in the same transaction. It moves only a queued or
// signed row, else it returns ErrNotQueued, so the amount is given back once.
func (l *Ledger) MarkFailed(ctx context.Context, id int64) error {
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after Commit

	const fail = `
		UPDATE withdrawals SET status = 'failed', updated_at = now()
		WHERE id = $1 AND status IN ('queued', 'signed')`

	tag, err := tx.Exec(ctx, fail, id)
	if err != nil {
		return fmt.Errorf("mark withdrawal %d failed: %w", id, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("mark withdrawal %d failed: %w", id, ErrNotQueued)
	}

	const refund = `
		INSERT INTO balances (user_id, token, amount)
		SELECT user_id, token, amount FROM withdrawals WHERE id = $1
		ON CONFLICT (user_id, token) DO UPDATE
		SET amount = balances.amount + EXCLUDED.amount, updated_at = now()`

	if _, err := tx.Exec(ctx, refund, id); err != nil {
		return fmt.Errorf("give back withdrawal %d: %w", id, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (l *Ledger) MarkCredited(ctx context.Context, id int64, head uint64) error {
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after Commit

	const credit = `
		UPDATE deposits
		SET status = 'credited', credited_at_block = $2, updated_at = now()
		WHERE id = $1 AND status = 'confirmed'`

	tag, err := tx.Exec(ctx, credit, id, head)
	if err != nil {
		return fmt.Errorf("credit deposit %d: %w", id, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("credit deposit %d: %w", id, ErrNotConfirmed)
	}

	const addBalance = `
		INSERT INTO balances (user_id, token, amount)
		SELECT user_id, token, amount FROM deposits WHERE id = $1
		ON CONFLICT (user_id, token) DO UPDATE
		SET amount = balances.amount + EXCLUDED.amount, updated_at = now()`

	if _, err := tx.Exec(ctx, addBalance, id); err != nil {
		return fmt.Errorf("add deposit %d to balance: %w", id, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

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

// ErrAddressTaken means the address already belongs to a user.
var ErrAddressTaken = errors.New("address already assigned")

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
// same (tx_hash, log_index) already exists.
func (l *Ledger) InsertDeposit(ctx context.Context, d Deposit) (inserted bool, err error) {
	return insertDeposit(ctx, l.pool, d)
}

func insertDeposit(ctx context.Context, db dbtx, d Deposit) (inserted bool, err error) {
	const q = `
		INSERT INTO deposits (user_id, address, tx_hash, log_index, token, amount, block_number, block_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (tx_hash, log_index) DO NOTHING`

	tag, err := db.Exec(ctx, q,
		d.UserID, d.Address, d.TxHash, d.LogIndex, d.Token, d.Amount, d.BlockNumber, d.BlockHash)
	if err != nil {
		return false, fmt.Errorf("insert deposit %s:%d: %w", d.TxHash, d.LogIndex, err)
	}

	// DO NOTHING on conflict means 0 rows affected.
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

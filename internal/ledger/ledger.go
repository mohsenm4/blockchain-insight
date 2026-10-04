package ledger

import (
	"context"
	"fmt"
	"math/big"

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
}

type Ledger struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Ledger {
	return &Ledger{pool: pool}
}

// InsertDeposit stores d. It returns inserted=false when a row with the
// same (tx_hash, log_index) already exists.
func (l *Ledger) InsertDeposit(ctx context.Context, d Deposit) (inserted bool, err error) {
	const q = `
		INSERT INTO deposits (user_id, address, tx_hash, log_index, token, amount, block_number, block_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (tx_hash, log_index) DO NOTHING`

	tag, err := l.pool.Exec(ctx, q,
		d.UserID, d.Address, d.TxHash, d.LogIndex, d.Token, d.Amount, d.BlockNumber, d.BlockHash)
	if err != nil {
		return false, fmt.Errorf("insert deposit %s:%d: %w", d.TxHash, d.LogIndex, err)
	}

	// DO NOTHING on conflict means 0 rows affected.
	return tag.RowsAffected() == 1, nil
}

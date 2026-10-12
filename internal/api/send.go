package api

import (
	"context"
	"errors"
	"log/slog"
	"math/big"
	"net/http"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/gin-gonic/gin"
	"github.com/mohsenm4/blockchain-insight/internal/ledger"
	"github.com/mohsenm4/blockchain-insight/internal/sender"
)

type Sender interface {
	Sign(ctx context.Context, to common.Address, amount *big.Int) (*types.Transaction, error)
	Broadcast(ctx context.Context, tx *types.Transaction) error
	Check(ctx context.Context, tx *types.Transaction) (sender.TxState, error)
	ResetNonce()
	Status(ctx context.Context, hash common.Hash) (string, error)
}

type sendRequest struct {
	UserID         int64  `json:"user_id" binding:"required,gt=0"`
	IdempotencyKey string `json:"idempotency_key" binding:"required,max=255"`
	To             string `json:"to" binding:"required"`
	Amount         string `json:"amount" binding:"required"`
}

type withdrawalResponse struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
	TxHash string `json:"tx_hash,omitempty"`
}

// PostSend sends tokens at most once per (user_id, idempotency_key). A
// retried request gets the first request's result instead of a second
// transfer.
func (s *Server) PostSend(c *gin.Context) {
	var req sendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	if !common.IsHexAddress(req.To) {
		c.JSON(400, gin.H{"error": "invalid address"})
		return
	}

	amount, ok := new(big.Int).SetString(req.Amount, 10)
	if !ok || amount.Cmp(big.NewInt(0)) <= 0 {
		c.JSON(400, gin.H{"error": "invalid amount"})
		return
	}

	ctx := c.Request.Context()
	w := ledger.Withdrawal{
		UserID:         req.UserID,
		IdempotencyKey: req.IdempotencyKey,
		To:             ledger.AddressKey(common.HexToAddress(req.To)),
		Token:          ledger.AddressKey(common.HexToAddress(s.config.TokenAddress)),
		Amount:         amount,
	}
	id, created, err := s.ledger.CreateWithdrawal(ctx, w)
	if err != nil {
		slog.Error("create withdrawal", "user_id", req.UserID, "key", req.IdempotencyKey, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create withdrawal"})
		return
	}
	if !created {
		s.replayWithdrawal(c, w)
		return
	}

	// Sign and save before sending: once the hash is stored, a broadcast
	// that fails halfway can be checked on the chain instead of guessed.
	tx, err := s.sender.Sign(ctx, common.HexToAddress(req.To), amount)
	if err != nil {
		// Nothing was signed, so nothing left.
		slog.Error("sign", "withdrawal", id, "err", err)
		s.markFailed(ctx, id)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "transfer failed", "id": id, "status": "failed"})
		return
	}
	raw, err := tx.MarshalBinary()
	if err == nil {
		err = s.ledger.MarkSigned(ctx, id, tx.Nonce(), tx.Hash().Hex(), hexutil.Encode(raw))
	}
	if err != nil {
		// Not saved, so never sent: give its nonce back.
		slog.Error("mark signed", "withdrawal", id, "tx", tx.Hash().Hex(), "err", err)
		s.sender.ResetNonce()
		s.markFailed(ctx, id)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "transfer failed", "id": id, "status": "failed"})
		return
	}

	// The tx may reach the node even if this request is cancelled now.
	ctx = context.WithoutCancel(ctx)
	if err := s.sender.Broadcast(ctx, tx); err != nil {
		slog.Warn("broadcast", "withdrawal", id, "tx", tx.Hash().Hex(), "err", err)
		// Next Sign reads the nonce from the chain; if this tx lands
		// after all, the chain counts it.
		s.sender.ResetNonce()
		s.settleSigned(c, id, tx)
		return
	}
	s.markBroadcast(ctx, id, tx)
	c.JSON(http.StatusOK, withdrawalResponse{ID: id, Status: "broadcast", TxHash: tx.Hash().Hex()})
}

// settleSigned answers for a signed withdrawal whose broadcast failed or
// was interrupted. It asks the chain whether tx left, and resends the same
// tx if it still can land: same nonce, so it pays at most once.
func (s *Server) settleSigned(c *gin.Context, id int64, tx *types.Transaction) {
	ctx := context.WithoutCancel(c.Request.Context())
	hash := tx.Hash().Hex()
	unknown := func(err error) {
		slog.Error("settle withdrawal", "withdrawal", id, "tx", hash, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "transfer state unknown, retry with the same key", "id": id, "status": "signed", "tx_hash": hash,
		})
	}

	state, err := s.sender.Check(ctx, tx)
	if err != nil {
		unknown(err)
		return
	}
	switch state {
	case sender.TxSent:
		slog.Info("settle withdrawal: tx is on the network", "withdrawal", id, "tx", hash)
	case sender.TxDropped:
		slog.Warn("settle withdrawal: nonce taken by another tx", "withdrawal", id, "tx", hash, "nonce", tx.Nonce())
		s.markFailed(ctx, id)
		c.JSON(http.StatusOK, withdrawalResponse{ID: id, Status: "failed", TxHash: hash})
		return
	case sender.TxNotSeen:
		if err := s.sender.Broadcast(ctx, tx); err != nil {
			unknown(err)
			return
		}
		slog.Info("settle withdrawal: resent", "withdrawal", id, "tx", hash)
	}
	s.markBroadcast(ctx, id, tx)
	c.JSON(http.StatusOK, withdrawalResponse{ID: id, Status: "broadcast", TxHash: hash})
}

// markBroadcast records that tx left. A failure is only logged: the money
// is out, and a 500 could make the client retry with a new key and pay
// twice.
func (s *Server) markBroadcast(ctx context.Context, id int64, tx *types.Transaction) {
	if err := s.ledger.MarkBroadcast(ctx, id, tx.Hash().Hex()); err != nil {
		slog.Error("mark broadcast", "withdrawal", id, "tx", tx.Hash().Hex(), "err", err)
	}
}

func (s *Server) markFailed(ctx context.Context, id int64) {
	if err := s.ledger.MarkFailed(context.WithoutCancel(ctx), id); err != nil {
		slog.Error("mark failed", "withdrawal", id, "err", err)
	}
}

// replayWithdrawal answers a request whose idempotency key was used before,
// from the stored row. It only ever resends the tx signed the first time.
func (s *Server) replayWithdrawal(c *gin.Context, req ledger.Withdrawal) {
	w, found, err := s.ledger.WithdrawalByKey(c.Request.Context(), req.UserID, req.IdempotencyKey)
	if err == nil && !found {
		err = errors.New("conflict on insert but no row") // rows are never deleted
	}
	if err != nil {
		slog.Error("withdrawal by key", "user_id", req.UserID, "key", req.IdempotencyKey, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load withdrawal"})
		return
	}

	if w.To != req.To || w.Amount.Cmp(req.Amount) != 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "idempotency_key already used for a different request"})
		return
	}
	switch w.Status {
	case "queued":
		// The first request is still signing. Nothing has left yet.
		c.JSON(http.StatusConflict, gin.H{"error": "withdrawal in progress, retry later", "id": w.ID, "status": w.Status})
	case "signed":
		tx, err := decodeTx(w.RawTx)
		if err != nil {
			slog.Error("decode signed tx", "withdrawal", w.ID, "err", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load withdrawal"})
			return
		}
		s.settleSigned(c, w.ID, tx)
	default:
		c.JSON(http.StatusOK, withdrawalResponse{ID: w.ID, Status: w.Status, TxHash: w.TxHash})
	}
}

func decodeTx(raw string) (*types.Transaction, error) {
	b, err := hexutil.Decode(raw)
	if err != nil {
		return nil, err
	}
	tx := new(types.Transaction)
	if err := tx.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return tx, nil
}

func (s *Server) GetTx(c *gin.Context) {
	h := c.Param("hash")
	b, err := hexutil.Decode(h)
	if err != nil || len(b) != common.HashLength {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tx hash"})
		return
	}

	status, err := s.sender.Status(c.Request.Context(), common.HexToHash(h))
	if err != nil {
		slog.Error("tx status", "hash", h, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "status lookup failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tx_hash": h, "status": status})
}

package api

import (
	"context"
	"errors"
	"log/slog"
	"math/big"
	"net/http"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gin-gonic/gin"
	"github.com/mohsenm4/blockchain-insight/internal/ledger"
)

type Sender interface {
	Transfer(ctx context.Context, to common.Address, amount *big.Int) (common.Hash, error)
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

	txHash, err := s.sender.Transfer(ctx, common.HexToAddress(req.To), amount)
	if err != nil {
		// The row stays queued: the tx may or may not have left, so a retry
		// with this key must not send again.
		slog.Error("transfer", "withdrawal", id, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "transfer failed"})
		return
	}

	// The money is out: record it even if the client has gone away.
	if err := s.ledger.MarkBroadcast(context.WithoutCancel(ctx), id, txHash.Hex()); err != nil {
		// Still answer with the hash. A 500 here could make the client retry
		// with a new key and pay twice.
		slog.Error("mark broadcast", "withdrawal", id, "tx", txHash.Hex(), "err", err)
	}
	c.JSON(http.StatusOK, withdrawalResponse{ID: id, Status: "broadcast", TxHash: txHash.Hex()})
}

// replayWithdrawal answers a request whose idempotency key was used before,
// from the stored row. It never sends.
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
	if w.Status == "queued" {
		// The first request is still sending, or died before it could
		// record the result. Either way there is no answer to give yet.
		c.JSON(http.StatusConflict, gin.H{"error": "withdrawal in progress, retry later", "id": w.ID, "status": w.Status})
		return
	}
	c.JSON(http.StatusOK, withdrawalResponse{ID: w.ID, Status: w.Status, TxHash: w.TxHash})
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

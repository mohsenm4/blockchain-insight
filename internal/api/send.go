package api

import (
	"context"
	"log/slog"
	"math/big"
	"net/http"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gin-gonic/gin"
)

type Sender interface {
	Transfer(ctx context.Context, to common.Address, amount *big.Int) (common.Hash, error)
	Status(ctx context.Context, hash common.Hash) (string, error)
}

type sendRequest struct {
	To     string `json:"to" binding:"required"`
	Amount string `json:"amount" binding:"required"`
}

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

	txHash, err := s.sender.Transfer(c.Request.Context(), common.HexToAddress(req.To), amount)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"tx_hash": txHash.Hex()})
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

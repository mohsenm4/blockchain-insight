package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Mohsen20031203/blockchain-insight/internal/contracts/erc20"
	"github.com/Mohsen20031203/blockchain-insight/internal/watch"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
)

type watchRequest struct {
	Address string `json:"address" binding:"required"`
}

type transferResponse struct {
	Block uint64 `json:"block"`
	Index uint   `json:"index"`
	From  string `json:"from"`
	To    string `json:"to"`
	Value string `json:"value"` // string: big.Int does not survive JSON numbers
	Tx    string `json:"tx"`
}

// PostWatch registers an address to watch for incoming transfers.
func (s *Server) PostWatch(c *gin.Context) {
	var req watchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !common.IsHexAddress(req.Address) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid address"})
		return
	}

	s.store.Watch(common.HexToAddress(req.Address))
	c.JSON(http.StatusOK, gin.H{"watching": req.Address})
}

// GetWatchTransfers returns the transfers seen so far for a watched address.
func (s *Server) GetWatchTransfers(c *gin.Context) {
	addr := c.Param("address")
	if !common.IsHexAddress(addr) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid address"})
		return
	}

	trs := s.store.Transfers(common.HexToAddress(addr))
	out := make([]transferResponse, 0, len(trs))
	for _, tr := range trs {
		out = append(out, transferResponse{
			Block: tr.Block,
			Index: tr.Index,
			From:  tr.From.Hex(),
			To:    tr.To.Hex(),
			Value: tr.Value.String(),
			Tx:    tr.Tx.Hex(),
		})
	}
	c.JSON(http.StatusOK, gin.H{"transfers": out})
}

const sepoliaUSDC = "0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238" // TODO: move to config

// StartWatcher polls for ERC20 transfers in the background until ctx is
// cancelled, feeding every transfer into the server's store.
func (s *Server) StartWatcher(ctx context.Context) error {
	client, err := ethclient.DialContext(ctx, s.config.RPCURL)
	if err != nil {
		return err
	}

	token, err := erc20.NewERC20(common.HexToAddress(sepoliaUSDC), client)
	if err != nil {
		client.Close()
		return err
	}

	out := make(chan watch.Transfer)

	go func() {
		defer client.Close()
		defer close(out)
		if err := watch.Run(ctx, watch.NewERC20Source(client, token), 10*time.Second, out); err != nil {
			slog.Info("watcher stopped", "err", err)
		}
	}()

	go func() {
		for tr := range out {
			s.store.Add(tr)
		}
	}()

	return nil
}

package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/mohsenm4/blockchain-insight/internal/contracts/erc20"
	"github.com/mohsenm4/blockchain-insight/internal/ledger"
	"github.com/mohsenm4/blockchain-insight/internal/watch"
)

// Ledger is the part of ledger.Ledger the server needs.
type Ledger interface {
	AssignAddress(ctx context.Context, userID int64, addr common.Address) error
	UserByAddress(ctx context.Context, addr common.Address) (userID int64, found bool, err error)
	DepositsByAddress(ctx context.Context, addr common.Address) ([]ledger.Deposit, error)
	Cursor(ctx context.Context, name string) (block uint64, found bool, err error)
	RecordBatch(ctx context.Context, name string, block uint64, deps []ledger.Deposit) (inserted int, err error)
	CreateWithdrawal(ctx context.Context, w ledger.Withdrawal) (id int64, created bool, err error)
	WithdrawalByKey(ctx context.Context, userID int64, key string) (w ledger.Withdrawal, found bool, err error)
	MarkBroadcast(ctx context.Context, id int64, txHash string) error
}

type watchRequest struct {
	Address string `json:"address" binding:"required"`
	UserID  int64  `json:"user_id" binding:"required,gt=0"`
}

type depositResponse struct {
	Block  uint64 `json:"block"`
	Index  uint   `json:"index"`
	To     string `json:"to"`
	Value  string `json:"value"` // string: big.Int does not survive JSON numbers
	Tx     string `json:"tx"`
	Status string `json:"status"`
}

// PostWatch makes an address a deposit address of a user.
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

	err := s.ledger.AssignAddress(c.Request.Context(), req.UserID, common.HexToAddress(req.Address))
	if errors.Is(err, ledger.ErrAddressTaken) {
		c.JSON(http.StatusConflict, gin.H{"error": "address already assigned"})
		return
	}
	if err != nil {
		slog.Error("assign address", "address", req.Address, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not watch address"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"watching": req.Address})
}

// GetWatchTransfers returns the deposits recorded so far for an address.
func (s *Server) GetWatchTransfers(c *gin.Context) {
	addr := c.Param("address")
	if !common.IsHexAddress(addr) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid address"})
		return
	}

	deps, err := s.ledger.DepositsByAddress(c.Request.Context(), common.HexToAddress(addr))
	if err != nil {
		slog.Error("deposits by address", "address", addr, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load deposits"})
		return
	}

	out := make([]depositResponse, 0, len(deps))
	for _, d := range deps {
		out = append(out, depositResponse{
			Block:  d.BlockNumber,
			Index:  d.LogIndex,
			To:     d.Address,
			Value:  d.Amount.String(),
			Tx:     d.TxHash,
			Status: d.Status,
		})
	}
	c.JSON(http.StatusOK, gin.H{"transfers": out})
}

const sepoliaUSDC = "0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238" // TODO: move to config

// watcherName is this watcher's row in watcher_cursor.
const watcherName = "erc20:" + sepoliaUSDC

// StartWatcher polls for ERC20 transfers in the background until ctx is
// cancelled, recording deposits to our addresses in the ledger. It resumes
// after the block saved in the cursor, or from the current head on the
// first run.
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
	src := watch.NewERC20Source(client, token)

	last, found, err := s.ledger.Cursor(ctx, watcherName)
	if err != nil {
		client.Close()
		return err
	}
	if !found {
		// First run: start from now instead of scanning the whole chain.
		if last, err = src.BlockNumber(ctx); err != nil {
			client.Close()
			return err
		}
	}
	slog.Info("watcher starting", "after_block", last, "resumed", found)

	go func() {
		defer client.Close()
		if err := watch.Run(ctx, src, last, 10*time.Second, s.recordBatch); err != nil {
			slog.Info("watcher stopped", "err", err)
		}
	}()

	return nil
}

// recordBatch keeps the transfers that land on our deposit addresses and
// moves the cursor to b.To, in one transaction.
func (s *Server) recordBatch(ctx context.Context, b watch.Batch) error {
	var deps []ledger.Deposit
	for _, tr := range b.Transfers {
		userID, found, err := s.ledger.UserByAddress(ctx, tr.To)
		if err != nil {
			return err
		}
		if !found {
			continue // not one of our addresses: not a deposit
		}
		deps = append(deps, ledger.Deposit{
			UserID:      userID,
			Address:     ledger.AddressKey(tr.To),
			TxHash:      tr.Tx.Hex(),
			LogIndex:    tr.Index,
			Token:       ledger.AddressKey(common.HexToAddress(sepoliaUSDC)),
			Amount:      tr.Value,
			BlockNumber: tr.Block,
			BlockHash:   tr.BlockHash.Hex(),
		})
	}

	inserted, err := s.ledger.RecordBatch(ctx, watcherName, b.To, deps)
	if err != nil {
		return err
	}
	if inserted > 0 {
		slog.Info("deposits recorded", "count", inserted, "from", b.From, "to", b.To)
	}
	return nil
}

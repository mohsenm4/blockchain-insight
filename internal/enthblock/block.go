package enthblock

import (
	"context"
	"math/big"

	"github.com/Mohsen20031203/blockchain-insight/internal/models"
	"github.com/ethereum/go-ethereum/core/types"
)

func (c *Client) GetLatestBlockNumber() (uint64, error) {
	blockNumber, err := c.Eth.BlockNumber(context.Background())
	if err != nil {
		return 0, err
	}
	return blockNumber, nil
}

func (c *Client) GetBlockByNumber(blockNumber uint64) (*models.Block, error) {
	block, err := c.Eth.BlockByNumber(context.Background(), big.NewInt(int64(blockNumber)))
	if err != nil {
		return nil, err
	}

	return &models.Block{
		Number:       block.Number().Uint64(),
		Hash:         block.Hash().Hex(),
		TxCount:      len(block.Transactions()),
		Timestamp:    block.Time(),
		Transactions: convertTxs(block.Transactions()),
	}, nil
}

func convertTxs(txs types.Transactions) []models.Transaction {
	var result []models.Transaction
	for _, tx := range txs {
		to := ""
		if tx.To() != nil {
			to = tx.To().Hex()
		}

		from := ""
		signer := types.LatestSignerForChainID(tx.ChainId())
		sender, err := types.Sender(signer, tx)
		if err == nil {
			from = sender.Hex()
		}

		value := tx.Value().String()
		result = append(result, models.Transaction{
			Hash:   tx.Hash().Hex(),
			Value:  value,
			From:   from,
			To:     to,
			GasFee: tx.GasPrice().String(),
		})
	}
	return result
}

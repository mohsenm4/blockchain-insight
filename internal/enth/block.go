package enth

import (
	"context"
	"math/big"

	"github.com/Mohsen20031203/blockchain-insight/internal/models"
)

func (c *Client) GetlastBlockNumber() (uint64, error) {
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
	transaction := make([]models.Transaction, 0)
	for _, tx := range block.Transactions() {
		transaction = append(transaction, c.ConvertTx(tx))
	}

	blockchange := &models.Block{
		Number:       block.Number().Uint64(),
		Hash:         block.Hash().Hex(),
		TxCount:      len(block.Transactions()),
		Timestamp:    block.Time(),
		Transactions: transaction,
	}

	return blockchange, nil
}

package enthblock

import (
	"context"

	"github.com/Mohsen20031203/blockchain-insight/internal/models"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func (c *Client) GetTxByHash(hash string) (*types.Transaction, error) {

	hexHash := common.HexToHash(hash)
	trans, ok, err := c.Eth.TransactionByHash(context.Background(), hexHash)
	if err != nil || !ok {
		return nil, err
	}
	return trans, nil
}

func (c *Client) ConvertTx(tx *types.Transaction) models.Transaction {
	var result models.Transaction
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
	result = models.Transaction{
		Hash:   tx.Hash().Hex(),
		Value:  value,
		From:   from,
		To:     to,
		GasFee: tx.GasPrice().String(),
	}

	return result
}

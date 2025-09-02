package enthblock

import (
	"context"

	"github.com/Mohsen20031203/blockchain-insight/internal/models"
	"github.com/ethereum/go-ethereum/ethclient"
)

func GetLatestBlockNumber(client *ethclient.Client) (uint64, error) {
	blockNumber, err := client.BlockNumber(context.Background())
	if err != nil {
		return 0, err
	}
	return blockNumber, nil
}

func GetBlockByNumber(client *ethclient.Client, blockNumber uint64) (*models.Block, error) {

	block, err := client.BlockByNumber(context.Background(), nil)
	if err != nil {
		return nil, err
	}

	return &models.Block{
		Number:       block.Number().Uint64(),
		Hash:         block.Hash().Hex(),
		TxCount:      len(block.Transactions()),
		Timestamp:    block.Time(),
		Transactions: nil,
	}, nil
}

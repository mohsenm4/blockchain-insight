package api

import (
	"math/big"

	"github.com/Mohsen20031203/blockchain-insight/internal/models"
)

type EthClient interface {
	GetBlockByNumber(blockNumber uint64) (*models.Block, error)
	GetLastBlockNumber() (uint64, error)
	GetBalance(address string) (*big.Int, error)
}

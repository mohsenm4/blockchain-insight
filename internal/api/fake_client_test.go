package api

import (
	"math/big"

	"github.com/Mohsen20031203/blockchain-insight/internal/models"
)

type fakeClient struct {
	block           *models.Block
	lastBlockNumber uint64
	balance         *big.Int
	err             error
}

func (f *fakeClient) GetBalance(address string) (*big.Int, error) {
	return f.balance, f.err
}

func (f *fakeClient) GetBlockByNumber(number uint64) (*models.Block, error) {
	return f.block, f.err
}

func (f *fakeClient) GetLastBlockNumber() (uint64, error) {
	return f.lastBlockNumber, f.err
}

package api

import (
	"math/big"
	"sync/atomic"
	"time"

	"github.com/Mohsen20031203/blockchain-insight/internal/models"
)

type fakeClient struct {
	block             *models.Block
	lastBlockNumber   uint64
	balance           *big.Int
	err               error
	getLastBlockCount atomic.Int64
	getBlockCount     atomic.Int64
	latency           time.Duration
}

func (f *fakeClient) GetBalance(address string) (*big.Int, error) {
	return f.balance, f.err
}

func (f *fakeClient) GetBlockByNumber(number uint64) (*models.Block, error) {
	f.getBlockCount.Add(1)
	if f.latency > 0 {
		time.Sleep(f.latency)
	}
	return f.block, f.err
}

func (f *fakeClient) GetLastBlockNumber() (uint64, error) {
	f.getLastBlockCount.Add(1)
	if f.latency > 0 {
		time.Sleep(f.latency)
	}
	return f.lastBlockNumber, f.err
}

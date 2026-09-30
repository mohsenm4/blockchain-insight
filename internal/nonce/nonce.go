package nonce

import (
	"context"
	"sync"

	"github.com/ethereum/go-ethereum/common"
)

type NonceProvider interface {
	PendingNonceAt(ctx context.Context, addr common.Address) (uint64, error)
}

type NonceManager struct {
	nextNonce   uint64
	mu          sync.Mutex
	initialized bool
	provider    NonceProvider
	addr        common.Address
}

func NewNonceManager(addr common.Address, provider NonceProvider) *NonceManager {
	return &NonceManager{
		addr:     addr,
		provider: provider,
	}
}

func (nm *NonceManager) NextNonce(ctx context.Context) (uint64, error) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if !nm.initialized {
		nonce, err := nm.provider.PendingNonceAt(ctx, nm.addr)
		if err != nil {
			return 0, err
		}
		nm.nextNonce = nonce
		nm.initialized = true
	}

	nonce := nm.nextNonce
	nm.nextNonce++
	return nonce, nil
}

func (nm *NonceManager) Reset() {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	nm.initialized = false
}

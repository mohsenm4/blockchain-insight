package watch

import (
	"sync"

	"github.com/ethereum/go-ethereum/common"
)

// Store keeps watched addresses and the transfers seen for them.
// It is safe for concurrent use.
type Store struct {
	mu        sync.RWMutex
	watched   map[common.Address]bool
	transfers map[common.Address][]Transfer
}

func NewStore() *Store {
	return &Store{
		watched:   make(map[common.Address]bool),
		transfers: make(map[common.Address][]Transfer),
	}
}

func (s *Store) Watch(addr common.Address) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watched[addr] = true
}

// Add records tr if its recipient is watched.
func (s *Store) Add(tr Transfer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.watched[tr.To] {
		return
	}
	s.transfers[tr.To] = append(s.transfers[tr.To], tr)
}

func (s *Store) Transfers(addr common.Address) []Transfer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Transfer(nil), s.transfers[addr]...) // copy: caller must not see our slice
}

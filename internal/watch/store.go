package watch

import (
	"sync"

	"github.com/ethereum/go-ethereum/common"
)

// eventKey identifies one log uniquely: a tx can contain many transfers.
type eventKey struct {
	Tx    common.Hash
	Index uint
}

// Store keeps watched addresses and the transfers seen for them.
// It is safe for concurrent use.
type Store struct {
	mu        sync.RWMutex
	watched   map[common.Address]bool
	transfers map[common.Address][]Transfer
	seen      map[eventKey]bool
}

func NewStore() *Store {
	return &Store{
		watched:   make(map[common.Address]bool),
		transfers: make(map[common.Address][]Transfer),
		seen:      make(map[eventKey]bool),
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
	key := eventKey{Tx: tr.Tx, Index: tr.Index}
	if s.seen[key] {
		return
	}
	s.seen[key] = true

	s.transfers[tr.To] = append(s.transfers[tr.To], tr)
}

func (s *Store) Transfers(addr common.Address) []Transfer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Transfer(nil), s.transfers[addr]...) // copy: caller must not see our slice
}

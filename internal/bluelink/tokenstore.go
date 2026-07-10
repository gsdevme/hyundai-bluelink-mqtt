package bluelink

import (
	"context"
	"sync"
)

// TokenStore persists authentication tokens so restarts avoid a fresh, rate
// limited login. Implementations must be safe for concurrent use.
type TokenStore interface {
	// Load returns the stored tokens. The bool is false when nothing is stored.
	Load(ctx context.Context) (Tokens, bool, error)
	// Save persists tokens, overwriting any previous value.
	Save(ctx context.Context, t Tokens) error
}

// memoryStore is an in-process TokenStore for local dev and tests.
type memoryStore struct {
	mu     sync.Mutex
	tokens Tokens
	set    bool
}

// NewMemoryStore returns an in-memory TokenStore.
func NewMemoryStore() TokenStore { return &memoryStore{} }

func (m *memoryStore) Load(context.Context) (Tokens, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tokens, m.set, nil
}

func (m *memoryStore) Save(_ context.Context, t Tokens) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokens = t
	m.set = true
	return nil
}

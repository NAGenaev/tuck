package physical

import (
	"context"
	"strings"
	"sync"
)

// InMem is an in-memory Backend, used by tests and ephemeral runs.
type InMem struct {
	mu   sync.RWMutex
	data map[string][]byte
}

// NewInMem returns an empty in-memory backend.
func NewInMem() *InMem {
	return &InMem{data: make(map[string][]byte)}
}

func (m *InMem) Get(_ context.Context, key string) (*Entry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[key]
	if !ok {
		return nil, nil
	}
	val := make([]byte, len(v))
	copy(val, v)
	return &Entry{Key: key, Value: val}, nil
}

func (m *InMem) Put(_ context.Context, entry *Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	val := make([]byte, len(entry.Value))
	copy(val, entry.Value)
	m.data[entry.Key] = val
	return nil
}

func (m *InMem) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *InMem) List(_ context.Context, prefix string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var keys []string
	for k := range m.data {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

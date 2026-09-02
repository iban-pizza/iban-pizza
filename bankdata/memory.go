package bankdata

import (
	"context"
	"sort"
	"strings"
	"sync"
)

// MemoryStore holds bank data in maps. It backs both the embedded snapshot and
// a snapshot loaded from disk, and it is the only backend that needs no
// external service, which is what makes the zero configuration binary possible.
//
// It is safe for concurrent use. Replace swaps one source at a time under a
// write lock; reads take a read lock and never block each other.
type MemoryStore struct {
	mu      sync.RWMutex
	byKey   map[Key]Bank
	byBIC   map[string][]Bank
	sources map[string]SourceInfo
}

var (
	_ Repository = (*MemoryStore)(nil)
	_ Writer     = (*MemoryStore)(nil)
)

// NewMemoryStore returns an empty store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		byKey:   make(map[Key]Bank),
		byBIC:   make(map[string][]Bank),
		sources: make(map[string]SourceInfo),
	}
}

func (s *MemoryStore) Find(_ context.Context, key Key) (Bank, error) {
	key = key.Normalize()
	s.mu.RLock()
	defer s.mu.RUnlock()

	b, ok := s.byKey[key]
	if !ok {
		return Bank{}, ErrNotFound
	}
	return b, nil
}

func (s *MemoryStore) FindByBIC(_ context.Context, bic string) ([]Bank, error) {
	bic = strings.ToUpper(strings.TrimSpace(bic))
	s.mu.RLock()
	defer s.mu.RUnlock()

	banks, ok := s.byBIC[bic]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]Bank(nil), banks...), nil
}

func (s *MemoryStore) Search(_ context.Context, q Query) ([]Bank, error) {
	limit := q.clampLimit()
	country := strings.ToUpper(strings.TrimSpace(q.Country))
	bic := strings.ToUpper(strings.TrimSpace(q.BIC))
	name := strings.ToLower(strings.TrimSpace(q.Name))

	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Bank, 0, min(limit, len(s.byKey)))
	for _, b := range s.byKey {
		if country != "" && b.Country != country {
			continue
		}
		if bic != "" && !strings.HasPrefix(b.BIC, bic) {
			continue
		}
		if name != "" && !strings.Contains(strings.ToLower(b.Name), name) {
			continue
		}
		out = append(out, b)
	}

	// Map iteration order is random, so sort before truncating. Without this a
	// caller paging through results would see a different arbitrary subset on
	// every request.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Country != out[j].Country {
			return out[i].Country < out[j].Country
		}
		return out[i].BankCode < out[j].BankCode
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *MemoryStore) Stats(_ context.Context) (Stats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	st := Stats{TotalRecords: len(s.byKey), Sources: make([]SourceInfo, 0, len(s.sources))}
	for _, src := range s.sources {
		st.Sources = append(st.Sources, src)
	}
	sort.Slice(st.Sources, func(i, j int) bool { return st.Sources[i].Country < st.Sources[j].Country })
	return st, nil
}

// Replace swaps every record belonging to info.Name for the given banks.
func (s *MemoryStore) Replace(_ context.Context, info SourceInfo, banks []Bank) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, b := range s.byKey {
		if b.Source == info.Name {
			delete(s.byKey, key)
		}
	}

	for _, b := range banks {
		s.byKey[b.Key().Normalize()] = b
	}

	info.RecordCount = 0
	for _, b := range s.byKey {
		if b.Source == info.Name {
			info.RecordCount++
		}
	}
	s.sources[info.Name] = info

	s.reindexBIC()
	return nil
}

// reindexBIC rebuilds the BIC index. The caller must hold the write lock.
func (s *MemoryStore) reindexBIC() {
	s.byBIC = make(map[string][]Bank, len(s.byKey))
	for _, b := range s.byKey {
		if b.BIC == "" {
			continue
		}
		s.byBIC[b.BIC] = append(s.byBIC[b.BIC], b)
	}
	for bic := range s.byBIC {
		banks := s.byBIC[bic]
		sort.Slice(banks, func(i, j int) bool {
			if banks[i].Country != banks[j].Country {
				return banks[i].Country < banks[j].Country
			}
			return banks[i].BankCode < banks[j].BankCode
		})
	}
}

// Len reports how many records the store holds.
func (s *MemoryStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.byKey)
}

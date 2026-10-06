package token

import "context"

// DefaultCountLimit bounds how many token records CountByNamespace decodes.
const DefaultCountLimit = 20000

// RootKey is the CountByNamespace map key for tokens bound to the root
// namespace (their Namespace field is empty).
const RootKey = ""

// CountByNamespace counts the live (not expired) tokens per namespace. Token
// IDs are stored under their hash, so each record has to be decoded; at most
// limit records are read (<=0 means DefaultCountLimit) and truncated reports
// whether more existed. Only counts are returned, never IDs.
func (s *Store) CountByNamespace(ctx context.Context, limit int) (counts map[string]int, truncated bool, err error) {
	if limit <= 0 {
		limit = DefaultCountLimit
	}
	keys, err := s.barrier.List(ctx, tokenPrefix)
	if err != nil {
		return nil, false, err
	}
	counts = make(map[string]int)
	for i, k := range keys {
		if i >= limit {
			truncated = true
			break
		}
		tok, err := s.getByStorageKey(ctx, k)
		if err != nil || tok.IsExpired() {
			continue
		}
		ns := tok.Namespace
		if ns == "root" {
			ns = RootKey
		}
		counts[ns]++
	}
	return counts, truncated, nil
}

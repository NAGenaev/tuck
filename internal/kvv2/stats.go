package kvv2

import (
	"context"
	"strings"
	"time"
)

// DefaultStatsScanLimit bounds how many metadata records Stats decodes.
const DefaultStatsScanLimit = 5000

// Stats summarises a KV v2 store from metadata only: secret values are never
// read.
type Stats struct {
	// Secrets is the number of secret paths in the store. It is exact even
	// when the scan was truncated or some secrets are not readable.
	Secrets int `json:"secrets"`
	// Versions counts version records still tracked, including soft-deleted
	// and destroyed ones, over the scanned secrets.
	Versions int `json:"versions"`
	// DeletedVersions counts soft-deleted (recoverable) versions.
	DeletedVersions int `json:"deleted_versions"`
	// DestroyedVersions counts permanently destroyed versions.
	DestroyedVersions int `json:"destroyed_versions"`
	// LastWriteAt is the newest write time seen; zero when nothing was scanned.
	LastWriteAt time.Time `json:"last_write_at,omitzero"`
	// Hidden counts secrets left out of the version figures because the
	// caller may not read their metadata.
	Hidden int `json:"hidden,omitempty"`
	// Truncated is true when the store has more secrets than the scan limit,
	// so the version figures cover only part of it.
	Truncated bool `json:"truncated,omitempty"`
}

// CountSecrets returns the number of secret paths. It lists keys only and
// decodes nothing, so it is cheap.
func (s *Store) CountSecrets(ctx context.Context) (int, error) {
	keys, err := s.b.List(ctx, metaPrefix)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, k := range keys {
		if strings.TrimPrefix(k, metaPrefix) != "" {
			n++
		}
	}
	return n, nil
}

// Stats counts secrets and aggregates version metadata. At most scanLimit
// metadata records are decoded (<=0 means DefaultStatsScanLimit). readable,
// when non-nil, reports whether the caller may read metadata of the secret at
// the given path (relative to the mount); other secrets are counted in
// Secrets and Hidden but contribute nothing to the version figures.
func (s *Store) Stats(ctx context.Context, scanLimit int, readable func(secretPath string) bool) (Stats, error) {
	if scanLimit <= 0 {
		scanLimit = DefaultStatsScanLimit
	}
	keys, err := s.b.List(ctx, metaPrefix)
	if err != nil {
		return Stats{}, err
	}
	var st Stats
	scanned := 0
	for _, k := range keys {
		p := strings.TrimPrefix(k, metaPrefix)
		if p == "" {
			continue
		}
		st.Secrets++
		if readable != nil && !readable(p) {
			st.Hidden++
			continue
		}
		if scanned >= scanLimit {
			st.Truncated = true
			continue
		}
		m, err := s.getMeta(ctx, p)
		if err != nil || m == nil {
			continue
		}
		scanned++
		for _, v := range m.Versions {
			if v == nil {
				continue
			}
			st.Versions++
			switch {
			case v.Destroyed:
				st.DestroyedVersions++
			case v.DeletedAt != nil:
				st.DeletedVersions++
			}
		}
		if m.UpdatedAt.After(st.LastWriteAt) {
			st.LastWriteAt = m.UpdatedAt
		}
	}
	return st, nil
}

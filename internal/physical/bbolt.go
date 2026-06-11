package physical

import (
	"bytes"
	"context"
	"fmt"

	bolt "go.etcd.io/bbolt"
)

var bucketName = []byte("tuck")

// Bolt is a Backend backed by a single bbolt file on local disk. No external
// database, no daemon — this is what keeps Tuck a single self-contained binary.
type Bolt struct {
	db *bolt.DB
}

// OpenBolt opens (or creates) a bbolt database at path.
func OpenBolt(path string) (*Bolt, error) {
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("open bolt: %w", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		_, e := tx.CreateBucketIfNotExists(bucketName)
		return e
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create bucket: %w", err)
	}
	return &Bolt{db: db}, nil
}

// Close releases the underlying database file.
func (b *Bolt) Close() error { return b.db.Close() }

func (b *Bolt) Get(_ context.Context, key string) (*Entry, error) {
	var out *Entry
	err := b.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketName).Get([]byte(key))
		if v == nil {
			return nil
		}
		// bbolt values are only valid inside the transaction; copy out.
		val := make([]byte, len(v))
		copy(val, v)
		out = &Entry{Key: key, Value: val}
		return nil
	})
	return out, err
}

func (b *Bolt) Put(_ context.Context, entry *Entry) error {
	return b.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketName).Put([]byte(entry.Key), entry.Value)
	})
}

func (b *Bolt) Delete(_ context.Context, key string) error {
	return b.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketName).Delete([]byte(key))
	})
}

func (b *Bolt) List(_ context.Context, prefix string) ([]string, error) {
	var keys []string
	err := b.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bucketName).Cursor()
		p := []byte(prefix)
		for k, _ := c.Seek(p); k != nil && bytes.HasPrefix(k, p); k, _ = c.Next() {
			keys = append(keys, string(k))
		}
		return nil
	})
	return keys, err
}

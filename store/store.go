// Package store is copyzen's byte-faithful clipboard store on bbolt.
package store

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	bucketHistory = "history"
	bucketPinned  = "pinned"
	bucketMeta    = "meta"
)

// DefaultCap is the history size used when no positive cap is supplied.
const DefaultCap = 100

// ErrNotFound is returned when an id is absent from both buckets.
var ErrNotFound = errors.New("copyzen: id not found")

// Store wraps a bbolt database holding clipboard history and pins.
type Store struct {
	db *bolt.DB
}

// Entry is one listed row.
type Entry struct {
	ID      uint64
	Pinned  bool
	Preview string
}

// DefaultPath is $XDG_DATA_HOME/copyzen/store.db, falling back to ~/.local/share.
func DefaultPath() (string, error) {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dir, "copyzen", "store.db"), nil
}

// Open opens (creating as needed) the DB and ensures the buckets exist. The short
// lock timeout lets a manual command wait out the recorder rather than erroring.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, err
	}
	err = db.Update(func(tx *bolt.Tx) error {
		for _, name := range []string{bucketHistory, bucketPinned, bucketMeta} {
			if _, err := tx.CreateBucketIfNotExists([]byte(name)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

func itob(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

func btoi(b []byte) uint64 { return binary.BigEndian.Uint64(b) }

// nextID returns the next value of the single global counter in the meta bucket.
func nextID(tx *bolt.Tx) (uint64, error) {
	m := tx.Bucket([]byte(bucketMeta))
	var cur uint64
	if v := m.Get([]byte("seq")); v != nil {
		cur = btoi(v)
	}
	cur++
	return cur, m.Put([]byte("seq"), itob(cur))
}

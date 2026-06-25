// Package store is copyzen's byte-faithful clipboard store on bbolt.
package store

import (
	"bytes"
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

// lookup returns a heap copy of the payload for id from history then pinned, or nil.
// The copy keeps the bytes valid after the transaction and across same-transaction
// writes that may remap the underlying mmap.
func lookup(tx *bolt.Tx, id uint64) []byte {
	key := itob(id)
	for _, name := range []string{bucketHistory, bucketPinned} {
		if v := tx.Bucket([]byte(name)).Get(key); v != nil {
			return append([]byte(nil), v...)
		}
	}
	return nil
}

// Add stores payload in history. Empty input is skipped; a payload identical to the
// most-recent history entry is deduped.
func (s *Store) Add(payload []byte, capN int) error {
	if len(payload) == 0 {
		return nil
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		h := tx.Bucket([]byte(bucketHistory))
		if k, v := h.Cursor().Last(); k != nil && bytes.Equal(v, payload) {
			return nil
		}
		id, err := nextID(tx)
		if err != nil {
			return err
		}
		if err := h.Put(itob(id), payload); err != nil {
			return err
		}
		return evict(h, capN)
	})
}

// evict deletes the oldest history entries while the count exceeds capN. It only
// ever touches the history bucket — that is why pinned entries can never be evicted.
// The count comes from cursor traversal, not Stats().KeyN: Stats reflects only
// committed state and excludes the key just Put in this same transaction.
func evict(h *bolt.Bucket, capN int) error {
	if capN <= 0 {
		capN = DefaultCap
	}
	c := h.Cursor()
	count := 0
	for k, _ := c.First(); k != nil; k, _ = c.Next() {
		count++
	}
	for count > capN {
		if k, _ := c.First(); k == nil {
			break
		}
		if err := c.Delete(); err != nil {
			return err
		}
		count--
	}
	return nil
}

// Get returns the exact payload for id, searching history then pinned.
func (s *Store) Get(id uint64) ([]byte, error) {
	var out []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		out = lookup(tx, id)
		if out == nil {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// List returns pinned entries (newest pin first) followed by history entries
// (newest first). bbolt iterates ascending, so reverse iteration gives newest-first.
func (s *Store) List() ([]Entry, error) {
	var entries []Entry
	err := s.db.View(func(tx *bolt.Tx) error {
		appendBucket := func(name string, pinned bool) {
			c := tx.Bucket([]byte(name)).Cursor()
			for k, v := c.Last(); k != nil; k, v = c.Prev() {
				entries = append(entries, Entry{ID: btoi(k), Pinned: pinned, Preview: Preview(v)})
			}
		}
		appendBucket(bucketPinned, true)
		appendBucket(bucketHistory, false)
		return nil
	})
	return entries, err
}

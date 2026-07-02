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
	Mime    string // image MIME (e.g. "image/png"); empty for non-images
	Active  bool   // payload equals the current clipboard (the live entry)
}

// Active returns the id of the entry whose payload byte-for-byte equals clip (the current
// clipboard), or 0 if none matches. Pinned is checked before history so a clipboard that
// matches a pinned payload resolves to the pin (the row actually shown). clip of length 0
// never matches (avoids marking on an empty/unavailable clipboard).
func (s *Store) Active(clip []byte) (uint64, error) {
	if len(clip) == 0 {
		return 0, nil
	}
	var id uint64
	err := s.db.View(func(tx *bolt.Tx) error {
		for _, name := range []string{bucketPinned, bucketHistory} {
			c := tx.Bucket([]byte(name)).Cursor()
			for k, v := c.First(); k != nil; k, v = c.Next() {
				if bytes.Equal(v, clip) {
					id = btoi(k)
					return nil
				}
			}
		}
		return nil
	})
	return id, err
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

// SetPersistEcho records the checksum of a payload copyzen itself just re-owned on the
// clipboard, so the watch event that copy fires back can be recognized as our own.
func (s *Store) SetPersistEcho(sum []byte) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(bucketMeta)).Put([]byte("persistEcho"), sum)
	})
}

// ConsumePersistEcho clears any recorded echo and reports whether it matched sum.
// Clearing unconditionally is what makes the guard one-shot: the echo event is always
// the next watch event, so whatever arrives next invalidates a stale marker.
func (s *Store) ConsumePersistEcho(sum []byte) (bool, error) {
	var hit bool
	err := s.db.Update(func(tx *bolt.Tx) error {
		m := tx.Bucket([]byte(bucketMeta))
		v := m.Get([]byte("persistEcho"))
		if v == nil {
			return nil
		}
		hit = bytes.Equal(v, sum)
		return m.Delete([]byte("persistEcho"))
	})
	return hit, err
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

// Add stores payload at the top of history. Empty input is skipped. Re-copying a value
// already in history is a move-to-top: every existing history entry with the same payload
// is removed first, so the list never accumulates duplicates of the same text.
func (s *Store) Add(payload []byte, capN int) error {
	if len(payload) == 0 {
		return nil
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		h := tx.Bucket([]byte(bucketHistory))
		if err := dropEqual(h, payload); err != nil {
			return err
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

// dropEqual deletes every entry in b whose payload is byte-identical to payload.
// Keys are collected first (cursor keys are only valid during the scan, so copy
// them) and deleted after.
func dropEqual(b *bolt.Bucket, payload []byte) error {
	var dups [][]byte
	c := b.Cursor()
	for k, v := c.First(); k != nil; k, v = c.Next() {
		if bytes.Equal(v, payload) {
			dups = append(dups, append([]byte(nil), k...))
		}
	}
	for _, k := range dups {
		if err := b.Delete(k); err != nil {
			return err
		}
	}
	return nil
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

// entryFrom builds a display Entry, labelling image payloads instead of byte-stripping them.
func entryFrom(id uint64, pinned bool, v []byte) Entry {
	if mime, _, ok := SniffImage(v); ok {
		return Entry{ID: id, Pinned: pinned, Preview: ImageLabel(mime, len(v)), Mime: mime}
	}
	return Entry{ID: id, Pinned: pinned, Preview: Preview(v)}
}

// List returns pinned entries (newest pin first) followed by history entries
// (newest first). Each distinct payload is shown once: a history entry is hidden if its
// payload is already pinned or already shown by a newer history entry. bbolt iterates
// ascending, so reverse iteration gives newest-first.
func (s *Store) List() ([]Entry, error) {
	var entries []Entry
	err := s.db.View(func(tx *bolt.Tx) error {
		seen := map[string]bool{}
		pc := tx.Bucket([]byte(bucketPinned)).Cursor()
		for k, v := pc.Last(); k != nil; k, v = pc.Prev() {
			seen[string(v)] = true
			entries = append(entries, entryFrom(btoi(k), true, v))
		}
		hc := tx.Bucket([]byte(bucketHistory)).Cursor()
		for k, v := hc.Last(); k != nil; k, v = hc.Prev() {
			if seen[string(v)] {
				continue
			}
			seen[string(v)] = true
			entries = append(entries, entryFrom(btoi(k), false, v))
		}
		return nil
	})
	return entries, err
}

// pinTx copies the payload for id into the pinned bucket under a new id, or is a
// no-op if an identical payload is already pinned. Runs in a writable transaction.
func pinTx(tx *bolt.Tx, id uint64) error {
	payload := lookup(tx, id)
	if payload == nil {
		return ErrNotFound
	}
	p := tx.Bucket([]byte(bucketPinned))
	c := p.Cursor()
	for k, v := c.First(); k != nil; k, v = c.Next() {
		if bytes.Equal(v, payload) {
			return nil
		}
	}
	newID, err := nextID(tx)
	if err != nil {
		return err
	}
	return p.Put(itob(newID), payload)
}

// Pin copies the payload identified by id into the pinned bucket under a new id.
// It is a no-op when an identical payload is already pinned. The history copy is
// left untouched.
func (s *Store) Pin(id uint64) error {
	return s.db.Update(func(tx *bolt.Tx) error { return pinTx(tx, id) })
}

// promoteToHistory drops any existing history copies of payload, inserts it as the
// newest history entry, and evicts beyond cap. Used when unpinning so the entry
// returns to the top of history as if freshly copied (and never duplicated).
func promoteToHistory(tx *bolt.Tx, payload []byte, capN int) error {
	h := tx.Bucket([]byte(bucketHistory))
	if err := dropEqual(h, payload); err != nil {
		return err
	}
	id, err := nextID(tx)
	if err != nil {
		return err
	}
	if err := h.Put(itob(id), payload); err != nil {
		return err
	}
	return evict(h, capN)
}

// unpinTx removes id from the pinned bucket and promotes its payload to the top of
// history, so an unpinned entry returns to the active history as the newest item.
func unpinTx(tx *bolt.Tx, id uint64, capN int) error {
	p := tx.Bucket([]byte(bucketPinned))
	key := itob(id)
	v := p.Get(key)
	if v == nil {
		return ErrNotFound
	}
	payload := append([]byte(nil), v...)
	if err := p.Delete(key); err != nil {
		return err
	}
	return promoteToHistory(tx, payload, capN)
}

// Toggle pins id if it is not already pinned, otherwise unpins it (moving the entry
// back to the top of history) — the picker's Ctrl+S action.
func (s *Store) Toggle(id uint64, capN int) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if tx.Bucket([]byte(bucketPinned)).Get(itob(id)) != nil {
			return unpinTx(tx, id, capN)
		}
		return pinTx(tx, id)
	})
}

// Unpin removes id from the pinned bucket and promotes its payload to the top of
// history (as if freshly copied), so the entry returns to the active history.
func (s *Store) Unpin(id uint64, capN int) error {
	return s.db.Update(func(tx *bolt.Tx) error { return unpinTx(tx, id, capN) })
}

// Delete removes id from whichever bucket holds it (ids are unique across buckets).
// Deleting a pinned entry also drops byte-identical history copies: Pin leaves the
// history copy in place and List hides it behind the pin, so the picker shows one
// row — deleting that row must delete the content, not resurface the hidden twin
// as if it were merely unpinned. Deleting a history entry never touches pins.
func (s *Store) Delete(id uint64) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		key := itob(id)
		h := tx.Bucket([]byte(bucketHistory))
		if h.Get(key) != nil {
			return h.Delete(key)
		}
		p := tx.Bucket([]byte(bucketPinned))
		v := p.Get(key)
		if v == nil {
			return ErrNotFound
		}
		payload := append([]byte(nil), v...)
		if err := p.Delete(key); err != nil {
			return err
		}
		return dropEqual(h, payload)
	})
}

// Wipe clears history only; pinned entries and the id counter are untouched.
func (s *Store) Wipe() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket([]byte(bucketHistory)); err != nil {
			return err
		}
		_, err := tx.CreateBucket([]byte(bucketHistory))
		return err
	})
}

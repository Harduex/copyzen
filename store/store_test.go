package store

import (
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestOpenCreatesBuckets(t *testing.T) {
	s := newTestStore(t)
	err := s.db.View(func(tx *bolt.Tx) error {
		for _, name := range []string{bucketHistory, bucketPinned, bucketMeta} {
			if tx.Bucket([]byte(name)) == nil {
				t.Errorf("bucket %q missing", name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestIDCodecRoundTrip(t *testing.T) {
	for _, v := range []uint64{0, 1, 42, 1 << 40} {
		if got := btoi(itob(v)); got != v {
			t.Errorf("codec %d -> %d", v, got)
		}
	}
}

func TestNextIDIncrementsAndPersists(t *testing.T) {
	s := newTestStore(t)
	var a, b uint64
	_ = s.db.Update(func(tx *bolt.Tx) error { a, _ = nextID(tx); return nil })
	_ = s.db.Update(func(tx *bolt.Tx) error { b, _ = nextID(tx); return nil })
	if a != 1 || b != 2 {
		t.Fatalf("want 1,2 got %d,%d", a, b)
	}
}

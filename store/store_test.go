package store

import (
	"bytes"
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

func TestAddGetRoundTrip(t *testing.T) {
	s := newTestStore(t)
	payloads := [][]byte{
		[]byte("plain"),
		[]byte("trailing\n"),
		[]byte("no-trailing"),
		[]byte("  spaced  "),
		[]byte("multi\nline\ntext"),
		[]byte("crlf\r\nline"),
		[]byte("héllo 世界 🚀"),
		[]byte("tab\tand\nnewline"),
	}
	for _, p := range payloads {
		if err := s.Add(p, 100); err != nil {
			t.Fatal(err)
		}
	}
	for i, p := range payloads {
		got, err := s.Get(uint64(i + 1))
		if err != nil {
			t.Fatalf("Get(%d): %v", i+1, err)
		}
		if !bytes.Equal(got, p) {
			t.Errorf("round-trip mismatch: stored %q got %q", p, got)
		}
	}
}

func TestAddSkipsEmpty(t *testing.T) {
	s := newTestStore(t)
	if err := s.Add(nil, 100); err != nil {
		t.Fatal(err)
	}
	if err := s.Add([]byte{}, 100); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(1); err != ErrNotFound {
		t.Errorf("empty input should store nothing, got %v", err)
	}
}

func TestAddDedupsConsecutive(t *testing.T) {
	s := newTestStore(t)
	_ = s.Add([]byte("x"), 100)
	_ = s.Add([]byte("x"), 100) // deduped
	_ = s.Add([]byte("y"), 100)
	_ = s.Add([]byte("x"), 100) // non-consecutive: kept
	if _, err := s.Get(1); err != nil {
		t.Error("id 1 missing")
	}
	if _, err := s.Get(2); err != nil {
		t.Error("id 2 missing")
	}
	if _, err := s.Get(3); err != nil {
		t.Error("id 3 missing")
	}
	if _, err := s.Get(4); err != ErrNotFound {
		t.Errorf("expected only 3 entries, got id 4: %v", err)
	}
}

func TestGetUnknown(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Get(999); err != ErrNotFound {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestEvictionRespectsCap(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 5; i++ {
		if err := s.Add([]byte{byte('a' + i)}, 3); err != nil {
			t.Fatal(err)
		}
	}
	// cap 3 → ids 1,2 evicted; 3,4,5 remain.
	for _, gone := range []uint64{1, 2} {
		if _, err := s.Get(gone); err != ErrNotFound {
			t.Errorf("id %d should be evicted", gone)
		}
	}
	for _, kept := range []uint64{3, 4, 5} {
		if _, err := s.Get(kept); err != nil {
			t.Errorf("id %d should remain: %v", kept, err)
		}
	}
}

func TestListNewestFirst(t *testing.T) {
	s := newTestStore(t)
	for _, p := range []string{"one", "two", "three"} {
		_ = s.Add([]byte(p), 100)
	}
	entries, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("want 3 entries, got %d", len(entries))
	}
	if entries[0].Preview != "three" || entries[2].Preview != "one" {
		t.Errorf("not newest-first: %+v", entries)
	}
	for _, e := range entries {
		if e.Pinned {
			t.Error("nothing pinned yet")
		}
	}
}

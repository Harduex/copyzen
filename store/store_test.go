package store

import (
	"bytes"
	"math/rand"
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

func TestPinUnpin(t *testing.T) {
	s := newTestStore(t)
	_ = s.Add([]byte("keep me"), 100) // id 1

	if err := s.Pin(1); err != nil {
		t.Fatal(err)
	}
	entries, _ := s.List()
	if len(entries) != 2 || !entries[0].Pinned || entries[0].Preview != "keep me" {
		t.Fatalf("pinned copy should be first: %+v", entries)
	}
	pinnedID := entries[0].ID
	got, _ := s.Get(pinnedID)
	if string(got) != "keep me" {
		t.Errorf("pinned payload = %q", got)
	}

	// Pinning the same payload again is a no-op.
	if err := s.Pin(1); err != nil {
		t.Fatal(err)
	}
	if entries, _ := s.List(); len(entries) != 2 {
		t.Errorf("re-pin duplicated: %d entries", len(entries))
	}

	if err := s.Unpin(pinnedID); err != nil {
		t.Fatal(err)
	}
	if entries, _ := s.List(); len(entries) != 1 || entries[0].Pinned {
		t.Errorf("unpin failed: %+v", entries)
	}
}

func TestPinUnknownAndUnpinNotPinned(t *testing.T) {
	s := newTestStore(t)
	_ = s.Add([]byte("x"), 100) // id 1, history only
	if err := s.Pin(999); err != ErrNotFound {
		t.Errorf("pin unknown: want ErrNotFound, got %v", err)
	}
	if err := s.Unpin(1); err != ErrNotFound {
		t.Errorf("unpin a non-pinned id: want ErrNotFound, got %v", err)
	}
}

func pinnedSnapshot(t *testing.T, s *Store) map[uint64]string {
	t.Helper()
	entries, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	m := map[uint64]string{}
	for _, e := range entries {
		if e.Pinned {
			p, err := s.Get(e.ID)
			if err != nil {
				t.Fatal(err)
			}
			m[e.ID] = string(p)
		}
	}
	return m
}

// Invariant: decode(store(x)) == x for arbitrary non-empty bytes.
func TestProperty_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 1000; i++ {
		payload := make([]byte, 1+rng.Intn(64))
		rng.Read(payload)
		if err := s.Add(payload, 100); err != nil {
			t.Fatal(err)
		}
		entries, err := s.List()
		if err != nil {
			t.Fatal(err)
		}
		got, err := s.Get(entries[0].ID) // newest (no pins in this test)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("iter %d: round-trip mismatch", i)
		}
	}
}

// Invariant: Add (and thus eviction) never modifies the pinned bucket.
func TestProperty_AddNeverTouchesPinned(t *testing.T) {
	s := newTestStore(t)
	rng := rand.New(rand.NewSource(2))
	const capN = 5
	for i := 0; i < 1000; i++ {
		if rng.Intn(3) == 0 {
			if entries, _ := s.List(); len(entries) > 0 {
				_ = s.Pin(entries[rng.Intn(len(entries))].ID)
			}
		}
		before := pinnedSnapshot(t, s)
		payload := make([]byte, 1+rng.Intn(16))
		rng.Read(payload)
		if err := s.Add(payload, capN); err != nil {
			t.Fatal(err)
		}
		after := pinnedSnapshot(t, s)
		if len(before) != len(after) {
			t.Fatalf("iter %d: Add changed pinned count %d -> %d", i, len(before), len(after))
		}
		for id, want := range before {
			if after[id] != want {
				t.Fatalf("iter %d: Add mutated pinned id %d", i, id)
			}
		}
	}
}

func TestDelete(t *testing.T) {
	s := newTestStore(t)
	_ = s.Add([]byte("a"), 100) // id 1
	_ = s.Pin(1)                // pinned copy id 2
	pinnedID := uint64(2)

	if err := s.Delete(1); err != nil { // delete history copy
		t.Fatal(err)
	}
	if _, err := s.Get(1); err != ErrNotFound {
		t.Error("history id 1 should be gone")
	}
	if _, err := s.Get(pinnedID); err != nil {
		t.Error("pinned copy must survive deleting the history entry")
	}
	if err := s.Delete(pinnedID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(pinnedID); err != ErrNotFound {
		t.Error("pinned id should be gone")
	}
	if err := s.Delete(999); err != ErrNotFound {
		t.Errorf("delete unknown: want ErrNotFound, got %v", err)
	}
}

func TestWipeKeepsPins(t *testing.T) {
	s := newTestStore(t)
	_ = s.Add([]byte("h1"), 100)
	_ = s.Add([]byte("h2"), 100)
	_ = s.Pin(2) // pin h2
	if err := s.Wipe(); err != nil {
		t.Fatal(err)
	}
	entries, _ := s.List()
	if len(entries) != 1 || !entries[0].Pinned || entries[0].Preview != "h2" {
		t.Fatalf("wipe should leave only the pin: %+v", entries)
	}
}

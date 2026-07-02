package main

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"io"
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	// Tests must never reach the session's real clipboard: neuter the persistence
	// hooks; tests that exercise persistence install their own recording stubs.
	listClipboardTypes = func() ([]string, error) { return nil, nil }
	reownClipboard = func(string, []byte) error { return errors.New("reown disabled in tests") }
	os.Exit(m.Run())
}

type persistCall struct {
	mime string
	data string
}

// stubPersist makes the live offer look like `types` and records re-own calls.
func stubPersist(t *testing.T, types []string) (calls *[]persistCall, lookups *int) {
	t.Helper()
	calls, lookups = &[]persistCall{}, new(int)
	prevList, prevReown := listClipboardTypes, reownClipboard
	listClipboardTypes = func() ([]string, error) { *lookups++; return types, nil }
	reownClipboard = func(mime string, data []byte) error {
		*calls = append(*calls, persistCall{mime, string(data)})
		return nil
	}
	t.Cleanup(func() { listClipboardTypes, reownClipboard = prevList, prevReown })
	return calls, lookups
}

func TestRunRoundTrip(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	const payload = "héllo\nworld\n" // trailing newline must survive

	if err := run([]string{"store"}, strings.NewReader(payload), io.Discard); err != nil {
		t.Fatal(err)
	}
	var listed bytes.Buffer
	if err := run([]string{"list"}, nil, &listed); err != nil {
		t.Fatal(err)
	}
	line := listed.String() // "1\théllo world"
	if !strings.HasPrefix(line, "1\t") {
		t.Fatalf("unexpected list output: %q", line)
	}
	var decoded bytes.Buffer
	if err := run([]string{"decode"}, strings.NewReader(line), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.String() != payload {
		t.Fatalf("round-trip through CLI mismatch: got %q want %q", decoded.String(), payload)
	}
}

func TestRunVersion(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"version"}, nil, &out); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) == "" {
		t.Error("version should print something")
	}
}

func TestRunUnknown(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := run([]string{"bogus"}, nil, io.Discard); err == nil {
		t.Error("unknown command should error")
	}
}

func TestRunMimetype(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	png := string([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 1, 2, 3})
	if err := run([]string{"store"}, strings.NewReader(png), io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"store"}, strings.NewReader("plain text"), io.Discard); err != nil {
		t.Fatal(err)
	}
	// id 1 = image, id 2 = text
	var img, txt bytes.Buffer
	if err := run([]string{"mimetype"}, strings.NewReader("1"), &img); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(img.String()) != "image/png" {
		t.Errorf("image mimetype: got %q want image/png", img.String())
	}
	if err := run([]string{"mimetype"}, strings.NewReader("2"), &txt); err != nil {
		t.Fatal(err)
	}
	if txt.String() != "" {
		t.Errorf("text mimetype: got %q want empty", txt.String())
	}
}

func TestRunListEmitsIcon(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, image.NewRGBA(image.Rect(0, 0, 30, 30))); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"store"}, bytes.NewReader(pngBuf.Bytes()), io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"store"}, strings.NewReader("plain text"), io.Discard); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"list"}, nil, &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "\x00icon\x1f") {
		t.Errorf("list output missing icon marker: %q", s)
	}
	if strings.Contains(strings.SplitN(s, "\n", 2)[0], "\x00icon") {
		// first (newest) row is the plain-text entry and must NOT carry an icon
		t.Error("text row should not carry an icon")
	}
}

func TestRunListMarkActive(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := run([]string{"store"}, strings.NewReader("alpha"), io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"store"}, strings.NewReader("beta"), io.Discard); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"list", "--mark-active"}, strings.NewReader("alpha"), &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	// newest-first: beta (id 2) then alpha (id 1); alpha is the clipboard
	if strings.Contains(lines[0], "•") {
		t.Errorf("beta (not clipboard) should have no dot: %q", lines[0])
	}
	if !strings.Contains(lines[1], "•") {
		t.Errorf("alpha (clipboard) should carry the dot: %q", lines[1])
	}
}

func TestRunActiveIndex(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	for _, v := range []string{"alpha", "beta", "gamma"} {
		if err := run([]string{"store"}, strings.NewReader(v), io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	// List is newest-first: gamma(0) beta(1) alpha(2). alpha is the clipboard.
	var out bytes.Buffer
	if err := run([]string{"active-index"}, strings.NewReader("alpha"), &out); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != "2" {
		t.Errorf("active-index for alpha = %q, want 2", got)
	}
	// A clipboard that matches no entry prints nothing.
	out.Reset()
	if err := run([]string{"active-index"}, strings.NewReader("nope"), &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "" {
		t.Errorf("active-index for absent value = %q, want empty", got)
	}
}

func TestRunActiveIndexFallbackNewest(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	for _, v := range []string{"alpha", "beta", "gamma"} {
		if err := run([]string{"store"}, strings.NewReader(v), io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	if err := run([]string{"pin"}, strings.NewReader("1"), io.Discard); err != nil {
		t.Fatal(err)
	}
	// List is pinned-first: alpha(0), then history newest-first: gamma(1) beta(2).
	// No live match: fall back to the newest unpinned entry, not the first pinned row.
	var out bytes.Buffer
	if err := run([]string{"active-index", "--fallback-newest"}, strings.NewReader("nope"), &out); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != "1" {
		t.Errorf("fallback for absent value = %q, want 1 (newest unpinned)", got)
	}
	// An empty clipboard (e.g. the owner crashed) falls back the same way.
	out.Reset()
	if err := run([]string{"active-index", "--fallback-newest"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != "1" {
		t.Errorf("fallback for empty clipboard = %q, want 1 (newest unpinned)", got)
	}
	// A live match still wins over the fallback.
	out.Reset()
	if err := run([]string{"active-index", "--fallback-newest"}, strings.NewReader("beta"), &out); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != "2" {
		t.Errorf("fallback with live match = %q, want 2 (the match)", got)
	}
}

func TestRunActiveIndexFallbackAllPinned(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := run([]string{"store"}, strings.NewReader("alpha"), io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"pin"}, strings.NewReader("1"), io.Discard); err != nil {
		t.Fatal(err)
	}
	// Only pinned entries exist: nothing to fall back to, print nothing.
	var out bytes.Buffer
	if err := run([]string{"active-index", "--fallback-newest"}, strings.NewReader("nope"), &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "" {
		t.Errorf("fallback with only pinned entries = %q, want empty", got)
	}
}

func TestRunStorePersistsScreenshotOffer(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	calls, _ := stubPersist(t, []string{"image/png"})
	payload := string([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 1, 2, 3})
	if err := run([]string{"store"}, strings.NewReader(payload), io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 {
		t.Fatalf("re-own calls = %d, want 1", len(*calls))
	}
	if (*calls)[0].mime != "image/png" {
		t.Errorf("re-owned mime = %q, want image/png", (*calls)[0].mime)
	}
	if (*calls)[0].data != payload {
		t.Errorf("re-owned payload differs from stored bytes")
	}
}

func TestRunStorePersistEchoGuard(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	calls, _ := stubPersist(t, []string{"image/png"})
	payload := string([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 1, 2, 3})
	for i, want := range []int{1, 1, 2} {
		// 1st store: fresh copy, re-owns. 2nd: our own echo event, must not re-own
		// again (no loop). 3rd: a fresh legitimate re-copy, re-owns again.
		if err := run([]string{"store"}, strings.NewReader(payload), io.Discard); err != nil {
			t.Fatal(err)
		}
		if len(*calls) != want {
			t.Fatalf("after store #%d: re-own calls = %d, want %d", i+1, len(*calls), want)
		}
	}
}

func TestRunStoreSkipsMultiMimeOffer(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	calls, _ := stubPersist(t, []string{"image/png", "text/html"})
	payload := string([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 1, 2, 3})
	if err := run([]string{"store"}, strings.NewReader(payload), io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 0 {
		t.Errorf("multi-mime offer re-owned %d times, want 0 (never flatten)", len(*calls))
	}
}

func TestRunStoreSkipsMimeMismatch(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	calls, _ := stubPersist(t, []string{"image/jpeg"})
	payload := string([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 1, 2, 3})
	if err := run([]string{"store"}, strings.NewReader(payload), io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 0 {
		t.Errorf("mismatched offer re-owned %d times, want 0", len(*calls))
	}
}

func TestRunStoreSkipsTextPayload(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	calls, lookups := stubPersist(t, []string{"text/plain;charset=utf-8"})
	if err := run([]string{"store"}, strings.NewReader("plain text"), io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 0 {
		t.Errorf("text payload re-owned %d times, want 0", len(*calls))
	}
	if *lookups != 0 {
		t.Errorf("text payload triggered %d type lookups, want 0 (text path stays free)", *lookups)
	}
}

func TestRunStorePersistKillSwitch(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("COPYZEN_PERSIST_IMAGES", "0")
	calls, _ := stubPersist(t, []string{"image/png"})
	payload := string([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 1, 2, 3})
	if err := run([]string{"store"}, strings.NewReader(payload), io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 0 {
		t.Errorf("kill switch on, yet re-owned %d times, want 0", len(*calls))
	}
}

func TestRunStorePersistFailureIsBestEffort(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	attempts := 0
	_, _ = stubPersist(t, []string{"image/png"})
	reownClipboard = func(string, []byte) error { attempts++; return errors.New("wl-copy exploded") }
	payload := string([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 1, 2, 3})
	if err := run([]string{"store"}, strings.NewReader(payload), io.Discard); err != nil {
		t.Fatalf("store must not fail when re-own fails: %v", err)
	}
	// A failed re-own leaves no echo, so the next identical copy tries again.
	if err := run([]string{"store"}, strings.NewReader(payload), io.Discard); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Errorf("re-own attempts = %d, want 2 (no echo recorded on failure)", attempts)
	}
}

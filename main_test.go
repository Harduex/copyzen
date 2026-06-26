package main

import (
	"bytes"
	"image"
	"image/png"
	"io"
	"strings"
	"testing"
)

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

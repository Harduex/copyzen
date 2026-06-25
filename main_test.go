package main

import (
	"bytes"
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

package store

import (
	"strings"
	"testing"
)

func TestPreview(t *testing.T) {
	cases := []struct{ in, want string }{
		{"hello", "hello"},
		{"  hi \t there \n you  ", "hi there you"},
		{"a\nb\nc", "a b c"},
		{"héllo 世界 🚀", "héllo 世界 🚀"},
		{"   ", ""},
		{"a\tb", "a b"},
	}
	for _, c := range cases {
		if got := Preview([]byte(c.in)); got != c.want {
			t.Errorf("Preview(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPreviewInvalidUTF8(t *testing.T) {
	got := Preview([]byte{'a', 0xff, 0xfe, 'b'})
	if got != "ab" {
		t.Errorf("invalid UTF-8 not sanitized: %q", got)
	}
}

func TestPreviewCaps(t *testing.T) {
	got := Preview([]byte(strings.Repeat("x", 250)))
	r := []rune(got)
	if len(r) != 101 || r[100] != '…' {
		t.Errorf("want 100 runes + ellipsis, got %d runes", len(r))
	}
}

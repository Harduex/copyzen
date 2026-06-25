package store

import "testing"

func TestSniffImage(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		mime string
		ext  string
		ok   bool
	}{
		{"png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0, 0}, "image/png", "png", true},
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0}, "image/jpeg", "jpg", true},
		{"gif87", []byte("GIF87a....."), "image/gif", "gif", true},
		{"gif89", []byte("GIF89a....."), "image/gif", "gif", true},
		{"webp", append([]byte("RIFF\x00\x00\x00\x00WEBP"), 0, 0), "image/webp", "webp", true},
		{"bmp", []byte("BM\x00\x00\x00\x00"), "image/bmp", "bmp", true},
		{"text", []byte("hello world"), "", "", false},
		{"empty", []byte{}, "", "", false},
		{"png-truncated", []byte{0x89, 'P', 'N'}, "", "", false},
		{"riff-not-webp", []byte("RIFF\x00\x00\x00\x00AVI "), "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mime, ext, ok := SniffImage(c.in)
			if ok != c.ok || mime != c.mime || ext != c.ext {
				t.Errorf("SniffImage(%q) = (%q,%q,%v), want (%q,%q,%v)", c.in, mime, ext, ok, c.mime, c.ext, c.ok)
			}
		})
	}
}

func TestImageLabel(t *testing.T) {
	cases := []struct {
		mime string
		n    int
		want string
	}{
		{"image/png", 512, "🖼 PNG · 512 B"},
		{"image/jpeg", 2048, "🖼 JPEG · 2.0 KB"},
		{"image/webp", 2421039, "🖼 WEBP · 2.3 MB"},
	}
	for _, c := range cases {
		if got := ImageLabel(c.mime, c.n); got != c.want {
			t.Errorf("ImageLabel(%q,%d) = %q, want %q", c.mime, c.n, got, c.want)
		}
	}
}

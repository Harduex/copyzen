package store

import (
	"bytes"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"

	"golang.org/x/image/bmp"
)

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

func encPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestThumbnail(t *testing.T) {
	// landscape 400x200 -> longest side clamped to 128 -> 128x64
	src := encPNG(t, 400, 200)
	out, err := Thumbnail(src, 128)
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("thumbnail is not a decodable image: %v", err)
	}
	if got := img.Bounds().Dx(); got != 128 {
		t.Errorf("width: got %d want 128", got)
	}
	if got := img.Bounds().Dy(); got != 64 {
		t.Errorf("height: got %d want 64", got)
	}
}

func TestThumbnailFormats(t *testing.T) {
	rgba := image.NewRGBA(image.Rect(0, 0, 20, 20))
	var jpg, g, bm bytes.Buffer
	if err := jpeg.Encode(&jpg, rgba, nil); err != nil {
		t.Fatal(err)
	}
	if err := gif.Encode(&g, rgba, nil); err != nil {
		t.Fatal(err)
	}
	if err := bmp.Encode(&bm, rgba); err != nil {
		t.Fatal(err)
	}
	for name, src := range map[string][]byte{"jpeg": jpg.Bytes(), "gif": g.Bytes(), "bmp": bm.Bytes()} {
		if _, err := Thumbnail(src, 128); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestThumbnailUndecodable(t *testing.T) {
	if _, err := Thumbnail([]byte("not an image"), 128); err == nil {
		t.Error("want error for undecodable input")
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

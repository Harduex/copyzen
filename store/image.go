package store

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "image/gif"
	_ "image/jpeg"

	"golang.org/x/image/draw"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

// SniffImage reports the image MIME type and file extension of b by inspecting its
// magic bytes. ok is false for any payload that is not one of the supported formats.
func SniffImage(b []byte) (mime, ext string, ok bool) {
	switch {
	case len(b) >= 8 && bytes.HasPrefix(b, []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}):
		return "image/png", "png", true
	case len(b) >= 3 && bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF}):
		return "image/jpeg", "jpg", true
	case len(b) >= 6 && (bytes.HasPrefix(b, []byte("GIF87a")) || bytes.HasPrefix(b, []byte("GIF89a"))):
		return "image/gif", "gif", true
	case len(b) >= 12 && bytes.HasPrefix(b, []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")):
		return "image/webp", "webp", true
	case len(b) >= 2 && bytes.HasPrefix(b, []byte("BM")):
		return "image/bmp", "bmp", true
	}
	return "", "", false
}

// ImageLabel renders the DISPLAY preview for an image entry, e.g. "🖼 PNG · 2.3 MB".
func ImageLabel(mime string, n int) string {
	return "🖼 " + strings.ToUpper(strings.TrimPrefix(mime, "image/")) + " · " + humanSize(n)
}

// humanSize formats a byte count as "512 B", "2.0 KB", "2.3 MB", etc.
func humanSize(n int) string {
	const unit = 1024
	if n < unit {
		return strconv.Itoa(n) + " B"
	}
	div, exp := int64(unit), 0
	for x := int64(n) / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}

// ThumbMax is the longest-side pixel cap for cached thumbnails.
const ThumbMax = 128

// Thumbnail decodes src (png/jpeg/gif/webp/bmp), downscales it so the longest side is
// at most maxPx (never upscaling, aspect preserved), and re-encodes it as PNG.
func Thumbnail(src []byte, maxPx int) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return nil, errors.New("copyzen: empty image")
	}
	tw, th := w, h
	if w > maxPx || h > maxPx {
		if w >= h {
			tw, th = maxPx, h*maxPx/w
		} else {
			tw, th = w*maxPx/h, maxPx
		}
		if tw < 1 {
			tw = 1
		}
		if th < 1 {
			th = 1
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// ThumbDir is $XDG_CACHE_HOME/copyzen/thumbs, falling back to ~/.cache.
func ThumbDir() (string, error) {
	dir := os.Getenv("XDG_CACHE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".cache")
	}
	return filepath.Join(dir, "copyzen", "thumbs"), nil
}

// ThumbPath is the cache file for id's thumbnail.
func ThumbPath(id uint64) (string, error) {
	dir, err := ThumbDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, strconv.FormatUint(id, 10)+".png"), nil
}

// EnsureThumb returns the cached thumbnail path for id, generating it if absent. src is
// invoked only on a cache miss, so callers avoid loading payload bytes for cache hits.
func EnsureThumb(id uint64, maxPx int, src func() ([]byte, error)) (string, error) {
	path, err := ThumbPath(id)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	data, err := src()
	if err != nil {
		return "", err
	}
	thumb, err := Thumbnail(data, maxPx)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, thumb, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// PruneThumbs removes cached thumbnails whose id is not in keep. Best-effort: a missing
// cache dir is not an error, and per-file removal errors are ignored.
func PruneThumbs(keep map[uint64]bool) error {
	dir, err := ThumbDir()
	if err != nil {
		return err
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range ents {
		name := e.Name()
		if !strings.HasSuffix(name, ".png") {
			continue
		}
		id, perr := strconv.ParseUint(strings.TrimSuffix(name, ".png"), 10, 64)
		if perr != nil || keep[id] {
			continue
		}
		_ = os.Remove(filepath.Join(dir, name))
	}
	return nil
}

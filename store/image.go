package store

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
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

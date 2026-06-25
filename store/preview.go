package store

import (
	"strings"
	"unicode"
)

const previewMax = 100

// Preview renders a payload for DISPLAY ONLY: invalid UTF-8 dropped, every run of
// whitespace collapsed to a single space, other control runes dropped, capped at
// previewMax runes with a trailing ellipsis. It never feeds back into decode.
func Preview(b []byte) string {
	s := strings.ToValidUTF8(string(b), "")
	var sb strings.Builder
	pendingSpace := false
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			pendingSpace = true
		case unicode.IsControl(r):
			// drop
		default:
			if pendingSpace && sb.Len() > 0 {
				sb.WriteByte(' ')
			}
			pendingSpace = false
			sb.WriteRune(r)
		}
	}
	out := sb.String()
	runes := []rune(out)
	if len(runes) > previewMax {
		out = string(runes[:previewMax]) + "…"
	}
	return out
}

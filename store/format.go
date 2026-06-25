package store

import (
	"errors"
	"strconv"
	"strings"
)

// FormatLine renders one list row as "id<TAB>preview". A pinned row prefixes the preview
// with a star (★) and the live-clipboard row with a small dot (•), in that order — so a
// pinned row always starts with ★ (the clear-confirm pin count relies on a leading ★).
func FormatLine(e Entry) string {
	prefix := ""
	if e.Pinned {
		prefix += "★ "
	}
	if e.Active {
		prefix += "• "
	}
	return strconv.FormatUint(e.ID, 10) + "\t" + prefix + e.Preview
}

// FormatLineIcon renders a list row, appending Rofi's extended dmenu icon marker
// (\0icon\x1f<path>) when iconPath is non-empty so fuzzel shows a thumbnail. fuzzel
// prints back only the text part (before the NUL) on selection, so ParseID is unaffected.
func FormatLineIcon(e Entry, iconPath string) string {
	line := FormatLine(e)
	if iconPath == "" {
		return line
	}
	return line + "\x00icon\x1f" + iconPath
}

// RenderList renders entries as newline-terminated FormatLine rows.
func RenderList(entries []Entry) string {
	var b strings.Builder
	for _, e := range entries {
		b.WriteString(FormatLine(e))
		b.WriteByte('\n')
	}
	return b.String()
}

// ParseID extracts the leading id from a bare id or a full "id<TAB>preview" line,
// so the picker's raw output can be piped straight in.
func ParseID(s string) (uint64, error) {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s) // trim before splitting so a leading space can't truncate the id
	if i := strings.IndexAny(s, "\t "); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return 0, errors.New("copyzen: no id in input")
	}
	return strconv.ParseUint(s, 10, 64)
}

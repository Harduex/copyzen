package store

import (
	"errors"
	"strconv"
	"strings"
)

// FormatLine renders one list row as "id<TAB>preview"; pinned rows prefix the
// preview field with a star so it shows under fuzzel --with-nth 2.
func FormatLine(e Entry) string {
	if e.Pinned {
		return strconv.FormatUint(e.ID, 10) + "\t★ " + e.Preview
	}
	return strconv.FormatUint(e.ID, 10) + "\t" + e.Preview
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

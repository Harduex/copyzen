package store

import "testing"

func TestFormatLine(t *testing.T) {
	if got := FormatLine(Entry{ID: 5, Pinned: false, Preview: "hi"}); got != "5\thi" {
		t.Errorf("history line = %q", got)
	}
	if got := FormatLine(Entry{ID: 7, Pinned: true, Preview: "hi"}); got != "7\t★ hi" {
		t.Errorf("pinned line = %q", got)
	}
}

func TestRenderList(t *testing.T) {
	out := RenderList([]Entry{{ID: 7, Pinned: true, Preview: "p"}, {ID: 5, Preview: "h"}})
	if out != "7\t★ p\n5\th\n" {
		t.Errorf("render = %q", out)
	}
}

func TestFormatLineIcon(t *testing.T) {
	e := Entry{ID: 5, Preview: "🖼 PNG · 1.0 KB", Mime: "image/png"}
	if got, want := FormatLineIcon(e, "/c/5.png"), "5\t🖼 PNG · 1.0 KB\x00icon\x1f/c/5.png"; got != want {
		t.Errorf("with icon: got %q want %q", got, want)
	}
	plain := Entry{ID: 6, Preview: "hello"}
	if got, want := FormatLineIcon(plain, ""), "6\thello"; got != want {
		t.Errorf("no icon: got %q want %q", got, want)
	}
}

func TestParseID(t *testing.T) {
	cases := map[string]uint64{
		"5":             5,
		"5\n":           5,
		"42\t★ preview": 42,
		"42\tplain":     42,
		"  9  ":         9,
		"7 preview\n":   7,
	}
	for in, want := range cases {
		got, err := ParseID(in)
		if err != nil || got != want {
			t.Errorf("ParseID(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	if _, err := ParseID("nope"); err == nil {
		t.Error("expected error for non-numeric id")
	}
	if _, err := ParseID(""); err == nil {
		t.Error("expected error for empty input")
	}
}

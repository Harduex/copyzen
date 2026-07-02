// Command copyzen is a byte-faithful Wayland clipboard-history store with pinning.
// It is fed by `wl-paste --watch --no-newline copyzen store` and read back through
// `copyzen decode | wl-copy`; the UI is fuzzel via the copyzen-menu wrapper.
package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Harduex/copyzen/store"
)

var version = "dev"

// listClipboardTypes and reownClipboard shell out to wl-clipboard; package variables
// so tests can stub the session away.
var listClipboardTypes = func() ([]string, error) {
	out, err := exec.Command("wl-paste", "--list-types").Output()
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(out)), nil
}

var reownClipboard = func(mime string, data []byte) error {
	cmd := exec.Command("wl-copy", "--type", mime)
	cmd.Stdin = bytes.NewReader(data)
	return cmd.Run()
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: copyzen {store|list|active-index|decode|delete|pin|unpin|toggle|wipe|mimetype|version}")
	}
	cmd := args[0]
	if cmd == "version" {
		_, err := fmt.Fprintln(stdout, version)
		return err
	}

	path, err := store.DefaultPath()
	if err != nil {
		return err
	}
	s, err := store.Open(path)
	if err != nil {
		return err
	}
	defer s.Close()

	switch cmd {
	case "store":
		data, err := io.ReadAll(stdin)
		if err != nil {
			return err
		}
		if err := s.Add(data, capFromEnv()); err != nil {
			return err
		}
		persistImage(s, data)
		return nil
	case "list":
		// With --mark-active, stdin carries the current clipboard; the matching entry is
		// flagged with ● so the live item is identifiable wherever the highlight moves.
		var activeID uint64
		for _, a := range args[1:] {
			if a == "--mark-active" {
				clip, err := io.ReadAll(stdin)
				if err != nil {
					return err
				}
				if activeID, err = s.Active(clip); err != nil {
					return err
				}
			}
		}
		entries, err := s.List()
		if err != nil {
			return err
		}
		keep := make(map[uint64]bool, len(entries))
		var b strings.Builder
		for _, e := range entries {
			keep[e.ID] = true
			e.Active = e.ID == activeID
			icon := ""
			if e.Mime != "" {
				id := e.ID
				if p, err := store.EnsureThumb(id, store.ThumbMax, func() ([]byte, error) { return s.Get(id) }); err == nil {
					icon = p
				}
			}
			b.WriteString(store.FormatLineIcon(e, icon))
			b.WriteByte('\n')
		}
		_ = store.PruneThumbs(keep) // best-effort; never break listing on cache errors
		_, err = io.WriteString(stdout, b.String())
		return err
	case "active-index":
		// Reads the current clipboard from stdin and prints the 0-based position of the
		// live entry within List() — the same ordering the picker shows — or nothing if
		// no entry is live. copyzen-menu feeds this to `fuzzel --select-index` so the
		// highlight lands on the live row (the • marker is the fallback on older fuzzel).
		// With --fallback-newest, a clipboard that matches nothing (or is empty — e.g.
		// its owner crashed) instead yields the newest unpinned entry, so the highlight
		// lands on the most recent copy rather than the first pinned row.
		fallback := false
		for _, a := range args[1:] {
			if a == "--fallback-newest" {
				fallback = true
			}
		}
		clip, err := io.ReadAll(stdin)
		if err != nil {
			return err
		}
		activeID, err := s.Active(clip)
		if err != nil {
			return err
		}
		if activeID == 0 && !fallback {
			return nil
		}
		entries, err := s.List()
		if err != nil {
			return err
		}
		for i, e := range entries {
			if e.ID == activeID || (activeID == 0 && !e.Pinned) {
				_, err = fmt.Fprintln(stdout, i)
				return err
			}
		}
		return nil
	case "decode":
		id, err := readID(stdin)
		if err != nil {
			return err
		}
		data, err := s.Get(id)
		if err != nil {
			return err
		}
		_, err = stdout.Write(data)
		return err
	case "mimetype":
		id, err := readID(stdin)
		if err != nil {
			return err
		}
		data, err := s.Get(id)
		if err != nil {
			return err
		}
		if mime, _, ok := store.SniffImage(data); ok {
			_, err = fmt.Fprintln(stdout, mime)
			return err
		}
		return nil
	case "delete":
		id, err := readID(stdin)
		if err != nil {
			return err
		}
		return s.Delete(id)
	case "pin":
		id, err := readID(stdin)
		if err != nil {
			return err
		}
		return s.Pin(id)
	case "unpin":
		id, err := readID(stdin)
		if err != nil {
			return err
		}
		return s.Unpin(id, capFromEnv())
	case "toggle":
		id, err := readID(stdin)
		if err != nil {
			return err
		}
		return s.Toggle(id, capFromEnv())
	case "wipe":
		return s.Wipe()
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}

// persistImage re-owns a just-recorded single-mime image offer via wl-copy, so the
// clipboard survives its original owner crashing (COSMIC's screenshot portal offers
// bare image/png and dies on compositor hiccups, taking the offer with it). Multi-mime
// offers are left alone so re-owning never flattens one, and text is never touched.
// Best-effort: failures log and recording always wins. COPYZEN_PERSIST_IMAGES=0 disables.
func persistImage(s *store.Store, data []byte) {
	if os.Getenv("COPYZEN_PERSIST_IMAGES") == "0" {
		return
	}
	// Consume before any other gate so a stale echo (lost event) is invalidated by
	// whatever arrives next, never by-passing a future legitimate re-own.
	sum := sha256.Sum256(data)
	if hit, err := s.ConsumePersistEcho(sum[:]); err != nil || hit {
		return
	}
	mime, _, ok := store.SniffImage(data)
	if !ok {
		return
	}
	types, err := listClipboardTypes()
	if err != nil || len(types) != 1 || types[0] != mime {
		return
	}
	if err := reownClipboard(mime, data); err != nil {
		fmt.Fprintln(os.Stderr, "copyzen: persist image:", err)
		return
	}
	if err := s.SetPersistEcho(sum[:]); err != nil {
		fmt.Fprintln(os.Stderr, "copyzen: persist image:", err)
	}
}

func readID(stdin io.Reader) (uint64, error) {
	data, err := io.ReadAll(stdin)
	if err != nil {
		return 0, err
	}
	return store.ParseID(string(data))
}

func capFromEnv() int {
	if v := os.Getenv("COPYZEN_MAX_HISTORY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return store.DefaultCap
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "copyzen:", err)
		os.Exit(1)
	}
}

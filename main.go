// Command copyzen is a byte-faithful Wayland clipboard-history store with pinning.
// It is fed by `wl-paste --watch --no-newline copyzen store` and read back through
// `copyzen decode | wl-copy`; the UI is fuzzel via the copyzen-menu wrapper.
package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/Harduex/copyzen/store"
)

var version = "dev"

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
		return s.Add(data, capFromEnv())
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
		clip, err := io.ReadAll(stdin)
		if err != nil {
			return err
		}
		activeID, err := s.Active(clip)
		if err != nil {
			return err
		}
		if activeID == 0 {
			return nil
		}
		entries, err := s.List()
		if err != nil {
			return err
		}
		for i, e := range entries {
			if e.ID == activeID {
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

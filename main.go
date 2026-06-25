// Command copyzen is a byte-faithful Wayland clipboard-history store with pinning.
// It is fed by `wl-paste --watch --no-newline copyzen store` and read back through
// `copyzen decode | wl-copy`; the UI is fuzzel via the copyzen-menu wrapper.
package main

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/Harduex/copyzen/store"
)

var version = "dev"

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: copyzen {store|list|decode|delete|pin|unpin|wipe|version}")
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
		entries, err := s.List()
		if err != nil {
			return err
		}
		_, err = io.WriteString(stdout, store.RenderList(entries))
		return err
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
		return s.Unpin(id)
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

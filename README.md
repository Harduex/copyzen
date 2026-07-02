# copyzen

A minimalist Wayland clipboard-history manager with pinning, for Pop!_OS COSMIC and any
Wayland compositor with `wl-clipboard`.

copyzen is a **store, not a clipboard client**. It never speaks the Wayland protocol — it
delegates every clipboard operation to `wl-clipboard` and uses fuzzel as the entire UI.
Its guiding value, borrowed from [cliphist](https://github.com/sentriz/cliphist), is
*"no concept of a picker, only pipes."*

## Architecture

```
« wl-clipboard + fuzzel — external »                             « copyzen »                                             « store »
┌───────────────────────────────────┐                            ┌───────────────────────────┐                           ┌───────────────────┐
│ wl-paste --watch  [records]       │─ feeds selection [stdin] ─▶│ copyzen  [Go]             │─ history + pins [bbolt] ─▶│ store.db  [bbolt] │
│  fuzzel | wl-copy  [pick + paste] │◀── picker + paste [dmenu] ─│  copyzen-menu  [POSIX sh] │                           │                   │
└───────────────────────────────────┘                            └───────────────────────────┘                           └───────────────────┘
```

Recording flows left→right; picking and pasting flow right→left across the same boundaries.
No daemon of our own, no Wayland-protocol code, no second datastore — `wl-paste --watch`
feeds bytes in, `wl-copy` sends the chosen bytes back, and the only persistent process is
that `wl-paste` watcher.

## Install

One-liner (pinned to a release tag — replace `v0.1.0` with the latest):

```sh
curl -fsSL https://raw.githubusercontent.com/Harduex/copyzen/v0.1.0/install.sh | sh
```

It detects your arch, downloads the prebuilt binary from this repo's Releases, **verifies
its SHA-256** (aborting on mismatch), installs `copyzen` + `copyzen-menu` to
`/usr/local/bin` (prompting for `sudo` only if needed), writes a scoped fuzzel config, sets
up the background recorder, and prints the Super+V binding steps. It is idempotent.

`copyzen` and `copyzen-menu` need [`wl-clipboard`](https://github.com/bugaevc/wl-clipboard)
and [`fuzzel`](https://codeberg.org/dnkl/fuzzel) installed; the installer checks for them and
tells you if either is missing (it won't guess your package manager).

### Manual (download, read, run)

```sh
curl -fsSLO https://raw.githubusercontent.com/Harduex/copyzen/v0.1.0/install.sh
less install.sh        # read before running
sh install.sh
```

Pin a specific build with `COPYZEN_VERSION=v0.1.0 sh install.sh`.

### From source

```sh
git clone https://github.com/Harduex/copyzen && cd copyzen
CGO_ENABLED=0 go build -o copyzen .
sudo install -m 0755 copyzen scripts/copyzen-menu scripts/copyzen-update /usr/local/bin/
mkdir -p ~/.config/copyzen ~/.config/autostart
cp dist/fuzzel.ini ~/.config/copyzen/fuzzel.ini
cp dist/copyzen.desktop ~/.config/autostart/copyzen.desktop   # background recorder
```

## Bind Super+V (COSMIC)

Settings → Desktop → Keyboard → Custom Shortcuts → **Add Shortcut**:

- **Command:** `copyzen-menu`
- **Shortcut:** `Super+V`

Press Super+V to open the picker.

In the picker:

- **Enter** — copy the highlighted entry back to the clipboard. Images paste back as images.
- **Ctrl+S** — pin / unpin the highlighted entry (pins survive history eviction and clear-all).
- **Ctrl+Shift+D** — delete the highlighted entry. A confirm view appears; press **Enter** to confirm, **Esc** to cancel.
- **Ctrl+Shift+C** — clear ALL history (pinned entries are kept). A confirm view appears; press **Enter** to confirm, **Esc** to cancel.
- **Esc** — quit.

The entry currently on your clipboard is marked with a leading **•** dot, so you can spot it even after moving the highlight. On **fuzzel ≥ 1.12** the picker also opens with that live entry pre-highlighted; on older fuzzel the **•** dot is the only indicator.

Copied **images** (PNG/JPEG/GIF/WebP/BMP) show a thumbnail in the picker (cached under
`$XDG_CACHE_HOME/copyzen/thumbs`) and paste back with their correct type.

## Pinning

Pins survive history rollover and appear once, with a `★`, at the top of the list — a
pinned entry is hidden from the history section, so it never shows up twice.

Pressing **Ctrl+S** on a history entry pins it (copied into a separate `pinned` store the
rolling history never evicts); on a `★` entry it unpins and returns the entry to the **top**
of history, as if just copied. The picker reopens after each toggle so you see the change.
You can also toggle from a terminal by id:

```sh
copyzen list                 # find the entry's id (first column)
echo <id> | copyzen toggle   # or: copyzen pin / copyzen unpin
```

## Commands

`copyzen` reads stdin and writes stdout; it is meant to be piped. `decode`/`delete`/`pin`/
`unpin` accept either a bare id or a full `id<TAB>preview` line (so the picker's raw output
pipes straight in).

| Command | stdin | Effect / output |
|---|---|---|
| `copyzen store` | raw bytes | store into history (skips empty, dedups vs most-recent, evicts beyond cap) |
| `copyzen list` | — | `id<TAB>preview` lines — pinned (`★`) first, then history, newest-first |
| `copyzen decode` | id / line | write the original payload to stdout, **byte-for-byte** |
| `copyzen delete` | id / line | remove the entry from whichever store holds it |
| `copyzen pin` | id / line | copy the entry into the pinned store |
| `copyzen unpin` | id / line | unpin, returning the entry to the top of history |
| `copyzen toggle` | id / line | pin if unpinned, else unpin (the picker's Ctrl+S) |
| `copyzen wipe` | — | clear history only (pins kept) |
| `copyzen version` | — | print the version |

Recording is `wl-paste --no-newline --watch copyzen store` — wl-paste options must come
**before** `--watch` (everything after `--watch` is the command it runs). Pasting back is
`copyzen decode | wl-copy`. Both round-trip the clipboard byte-for-byte.

When the recorder stores an image whose offer carries exactly one mime type (the shape
screenshot tools produce), it re-owns the offer via `wl-copy --type`, so the image stays
pasteable even if the app that copied it exits or crashes. Multi-mime offers are left
untouched, and text is never re-owned.

## Configuration

- `COPYZEN_MAX_HISTORY` — history cap (default `100`; non-positive/invalid values fall back
  to the default). Pinned entries are never capped.
- `COPYZEN_PERSIST_IMAGES` — set to `0` to disable the image re-own behavior described
  above (default on).
- Database: `$XDG_DATA_HOME/copyzen/store.db` (default `~/.local/share/copyzen/store.db`).

The installer starts the recorder now **and on every login** — preferring a systemd user
service (`copyzen.service`, which auto-restarts), and falling back to an XDG autostart entry
where a systemd-user instance isn't available. Manage it:

```sh
systemctl --user status copyzen.service           # check it
systemctl --user disable --now copyzen.service     # stop and don't start on login
# autostart fallback instead: remove ~/.config/autostart/copyzen.desktop
```

## Updating

```sh
copyzen-update    # downloads, verifies, and installs the latest release
```

`copyzen-update` just re-runs the installer (idempotent), so the equivalent is re-running
the install one-liner. It honors `COPYZEN_VERSION` / `PREFIX` / `COPYZEN_BASE_URL`.

## Uninstall

```sh
systemctl --user disable --now copyzen.service 2>/dev/null
pkill -f 'wl-paste.*--watch.*copyzen store$'
sudo rm -f /usr/local/bin/copyzen /usr/local/bin/copyzen-menu /usr/local/bin/copyzen-update
rm -rf ~/.config/copyzen ~/.config/autostart/copyzen.desktop \
       ~/.config/systemd/user/copyzen.service
rm -rf ~/.local/share/copyzen   # deletes history AND pins
```

## Releasing

Maintainers: a release is a `v*` git tag — see [docs/RELEASING.md](docs/RELEASING.md).

## License

[MIT](LICENSE)

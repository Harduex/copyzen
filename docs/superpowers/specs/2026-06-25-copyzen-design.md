# copyzen — Design Spec

- **Date:** 2026-06-25
- **Status:** Approved (ready for implementation planning)
- **Binary / command:** `copyzen`
- **Module:** `github.com/Harduex/copyzen`
- **Releases:** `Harduex/copyzen`

## TL;DR

A minimalist Wayland clipboard-history manager with item pinning, in Go. It is a
**store, not a clipboard client**: all clipboard I/O is delegated to the external
`wl-clipboard` tools (`wl-paste --watch` feeds history; `wl-copy` pastes back), and the
UI is fuzzel. Single static binary (`CGO_ENABLED=0`), `go.etcd.io/bbolt` for storage, no
daemon of our own. Guiding value: minimalism — *"no concept of a picker, only pipes."*

## Goals / Non-goals

**Goals:** byte-for-byte fidelity through `store → list → decode`; pins that survive
history rollover; ~1k-LOC codebase; GitHub-only install (Releases + raw `install.sh`, no
package managers, no website).

**Non-goals (v1):** image/MIME payloads (the data model must not *preclude* them, but v1 is
text-only); a daemon written by us; speaking the Wayland clipboard protocol directly;
distribution via AUR/apt/Homebrew/vanity domain.

## Architecture / data flow

```
                  Super+V
                     │
  clipboard change   ▼
  ┌──────────────┐   wl-paste --watch --no-newline      ┌──────────────────┐
  │  Wayland     │ ───────────────────────────────────▶ │ copyzen store    │
  │  selection   │            (raw bytes, stdin)         │  dedup + evict   │
  └──────────────┘                                       └────────┬─────────┘
                                                                  │ bbolt
                                                                  ▼
  ┌───────────────────────────────────────────────┐   ┌──────────────────────┐
  │ copyzen-menu (fuzzel wrapper, bound Super+V)    │   │ store.db (bbolt)     │
  │  copyzen list | fuzzel --dmenu --config <ini>   │◀─▶│  history / pinned /  │
  │   exit 0  → decode | wl-copy   (paste back)     │   │  meta buckets        │
  │   exit 10 → pin    (Ctrl+P, reopen list)        │   └──────────────────────┘
  │   else    → quit   (Esc / cancel / error)       │
  └───────────────────────────────────────────────┘
```

(The README ships a polished C4 version of this; built with the architecture-diagrams skill.)

## File layout

```
copyzen/
├── go.mod / go.sum                  # module github.com/Harduex/copyzen, go 1.23
├── main.go                          # CLI dispatch, stdin/stdout wiring, version (thin glue)
├── store/
│   ├── store.go                     # Store: Open/Close, Add, List, Get, Delete, Pin, Unpin, Wipe, eviction, id codec
│   ├── preview.go                   # display-only preview: collapse whitespace, cap, sanitize invalid UTF-8
│   ├── store_test.go                # round-trip, dedup, eviction-respects-pins, pin/unpin, delete, wipe, id-parse
│   └── preview_test.go              # preview formatting + invalid-UTF-8 sanitize
├── scripts/copyzen-menu             # POSIX sh fuzzel wrapper  → bind Super+V
├── dist/
│   ├── fuzzel.ini                   # scoped fuzzel config (custom-1=Control+p)
│   ├── copyzen.service              # systemd --user unit (alternative recorder)
│   └── copyzen.desktop              # XDG autostart entry (default recorder)
├── .github/workflows/release.yml    # tag v* → build amd64/arm64, sha256sums.txt, GH Release
├── install.sh                       # one-liner installer (download, verify, install, autostart)
├── README.md · LICENSE (MIT) · .gitignore
```

## Storage model

bbolt DB at `$XDG_DATA_HOME/copyzen/store.db` (fallback `~/.local/share/copyzen/store.db`),
**three buckets**:

| bucket    | key                         | value                |
|-----------|-----------------------------|----------------------|
| `history` | 8-byte big-endian `uint64`  | payload bytes (exact)|
| `pinned`  | 8-byte big-endian `uint64`  | payload bytes (exact)|
| `meta`    | `"seq"`                     | last-issued id       |

- **Single global id counter** (`meta.seq`). Every id is unique across both buckets, so
  `decode`/`delete` by bare id are unambiguous (an id lives in exactly one bucket). bbolt
  iterates keys ascending ⇒ **newest-first = reverse iteration** within a bucket.
- **Pinning copies** the payload into `pinned` under a *new* id; the history copy is
  untouched. Pinning a payload already present in `pinned` is a **no-op** (dedup by bytes),
  so re-pinning never duplicates.
- **Eviction** (in `store`): after insert, while `history` count > cap, delete the
  lowest id. It only ever touches `history` — `pinned` is structurally unreachable from the
  eviction path, which is *why* a pin can never be evicted. This is the headline invariant.
- **Dedup** (in `store`): compare the new payload only to the **most-recent** history entry;
  byte-equal ⇒ skip. Consecutive duplicates collapse; non-consecutive repeats are allowed.
- **Fidelity:** values are stored and returned as raw bytes. `decode` writes the value to
  stdout with **no added newline**. Tabs/newlines inside a payload are safe because `decode`
  looks the value up by id and returns it verbatim — it never re-parses the payload.
- **Images later, without migration:** v1 values are pure bytes. A future `mime` side-bucket
  (`id → mime`) adds image support with absence-of-mime ⇒ text; existing entries and `decode`
  are unaffected. No value envelope/header now (YAGNI).

## Subcommands

| cmd       | stdin                | behavior                                                        | stdout              |
|-----------|----------------------|----------------------------------------------------------------|---------------------|
| `store`   | raw bytes            | skip if empty; dedup vs most-recent; insert; evict beyond cap  | —                   |
| `list`    | —                    | pinned group first (`★`), then history; newest-first each      | `id<TAB>preview` lines |
| `decode`  | id *or* `id<TAB>…`   | look up id (history then pinned)                               | payload **byte-exact** |
| `delete`  | id *or* line         | remove id from whichever bucket holds it                       | —                   |
| `pin`     | id *or* line         | copy payload into `pinned` (no-op if already pinned)           | —                   |
| `unpin`   | id *or* line         | remove that id from `pinned`                                   | —                   |
| `wipe`    | —                    | clear `history` only; `pinned` + counter intact                | —                   |
| `version` | —                    | print version (`-ldflags -X main.version`)                     | string              |

- **Input parsing** (`decode`/`delete`/`pin`/`unpin`): first line of stdin → leading token
  before the first tab/space → `ParseUint`. Accepts a bare `5` or the picker's raw
  `5<TAB>★ preview` line identically.
- **List line format:** field 1 = decimal id; field 2 = `★ `+preview (pinned) or preview
  (history). The `★` lives in field 2 so it shows under `--with-nth 2` on newer fuzzel.

## Fidelity rules (the whole point)

- **Recorder:** `wl-paste --watch --no-newline copyzen store`. `wl-paste` appends a trailing
  newline by default (verified: 14-byte clipboard → 15 bytes without `-n`); `--no-newline`
  captures exact bytes.
- **Copy-back:** `copyzen decode | wl-copy`. `wl-copy` is verbatim by default (no `-n`, which
  would *trim*). `decode` adds no newline. Round-trip is byte-exact.

## Picker wrapper + fuzzel

`scripts/copyzen-menu` (POSIX sh), bound to Super+V. Loops, **branching on fuzzel's exit
code** (robust across fuzzel versions):

- `0` (Enter) → `printf '%s' "$sel" | copyzen decode | wl-copy` → quit
- `10` (Ctrl+P via `custom-1`) → `… | copyzen pin` → reopen list
- **anything else** (Esc=1, error=2, …) → quit

No `set -e` (it would abort on fuzzel's non-zero exits before `$?` is read). `--with-nth=2`
is added only if `fuzzel --help` advertises it (cosmetic id-hiding); otherwise omitted — the
id column is visible but everything still works, because the wrapper parses the id off the
returned line. `dist/fuzzel.ini` is a scoped config (`--config`) that sets
`[key-bindings] custom-1=Control+p`, so copyzen never touches the user's global fuzzel.

## Install / release / autostart

- **`release.yml`** (push tag `v*`, `permissions: contents: write`): cross-compile
  `CGO_ENABLED=0 GOOS=linux GOARCH={amd64,arm64} go build -trimpath -ldflags "-s -w -X main.version=$TAG"`
  → `copyzen-linux-{amd64,arm64}`; `sha256sum … > sha256sums.txt`; `gh release create`
  uploads binaries + `sha256sums.txt` + the three `dist/` files + `copyzen-menu`.
- **`install.sh`** (idempotent, POSIX sh): `uname` → Linux + amd64/arm64 (else clear error);
  download binary + assets from the **Release** (`releases/latest/download/…`, or
  `COPYZEN_VERSION=vX.Y.Z` to pin) — assets shipped via the Release means a single source of
  truth, no raw-ref/heredoc drift; **verify binary sha256, abort on mismatch**; install
  binary + wrapper to `/usr/local/bin` (writable ⇒ direct copy; else prompt via `/dev/tty`
  and `sudo` — never silent, works under `curl | sh`); write `~/.config/copyzen/fuzzel.ini`
  (only if absent); install autostart `.desktop` (default) + ship `.service` (alternative,
  not enabled — "use one, not both" note); start the recorder now if none is running; check
  `wl-clipboard`/`fuzzel` and **tell** the user to install if missing (no package-manager
  guessing); warn if fuzzel is too old for `--with-nth`; print exact **COSMIC** Super+V
  binding steps + a summary.

## Decisions

| id  | decision |
|-----|----------|
| D1  | History cap via env `COPYZEN_MAX_HISTORY` (default 100; invalid ⇒ default). No config file. |
| D2  | Preview: collapse all whitespace runs → single space, trim, cap **100 runes** + `…`; invalid UTF-8 sanitized for display only. |
| D3  | `store` skips only truly empty (0-byte) stdin; whitespace-only is stored exactly (fidelity > noise-filtering). |
| D4  | Recorder = `wl-paste --watch --no-newline copyzen store`; copy-back = `copyzen decode | wl-copy` (both verbatim). |
| D5  | bbolt opened with a short lock timeout (~2s) so a manual `list` racing the recorder waits rather than errors. |
| D6  | Install assets shipped as Release files (not heredocs/raw) ⇒ single source of truth. |
| Q1  | A pinned item **also remains in the history section** (different id). Pinned copy sits at top with `★`; no per-`list` payload de-dup scan. |

## Testing

Test-first on the `store` package. Unit + property-based coverage:

- **Round-trip fidelity** (byte-equality) on: trailing `\n`, no-trailing-`\n`, leading/
  trailing spaces, multiline, CRLF, unicode (`héllo 世界 🚀`), embedded tab+newline.
- **Property-based** (formal-verification skill): `decode(store(x)) == x` for arbitrary
  bytes; eviction never removes a pinned entry under arbitrary store/pin sequences.
- Dedup (consecutive vs non-consecutive) · **eviction respects pins** (headline) · pin/unpin
  + pin-dedup + pin-already-pinned no-op · delete per-bucket · wipe leaves pins · list
  ordering/grouping/`★`/format · decode accepts bare-id and full-line · preview
  collapse/cap/invalid-UTF-8-sanitized-but-decode-exact · empty stdin skipped.
- Shell: `sh -n` (and `shellcheck` if present) on `copyzen-menu` and `install.sh`.

## Out of scope (v1)

Image/MIME payloads; a self-written daemon; direct Wayland protocol; non-Linux targets;
non-GitHub distribution; toggling pin from the picker (only `pin` via Ctrl+P; `unpin` is a
CLI command).

## Verified environment facts (load-bearing)

- fuzzel **1.9.2**: `custom-1` exits **10** (man page); **no `--with-nth`** (absent from
  `--help`) ⇒ id-hiding unavailable here, installer warns. On selection fuzzel prints the
  full line; `--with-nth` (newer) only changes *display* unless `--accept-nth` is also set
  (we don't use it), so id parsing stays correct across versions.
- `wl-paste` appends a trailing newline by default ⇒ recorder needs `--no-newline`.
- `wl-copy` copies stdin verbatim by default ⇒ correct for copy-back.

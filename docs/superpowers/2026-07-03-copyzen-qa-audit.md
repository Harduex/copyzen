# copyzen QA audit — 2026-07-03

Four parallel QA agents audited the tree at `b717e35`, one lens each: the bbolt store
layer, the CLI command surface, the recorder/service/watcher, and shell/config/packaging.
Every finding marked **VERIFIED** was reproduced by execution against throwaway temp
stores (`XDG_DATA_HOME` under mktemp); the real store, live clipboard, and running
service were never touched. **STATIC** means code-reading only. Findings are
deduplicated (F1 and F3 were each found independently by two agents) and ranked by
severity.

## Major

### F1 — Text starting with `BM` (or `GIF87a`/`GIF89a`) is misclassified as an image
- **Location:** `store/image.go:35` (BMP), `store/image.go:31` (GIF); consumed by
  `store/store.go:248` (`entryFrom`) and `main.go:161` (`mimetype`).
- **Failure:** the BMP magic check matches only the two ASCII bytes `"BM"`, so copying
  text like `BMW motorrad review` renders in the picker as `🖼 BMP · 19 B`
  (unfindable as text), `copyzen mimetype` returns `image/bmp`, and the menu pastes it
  back via `wl-copy --type image/bmp` — pasting into a text field yields nothing. GIF
  has the same class of bug via the 6-byte `"GIF87a"/"GIF89a"` prefix (rarer trigger).
  Stored bytes remain byte-faithful; only classification is wrong. PNG/JPEG/WEBP magics
  are binary/anchored and did not false-positive.
- **Status:** VERIFIED end-to-end (`printf 'BMW is a great car' | copyzen store` →
  `list` shows `🖼 BMP · 18 B`, `mimetype` → `image/bmp`, `decode` → exact bytes), by
  two agents independently.
- **Fix sketch:** require a plausible BMP header (size field vs payload length) and
  binary-looking bytes after the GIF signature.

### F2 — `persistImage` holds the bbolt write lock across `wl-copy`/`wl-paste --list-types`
- **Location:** `main.go:47-55` (`store.Open` + `defer s.Close()` span all of `run`)
  with `main.go:202-227` (`persistImage` shells out inside that window); 2s lock
  timeout at `store/store.go:84`.
- **Failure:** `wl-copy` must acquire the Wayland selection before forking to the
  background; during the exact Wayland instability this feature targets, that can block
  for seconds. While it blocks, any concurrent copyzen process waits out the 2s bolt
  lock timeout and errors: a concurrent `store` event is **silently lost**; a picker
  `list`/`decode` fails.
- **Status:** VERIFIED (mechanism): with the DB flock held externally, `copyzen store`
  blocked ~1.97s, printed `copyzen: timeout`, exited 1, and the event was lost. The
  real-world trigger (slow `wl-copy`) is inferred, not reproduced.
- **Fix sketch:** don't hold the store open across `persistImage`'s subprocess calls —
  close after `Add`, reopen for `SetPersistEcho` (or restructure `run` so the store
  handle's lifetime covers only DB work).

### F3 — A non-default `PREFIX` install silently records nothing
- **Location:** `dist/copyzen.service:13` (`Environment=PATH=/usr/local/bin:/usr/bin:/bin`);
  `install.sh:11` (`PREFIX` default), `install.sh:141` (service file installed verbatim,
  no substitution). Same class: `dist/copyzen.desktop:5`.
- **Failure:** `install.sh` advertises `PREFIX=/path` and puts the binary there, but the
  service's hardcoded PATH never learns about it. With e.g. `PREFIX=$HOME/.local/bin`,
  `wl-paste --watch` can't exec `copyzen` on any clipboard event → nothing is recorded,
  while the unit shows *active (running)* and the installer's `is-active` success gate
  (`install.sh:143-144`) passes. The `.desktop` autostart fallback inherits the session
  PATH and does not have the bug, so behavior diverges by install path.
- **Status:** STATIC, found independently by two agents; default installs work only
  because the default PREFIX happens to be on the hardcoded PATH.
- **Fix sketch:** substitute `$PREFIX` into the service PATH at install time (or
  `ExecStart` with an absolute copyzen path).

## Minor

### F4 — Latent unbounded re-own loop when `SetPersistEcho` fails after a successful re-own
- **Location:** `main.go:220-226`.
- **Failure:** `reownClipboard` succeeds but `SetPersistEcho` errors → no echo marker →
  the echo event finds no marker, the live clipboard still matches, so it re-owns again,
  and again — an unbounded `wl-copy` storm. Requires a selective DB-write failure
  (narrow), but there is no attempt cap or rate limit to bound the blast radius.
- **Status:** STATIC.

### F5 — persistImage TOCTOU: stale bytes can clobber a newer same-mime copy
- **Location:** `main.go:212-220`.
- **Failure:** the single-mime gate compares the *live* clipboard's mime list but
  re-owns the *event's* captured bytes. If the user copies a different `image/png`
  between `Add` and the gate, the newer image is briefly overwritten with the stale one.
  Self-heals when the newer copy's own watch event fires.
- **Status:** STATIC.

### F6 — Unknown commands and `--help`/`-h` are validated only after opening the DB
- **Location:** `main.go:47-56` (`store.Open` precedes the subcommand switch),
  `main.go:192-193` (default case); only `version` (`main.go:42`) and the no-args usage
  path skip `Open`.
- **Failure:** `copyzen --help` creates `store.db` and prints `unknown command`; under
  lock contention a typo'd command waits ~2s and reports a misleading `copyzen: timeout`
  instead of failing instantly.
- **Status:** VERIFIED (with an external flock: `copyzen bogus` → `copyzen: timeout`
  after 1.98s; `copyzen version` → instant).

### F7 — Doubled `copyzen:` prefix on store-layer errors
- **Location:** `main.go:248` prepends `copyzen:` onto already-prefixed sentinels
  (`store/store.go:25`, `store/format.go:55`).
- **Failure:** `printf '999' | copyzen decode` → `copyzen: copyzen: id not found`.
- **Status:** VERIFIED.

### F8 — Confirm-clear dialog counts history text starting with `★` as pinned
- **Location:** `scripts/copyzen-menu:111-113` (`awk … $2 ~ /^★/`), relying on the
  display convention from `store/format.go:12-21`.
- **Failure:** a history entry whose payload begins with a literal `★` inflates the
  "N pinned kept" count in the Ctrl+Shift+C confirmation (e.g. "1 pinned kept" with
  zero pins). Cosmetic — `wipe` itself keeps only real pins.
- **Status:** VERIFIED.

### F9 — `Restart=on-failure` misses a clean (exit 0) wl-paste termination
- **Location:** `dist/copyzen.service:16`.
- **Failure:** if `wl-paste --watch` ever exits 0, recording stops silently until next
  login. Journal evidence shows the real socket-loss path exits non-zero (covered), so
  this is a hardening gap; `Restart=always` would close it.
- **Status:** STATIC.

### F10 — Recorder and picker can use different DBs under a custom `XDG_DATA_HOME`
- **Location:** `store/store.go:66-76` (`DefaultPath`); `dist/copyzen.service` sets no
  `XDG_DATA_HOME`.
- **Failure:** `XDG_DATA_HOME` set in a shell rc but not imported into the systemd user
  environment → the service writes `~/.local/share/copyzen/store.db` while the picker
  reads `$XDG_DATA_HOME/copyzen/store.db`; history looks empty/stale.
- **Status:** STATIC.

### F11 — `humanSize` panics (index out of range) for payloads ≥ 1 PiB
- **Location:** `store/image.go:57` (`"KMGT"[exp]`).
- **Failure:** `ImageLabel(mime, n)` with `n ≥ 1<<50` drives `exp` to 4 → panic.
  Unreachable with real clipboard data; latent hardening gap only.
- **Status:** VERIFIED in isolation.

## Docs

### F12 — README understates dedup as "vs most-recent"
`README.md:109` vs actual behavior (`dropEqual` scans the whole history bucket —
move-to-top across all entries, as CLAUDE.md correctly states).

### F13 — In-repo docs disagree on the fuzzel version that enables pre-highlight
`README.md:81` says ≥ 1.12; `scripts/copyzen-menu:52` says ≥ 1.11. The code gates on
`--select-index` flag presence (1.9.2 lacks it, 1.12.0 has it — verified via
`fuzzel --help`); which of 1.10/1.11 introduced it needs a probe of those versions.

### F14 — Installer SHA-256 verification covers only the binary
`install.sh:50-54` checksums `$asset` only; `copyzen-menu`, `copyzen-update`, and
`install.sh` itself (re-piped by `copyzen-update`) are fetched unverified. Standard for
a curl|sh installer (HTTPS+GitHub is the trust anchor), but the README's "verifies its
SHA-256" claim (`README.md:34`) reads broader than it is.

## Verified clean

Adversarial testing confirmed the core invariants hold at this commit:

- **Byte-for-byte fidelity** (store→list→decode): exact round-trips for empty input,
  single/embedded NULs, invalid UTF-8, CRLF, trailing whitespace, ANSI escapes, 8 MB
  random binaries, and payloads that mimic `list` output — proven with `cmp`/sha256.
- **List-format injection:** embedded tabs/newlines/icon-marker bytes cannot corrupt id
  routing; `Preview` collapses whitespace and drops control chars, so every row stays
  one line and the id column is unambiguous.
- **Menu injection surface:** all clipboard-derived values flow to `copyzen`/`wl-copy`
  via stdin, never argv — `` `whoami` ``, `$(id)`, `-n --type text/evil` payloads
  neither execute nor inject flags.
- **History semantics:** dedup move-to-top (including pinned values and hidden twins),
  eviction respects the cap and never touches pins, `wipe` keeps pins + counter,
  the b717e35 pinned-twin deletion is consistent, `ParseID` rejects garbage cleanly.
- **persistEcho protocol:** recording happens unconditionally before the echo check, so
  a user copy can never be swallowed; marker leaks are self-healing; the bolt file lock
  serializes marker set/consume in the normal path.
- **Recorder plumbing:** wl-paste flag order correct everywhere (options before
  `--watch`, `--no-newline` present); no pipe deadlocks or zombies; a failing per-event
  `store` doesn't kill the watcher; the d2f160e socket-outage self-heal was observed
  working in the live journal.
- **fuzzel config:** `Control+s` / `Control+Shift+d` / `Control+Shift+c` are free
  against fuzzel 1.12.0 defaults (man-page dump); exit-code routing 10/11/12 and
  `--with-nth=2` full-line return semantics confirmed; all menu-invoked subcommands
  exist; install/release/menu path cross-references all match; `go.mod` compliant
  (go 1.23, no toolchain line, x/image v0.21.0); all shell scripts pass `sh -n`/`dash -n`.

## Suggested priority

F1 (most user-visible, cheapest fix) → F2 (lock lifetime restructure) → F3 (install-time
PATH substitution) → the rest opportunistically.

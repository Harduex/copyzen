# copyzen — agent guide

A byte-faithful Wayland clipboard-history store on bbolt, driven by pipes (`wl-paste --watch
copyzen store` records, `copyzen decode | wl-copy` pastes back, fuzzel is the UI via
`scripts/copyzen-menu`). Below are lessons that only became obvious by building and running
it — read the relevant one before changing that area.

## Core invariants
- **Byte-for-byte fidelity is the whole point.** `store.Get` / `decode` return the exact
  stored payload — never strip, trim, or normalize bytes (control chars, tabs, trailing
  junk) on the store/decode path. `store.Preview` is display-only and never feeds back into
  decode.
- **History dedups across all entries (move-to-top):** re-copying a value removes its older
  entry and re-adds it on top — no duplicates anywhere in history.
- **The recorder re-owns single-mime image offers** (`persistImage`: the COSMIC screenshot
  portal offers bare `image/png` and its offer dies when the portal crashes) — never
  multi-mime offers (re-owning would flatten them) and never text. The `persistEcho` key in
  the meta bucket breaks the self-trigger loop: our own `wl-copy` re-fires the watcher, and
  the next event consumes-and-clears the marker. `COPYZEN_PERSIST_IMAGES=0` kills it.

## bbolt store
- **Eviction counts via cursor traversal, not `Stats().KeyN`** — Stats reflects only
  committed state and excludes the key just `Put` in the same transaction (off-by-one).
- **`ParseID` trims before splitting** so a leading space can't truncate the id.
- ids come from one global counter and are never reused; `wipe` clears history but keeps
  pins and the counter.

## fuzzel / the picker
- **Verify any keybinding or config by RUNNING fuzzel — its man page is incomplete.** Both
  `Control+d` and `Delete` are the `delete-next` default (undocumented); binding a taken key
  is **fatal** — fuzzel errors and the picker won't open. Defaults are also
  **version-dependent**: `Shift+Delete` became the `expunge` default in fuzzel 1.11, so a key
  free on 1.9.2 can be taken on 1.12 — probe every version you support. Check:
  `printf x | fuzzel --dmenu --config dist/fuzzel.ini` → look for `err:` on stderr.
- **Never override a fuzzel default** — bind only keys free in its defaults (additive), so
  copyzen works out of the box. Current: pin `Control+s`, delete `Control+Shift+d`, clear
  `Control+Shift+c` (`Control+p`=prev, `Control+c`/`Control+g`=cancel are taken).
- The menu routes on fuzzel **exit codes** (`custom-1`→10, `custom-2`→11, `custom-3`→12), so
  changing which key does what is a `dist/fuzzel.ini` edit, not a menu-script change.
- fuzzel is a **text launcher**: one uniform row height, small square left-anchored icons —
  no per-row height, no width-fit, no real image previews. Don't promise rich thumbnails;
  the valuable image feature is **type-aware paste-back** (`copyzen mimetype` → `wl-copy --type`).
- The recorder's `wl-paste` options **must precede `--watch`** — everything after `--watch`
  is the command to run.

## Build & ship
- Keep `go.mod` at **`go 1.23`** with **no `toolchain` line** (CI pins Go 1.23; a newer local
  toolchain silently bumps it and breaks the build). `golang.org/x/image` is pinned to a
  1.23-compatible release (`v0.21.0`; `@latest` needs Go ≥1.25).
- **Test on a real Wayland session before tagging.** Every real bug here surfaced only by
  running — wl-paste flag order, eviction off-by-one, ParseID trim, fuzzel keybinding
  defaults — never from unit tests. Release process: `docs/RELEASING.md`.

## Working conventions
- Commit as the repo author with **no AI co-author trailer**.
- Dispatch subagents on **Sonnet** (the floor — not Haiku); use **Opus** for hard tasks and
  the final whole-branch review.
- Confirm before any push or publish.

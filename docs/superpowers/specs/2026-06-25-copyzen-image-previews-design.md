# copyzen — image thumbnails + type-aware paste-back

Design for **v0.2.0** (shipped together with the delete/clear feature; see
`2026-06-25-copyzen-delete-clear-design.md`). Make copied images first-class in the picker: render a small
thumbnail instead of garbage text, and paste them back as real images. Keeps copyzen's
identity: single static `CGO_ENABLED=0` Go binary, no new *runtime* dependencies, store
stays byte-faithful.

## Problem (verified on the live store)

The recorder `wl-paste --no-newline --watch copyzen store` already captures image bytes
faithfully — e.g. history entry 69 is a valid 2.4 MB PNG (1917×1038), magic bytes intact
(`89 50 4E 47 …`). But:

- **Preview** runs `ToValidUTF8` over the raw bytes, so an image renders as stray
  printable bytes (`PNG … IHDR … IDAT`) — the "weird text".
- **Paste-back is type-blind**: `copyzen decode | wl-copy` re-copies image bytes as
  `text/plain` (wl-copy's default), so the image does not round-trip as an image.

`store.List()` computes `Preview()` on the fly from stored bytes; the store keeps only
raw bytes per id. So this is a **display + paste-path change with no schema change**, and
existing image entries are fixed retroactively.

## Decisions (resolved during brainstorming)

- **Thumbnails: pure-Go, downscaled, cached.** Decode + scale to ≤128px, cache a tiny PNG,
  point fuzzel at it. (Chosen over pointing fuzzel at raw multi-MB files, which risks
  sluggish rendering and large disk duplication.)
- **Formats: PNG, JPEG, GIF, WebP, BMP.** All decodable in pure Go (stdlib + `golang.org/x/image`).
- **Capture coverage: unchanged.** Keep the single default watcher; scope is previewing /
  pasting images that ARE captured (the common case — screenshots, single-image copies).
  Known limitation: apps offering image + text together may have wl-paste pick the text
  type. Deferred.
- **No store-schema change.** Detect images by sniffing magic bytes, not by storing a MIME.

## Architecture

copyzen sniffs bytes, serves bytes byte-faithfully, generates/caches thumbnails, and
exposes the sniffed MIME. The menu wraps `wl-copy --type`. copyzen still never calls
`wl-copy` itself (store/IO separation preserved).

```
list  ──▶ per image entry: ensure thumb cache, emit  id<TAB>label \0icon\x1f <thumbpath>
decode ─▶ raw bytes (unchanged, byte-faithful)
mimetype ▶ sniffed MIME (e.g. image/png) or empty
menu  ──▶ t=$(… | copyzen mimetype); … | copyzen decode | wl-copy ${t:+--type "$t"}
```

## Components

### 1. Sniff — `store.SniffImage(b []byte) (mime, ext string, ok bool)`
Explicit magic-byte detection for the five formats:

| Format | Signature (hex / ascii)                          | mime         | ext   |
|--------|--------------------------------------------------|--------------|-------|
| PNG    | `89 50 4E 47 0D 0A 1A 0A`                         | image/png    | png   |
| JPEG   | `FF D8 FF`                                        | image/jpeg   | jpg   |
| GIF    | `47 49 46 38` (`GIF8`)                            | image/gif    | gif   |
| WebP   | `52 49 46 46 …(4) 57 45 42 50` (`RIFF`…`WEBP`)    | image/webp   | webp  |
| BMP    | `42 4D` (`BM`)                                    | image/bmp    | bmp   |

Pure function. Returns `ok=false` for anything else (incl. truncated/too-short input).

### 2. Display label — `store.ImageLabel(mime string, n int) string`
Image entries get a clean label instead of garbage, e.g. `🖼 PNG · 2.4 MB`
(format from sniff, human-readable size from byte length — no decode needed, cheap).
`Entry` gains `Mime string` (empty = not an image), set in `List()` by sniffing the
already-loaded bytes. `List()`: if `SniffImage` ok → `Mime` set, `Preview = ImageLabel(…)`;
else `Preview = Preview(bytes)` as today.

### 3. Thumbnail — `store.Thumbnail(src []byte, maxPx int) ([]byte, error)`
Decode (`image/png`, `image/jpeg`, `image/gif` stdlib + `golang.org/x/image/webp`,
`…/bmp` blank imports), downscale preserving aspect so the longest side ≤ `maxPx` (128),
via `golang.org/x/image/draw` (ApproxBiLinear), re-encode as PNG. Pure function of bytes.
Returns an error on undecodable input (caller skips the icon).

### 4. Thumbnail cache (command layer, `main.go` `list`)
- Dir: `$XDG_CACHE_HOME/copyzen/thumbs/` (default `~/.cache/copyzen/thumbs/`), via a
  `store.ThumbDir()` helper mirroring `DefaultPath()`'s XDG convention.
- File: `<id>.png` (always PNG, regardless of source format).
- **Lazy generation**: for each entry with `Mime != ""`, if `<id>.png` is missing, fetch
  bytes via `Get(id)` (only on cache-miss — never hold all images in RAM), `Thumbnail()`,
  write. On hit, reuse. Decode failure → skip icon, keep label; never fail the whole list.
- **Prune orphans**: after building the list, remove thumb files whose id is not in the
  current entry set (best-effort, errors ignored). Bounded by ≤ history cap + pins.

### 5. fuzzel rendering (`main.go` `list` output)
For image rows, emit the Rofi extended dmenu icon protocol:
`<id>\t<label>\0icon\x1f<thumbpath>\n`. Non-image rows unchanged (`<id>\t<preview>\n`).
A pure helper `store.FormatLineIcon(e Entry, iconPath string) string` builds the line
(icon marker only when `iconPath != ""`); the command decides the path (IO). fuzzel prints
back only the text part (before `\0`) on selection, so `ParseID` and `--with-nth=2` are
unaffected.

### 6. `copyzen mimetype` command (`main.go`)
Reads an id from stdin (consistent with `decode`/`delete`), `Get(id)`, `SniffImage` →
prints the MIME (e.g. `image/png`) and a trailing newline, or prints nothing for a
non-image. `decode` is unchanged and stays byte-faithful.

### 7. Menu paste-path (`scripts/copyzen-menu`, Enter / exit 0)
```sh
if [ -n "$sel" ]; then
    t=$(printf '%s' "$sel" | copyzen mimetype)
    printf '%s' "$sel" | copyzen decode | wl-copy ${t:+--type "$t"}
fi
exit 0
```
Text behaves exactly as before (empty `t` → wl-copy default type); images get
`--type image/png` (etc.) so they paste as images.

### 8. Dependencies
Add `golang.org/x/image` (webp/bmp decoders + draw). Pure-Go module dep — `CGO_ENABLED=0`
static binary preserved, consistent with the existing `go.etcd.io/bbolt` dep. No new
runtime deps. fuzzel must be built with PNG support (it is: `+png +svg` on the target).

## Edge cases

- **Undecodable / corrupt image bytes** — sniff may say image but decode fails: skip the
  thumbnail, keep the `🖼 …` label; `mimetype` still returns the sniffed type.
- **Truncated headers** — `SniffImage` returns `ok=false`; treated as text (current path).
- **Cache dir uncreatable / write fails** — log nothing fatal; emit the row without an
  icon. The picker must never break because of the cache.
- **`Get(id)` race** (entry evicted between `List` and thumbnail fetch) — treat as
  cache-miss failure: emit row without icon.
- **WebP with no encoder** — we only *decode* webp; thumbnails are always re-encoded as
  PNG, so the missing webp encoder in `x/image` is irrelevant.

## Testing & verification

Unit (pure functions, table-driven):
- `SniffImage` — one fixture per format (small valid headers) + negatives: plain text,
  empty, truncated-to-3-bytes, RIFF-but-not-WEBP.
- `ImageLabel` — format string + human-size rounding (bytes/KB/MB).
- `Thumbnail` — synthesize tiny PNG/JPEG/GIF/BMP in-test (stdlib + `x/image/bmp` encoders),
  assert output decodes as PNG, longest side ≤ maxPx, aspect preserved. WebP via a small
  embedded fixture (decode-only path).
- `List` — store an image fixture, assert `Entry.Mime` set and preview is the label.
- `mimetype` — drive via `run()` with stdin id; image → MIME, text → empty.
- Cache + icon emission + prune — use a temp `XDG_CACHE_HOME`; assert thumb file created,
  icon marker present in `list` output, orphan removed.

Real-machine (this project's bugs surface only by running):
- Copy a real PNG, JPEG, and WebP (e.g. from a browser); open Super+V; confirm each shows
  a thumbnail and a clean label.
- Select an image → confirm it pastes as an actual image (not text) into an image-aware
  target.
- Confirm `--with-nth=2` + icon protocol coexist (id column hidden, thumbnail shown).
- Confirm text entries are visually and behaviorally unchanged.

## Out of scope (tracked separately)

- **Capture coverage** — guaranteeing image capture when a text type is co-offered.
- **Ubuntu/GNOME portability docs** — Super+V binding instructions for GNOME and a
  Wayland-vs-Xorg / fuzzel-availability note. Deferred to its own small docs task; the
  tool already works on Ubuntu *Wayland* with `wl-clipboard` + a recent `fuzzel`.
- **Store growth from large images** — history of multi-MB images inflates `store.db`;
  cap stays 100. Pre-existing; not addressed here.
- Animated GIF playback (thumbnail is the first frame).

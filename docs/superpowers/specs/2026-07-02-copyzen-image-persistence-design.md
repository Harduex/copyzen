# copyzen image clipboard persistence — design

## Problem

On COSMIC, a screenshot "Copy" leaves the clipboard owned by `xdg-desktop-portal-cosmic`
(it offers `image/png` directly). That daemon's Wayland thread dies on compositor
hiccups — observed repeatedly on 2026-07-01/02, including after amdgpu GPU resets — and
its clipboard offer dies with it. copyzen's recorder has already stored the image by
then, so the picker shows the row, but nothing is live: no highlight match, no `•`, and
Ctrl+V pastes nothing until the user re-picks the row.

## Fix

`copyzen store` re-owns qualifying image offers the moment it records them, via
`wl-copy --type <mime>`. `wl-copy` daemonizes, serves the offer until the next copy
replaces it (then exits — no process buildup), and doesn't care if the original owner
crashes. Screenshots stay pasteable through portal deaths.

## Gate — when to re-own

All must hold, otherwise do nothing:

1. `COPYZEN_PERSIST_IMAGES` is not `0` (kill switch; same env pattern as
   `COPYZEN_MAX_HISTORY`; default on).
2. The payload sniffs as an image (`store.SniffImage`).
3. The live offer advertises **exactly one** mime type (`wl-paste --list-types`) and it
   equals the sniffed mime. This is precisely the COSMIC screenshot shape. Multi-mime
   offers (e.g. Chrome's image+HTML copies) are never touched, so re-owning never
   flattens an offer. Text is never touched.

## Self-trigger guard

Our own `wl-copy` re-fires `wl-paste --watch`, which runs `copyzen store` again with
identical bytes. A one-shot echo hash in the existing `meta` bucket breaks the loop:

- After a successful re-own, store `sha256(payload)` as the echo.
- At the start of every persistence check, consume the echo (read **and clear**): if it
  matches the incoming payload, this event is our own echo — stop. The echo event is
  always the immediate next watch event, so unconditional clear-on-next-event is
  self-healing; a stale echo can suppress nothing beyond that one event.

Two images copied within one event cycle can cause a few redundant re-owns but always
converge on the newest; humanly (screenshots) events are seconds apart.

## Failure posture

Best-effort, like thumbnail pruning: `wl-copy`/`wl-paste` failures log to stderr and
never fail the store — recording always wins. The bbolt file lock serializes the echo
write against the echo event's own `store` invocation, so the guard is race-free.

## Testing

- Unit (TDD): the `wl-paste --list-types` lookup and `wl-copy` spawn are package-level
  func vars in `main.go`, overridden in tests — covers the gate (single-mime, mime
  mismatch, text, multi-mime), the echo guard (skip once, re-own again on a fresh
  re-copy), and the kill switch.
- Live, without going near the fragile screenshot overlay: copy a PNG via a throwaway
  `wl-copy`, confirm copyzen re-owned it, kill the original owner, verify the image
  still pastes — the same death the portal dies, simulated safely.

## Consequence

After a screenshot, the clipboard owner is copyzen's `wl-copy`, so highlight, `•`, and
Ctrl+V always agree. The picker's fallback highlight (`active-index --fallback-newest`)
remains for genuinely empty clipboards: fresh logins and deliberate clears, which
persistence must not resurrect.

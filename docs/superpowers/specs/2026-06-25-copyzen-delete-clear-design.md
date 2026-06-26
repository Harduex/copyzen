# copyzen — delete (Ctrl+D) + clear-history (Ctrl+Shift+C) with minimalist confirmation

Design for v0.2.0 (shipped together with image thumbnails; see
`2026-06-25-copyzen-image-previews-design.md`). Adds two destructive picker actions, each gated by a two-step
confirmation, while keeping copyzen's identity intact: single static Go binary +
fuzzel, zero new runtime dependencies, works out of the box.

## Goal

In the `copyzen-menu` fuzzel picker:

1. **Delete a record (`Ctrl+D`)** — delete the highlighted item (works on history *and*
   pinned items). Minimalist confirm: a single-item confirm view; press `Ctrl+D` again to
   confirm, `Esc` (or anything else) to cancel.
2. **Clear history (`Ctrl+Shift+C`)** — clear ALL history, **keeping pinned items**.
   Minimalist confirm: the window becomes a confirmation text (history items hidden);
   press `Ctrl+Shift+C` again to confirm, `Esc` (or anything else) to cancel.

## Decisions (resolved during brainstorming)

- **Picker: stay with fuzzel.** Switching to fzf-in-a-terminal would add two runtime deps
  (fzf + a terminal emulator) and installer complexity for a modest gain — even fzf can't
  do "press the same key twice to confirm" without external state. fuzzel's relaunch
  "blink" is already an accepted pattern (pinning), and a visible state-change is *good*
  for a destructive action.
- **Clear-all key: `Control+Shift+c`.** `Ctrl+C` is a fuzzel default (`cancel`), so binding
  it would override a default — against the standing "don't override a dependency's
  defaults" rule. `Control+Shift+c` (lowercase `c`; fuzzel never fires upper-case Shift
  combos) is free, additive, and keeps the "C = clear" mnemonic.
- **Delete key: `Control+Shift+d`.** (Two corrections from real-machine testing: (1) neither
  `Control+d` NOR the bare `Delete` key is free — both are fuzzel's `delete-next` default, which
  the man page omits and only fuzzel's own config-parse error revealed. (2) `Shift+Delete` — the
  original pick — became fuzzel's `expunge` default in 1.11, so it errors on newer fuzzel;
  `Control+Shift+d` is free in both 1.9.2 and 1.12 and keeps a "D = delete" mnemonic alongside
  `Control+Shift+c`. Exit code stays custom-2 → 11, so the menu logic is unchanged.)
- **Delete confirm view: single item** (not full-list-with-marked-row). Unambiguous,
  symmetric with the clear view, and avoids fuzzel's inability to preselect/recolor a row.
- **Clear confirm shows counts** (e.g. `Clear 37 history items · 4 pinned kept`).
- **"Red" is a text/emoji marker** (🗑 / ⚠), not true color — fuzzel dmenu has no per-entry
  color. Accepted trade-off.

## Backend (unchanged — verified)

No store or `main.go` changes. The CLI already provides exactly what's needed:

- `copyzen delete <id>` — removes `id` from whichever bucket holds it (history OR pinned;
  ids are unique across buckets). Covered by `store.TestDelete`.
- `copyzen wipe` — clears the `history` bucket only; `pinned` and the id counter are
  untouched. Covered by `store.TestWipeKeepsPins`.

`copyzen list` renders `id<TAB>preview`, with pinned rows prefixed `★ ` in the preview
field (`store.FormatLine`). The wrapper counts pinned vs history lines from this output.

## Keybindings — `dist/fuzzel.ini`

All additive; none override a fuzzel default binding:

```ini
[key-bindings]
custom-1=Control+s         # toggle pin/unpin   -> exit 10 (existing)
custom-2=Control+Shift+d   # delete (confirmed) -> exit 11  (Shift+Delete=expunge, Control+Delete=delete-next-word on fuzzel >=1.11)
custom-3=Control+Shift+c   # clear all history  -> exit 12
```

fuzzel custom-N exit codes: custom-1→10, custom-2→11, custom-3→12. On a custom-key exit,
fuzzel prints the currently-highlighted entry.

## Wrapper — `scripts/copyzen-menu`

The wrapper becomes a small state machine. The bindings are static; the wrapper interprets
each exit code **by state**. Crucially: **in a confirm state, only the exact same key
confirms; Enter, Esc, or any other exit cancels back to the list.** A destructive action
never fires on stray input (fail-safe default).

States: `list` → `confirm-delete:<id>` → `list`, and `list` → `confirm-clear` → `list`.

### `list` state (normal picker)

Run `copyzen list | fuzzel <list-args>`; branch on exit code:

| Exit | Key            | Action                                                        |
|------|----------------|---------------------------------------------------------------|
| 0    | Enter          | `decode | wl-copy` the selection; **exit 0**                  |
| 10   | Ctrl+S         | `copyzen toggle <id>`; stay in `list` (reopen)               |
| 11   | Ctrl+D         | capture id+preview of selection → enter `confirm-delete:<id>`. If selection empty, stay in `list`. |
| 12   | Ctrl+Shift+C   | enter `confirm-clear`                                         |
| *    | Esc / cancel   | **exit 0** (quit)                                             |

### `confirm-delete:<id>` state

Show **only the captured item's preview** as the single entry, with a prompt:

```
🗑 Delete this entry?  Ctrl+D = confirm · Esc = cancel
```

(Run via `printf '%s\n' "$preview" | fuzzel --prompt "<text>" <args>`.) Branch:

| Exit | Meaning   | Action                                              |
|------|-----------|-----------------------------------------------------|
| 11   | Ctrl+D    | `copyzen delete <id>`; return to `list`             |
| *    | anything  | cancel; return to `list` (no delete)                |

The delete targets the **captured id**, not the cursor — so highlight position in the
confirm view is irrelevant.

### `confirm-clear` state

Compute counts from `copyzen list`:
- `pinned` = lines whose preview field begins with `★` (match tab+star).
- `total`  = non-empty lines; `history = total − pinned`.

Show **only confirmation text** (history items hidden), e.g. a single body line and prompt:

```
body:   Clear <history> history items · <pinned> pinned kept
prompt: ⚠ Clear ALL history?  Ctrl+Shift+C = confirm · Esc = cancel
```

Branch:

| Exit | Meaning      | Action                                          |
|------|--------------|-------------------------------------------------|
| 12   | Ctrl+Shift+C | `copyzen wipe`; return to `list`                |
| *    | anything     | cancel; return to `list` (no wipe)              |

## Edge cases

- **Ctrl+D on empty/no selection** — id parse yields nothing → stay in `list`, no-op.
- **Delete the last item** — list becomes empty (or pins-only); reopen shows that. Fine.
- **Delete a pinned item** — `copyzen delete` removes it from the `pinned` bucket. This is
  the intended "remove pinned one-by-one" path.
- **Clear with empty history** — `wipe` is a harmless no-op; counts show `0 history items`.
  Confirmation still shown (kept simple).
- **`--with-nth=2`** — already conditionally added for display; it hides the id column but
  does not affect fuzzel's printed output, so id capture/parse is unaffected. The confirm
  views reuse the same args so the id column stays hidden there too.

## Testing & verification

- **Store semantics** — already covered by `store.TestDelete` and
  `store.TestWipeKeepsPins`; no new Go tests needed.
- **Wrapper** — POSIX-sh control flow, verified by **running on the real machine** (this
  project's bugs have only ever surfaced by running). Manual checklist: delete a history
  item; cancel a delete; delete a pinned item; clear with mixed history+pins (pins
  survive, counts correct); cancel a clear; Ctrl+D on empty selection.

## Out of scope

- True per-entry color, in-place reload, or any fzf/terminal-based picker.
- Any store API change.
- Bulk-delete selection (multi-select). Clear-all + per-item delete cover the need.

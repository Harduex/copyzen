# copyzen — next session: delete (Ctrl+D) + clear-history (Ctrl+C) with minimalist confirmation

Paste this into a new chat (run from `/home/simeon/Projects/Other/copy-zen`). The project's
memory (commit rules, "don't override dependency defaults", git-push helper) auto-loads there.

## What copyzen is (already built + shipped)
Minimalist Wayland clipboard-history manager with pinning, in Go. Published: **github.com/Harduex/copyzen, v0.1.0 (public)**. It's a STORE driven by pipes — `wl-paste --no-newline --watch copyzen store` feeds history, `copyzen decode | wl-copy` pastes back, and fuzzel is the UI via `scripts/copyzen-menu` (bound to Super+V). Single static Go binary (`CGO_ENABLED=0`), `go.etcd.io/bbolt`. Full design/plan in `docs/superpowers/specs/2026-06-25-copyzen-design.md` and `…/plans/2026-06-25-copyzen.md`.

Architecture:
- `store/` (bbolt): buckets `history` (rolling, cap 100), `pinned` (never evicted), `meta` (one global id counter → ids unique across buckets). Byte-for-byte fidelity; eviction touches only history (property-tested). API: `Add, Get, List, Pin, Unpin, Toggle, Delete, Wipe` + pure `Preview/FormatLine/RenderList/ParseID`.
- `main.go`: CLI `store|list|decode|delete|pin|unpin|toggle|wipe|version` (reads stdin, byte-exact).
- `scripts/copyzen-menu`: POSIX-sh fuzzel wrapper. Loop: `copyzen list | fuzzel --dmenu --config <scoped ini>`; exit 0 (Enter) → `decode | wl-copy`, quit; exit 10 (Ctrl+S = `custom-1`) → `copyzen toggle`, **reopen** (the reopen "blink" is accepted); else quit.
- `dist/fuzzel.ini`: scoped config, `[key-bindings] custom-1=Control+s`. `dist/copyzen.service` (systemd-user recorder, preferred), `dist/copyzen.desktop` (autostart fallback).
- `install.sh` (GitHub-only: download + sha256 verify + /usr/local/bin + systemd recorder + COSMIC Super+V steps; env `COPYZEN_VERSION/PREFIX/COPYZEN_BASE_URL`). `scripts/copyzen-update` re-runs it. `.github/workflows/release.yml`: tag `v*` → build amd64/arm64 + checksums + GH Release.

Current picker UX: Enter=copy; **Ctrl+S = toggle pin/unpin** (pinned shown once with ★ at top, hidden from history; unpin moves item back to top of history); reopen-on-pin blink accepted.

## The work to add
1. **Delete a record — Ctrl+D**: in the picker, delete the highlighted item. Minimalist confirm: the item turns **red** and asks to press **Ctrl+D again** to confirm (or **Esc** to cancel).
2. **Clear history — Ctrl+C**: clear ALL history. Minimalist confirm: the window becomes a **confirmation text** (items hidden temporarily), asking to press **Ctrl+C again** to confirm (or Esc to cancel).
3. **Clear does NOT touch pinned items** — they're removed one-by-one with Ctrl+D, or unpinned first.

## Backend is essentially ready (verify, probably no store changes)
- `copyzen delete <id>` already removes from whichever bucket holds the id (history OR pinned) → **Ctrl+D → `copyzen delete`** (works on pinned items too).
- `copyzen wipe` already clears history only, keeping pinned → **Ctrl+C → `copyzen wipe`** (already satisfies "clear keeps pins").
- So the new work is almost entirely the **picker/wrapper** (new keybindings + the confirmation interaction) and `dist/fuzzel.ini` — not the store.

## ⚠ Resolve this design decision FIRST — do not jump to code
The requested confirmation UX runs into the **same wall as in-place pinning, but harder**:
- **fuzzel exits on every action key and cannot reload/recolor its list.** "The item turns red and waits for a second keypress" can't happen in place — it needs relaunching fuzzel into a confirm-state (a blink), and fuzzel dmenu has **no per-entry color**, so "red" can only be a text marker, not true red. (Re-verify, but this is the known limitation.)
- **Ctrl+C is a fuzzel default binding** (it's in fuzzel's default key list — verified earlier; likely "cancel"). Binding it to clear-history conflicts with a default — same class as the earlier Ctrl+P-vs-`prev` bug. **Ctrl+D is free.** The user's standing rule: *don't override a dependency's default bindings* (so it works for every new user). Resolve Ctrl+C with the user (override in the scoped config? choose another key?).
- **fzf-in-a-terminal** does true in-place reload + ANSI color (real red) + repeat-key handling natively — it matches this UX cleanly. It was offered before and deferred to keep fuzzel + zero extra deps. **These confirmation features may tip that decision.**

Likely forks to brainstorm with the user:
- **(A) Stay fuzzel** — approximate confirmations by relaunching into a confirm-state: the selected row replaced with a `‼ Ctrl+D again / Esc` marker (delete), or a single confirmation line with items hidden (clear). Blinks per step; no true red; must resolve the Ctrl+C conflict.
- **(B) Switch to fzf (terminal popup)** — true in-place red + reload + repeat-key confirm; installer provisions fzf (+ a small terminal like `foot`) per env. Bigger change; best UX match.

## Process & rules
- Superpowers workflow: **brainstorming → writing-plans → executing-plans (TDD) → code-review → finishing-a-development-branch**. Brainstorm the picker+confirmation design and get approval BEFORE coding. Project skills are in `.claude/skills/` (write-test-cases, code-review, debugger, formal-verification, architecture-diagrams).
- Running it on the real machine is essential — every real bug here surfaced only by running (ParseID trim order, bbolt `Stats()` eviction off-by-one, `wl-paste --watch` flag order). To test installs without GitHub, use `COPYZEN_BASE_URL=` against a local `python3 -m http.server` serving a built release (see how v0.1.0 was test-installed).
- Commits: author **`Harduex <simeon.simka@gmail.com>`, NO Claude co-author trailer** (repo-local git config already set).
- `git push` needs gh's helper: `git -c credential.helper= -c credential.helper='!gh auth git-credential' push origin <ref>` (global `wincred` is broken on Linux). `gh` is authed as Harduex.
- Confirm before any push/publish. Ship as **v0.2.0** (tag → release.yml builds it).
- fuzzel custom-N exit codes: custom-1→10, custom-2→11, custom-3→12. Ctrl+S=custom-1 already.

## First step
Read the repo + the two design docs, confirm `delete`/`wipe` semantics still fit, then **brainstorm the confirmation UX + the picker decision (fuzzel-approximate vs fzf) with me and propose a plan before implementing.**

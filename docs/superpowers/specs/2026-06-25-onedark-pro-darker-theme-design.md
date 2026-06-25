# copyzen — One Dark Pro Darker default theme

**Date:** 2026-06-25
**Status:** Approved (design)
**Scope:** Single file — `dist/fuzzel.ini`

## Goal

Make copyzen's default picker look like the **One Dark Pro Darker** VSCode theme,
out of the box, for fresh installs. Today `dist/fuzzel.ini` carries only the
`Control+s` pin binding and no colors, so the picker falls back to fuzzel's
built-in theme.

## Approach

copyzen never draws its own UI — the picker *is* fuzzel, launched by
`scripts/copyzen-menu` with `--config "$XDG_CONFIG_HOME/copyzen/fuzzel.ini"`.
So theming is purely a matter of what that config file contains. We add `[main]`
(font) and `[colors]` blocks to the shipped default `dist/fuzzel.ini`. No Go
code, no installer changes, no script changes.

## Color mapping

fuzzel colors are `RRGGBBAA` (8 hex digits, no `#`). Values are taken from the
official theme file
[`OneDark-Pro-darker.json`](https://raw.githubusercontent.com/Binaryify/OneDark-Pro/master/themes/OneDark-Pro-darker.json).

| fuzzel key        | One Dark Pro Darker source            | value      |
| ----------------- | ------------------------------------- | ---------- |
| `background`      | `editor.background`                   | `23272eff` |
| `text`            | `editor.foreground`                   | `abb2bfff` |
| `match`           | accent blue (matched fuzzy chars)     | `61afefff` |
| `selection`       | `list.activeSelectionBackground`      | `2c313aff` |
| `selection-text`  | `list.activeSelectionForeground`      | `d7dae0ff` |
| `selection-match` | accent blue on the selected row       | `61afefff` |
| `border`          | `focusBorder`                         | `3e4452ff` |
| `counter`         | comment grey                          | `7f848eff` |
| `placeholder`     | comment grey                          | `7f848eff` |

## Font

`font=monospace:size=11`. The generic `monospace` family is always resolved by
fontconfig to the user's configured monospace face, so the shipped default can
never break the picker on a system that lacks a specific font. A comment shows
how to swap in a named face (e.g. `JetBrainsMono Nerd Font:size=11`).

## Resulting `dist/fuzzel.ini`

```ini
# Scoped fuzzel config for copyzen-menu (loaded via --config) so this binding
# never touches your global fuzzel config.

[main]
# Generic monospace resolves via fontconfig to your system mono — always present.
# Swap for a specific face if you like, e.g. font=JetBrainsMono Nerd Font:size=11
font=monospace:size=11

# One Dark Pro Darker palette (from Binaryify/OneDark-Pro OneDark-Pro-darker.json).
[colors]
background=23272eff
text=abb2bfff
match=61afefff
selection=2c313aff
selection-text=d7dae0ff
selection-match=61afefff
border=3e4452ff
counter=7f848eff
placeholder=7f848eff

# Control+s is unused by fuzzel's default key-bindings, so adding it for pinning
# overrides no built-in binding (Control+p is taken by `prev`).
[key-bindings]
custom-1=Control+s
```

## Out of scope / notes

- **Existing users are unaffected.** `install.sh:78` keeps any existing
  `~/.config/copyzen/fuzzel.ini` and only writes the default on a fresh install.
  To adopt the new theme, an existing user deletes their file and re-runs the
  installer, or copies these `[main]`/`[colors]` blocks into their own config.
- The `Control+s` binding **must** stay — `copyzen-menu` keys pin/unpin off
  fuzzel exit code `10` from `custom-1`.
- No layout chrome (border width/radius, padding) is changed; a color theme is
  just colors plus the requested font.

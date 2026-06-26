# Releasing copyzen (maintainers)

copyzen has no version constant to bump: `main.go` ships `version = "dev"` and the release
workflow injects the real version from the git tag (`-ldflags -X main.version=$tag`). A
release is therefore just a **`v*` tag pushed to GitHub** —
[`.github/workflows/release.yml`](../.github/workflows/release.yml) builds the static
amd64/arm64 binaries, generates checksums, and publishes a GitHub Release with the installer
assets. Users then pull it with `copyzen-update`.

## Steps

1. **Test the working tree on a real Wayland session.** copyzen's bugs (wl-paste flag order,
   eviction off-by-one, ParseID, fuzzel keybinding defaults) have only ever surfaced by
   running it, not from unit tests. Install the current build over your system copy:

   ```sh
   scripts/copyzen-devinstall install vX.Y.Z
   ```

   Open the picker (Super+V) and exercise the change. **Do not tag until it's verified.**

2. **Tag and push.** Tagging publishes a public release — be sure first.

   ```sh
   git tag -a vX.Y.Z -m "copyzen vX.Y.Z — <summary>"
   git push origin main vX.Y.Z
   ```

   The tag push triggers `release.yml`. Watch it: `gh run watch` (or `gh run list`).

3. **Verify and adopt.** Once published (`gh release view vX.Y.Z`), pull the verified,
   checksum-validated build:

   ```sh
   copyzen-update            # or: scripts/copyzen-devinstall restore
   ```

## Notes

- **When `dist/fuzzel.ini` changes, append its new `sha256sum dist/fuzzel.ini` to
  `shipped_default_hashes` in `install.sh`** (keep the older hashes). That list is how the
  installer refreshes the config for users still on an *unmodified* older default so fixes
  (e.g. a keybinding a newer fuzzel claimed) reach them; a *customized* config is never
  overwritten — it gets a `fuzzel.ini.new` to merge instead.
- Releases build with **Go 1.23** (pinned in `release.yml`); keep `go.mod`'s `go` directive
  at or below that, with no `toolchain` line, so CI doesn't try to fetch a newer toolchain.
- `scripts/copyzen-devinstall` is a dev-only helper (build + install the working tree, or
  `restore` the released build). It is intentionally **not** shipped as a release asset.

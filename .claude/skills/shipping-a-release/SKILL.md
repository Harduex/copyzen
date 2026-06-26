---
name: shipping-a-release
description: Use when releasing, deploying, shipping, cutting, tagging, or publishing a new copyzen version — anything that ends in a git tag and a GitHub release.
---

# Shipping a copyzen release

A release is just a **`v*` tag pushed to GitHub**: `main.go` ships `version = "dev"` and
`.github/workflows/release.yml` injects the real version from the tag, builds the static
amd64/arm64 binaries, and publishes the GitHub Release with the installer assets. Users
then run `copyzen-update`. The full command reference lives in
[`docs/RELEASING.md`](../../../docs/RELEASING.md) — this skill is the agent procedure and
the guardrails.

## Procedure

1. **Install the working-tree build over the system copy** (sudo for `/usr/local/bin` —
   run it as the normal user, the script elevates only the install step):

   ```sh
   scripts/copyzen-devinstall install vX.Y.Z
   ```

2. **Ask the user to test, then STOP and wait for explicit confirmation.** Give them the
   change's real-machine checklist. A failing or unrun test is a hard stop — fix and
   reinstall (step 1); do not tag.

3. **On confirmation, deploy.** Fast-forward `main` to the work, then tag and push (use the
   repo's normal git auth):

   ```sh
   git checkout main && git merge --ff-only <branch>
   git tag -a vX.Y.Z -m "copyzen vX.Y.Z — <summary>"
   git push origin main vX.Y.Z
   ```

   The tag push triggers `release.yml`; watch it with `gh run watch`.

4. **Prompt the user to update** once the release is published — this replaces the local
   dev build with the verified, checksum-validated release:

   ```sh
   copyzen-update            # or: scripts/copyzen-devinstall restore
   ```

## Guardrails

- **No tag before a confirmed real-machine test** (step 2) — tests passing ≠ verified.
- **No push without an explicit go-ahead** (step 3) — tagging publishes to a public repo.
- **Leave nothing on an unverified dev build**: finish with `copyzen-update` / `restore`.
- **If `dist/fuzzel.ini` changed this release, append its new `sha256sum` to
  `shipped_default_hashes` in `install.sh`** before tagging — otherwise the config fix won't
  reach existing users on update (see docs/RELEASING.md).

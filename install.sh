#!/bin/sh
# copyzen installer — downloads the binary + assets from GitHub Releases, verifies
# the checksum, installs to /usr/local/bin, and sets up the clipboard recorder.
# Idempotent. Env overrides:
#   COPYZEN_VERSION=vX.Y.Z   pin a release (default: latest)
#   PREFIX=/path             install dir (default: /usr/local/bin)
#   COPYZEN_BASE_URL=url      fetch assets from a custom base (mirror / offline / testing)
set -eu

REPO="Harduex/copyzen"
PREFIX="${PREFIX:-/usr/local/bin}"
VERSION="${COPYZEN_VERSION:-latest}"
if [ -n "${COPYZEN_BASE_URL:-}" ]; then
	BASE="$COPYZEN_BASE_URL"
elif [ "$VERSION" = "latest" ]; then
	BASE="https://github.com/$REPO/releases/latest/download"
else
	BASE="https://github.com/$REPO/releases/download/$VERSION"
fi

say()  { printf '==> %s\n' "$1"; }
warn() { printf 'warning: %s\n' "$1" >&2; }
die()  { printf 'error: %s\n' "$1" >&2; exit 1; }

os="$(uname -s)"
[ "$os" = "Linux" ] || die "copyzen supports Linux/Wayland only (got $os)."
arch="$(uname -m)"
case "$arch" in
	x86_64 | amd64) arch="amd64" ;;
	aarch64 | arm64) arch="arm64" ;;
	*) die "unsupported architecture: $arch (need x86_64 or aarch64)." ;;
esac
asset="copyzen-linux-$arch"

command -v curl >/dev/null 2>&1 || die "missing required tool: curl"
command -v install >/dev/null 2>&1 || die "missing required tool: install"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
say "Downloading $asset ($VERSION)…"
for f in "$asset" sha256sums.txt copyzen-menu copyzen-update fuzzel.ini copyzen.service copyzen.desktop; do
	curl -fsSL "$BASE/$f" -o "$tmp/$f" || die "download failed: $BASE/$f"
done

say "Verifying checksum…"
if command -v sha256sum >/dev/null 2>&1; then
	have="$(sha256sum "$tmp/$asset" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
	have="$(shasum -a 256 "$tmp/$asset" | awk '{print $1}')"
else
	die "no sha256 tool (sha256sum or shasum) found."
fi
want="$(awk -v a="$asset" '$2 == a || $2 == "*" a {print $1}' "$tmp/sha256sums.txt")"
[ -n "$want" ] || die "no checksum for $asset in sha256sums.txt."
[ "$want" = "$have" ] || die "checksum mismatch for $asset (expected $want, got $have)."

install_bin() { # src destname
	if [ -w "$PREFIX" ]; then
		install -m 0755 "$1" "$PREFIX/$2"
	elif command -v sudo >/dev/null 2>&1 && [ -e /dev/tty ]; then
		printf 'Install to %s needs root. Use sudo? [y/N] ' "$PREFIX" >/dev/tty
		read -r ans </dev/tty || ans=""
		case "$ans" in
			[yY]*) sudo install -m 0755 "$1" "$PREFIX/$2" ;;
			*) die "aborted; re-run with write access to $PREFIX." ;;
		esac
	else
		die "$PREFIX not writable and sudo unavailable; run as root or set PREFIX."
	fi
}
say "Installing copyzen, copyzen-menu, copyzen-update to $PREFIX…"
install_bin "$tmp/$asset" copyzen
install_bin "$tmp/copyzen-menu" copyzen-menu
install_bin "$tmp/copyzen-update" copyzen-update

cfg_dir="${XDG_CONFIG_HOME:-$HOME/.config}/copyzen"
mkdir -p "$cfg_dir"
if [ -f "$cfg_dir/fuzzel.ini" ]; then
	say "Keeping existing $cfg_dir/fuzzel.ini"
else
	install -m 0644 "$tmp/fuzzel.ini" "$cfg_dir/fuzzel.ini"
	say "Wrote $cfg_dir/fuzzel.ini"
fi

for dep in wl-paste wl-copy fuzzel; do
	command -v "$dep" >/dev/null 2>&1 || warn "'$dep' not found — install it (wl-clipboard provides wl-paste/wl-copy; fuzzel is its own package)."
done
if command -v fuzzel >/dev/null 2>&1 && ! fuzzel --help 2>&1 | grep -q -- '--with-nth'; then
	warn "your fuzzel lacks --with-nth: the id column shows in the picker (cosmetic). Ctrl+S pinning still works; update fuzzel to hide it."
fi

# Recorder: start now + on every login. Prefer a systemd --user service (auto-restarts on
# crash); fall back to an XDG autostart entry where a systemd --user instance isn't usable.
# wl-paste options MUST precede --watch — everything after --watch is the command to run.
cfg="${XDG_CONFIG_HOME:-$HOME/.config}"
autostart="$cfg/autostart/copyzen.desktop"
recorder_done=0
if command -v systemctl >/dev/null 2>&1 && systemctl --user show-environment >/dev/null 2>&1; then
	mkdir -p "$cfg/systemd/user"
	install -m 0644 "$tmp/copyzen.service" "$cfg/systemd/user/copyzen.service"
	systemctl --user daemon-reload
	if systemctl --user enable --now copyzen.service >/dev/null 2>&1 &&
		[ "$(systemctl --user is-active copyzen.service 2>/dev/null)" = active ]; then
		rm -f "$autostart" # one mechanism only — drop autostart so it can't double the recorder
		recorder_done=1
		say "Recorder: systemd --user service enabled (starts on login, auto-restarts)."
	else
		systemctl --user disable copyzen.service >/dev/null 2>&1 || true
		warn "systemd --user service didn't start; falling back to XDG autostart."
	fi
fi
if [ "$recorder_done" = 0 ]; then
	mkdir -p "$(dirname "$autostart")"
	install -m 0644 "$tmp/copyzen.desktop" "$autostart"
	say "Recorder: installed XDG autostart entry (starts on next login)."
	if command -v wl-paste >/dev/null 2>&1 && ! pgrep -f 'wl-paste.*--watch.*copyzen store$' >/dev/null 2>&1; then
		setsid wl-paste --no-newline --watch copyzen store >/dev/null 2>&1 &
		say "Started the clipboard recorder for this session."
	fi
fi

cat <<EOF

copyzen installed.
  binary : $PREFIX/copyzen
  picker : $PREFIX/copyzen-menu
  config : $cfg_dir/fuzzel.ini
  history: \${XDG_DATA_HOME:-\$HOME/.local/share}/copyzen/store.db

Bind Super+V in COSMIC:
  Settings → Desktop → Keyboard → Custom Shortcuts → Add Shortcut
    Command : copyzen-menu
    Shortcut: Super+V

Pinning: open the picker (Super+V), highlight an entry, press Ctrl+S to pin/unpin.
Pinned entries show a ★ at the top, are hidden from history, and survive rollover.
Toggle from a terminal too:  copyzen list   then   echo <id> | copyzen toggle

The recorder is running now and starts on every login. To stop it:
  systemctl --user disable --now copyzen.service   # (systemd setup)
  # or remove ~/.config/autostart/copyzen.desktop  # (autostart fallback)

Update any time:  copyzen-update
EOF

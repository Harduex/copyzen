#!/bin/sh
# copyzen installer — downloads the binary + assets from GitHub Releases, verifies
# the checksum, installs to /usr/local/bin, and sets up the clipboard recorder.
# Idempotent. Pin a version with COPYZEN_VERSION=vX.Y.Z; default installs latest.
set -eu

REPO="Harduex/copyzen"
PREFIX="/usr/local/bin"
VERSION="${COPYZEN_VERSION:-latest}"
if [ "$VERSION" = "latest" ]; then
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
for f in "$asset" sha256sums.txt copyzen-menu fuzzel.ini copyzen.service copyzen.desktop; do
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
say "Installing copyzen and copyzen-menu to $PREFIX…"
install_bin "$tmp/$asset" copyzen
install_bin "$tmp/copyzen-menu" copyzen-menu

cfg_dir="${XDG_CONFIG_HOME:-$HOME/.config}/copyzen"
mkdir -p "$cfg_dir"
if [ -f "$cfg_dir/fuzzel.ini" ]; then
	say "Keeping existing $cfg_dir/fuzzel.ini"
else
	install -m 0644 "$tmp/fuzzel.ini" "$cfg_dir/fuzzel.ini"
	say "Wrote $cfg_dir/fuzzel.ini"
fi

autostart_dir="${XDG_CONFIG_HOME:-$HOME/.config}/autostart"
mkdir -p "$autostart_dir"
install -m 0644 "$tmp/copyzen.desktop" "$autostart_dir/copyzen.desktop"
say "Installed autostart entry: $autostart_dir/copyzen.desktop"

unit_dir="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
mkdir -p "$unit_dir"
install -m 0644 "$tmp/copyzen.service" "$unit_dir/copyzen.service"
say "Shipped systemd unit (alternative, not enabled): $unit_dir/copyzen.service"

for dep in wl-paste wl-copy fuzzel; do
	command -v "$dep" >/dev/null 2>&1 || warn "'$dep' not found — install it (wl-clipboard provides wl-paste/wl-copy; fuzzel is its own package)."
done
if command -v fuzzel >/dev/null 2>&1 && ! fuzzel --help 2>&1 | grep -q -- '--with-nth'; then
	warn "your fuzzel lacks --with-nth: the id column shows in the picker (cosmetic). Ctrl+S pinning still works; update fuzzel to hide it."
fi

if pgrep -f 'wl-paste --watch.*copyzen store$' >/dev/null 2>&1; then
	say "Recorder already running."
elif command -v wl-paste >/dev/null 2>&1; then
	setsid wl-paste --watch --no-newline copyzen store >/dev/null 2>&1 &
	say "Started the clipboard recorder."
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

Pinning: open the picker (Super+V), highlight an entry, press Ctrl+S. Pinned
entries show a ★ at the top and survive history rollover. Unpin from a terminal:
  copyzen list          # find the ★ entry's id
  echo <id> | copyzen unpin

Recorder runs via XDG autostart. To use systemd instead:
  systemctl --user enable --now copyzen.service
  rm $autostart_dir/copyzen.desktop   # so you don't run two recorders
EOF

#!/bin/sh
# Installs the plimsoll binary for this machine from a GitHub release and checks its SHA-256 checksum.
#   curl -fsSL https://raw.githubusercontent.com/juzhxng/plimsoll/main/install.sh | sh
# PLIMSOLL_VERSION picks a release (for example v0.1.0; default: the latest).
# PLIMSOLL_INSTALL_DIR picks the folder (default: ~/.local/bin).
set -eu

repo=https://github.com/juzhxng/plimsoll
version=${PLIMSOLL_VERSION:-latest}
dir=${PLIMSOLL_INSTALL_DIR:-$HOME/.local/bin}

case $(uname -s) in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) echo "plimsoll: no release for $(uname -s); build it with Go instead" >&2; exit 1 ;;
esac
case $(uname -m) in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) echo "plimsoll: no release for $(uname -m); build it with Go instead" >&2; exit 1 ;;
esac

if [ "$version" = latest ]; then base=$repo/releases/latest/download; else base=$repo/releases/download/$version; fi
name=plimsoll_${os}_${arch}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

curl -fsSL -o "$work/$name" "$base/$name"
curl -fsSL -o "$work/checksums.txt" "$base/checksums.txt"
grep " $name\$" "$work/checksums.txt" > "$work/sum" || { echo "plimsoll: $name is not in checksums.txt" >&2; exit 1; }
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$work" && sha256sum -c --status sum)
else
  (cd "$work" && shasum -a 256 -c --status sum)
fi || { echo "plimsoll: checksum mismatch, nothing installed" >&2; exit 1; }

mkdir -p "$dir"
chmod 755 "$work/$name"
mv "$work/$name" "$dir/plimsoll"
echo "plimsoll installed to $dir/plimsoll"
case ":$PATH:" in *":$dir:"*) ;; *) echo "add $dir to your PATH to run it by name" ;; esac

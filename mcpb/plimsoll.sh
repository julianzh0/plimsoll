#!/bin/sh
# Starts the plimsoll binary for this machine. An MCP bundle names one command per operating system and none per
# processor, so on macOS and Linux this script picks the binary by the processor.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) echo "plimsoll: this bundle has no binary for $(uname -s); install plimsoll from its release instead" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) echo "plimsoll: this bundle has no binary for $(uname -m); install plimsoll from its release instead" >&2; exit 1 ;;
esac
binary="$here/plimsoll_${os}_${arch}"
[ -x "$binary" ] || chmod u+x "$binary"
exec "$binary" "$@"

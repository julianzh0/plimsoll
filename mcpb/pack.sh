#!/bin/sh
# Packs the release binaries in a folder into plimsoll.mcpb beside them and writes the server.json the MCP registry
# receives, with the bundle's SHA-256.
# Usage: mcpb/pack.sh <folder with the plimsoll_<os>_<arch> binaries> <version without v> <server.json to write>
set -eu
dist=$1
version=$2
out=$3
root=$(cd "$(dirname "$0")/.." && pwd)
# The committed server.json names the release it is for; a release whose version it does not name stops here.
listed=$(jq -r '.version' "$root/server.json")
test "$listed" = "$version" || { echo "server.json names version $listed, not $version: update it with the release" >&2; exit 1; }
jq -e --arg v "$version" '.packages[0].identifier | endswith("/v" + $v + "/plimsoll.mcpb")' "$root/server.json" >/dev/null ||
  { echo "server.json's bundle URL is not the one of v$version" >&2; exit 1; }
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
mkdir "$stage/server"
jq --arg v "$version" '.version = $v' "$root/mcpb/manifest.json" > "$stage/manifest.json"
cp "$root/LICENSE" "$root/THIRD_PARTY_NOTICES.md" "$stage/"
cp "$root/mcpb/plimsoll.sh" "$stage/server/"
for binary in plimsoll_darwin_amd64 plimsoll_darwin_arm64 plimsoll_linux_amd64 plimsoll_linux_arm64 plimsoll_windows_amd64.exe; do
  cp "$dist/$binary" "$stage/server/"
done
chmod 755 "$stage/server/"*
# The official packer validates the manifest and writes the archive (pinned; a development tool, not a dependency).
npx --yes @anthropic-ai/mcpb@2.1.2 pack "$stage" "$dist/plimsoll.mcpb"
sum=$( (sha256sum "$dist/plimsoll.mcpb" 2>/dev/null || shasum -a 256 "$dist/plimsoll.mcpb") | cut -d' ' -f1)
jq --arg sum "$sum" '.packages[0].fileSha256 = $sum' "$root/server.json" > "$out"
echo "plimsoll.mcpb $sum"

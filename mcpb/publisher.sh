#!/bin/sh
# Puts the pinned mcp-publisher into the given folder after verifying its Sigstore bundle against the registry's own
# release workflow, as the registry's documentation does. Needs cosign. Usage: mcpb/publisher.sh <folder>
set -eu
version=v1.8.1
os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
archive="mcp-publisher_${os}_${arch}.tar.gz"
base="https://github.com/modelcontextprotocol/registry/releases/download/$version"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
curl --fail --location --silent --show-error --output "$work/$archive" "$base/$archive"
curl --fail --location --silent --show-error --output "$work/$archive.sigstore.json" "$base/$archive.sigstore.json"
cosign verify-blob --bundle "$work/$archive.sigstore.json" \
  --certificate-identity "https://github.com/modelcontextprotocol/registry/.github/workflows/release.yml@refs/tags/$version" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com "$work/$archive"
tar -xzf "$work/$archive" -C "$1" mcp-publisher

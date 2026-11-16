#!/bin/sh
# build.sh record|check: builds every grammar module of pins.txt from its pinned sources with zig.
#   record: writes <name>.wasm.gz and fills the archive and module hashes still marked "-" in pins.txt.
#   check:  rebuilds and fails, naming each module, when a hash differs from pins.txt or from the committed file.
# GRAMMAR_WORK is the folder for archives and builds (default bin/_grammars). Run from the repository's root.
set -eu
mode=$1
here=internal/parse/grammars
work=${GRAMMAR_WORK:-bin/_grammars}
mkdir -p "$work"
sha() { shasum -a 256 "$1" | cut -d' ' -f1; }

# fetch <name> <repository> <commit> <archive-sha256>: unpacks the archive into $work/<name>-<commit>, checking its
# hash, and sets $tree to that folder. A folder unpacked before is used as it is.
fetch() {
  archive=$work/$1-$3.tar.gz
  tree=$work/$1-$3
  [ -f "$archive" ] || curl -fsSL "$2/archive/$3.tar.gz" -o "$archive"
  got=$(sha "$archive")
  if [ "$4" != "-" ] && [ "$4" != "$got" ]; then echo "$1: archive hash $got, pinned $4" >&2; exit 1; fi
  if [ "$4" = "-" ]; then set_field "$1" 4 "$got"; fi
  if [ ! -d "$tree" ]; then
    mkdir -p "$tree.part"
    tar -xzf "$archive" -C "$tree.part" --strip-components 1
    mv "$tree.part" "$tree"
  fi
}

# set_field <name> <field> <value>: replaces one field of a row of pins.txt (record mode only).
set_field() {
  [ "$mode" = record ] || { echo "$1: field $2 of pins.txt is not recorded" >&2; exit 1; }
  awk -v n="$1" -v f="$2" -v v="$3" '$1 == n && !/^#/ { $f = v } { print }' "$here/pins.txt" > "$work/pins.txt"
  mv "$work/pins.txt" "$here/pins.txt"
}

core_row=$(awk '$1 == "core"' "$here/pins.txt")
set -- $core_row
fetch core "$2" "$3" "$4"
core=$tree

: > "$work/failed"
grep -v '^#' "$here/pins.txt" | awk '$7 != "-" && $1 != "core"' | while read -r name repository commit archive_sha licence folder function module_sha; do
  fetch "$name" "$repository" "$commit" "$archive_sha"
  grammar=$tree/$folder
  scanner=""
  [ -f "$grammar/scanner.c" ] && scanner=$grammar/scanner.c
  # One C file at a time, then one link: zig compiles the files of one command side by side, and a large grammar's
  # parser.c alone takes gigabytes.
  objects=""
  for source in "$core/lib/src/lib.c" "$grammar/parser.c" $scanner "$here/shim.c"; do
    object=$work/$name-$(basename "$source" .c).o
    zig cc -c -target wasm32-wasi -O2 -Werror -DLANGUAGE="$function" \
      -D_POSIX_C_SOURCE=200112L -D_DEFAULT_SOURCE \
      -I "$core/lib/include" -I "$core/lib/src" -I "$core/lib/src/wasm" -I "$grammar" \
      "$source" -o "$object"
    objects="$objects $object"
  done
  zig cc -target wasm32-wasi -O2 -s -mexec-model=reactor $objects -o "$work/$name.wasm"
  got=$(sha "$work/$name.wasm")
  if [ "$mode" = record ]; then
    gzip -9 -n -c "$work/$name.wasm" > "$here/$name.wasm.gz"
    set_field "$name" 8 "$got"
    echo "$name: $got"
  else
    committed=$(gzip -dc "$here/$name.wasm.gz" | shasum -a 256 | cut -d' ' -f1)
    if [ "$got" != "$module_sha" ] || [ "$committed" != "$module_sha" ]; then
      echo "$name: built $got, committed $committed, pinned $module_sha" >&2
      echo "$name" >> "$work/failed"
    fi
  fi
done
if [ "$mode" = check ]; then
  if [ -s "$work/failed" ]; then exit 1; fi
  echo "every module matches the pins"
fi

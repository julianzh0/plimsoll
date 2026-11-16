package parse

import (
	"bytes"
	"compress/gzip"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"
)

// modules are the grammar modules, gzip-compressed; grammars/pins.txt names their sources and hashes.
//
//go:embed grammars/*.wasm.gz
var modules embed.FS

// Grammars returns the names of the embedded grammars, sorted.
func Grammars() []string {
	entries, _ := fs.ReadDir(modules, "grammars")
	var names []string
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e.Name(), ".wasm.gz"))
	}
	sort.Strings(names)
	return names
}

// moduleBytes inflates the named grammar's module.
func moduleBytes(grammar string) ([]byte, error) {
	compressed, err := modules.ReadFile("grammars/" + grammar + ".wasm.gz")
	if err != nil {
		return nil, fmt.Errorf("no grammar named %q", grammar)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("the %s grammar's module: %w", grammar, err)
	}
	return io.ReadAll(reader)
}

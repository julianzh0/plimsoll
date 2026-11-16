package parse

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero"
)

func TestModulesImportOnlyTheFailingHost(t *testing.T) {
	ctx := context.Background()
	runtime := wazero.NewRuntime(ctx)
	t.Cleanup(func() { _ = runtime.Close(ctx) })
	offered := []string{"wasi_snapshot_preview1.fd_close", "wasi_snapshot_preview1.fd_seek", "wasi_snapshot_preview1.fd_write",
		"wasi_snapshot_preview1.proc_exit"}
	for _, name := range Grammars() {
		bytes, err := moduleBytes(name)
		if err != nil {
			t.Fatalf("moduleBytes(%s) error = %v", name, err)
		}
		compiled, err := runtime.CompileModule(ctx, bytes)
		if err != nil {
			t.Fatalf("CompileModule(%s) error = %v", name, err)
		}
		var got []string
		for _, f := range compiled.ImportedFunctions() {
			module, field, _ := f.Import()
			got = append(got, module+"."+field)
		}
		for _, f := range got {
			if !slices.Contains(offered, f) {
				t.Errorf("imports of %s = %v, want only some of %v", name, got, offered)
			}
		}
	}
}

func TestEmbeddedModulesMatchThePins(t *testing.T) {
	pins, err := os.ReadFile("grammars/pins.txt")
	if err != nil {
		t.Fatal(err)
	}
	pinned := map[string]string{}
	for _, line := range strings.Split(string(pins), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 8 && !strings.HasPrefix(line, "#") && fields[6] != "-" {
			pinned[fields[0]] = fields[7]
		}
	}
	var names []string
	for name := range pinned {
		names = append(names, name)
	}
	sort.Strings(names)
	if got := Grammars(); !reflect.DeepEqual(got, names) {
		t.Errorf("Grammars() = %v, want the grammar rows of pins.txt %v", got, names)
	}
	for _, name := range Grammars() {
		bytes, err := moduleBytes(name)
		if err != nil {
			t.Fatalf("moduleBytes(%s) error = %v", name, err)
		}
		sum := sha256.Sum256(bytes)
		if got := hex.EncodeToString(sum[:]); got != pinned[name] {
			t.Errorf("SHA-256 of the embedded %s module = %s, want %s from pins.txt", name, got, pinned[name])
		}
	}
}

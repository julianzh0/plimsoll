package conventions_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// ruleKinds reads, from the detector packages' source, every "<detector>.<rule>" a detector can give: the value of a
// package's `name` constant joined with each `rule…` constant. A rule added to a detector is found without a list.
func ruleKinds(t *testing.T) []string {
	t.Helper()
	dirs, err := filepath.Glob("../*")
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, dir := range dirs {
		files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		consts := map[string]string{}
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			ast.Inspect(f, func(n ast.Node) bool {
				spec, ok := n.(*ast.ValueSpec)
				if !ok {
					return true
				}
				for i, id := range spec.Names {
					if i < len(spec.Values) {
						if lit, ok := spec.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
							consts[id.Name], _ = strconv.Unquote(lit.Value)
						}
					}
				}
				return true
			})
		}
		detector, found := consts["name"]
		if !found {
			continue
		}
		for constName, rule := range consts {
			if strings.HasPrefix(constName, "rule") && rule != "" && !strings.ContainsAny(rule, " :") {
				kinds = append(kinds, detector+"."+rule)
			}
		}
	}
	return kinds
}

func TestEveryKindOfMarkIsInExactlyOneArea(t *testing.T) {
	areas := conventions.Default().Areas
	kinds := ruleKinds(t)
	if len(kinds) < 17 {
		t.Fatalf("found %d kinds of mark in the detectors' source, want at least 17: %q", len(kinds), kinds)
	}
	for _, kind := range kinds {
		var in []string
		for _, a := range areas {
			for _, k := range a.Kinds {
				if k == kind {
					in = append(in, a.Name)
				}
			}
		}
		if len(in) != 1 {
			t.Errorf("%s is in the areas %q, want exactly one", kind, in)
		}
	}
	want := []string{"size", "dependencies", "wiring", "functions", "settings", "process"}
	for i, a := range areas {
		if i >= len(want) || a.Name != want[i] {
			t.Fatalf("areas = %+v, want %q in this order", areas, want)
		}
	}
}

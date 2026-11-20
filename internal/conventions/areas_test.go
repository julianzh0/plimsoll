package conventions_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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
	var names []string
	for _, a := range areas {
		names = append(names, a.Name)
	}
	if want := []string{"size", "dependencies", "wiring", "functions", "settings", "process"}; !slices.Equal(names, want) {
		t.Fatalf("areas %q, want %q in this order", names, want)
	}
	for _, kind := range kinds {
		var in []string
		for _, a := range areas {
			if slices.Contains(a.Kinds, kind) {
				in = append(in, a.Name)
			}
		}
		if len(in) != 1 {
			t.Errorf("%s is in the areas %q, want exactly one", kind, in)
		}
	}
}

// The rules page and the setup page name every mark the code can give, and no other: the setup page links to the
// definitions instead of restating them, so the three lists are the only thing that could drift.
func TestDocumentsNameEveryMarkOfTheCode(t *testing.T) {
	read := func(content []byte, err error) string {
		if err != nil {
			t.Fatal(err)
		}
		return string(content)
	}
	tableIDs := func(content string) (ids []string, rows []string) {
		for line := range strings.Lines(content) {
			if id := markRow.FindStringSubmatch(line); id != nil {
				ids, rows = append(ids, id[1]), append(rows, line)
			}
		}
		slices.Sort(ids)
		return ids, rows
	}
	code := ruleKinds(t)
	slices.Sort(code)
	rules, _ := tableIDs(read(os.ReadFile("../../docs/rules.md")))
	setup, rows := tableIDs(read(os.ReadFile("../../docs/agent-setup.md")))
	if !slices.Equal(rules, code) || !slices.Equal(setup, code) {
		t.Errorf("mark identifiers: docs/rules.md %q, docs/agent-setup.md %q; want the code's %q on both", rules, setup, code)
	}
	for _, row := range rows {
		cells := strings.Split(row, "|")
		if len(cells) < 5 || strings.TrimSpace(cells[3]) == "" || strings.ContainsAny(row, "0123456789") {
			t.Errorf("setup page row %q: want an identifier, what it measures and a suggestion, and no limit number", strings.TrimSpace(row))
		}
	}
}

// markRow is a table row that starts with a mark identifier: "| `detector.rule` …" or "| `detector.rule:<subject>` …".
var markRow = regexp.MustCompile("^\\| `([a-z]+(?:-[a-z]+)*\\.[a-z]+(?:-[a-z]+)*)[`:]")

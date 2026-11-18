package snapshot

import (
	"slices"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/parse"
)

// readUnits reads one file with its declarations.
func readUnits(t *testing.T, path, content string) reading {
	t.Helper()
	parser, err := parse.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(parser.Close)
	readings := NewReadings(parser)
	language, _ := conventions.Default().LanguageOf(path)
	return readings.of(language, language.GrammarOf(path), []byte(content), readings.begin(), wanted{declarations: true})
}

// unitsOf reads one file with its declarations and gives its units as "<names> in <context>" and its pure imports'
// first words.
func unitsOf(t *testing.T, path, content string) (units, imports []string) {
	t.Helper()
	r := readUnits(t, path, content)
	if r.unread || r.syntaxError {
		t.Fatalf("%s: unread %v, syntax error %v", path, r.unread, r.syntaxError)
	}
	for _, u := range r.units {
		line := strings.Join(u.names, ",")
		if u.context != "" {
			line += " in " + strings.ReplaceAll(strings.TrimSuffix(u.context, "\n"), "\n", "/")
		}
		units = append(units, line)
	}
	for _, s := range r.pureImports {
		imports = append(imports, strings.Fields(content[s.start : s.end+1])[0])
	}
	return units, imports
}

func TestUnitsAreTheDeclarationsOfAFileWithTheNamesTheyDeclareDirectly(t *testing.T) {
	cases := []struct {
		path, content string
		units         []string // in file order
		imports       int
	}{
		{"a.go", "package a\n\nimport \"fmt\"\n\nfunc Run() { fmt.Println() }\n\nfunc (s *Server) Handle() {}\n\ntype Server struct{ n int }\n\n" +
			"type (\n\tA int\n\tB string\n)\n\nvar x, y = 1, 2\n\nconst (\n\tC = 1\n\tD = 2\n)\n",
			[]string{"Run", "Server", "Server", "A", "B", "x,y", "C", "D"}, 1},
		{"src/a.rs", "use crate::b::B;\npub use crate::c::C;\n\npub fn run() {}\n\nstruct S;\n\nimpl S { fn m(&self) {} }\n\n" +
			"mod inner {\n    pub fn deep() {}\n}\n\nconst K: u8 = 1;\nmacro_rules! m { () => {} }\n",
			[]string{"run", "S", "S", "inner", "deep in inner", "K", "m"}, 1},
		{"src/a.py", "import os\nfrom .m import X\n\nLIMIT = 3\n\ndef run():\n    def inner(): pass\n\n@decorator\ndef handler(): pass\n\nclass Model:\n    def method(self): pass\n",
			[]string{"LIMIT", "run", "handler", "Model"}, 2},
		{"src/a.ts", "import { b } from './b';\nexport * from './c';\n\nexport function run() {}\nconst local = 1;\nclass Shape { area() {} }\n" +
			"namespace NS {\n  export function inner() {}\n}\n",
			[]string{"run", "local", "Shape", "NS", "inner in NS"}, 1},
		{"lib/a.dart", "import 'b.dart';\n\nvoid run() {\n  print(1);\n}\n\nclass Shape {\n  void area() {}\n}\n\nfinal limit = 3;\n",
			[]string{"run", "Shape", "limit"}, 1},
		{"Sources/A/a.swift", "import Foundation\n\nfunc run() {}\n\nclass Shape { func area() {} }\n\nextension Shape { func more() {} }\n\nlet limit = 3\n",
			[]string{"run", "Shape", "Shape", "limit"}, 1},
		{"src/main/kotlin/a/A.kt", "package a\n\nimport b.B\n\nfun run() {}\n\nclass Shape { fun area() {} }\n\nval limit = 3\n",
			[]string{"run", "Shape", "limit"}, 1},
		{"src/main/java/a/A.java", "package a;\n\nimport b.B;\n\npublic class A { void m() {} }\n\ninterface I {}\n",
			[]string{"A", "I"}, 1},
		{"src/A.cs", "using System;\n\nnamespace Acme {\n  public class A { void M() {} }\n  delegate void D();\n}\n",
			[]string{"Acme", "A in Acme", "D in Acme"}, 1},
		{"src/a.c", "#include \"a.h\"\n\n#define LIMIT 3\n\nint count;\n\nstatic int helper(int x) { return x; }\n\nint proto(void);\n\ntypedef struct point { int x; } point_t;\n",
			[]string{"LIMIT", "count", "helper", "proto", "point_t"}, 1},
		{"src/a.cc", "#include \"a.hpp\"\n\nnamespace acme {\nvoid Store::add(int n) {}\nclass Widget { void draw(); };\n}\n",
			[]string{"acme", "Store in acme", "Widget in acme"}, 1},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			units, imports := unitsOf(t, c.path, c.content)
			if !slices.Equal(units, c.units) || len(imports) != c.imports {
				t.Errorf("units(%s) = %q with %d pure imports %q, want %q with %d", c.path, units, len(imports), imports, c.units, c.imports)
			}
		})
	}
}

func TestAnAttributeBeforeAUnitIsPartOfIt(t *testing.T) {
	content := "#[derive(Debug)]\n#[allow(dead_code)]\nstruct S;\n\n#[test]\nfn t() {}\n"
	r := readUnits(t, "a.rs", content)
	if len(r.units) != 2 || r.units[0].start != 0 || content[r.units[1].start:r.units[1].start+7] != "#[test]" {
		t.Errorf("units of %q = %+v, want each starting at its attributes", content, r.units)
	}
}

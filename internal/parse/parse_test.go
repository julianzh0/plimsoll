package parse_test

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/julianzh0/plimsoll/internal/parse"
	"github.com/julianzh0/plimsoll/internal/parse/parsetest"
)

// newParser is a parser of the test's own, for a test whose subject is one parser's state: its memory, its closing.
func newParser(t *testing.T) *parse.Parser {
	t.Helper()
	parser, err := parse.New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(parser.Close)
	return parser
}

// goVars is a Go file of n lines "var v<i> = <i>", a shape whose parse grows with its length.
func goVars(n int) []byte {
	var b strings.Builder
	b.WriteString("package a\n")
	for i := range n {
		fmt.Fprintf(&b, "var v%d = %d\n", i, i)
	}
	return []byte(b.String())
}

func TestEveryGrammarAnswersAQuery(t *testing.T) {
	parser := parsetest.Parser(t)
	lines := map[string]string{
		"go": "package a\n", "typescript": "const a = 1;\n", "tsx": "const a = 1;\n", "python": "a = 1\n",
		"java": "class A {}\n", "csharp": "class A {}\n", "kotlin": "val a = 1\n", "rust": "fn f() {}\n",
		"swift": "let a = 1\n", "dart": "var a = 1;\n", "c": "int a = 1;\n", "cpp": "int a = 1;\n",
	}
	if got := parse.Grammars(); len(got) != len(lines) {
		t.Fatalf("Grammars() = %v, want the %d of %v", got, len(lines), lines)
	}
	for _, grammar := range parse.Grammars() {
		result, err := parser.Read(grammar, "(_) @node", []byte(lines[grammar]))
		if err != nil || len(result.Matches) == 0 || len(result.Top) == 0 {
			t.Errorf("Read(%q, (_) @node, %q) = %d matches, %d top, %v; want some of both and no error",
				grammar, lines[grammar], len(result.Matches), len(result.Top), err)
		}
		tree, err := parser.Nodes(grammar, []byte(lines[grammar]))
		if err != nil || len(tree.Nodes) == 0 || len(tree.Kinds) == 0 {
			t.Errorf("Nodes(%q, %q) = %d nodes, %d kinds, %v; want some of both and no error", grammar, lines[grammar], len(tree.Nodes), len(tree.Kinds), err)
		}
	}
}

func TestReadAppliesPredicates(t *testing.T) {
	parser := parsetest.Parser(t)
	content := []byte("a = 1\nb = 2\n")
	cases := []struct {
		query string
		lines []int
	}{
		{`((identifier) @name (#eq? @name "b"))`, []int{2}},
		{`((identifier) @name (#not-eq? @name "b"))`, []int{1}},
		{`((identifier) @name (#match? @name "^[ab]$"))`, []int{1, 2}},
		{`((identifier) @name (#not-match? @name "^a"))`, []int{2}},
	}
	for _, c := range cases {
		result, err := parser.Read("python", c.query, content)
		var lines []int
		for _, match := range result.Matches {
			lines = append(lines, match[0].Line)
		}
		if err != nil || !reflect.DeepEqual(lines, c.lines) {
			t.Errorf("Read(python, %s) = lines %v, %v; want %v", c.query, lines, err, c.lines)
		}
	}
	result, err := parser.Read("python", `((identifier) @name (#eq? @name "b") (#strip-prefix! @name "b"))`, content)
	if err != nil || len(result.Matches) != 1 || result.Matches[0][0].Start != result.Matches[0][0].End {
		t.Errorf("Read(python, #strip-prefix!) = %+v, %v; want one capture with nothing left", result.Matches, err)
	}
}

func TestReadRefusesWhatItDoesNotKnow(t *testing.T) {
	parser := parsetest.Parser(t)
	cases := []struct{ grammar, query string }{
		{"cobol", "(_) @node"},
		{"python", "(no_such_node) @x"},
		{"python", `((identifier) @x (#frob? @x "a"))`},
	}
	for _, c := range cases {
		if _, err := parser.Read(c.grammar, c.query, []byte("a = 1\n")); err == nil {
			t.Errorf("Read(%q, %q) error = nil, want an error", c.grammar, c.query)
		}
		if _, err := parser.Read("python", "(identifier) @x", []byte("a = 1\n")); err != nil {
			t.Errorf("Read(python) after Read(%q, %q) error = %v, want none", c.grammar, c.query, err)
		}
	}
}

func TestReadTakesAnyBytes(t *testing.T) {
	parser := parsetest.Parser(t)
	for _, content := range [][]byte{{}, []byte("# only\n"), {0xff, 0xfe, '\n'}} {
		if _, err := parser.Read("python", "(identifier) @x", content); err != nil {
			t.Errorf("Read(python, %q) error = %v, want none", content, err)
		}
	}
}

func TestADeeplyNestedFileIsReadOrRefused(t *testing.T) {
	parser := newParser(t)
	deep := "a = " + strings.Repeat("(", 100_000) + "1" + strings.Repeat(")", 100_000) + "\n"
	_, _ = parser.Read("python", "(identifier) @x", []byte(deep)) // either outcome is right; the process lives
	if _, err := parser.Read("python", "(identifier) @x", []byte("a = 1\n")); err != nil {
		t.Errorf("Read(python, a = 1) after a deeply nested file error = %v, want none", err)
	}
	_, _ = parser.Nodes("python", []byte(deep))
	if _, err := parser.Nodes("python", []byte("a = 1\n")); err != nil {
		t.Errorf("Nodes(python, a = 1) after a deeply nested file error = %v, want none", err)
	}
}

func TestAFileOverTheCapIsAnErrorAndTheNextCallWorks(t *testing.T) {
	parser := newParser(t)
	if _, err := parser.Read("go", "(identifier) @x", goVars(overTheCap)); err == nil {
		t.Errorf("Read(go, %d var lines) error = nil, want the cap of %d MiB", overTheCap, parse.MemoryCapMiB)
	}
	if _, err := parser.Read("go", "(package_identifier) @x", []byte("package a\n")); err != nil {
		t.Errorf("Read(go, package a) after a file over the cap error = %v, want none", err)
	}
	if _, err := parser.Read("go", "(identifier) @x", goVars(wellUnderTheCap)); err != nil {
		t.Errorf("Read(go, %d var lines) error = %v, want none: the size is what fails", wellUnderTheCap, err)
	}
	if _, err := parser.Nodes("go", goVars(overTheCap)); err == nil {
		t.Errorf("Nodes(go, %d var lines) error = nil, want the cap of %d MiB", overTheCap, parse.MemoryCapMiB)
	}
	if _, err := parser.Nodes("go", []byte("package a\n")); err != nil {
		t.Errorf("Nodes(go, package a) after a file over the cap error = %v, want none", err)
	}
}

func TestAFileOverTheRecycleSizeIsReadAndTheNextCallWorks(t *testing.T) {
	parser := newParser(t)
	if _, err := parser.Read("go", "(identifier) @x", goVars(overTheRecycleSize)); err != nil {
		t.Errorf("Read(go, %d var lines) error = %v, want none", overTheRecycleSize, err)
	}
	if _, err := parser.Read("go", "(package_identifier) @x", []byte("package a\n")); err != nil {
		t.Errorf("Read(go, package a) after a large file error = %v, want none", err)
	}
	if _, err := parser.Nodes("go", goVars(overTheRecycleSize)); err != nil {
		t.Errorf("Nodes(go, %d var lines) error = %v, want none: a file Read reads is walked too", overTheRecycleSize, err)
	}
	if _, err := parser.Read("go", "(identifier) @x", goVars(wellUnderTheCap)); err != nil {
		t.Errorf("Read(go, %d var lines) after a large walk error = %v, want none: a walk keeps no memory", wellUnderTheCap, err)
	}
}

func TestReadsFromManyGoroutinesAllSucceed(t *testing.T) {
	parser := parsetest.Parser(t)
	inputs := []struct{ grammar, content string }{{"python", "a = 1\nb = 2\n"}, {"go", "package a\nvar b = 1\n"}}
	want := make([]parse.Result, len(inputs))
	wantTrees := make([]parse.Tree, len(inputs))
	for i, in := range inputs {
		var err error
		if want[i], err = parser.Read(in.grammar, "(identifier) @x", []byte(in.content)); err != nil {
			t.Fatalf("Read(%s) error = %v", in.grammar, err)
		}
		if wantTrees[i], err = parser.Nodes(in.grammar, []byte(in.content)); err != nil {
			t.Fatalf("Nodes(%s) error = %v", in.grammar, err)
		}
	}
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			for range 20 {
				in := inputs[g%len(inputs)]
				got, err := parser.Read(in.grammar, "(identifier) @x", []byte(in.content))
				if err != nil || !reflect.DeepEqual(got, want[g%len(inputs)]) {
					t.Errorf("Read(%s) from goroutine %d = %+v, %v; want %+v", in.grammar, g, got, err, want[g%len(inputs)])
					return
				}
				tree, err := parser.Nodes(in.grammar, []byte(in.content))
				if err != nil || !reflect.DeepEqual(tree, wantTrees[g%len(inputs)]) {
					t.Errorf("Nodes(%s) from goroutine %d = %d nodes, %v; want %d", in.grammar, g, len(tree.Nodes), err, len(wantTrees[g%len(inputs)].Nodes))
					return
				}
			}
		})
	}
	wg.Wait()
}

func TestClosingOneParserLeavesAnotherWorking(t *testing.T) {
	first, err := parse.New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	second := newParser(t)
	if _, err := first.Read("python", "(identifier) @x", []byte("a = 1\n")); err != nil {
		t.Fatalf("Read on the first parser error = %v", err)
	}
	first.Close()
	if _, err := second.Read("python", "(identifier) @x", []byte("a = 1\n")); err != nil {
		t.Errorf("Read on the second parser after closing the first error = %v, want none", err)
	}
	if _, err := first.Read("python", "(identifier) @x", []byte("a = 1\n")); err == nil {
		t.Errorf("Read on a closed parser error = nil, want an error")
	}
}

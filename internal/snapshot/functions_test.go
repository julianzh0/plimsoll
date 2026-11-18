package snapshot_test

import (
	"cmp"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

// shown is one line per listed function: "<path> <name> c=<complexity> n=<nesting>@<line>", with "<before>→" before a
// number when the function existed at the base.
func shown(list []snapshot.FunctionChange) []string {
	var lines []string
	for _, f := range list {
		if f.New {
			lines = append(lines, fmt.Sprintf("%s %s c=%d n=%d@%d", f.Path, f.Name, f.Complexity, f.Nesting, f.NestingLine))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s %s c=%d→%d n=%d→%d@%d", f.Path, f.Name, f.ComplexityBefore, f.Complexity, f.NestingBefore, f.Nesting, f.NestingLine))
	}
	return lines
}

func functionsIn(t *testing.T, rules conventions.Rules, base, now map[string]string, changes ...snapshot.Change) snapshot.Functions {
	t.Helper()
	source := snapshottest.Files(base, now)
	if len(changes) > 0 {
		source.Changes = changes
	}
	return snapshot.Build(source, rules, snapshottest.Readings(t)).Functions
}

func TestBuildListsFunctionsAChangeMadeComplexOrDeeper(t *testing.T) {
	fn := snapshottest.Function
	one := func(content string) map[string]string { return map[string]string{"src/a.ts": content} }
	four := fn("a", 7, 0) + "\n" + fn("b", 8, 0) + "\n" + fn("c", 9, 0) + "\n" + fn("d", 7, 1)
	cases := []struct {
		name            string
		base, now       map[string]string
		changes         []snapshot.Change
		complex, deeper []string
		crossed         [2]int // ComplexCrossed, DeeperCrossed
	}{
		{"new, over the limit, nested 7 deep", nil, one(fn("seven", 7, 0)), nil, []string{"src/a.ts seven c=28 n=7@8"}, nil, [2]int{1, 0}},
		{"new, over the limit, nested 3 deep", nil, one(fn("wide", 3, 20)), nil, nil, nil, [2]int{0, 0}},
		{"pushed over the limit by 8", one(fn("grow", 5, 4)), one(fn("grow", 6, 6)), nil, []string{"src/a.ts grow c=19→27 n=5→6@7"}, nil, [2]int{1, 0}},
		{"pushed over the limit by 7", one(fn("grow", 5, 4)), one(fn("grow", 6, 5)), nil, nil, []string{"src/a.ts grow c=19→26 n=5→6@7"}, [2]int{0, 1}},
		{"over the limit at the base and grown", one(fn("grow", 7, 0)), one(fn("grow", 7, 9)), nil, nil, nil, [2]int{0, 0}},
		{"nesting 3 to 4", one(fn("walk", 3, 0)), one(fn("walk", 4, 0)), nil, nil, []string{"src/a.ts walk c=6→10 n=3→4@5"}, [2]int{0, 1}},
		{"nesting 4 to 5", one(fn("walk", 4, 0)), one(fn("walk", 5, 0)), nil, nil, []string{"src/a.ts walk c=10→15 n=4→5@6"}, [2]int{0, 1}},
		{"nesting 4 to 4", one(fn("walk", 4, 0)), one(fn("walk", 4, 1)), nil, nil, nil, [2]int{0, 0}},
		{"four over the limit", nil, map[string]string{"src/four.ts": four}, nil,
			[]string{"src/four.ts c c=45 n=9@48", "src/four.ts b c=36 n=8@27", "src/four.ts d c=29 n=7@68"},
			[]string{"src/four.ts a c=28 n=7@8"}, [2]int{4, 1}},
		{"a renamed file", map[string]string{"old/a.ts": fn("grow", 5, 4)}, one(fn("grow", 6, 6)),
			[]snapshot.Change{{Path: "src/a.ts", From: "old/a.ts"}}, []string{"src/a.ts grow c=19→27 n=5→6@7"}, nil, [2]int{1, 0}},
		{"a test file", nil, map[string]string{"tests/a.test.ts": fn("seven", 7, 0)}, nil, nil, nil, [2]int{0, 0}},
		{"other code", nil, map[string]string{"scripts/a.ts": fn("seven", 7, 0)}, nil, nil, nil, [2]int{0, 0}},
		{"a function the change removed", one(fn("seven", 7, 0) + "\n" + fn("kept", 1, 0)), one(fn("kept", 1, 0)), nil, nil, nil, [2]int{0, 0}},
		{"two functions of one name, one complex, one deeper", nil,
			map[string]string{"src/two.ts": "class A {\n" + strings.Replace(fn("m", 7, 0), "function ", "", 1) + "}\nclass B {\n" + strings.Replace(fn("m", 4, 0), "function ", "", 1) + "}\n"},
			nil, []string{"src/two.ts m c=28 n=7@9"}, []string{"src/two.ts m c=10 n=4@25"}, [2]int{1, 1}},
		{"only the line ends changed", one(fn("seven", 7, 0)), one(strings.ReplaceAll(fn("seven", 7, 0), "\n", "\r\n")), nil, nil, nil, [2]int{0, 0}},
		// the function's own old body is no code moved from elsewhere
		{"pushed over by 8 with new line ends", one(fn("grow", 5, 4)), one(strings.ReplaceAll(fn("grow", 6, 6), "\n", "\r\n")), nil,
			[]string{"src/a.ts grow c=19→27 n=5→6@7"}, nil, [2]int{1, 0}},
	}
	for _, tc := range cases {
		got := functionsIn(t, conventions.Default(), tc.base, tc.now, tc.changes...)
		if !slices.Equal(shown(got.Complex), tc.complex) || !slices.Equal(shown(got.Deeper), tc.deeper) ||
			got.ComplexCrossed != tc.crossed[0] || got.DeeperCrossed != tc.crossed[1] {
			t.Errorf("Build(%s).Functions = complex %q, deeper %q, crossed %d and %d; want %q, %q, %v",
				tc.name, shown(got.Complex), shown(got.Deeper), got.ComplexCrossed, got.DeeperCrossed, tc.complex, tc.deeper, tc.crossed)
		}
	}
	renamed := functionsIn(t, conventions.Default(), map[string]string{"old/a.ts": fn("grow", 5, 4)}, one(fn("grow", 6, 6)), snapshot.Change{Path: "src/a.ts", From: "old/a.ts"})
	if len(renamed.Complex) != 1 || renamed.Complex[0].BasePath != "old/a.ts" || renamed.Complex[0].BaseStart != 1 || renamed.Complex[0].BaseEnd != 17 ||
		renamed.Complex[0].Start != 1 || renamed.Complex[0].End != 21 || renamed.Files != 1 || renamed.Measured != 1 {
		t.Errorf("Build(a renamed file).Functions = %+v, want grow at lines 1 to 21, from lines 1 to 17 of old/a.ts, 1 file and 1 function measured", renamed)
	}
}

// everyFunction are rules under which every function a change made new is listed, whatever its measures.
func everyFunction() conventions.Rules {
	rules := conventions.Default()
	rules.Limits.FunctionComplexity.Value = -1
	rules.Limits.FunctionNesting.Value = 0
	rules.FunctionRules.PerMeasure = 100
	return rules
}

func must(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}

func TestBuildMeasuresFunctionsInEveryLanguage(t *testing.T) {
	want := map[string][]string{ // "<name> c=<complexity> n=<nesting>", in text order
		"src/a.go":    {"flat c=5 n=1", "deep c=10 n=4"},
		"src/a.py":    {"branches c=5 n=1", "loops c=5 n=3", "inner c=1 n=1"},
		"src/a.ts":    {"pick c=5 n=1", "arrow c=3 n=2", "save c=1 n=1", "load c=1 n=1"},
		"src/a.tsx":   {"Panel c=1 n=1"},
		"src/a.rs":    {"route c=6 n=3", "chain c=4 n=1"},
		"src/a.dart":  {"resolve c=10 n=4", "chain c=4 n=1"},
		"src/A.java":  {"pick c=6 n=1"},
		"src/a.kt":    {"pick c=4 n=1", "chain c=4 n=1"},
		"src/A.cs":    {"Pick c=7 n=3", "Chain c=4 n=1"},
		"src/a.swift": {"pick c=3 n=1", "chain c=5 n=1"},
		"src/a.c":     {"pick c=13 n=2"},
		"src/a.cpp":   {"chain c=2 n=0", "Store::route c=5 n=3"},
		"src/n.c":     {"pointer c=0 n=0", "plain c=0 n=0"},
		"src/n.cpp":   {"get c=0 n=0", "~Foo c=0 n=0", "operator+ c=0 n=0", "ns::Baz<T>::qux c=0 n=0"},
		"src/m.cpp":   {"pick c=1 n=1"},                                  // a macro outside the function makes the file hold a syntax error
		"src/k.cpp":   {"pick c=1 n=1", "keep c=0 n=0", "clean c=1 n=1"}, // attribute macros before two declarations
		"src/w.c":     {"open_session c=1 n=1", "count_streams c=3 n=2"}, // annotations in the parameters; a loop macro
		"src/q.cpp":   {"Panel::size c=3 n=2"},                           // Qt: Q_OBJECT, Q_PROPERTY, slots, signals, Q_D, foreach, emit
		"src/g.c":     {"fu_plugin_load c=1 n=1"},                        // GLib: G_DEFINE_TYPE, G_GNUC_*, g_autofree, g_autoptr, g_assert_cmpint
		"src/f.cpp":   {"count c=3 n=2"},                                 // a Qt foreach the grammar reads as a function inside the body
		"src/n.go":    {"Method c=0 n=0", "Plain c=0 n=0"},
		"src/n.py":    {"save c=0 n=0", "inner c=0 n=0", "fetch c=0 n=0"},
		"src/n.ts":    {"plain c=0 n=0", "arrow c=0 n=0", "constructor c=0 n=0", "save c=0 n=0", "field c=0 n=0", "size c=0 n=0", "method_definition c=0 n=0", "method_definition c=0 n=0"},
		"src/N.java":  {"N c=0 n=0", "size c=0 n=0"},
		"src/n.dart":  {"Store c=0 n=0", "size c=0 n=0", "save c=0 n=0", "top c=0 n=0"},
		"src/n.kt":    {"save c=0 n=0", "getter c=0 n=0", "top c=0 n=0", "weird name c=0 n=0"},
		"src/N.cs":    {"N c=0 n=0", "get c=0 n=0", "Save c=0 n=0", "Local c=0 n=0"},
		"src/n.swift": {"init c=0 n=0", "save c=0 n=0", "deinit c=0 n=0", "top c=0 n=0"},
		"src/n.rs":    {"save c=0 n=0", "top c=0 n=0", "inner c=0 n=0"},
		"src/l.dart":  {"outer c=0 n=0", "inner c=6 n=3"},
		"src/O.cs":    {"M c=0 n=0", "operator c=0 n=0"},
	}
	for path, wanted := range want {
		content, err := fs.ReadFile(os.DirFS("testdata/functions/now"), path)
		if err != nil {
			t.Fatal(err)
		}
		listed := functionsIn(t, everyFunction(), nil, map[string]string{path: string(content)}).Complex
		// text order; two functions on one line keep the order the want list gives them
		slices.SortFunc(listed, func(a, b snapshot.FunctionChange) int {
			return cmp.Or(a.Start-b.Start, slices.IndexFunc(wanted, func(w string) bool { return strings.HasPrefix(w, a.Name+" ") })-
				slices.IndexFunc(wanted, func(w string) bool { return strings.HasPrefix(w, b.Name+" ") }))
		})
		var got []string
		for _, f := range listed {
			got = append(got, fmt.Sprintf("%s c=%d n=%d", f.Name, f.Complexity, f.Nesting))
		}
		if !slices.Equal(got, wanted) {
			t.Errorf("Build(%s) measures %q, want %q", path, got, wanted)
		}
	}
	deep := functionsIn(t, everyFunction(), nil, map[string]string{"src/a.go": string(must(os.ReadFile("testdata/functions/now/src/a.go")))}).Complex
	if i := slices.IndexFunc(deep, func(f snapshot.FunctionChange) bool { return f.Name == "deep" }); i < 0 || deep[i].NestingLine != 19 || deep[i].Start != 13 || deep[i].End != 32 {
		t.Errorf("Build(src/a.go) deep = %+v, want lines 13 to 32 and nesting 4 reached at line 19", deep)
	}
}

func TestBuildListsAFunctionThatRepeatsABlock(t *testing.T) {
	rules := conventions.Default()
	rules.Limits.FunctionComplexity.Value = -1
	rules.Limits.FunctionNesting.Value = 99 // only a repeated block can be the reason
	// block is n statements "    const <name> = load(<arguments>);", NAME in the arguments standing for the name.
	block := func(n int, arguments string) string {
		lines := ""
		for _, name := range []string{"first", "second", "third", "fourth"}[:n] {
			lines += "    const " + name + " = load(" + strings.ReplaceAll(arguments, "NAME", name) + ");\n"
		}
		return lines
	}
	sync := func(body string) string {
		return "export function sync(source: string, options: object, fast: boolean) {\n  if (fast) {\n" + body +
			"    return 1;\n  } else {\n" + body + "    return 2;\n  }\n}\n"
	}
	both := []snapshot.LineRange{{Start: 3, End: 6}, {Start: 9, End: 12}}
	cases := []struct {
		name, content string
		want          []snapshot.LineRange // nil: not listed
	}{
		{"four statements of 12 tokens, twice", sync(block(4, `source, "NAME", options`)), both},
		{"three statements of 12 tokens, twice", sync(block(3, `source, "NAME", options`)), nil},
		{"four statements of 8 tokens, twice", sync(block(4, `"NAME"`)), nil},
		{"bytes that are no UTF-8", sync(block(4, "source, \"NAME\", \"\xff\xfe\"")), both},
	}
	for _, tc := range cases {
		got := functionsIn(t, rules, nil, map[string]string{"src/r.ts": tc.content}).Complex
		if (tc.want == nil) != (len(got) == 0) || len(got) == 1 && !slices.Equal(got[0].Repeated, tc.want) {
			t.Errorf("Build(%s).Functions.Complex = %+v, want the block at %v", tc.name, got, tc.want)
		}
	}
	once := "export function sync(source: string, options: object) {\n" + block(4, `source, "NAME", options`) + "  return 1;\n}\n"
	if got := functionsIn(t, rules, nil, map[string]string{"src/r.ts": once}).Complex; len(got) != 0 {
		t.Errorf("Build(a block written once).Functions.Complex = %+v, want none", got)
	}
	fn := snapshottest.Function
	// twice writes the flat ifs of a function (the lines "  if (a === N) { a += 1; }") a second time right after them.
	twice := func(text string) string {
		var out, run []string
		for _, line := range strings.SplitAfter(text, "\n") {
			if strings.HasPrefix(line, "  if (a === ") && strings.HasSuffix(line, "}\n") {
				run = append(run, line)
				continue
			}
			out = append(out, run...)
			out = append(out, run...)
			run = nil
			out = append(out, line)
		}
		return strings.Join(out, "")
	}
	var loads strings.Builder
	for n := 1; n <= 40; n++ {
		fmt.Fprintf(&loads, "  const v%d = load(a, \"n%d\", %d);\n", n, n, n)
	}
	grown := fn("grow", 5, 0)
	end := strings.LastIndex(grown, "  return a;\n")
	flat := strings.Join(slices.Collect(func(yield func(string) bool) {
		for line := range strings.SplitAfterSeq(fn("x", 0, 8), "\n") {
			if strings.HasPrefix(line, "  if (a === ") && !yield(line) {
				return
			}
		}
	}), "")
	movedIn := grown[:end] + loads.String() + flat + flat + grown[end:]
	existed := []struct {
		name      string
		base, now map[string]string
		complex   []string
		repeated  []snapshot.LineRange
	}{
		{"pasted from its own body", map[string]string{"src/a.ts": fn("grow", 5, 8)}, map[string]string{"src/a.ts": twice(fn("grow", 5, 8))},
			[]string{"src/a.ts grow c=23→31 n=5→5@6"}, []snapshot.LineRange{{Start: 12, End: 15}, {Start: 20, End: 23}}},
		{"twice at the base already", map[string]string{"src/a.ts": twice(fn("grow", 3, 8))}, map[string]string{"src/a.ts": twice(fn("grow", 5, 8))}, nil, nil},
		{"both copies moved in", map[string]string{"src/a.ts": fn("grow", 5, 0), "src/b.ts": twice(fn("old", 0, 8))},
			map[string]string{"src/a.ts": movedIn, "src/b.ts": fn("old", 0, 0)}, nil, nil},
	}
	defaults := conventions.Default()
	defaults.Limits.FunctionNesting.Value = 99
	for _, tc := range existed {
		got := functionsIn(t, defaults, tc.base, tc.now)
		if !slices.Equal(shown(got.Complex), tc.complex) || len(got.Complex) == 1 && !slices.Equal(got.Complex[0].Repeated, tc.repeated) || len(got.LeftOut) != 0 {
			t.Errorf("Build(%s).Functions = complex %q, left out %q; want %q with the block at %v", tc.name, shown(got.Complex), leftOut(got.LeftOut), tc.complex, tc.repeated)
		}
	}
}

func TestBuildMeasuresOnlyProductCodeItCanRead(t *testing.T) {
	f := snapshottest.Function("seven", 7, 0)
	rust := string(must(os.ReadFile("testdata/functions/now/src/t.rs")))
	cases := []struct {
		path, content string
		listed        bool
		minified      []string
		measured      int // -1: not checked
	}{
		{"src/workbench/a.ts", f, true, nil, -1},
		{"src/bench/a.ts", f, false, nil, -1},
		{"src/benchmarks/a.ts", f, false, nil, -1},
		{"src/RoutingBenchmarks.ts", f, false, nil, -1},
		{"src/benchmark_utils.ts", f, false, nil, -1},
		{"src/fixtures/a.ts", f, false, nil, -1},
		{"src/generated/a.ts", f, false, nil, -1},
		{"packages/x/scripts/a.ts", f, false, nil, -1},
		{"packages/x/docs/a.ts", f, false, nil, -1},
		{"src/min.ts", strings.ReplaceAll(f, "\n", " "), false, []string{"src/min.ts"}, -1},
		{"src/gen.ts", "// Code generated by x. DO NOT EDIT.\n" + f, false, nil, -1},
		{"src/a.rs", rust, false, nil, 1},
		{"src/a.c", "int f(void) { return 1; }\n", false, nil, 1},
		{"src/a.rb", "def f\n  1\nend\n", false, nil, -1},
	}
	for _, tc := range cases {
		got := functionsIn(t, conventions.Default(), nil, map[string]string{tc.path: tc.content})
		if (len(got.Complex) == 1) != tc.listed || !slices.Equal(got.Minified, tc.minified) || tc.measured >= 0 && got.Measured != tc.measured {
			t.Errorf("Build(%s).Functions = %d listed, minified %v, measured %d; want listed %v, %v, %d",
				tc.path, len(got.Complex), got.Minified, got.Measured, tc.listed, tc.minified, tc.measured)
		}
	}
	for _, path := range []string{"src/bom.ts", "src/crlf.ts"} {
		content := "\uFEFF" + f
		if path == "src/crlf.ts" {
			content = strings.ReplaceAll(f, "\n", "\r\n")
		}
		got := functionsIn(t, conventions.Default(), nil, map[string]string{path: content}).Complex
		if len(got) != 1 || got[0].Start != 1 || got[0].End != 17 || got[0].NestingLine != 8 {
			t.Errorf("Build(%s).Functions.Complex = %+v, want seven at lines 1 to 17, nesting reached at line 8", path, got)
		}
	}
}

func TestBuildCountsFunctionsItCannotMeasure(t *testing.T) {
	broken := functionsIn(t, everyFunction(), nil, map[string]string{"src/e.ts": string(must(os.ReadFile("testdata/functions/now/src/e.ts")))})
	if len(broken.Complex) != 1 || broken.Complex[0].Name != "sound" || broken.Broken != 1 {
		t.Errorf("Build(a function with a syntax error).Functions = %+v, want sound listed and 1 broken", broken)
	}
	grow := snapshottest.Function("grow", 5, 4)
	first := strings.Index(grow, "\n") + 1
	base := grow[:first] + "    const = 1 +;\n" + grow[first:]
	got := functionsIn(t, conventions.Default(), map[string]string{"src/a.ts": base}, map[string]string{"src/a.ts": snapshottest.Function("grow", 6, 6)})
	if len(got.Complex) != 0 || len(got.Deeper) != 0 || got.Broken != 1 {
		t.Errorf("Build(a base version with a syntax error).Functions = %+v, want nothing listed and 1 broken", got)
	}
	renamedBroken := functionsIn(t, conventions.Default(), map[string]string{"src/a.ts": strings.Replace(snapshottest.Function("oldName", 7, 0), "  return a;", "  const = 1 +;\n  return a;", 1)},
		map[string]string{"src/a.ts": snapshottest.Function("newName", 7, 0)})
	if len(renamedBroken.Complex) != 0 || renamedBroken.Broken != 1 {
		t.Errorf("Build(a renamed function whose base version had a syntax error).Functions = %+v, want nothing listed and 1 broken", renamedBroken)
	}
	small := functionsIn(t, conventions.Default(), map[string]string{"src/a.ts": "function a() { const = 1 +; }\n"}, map[string]string{"src/a.ts": "function a() { return 2; }\n"})
	if small.Broken != 0 || small.Measured != 1 {
		t.Errorf("Build(a small function whose base version had a syntax error).Functions = %+v, want 0 broken, 1 measured: it could never cross", small)
	}
	source := snapshottest.Files(map[string]string{"src/a.ts": snapshottest.Function("grow", 7, 0)}, map[string]string{"src/a.ts": snapshottest.Function("grow", 7, 1)})
	for i := range source.Before {
		source.Before[i].Content, source.Before[i].Binary = nil, true
	}
	unreadBase := snapshot.Build(source, conventions.Default(), snapshottest.Readings(t)).Functions
	if len(unreadBase.Complex) != 0 || !slices.Equal(unreadBase.Unread, []string{"src/a.ts"}) {
		t.Errorf("Build(a base version that cannot be read).Functions = %+v, want nothing listed and src/a.ts unread", unreadBase)
	}
}

// leftOut is one line per function left out: "<path> <name> <reason>[ <source>]".
func leftOut(list []snapshot.LeftOutFunction) []string {
	var lines []string
	for _, f := range list {
		line := fmt.Sprintf("%s %s %s", f.Path, f.Name, f.Reason)
		if f.Source != "" {
			line += " " + f.Source
		}
		lines = append(lines, line)
	}
	return lines
}

func TestBuildLeavesOutCodeTheChangeDidNotWrite(t *testing.T) {
	fn := snapshottest.Function
	cases := []struct {
		name             string
		base, now        map[string]string
		complex, leftOut []string
	}{
		{"moved to another file", map[string]string{"src/a.ts": fn("all", 7, 3)},
			map[string]string{"src/a.ts": fn("all", 0, 3), "src/b.ts": fn("part", 7, 0)}, nil, []string{"src/b.ts part moved"}},
		{"split out in its file", map[string]string{"src/a.ts": fn("all", 7, 3)},
			map[string]string{"src/a.ts": fn("part", 7, 0) + fn("all", 0, 3)}, nil, []string{"src/a.ts part extracted"}},
		{"renamed", map[string]string{"src/a.ts": fn("oldName", 7, 0)}, map[string]string{"src/a.ts": fn("newName", 7, 0)}, nil, nil},
		{"renamed and pushed over", map[string]string{"src/a.ts": fn("oldName", 5, 4)}, map[string]string{"src/a.ts": fn("newName", 6, 6)},
			[]string{"src/a.ts newName c=19→27 n=5→6@7"}, nil},
		{"new beside removed code of another kind", map[string]string{"src/a.ts": fn("small", 1, 0)}, map[string]string{"src/b.ts": fn("seven", 7, 0)},
			[]string{"src/b.ts seven c=28 n=7@8"}, nil},
	}
	for _, tc := range cases {
		got := functionsIn(t, conventions.Default(), tc.base, tc.now)
		if !slices.Equal(shown(got.Complex), tc.complex) || !slices.Equal(leftOut(got.LeftOut), tc.leftOut) {
			t.Errorf("Build(%s).Functions = complex %q, left out %q; want %q, %q", tc.name, shown(got.Complex), leftOut(got.LeftOut), tc.complex, tc.leftOut)
		}
		if tc.name == "renamed and pushed over" && (len(got.Complex) != 1 || !got.Complex[0].ByBody || got.Complex[0].BasePath != "src/a.ts") {
			t.Errorf("Build(%s).Functions.Complex = %+v, want matched by body to src/a.ts", tc.name, got.Complex)
		}
	}
}

func TestBuildLeavesOutFilesFromAnotherProject(t *testing.T) {
	f := snapshottest.Function("seven", 7, 0)
	h := "// Copyright (c) Meta Platforms, Inc. and affiliates.\n"
	neighbours := func(header string) map[string]string {
		files := map[string]string{}
		for _, name := range []string{"src/n1.ts", "src/n2.ts", "src/n3.ts"} {
			files[name] = header + "export const v = 1;\n"
		}
		return files
	}
	with := func(files ...map[string]string) map[string]string {
		all := map[string]string{}
		for _, m := range files {
			maps.Copy(all, m)
		}
		return all
	}
	url := "// See https://github.com/acme/widgets for the format\n"
	apache := "Copyright 2020 Acme Corp.\n\nSubject to the terms of this License, each Contributor grants a perpetual, worldwide, non-exclusive, no-charge, royalty-free, irrevocable copyright license\n"
	notes := strings.Repeat("// note\n", 40)
	cases := []struct {
		name      string
		base, now map[string]string
		listed    bool
	}{
		{"a foreign copyright", nil, map[string]string{"src/x.ts": h + f}, false},
		{"the repository's own holder by its licence", map[string]string{"LICENSE": "Copyright (c) Meta Platforms, Inc.\n"},
			map[string]string{"LICENSE": "Copyright (c) Meta Platforms, Inc.\n", "src/x.ts": h + f}, true},
		{"the holder its neighbours share", neighbours(h), with(neighbours(h), map[string]string{"src/x.ts": h + f}), true},
		{"a copyright without a holder", nil, map[string]string{"src/x.ts": "// Copyright (c) 2026\n" + f}, true},
		{"a comment that speaks of copyright", nil, map[string]string{"src/x.ts": "// the copyright holders of the root licence files are the own ones\n" + f}, true},
		{"an SPDX notice", nil, map[string]string{"src/x.ts": "// SPDX-FileCopyrightText: 2024 Other Holder GmbH\n" + f}, false},
		{"a note that it was ported", nil, map[string]string{"src/x.ts": "// Ported from an older tool\n" + f}, false},
		{"an upstream address", nil, map[string]string{"src/x.ts": url + f}, false},
		{"an address its neighbours share", neighbours(url), with(neighbours(url), map[string]string{"src/x.ts": url + f}), true},
		{"a header past the first 40 lines", nil, map[string]string{"src/x.ts": notes + h + f}, true},
		{"a vendor folder with its own licence", nil, map[string]string{"src/external/zlib/inflate.ts": f, "src/external/LICENSE": "zlib\n"}, false},
		{"a vendor folder without a licence", nil, map[string]string{"src/external/zlib/inflate.ts": f}, true},
		{"a vendor folder whose project holds its licence", nil, map[string]string{"src/external/zlib/inflate.ts": f, "src/external/zlib/LICENSE": "zlib\n"}, false},
		{"a licence's own wording is no holder", map[string]string{"LICENSE": apache},
			map[string]string{"LICENSE": apache, "src/x.ts": "// Copyright (C) 2008 Free Software Foundation, Inc.\n" + f}, false},
		{"a file that was there, with another project's header", map[string]string{"src/x.ts": h + snapshottest.Function("grow", 5, 4)},
			map[string]string{"src/x.ts": h + snapshottest.Function("grow", 6, 6)}, false},
	}
	for _, tc := range cases {
		got := functionsIn(t, conventions.Default(), tc.base, tc.now)
		foreign := slices.ContainsFunc(got.LeftOut, func(l snapshot.LeftOutFunction) bool { return l.Reason == snapshot.FromAnotherProject })
		if (len(got.Complex) == 1) != tc.listed || foreign == tc.listed {
			t.Errorf("Build(%s).Functions = complex %q, left out %q; want listed %v", tc.name, shown(got.Complex), leftOut(got.LeftOut), tc.listed)
		}
	}
}

func TestBuildLeavesOutCopiesOfCodeAtTheBase(t *testing.T) {
	fn := snapshottest.Function
	f7 := fn("seven", 7, 0)
	long := func(text string) string {
		first := strings.Index(text, "\n") + 1
		return text[:first] + "  const configurationValue = resolveConfiguration(a);\n" + text[first:]
	}
	other := strings.ReplaceAll(fn("other", 7, 0), "(a >", "(a <")
	nested := strings.Join(strings.SplitAfter(other, "\n")[1:15], "")
	grow := fn("grow", 5, 4)
	ret := strings.LastIndex(grow, "  return a;\n")
	twice := func(text string) string {
		var out, run []string
		for _, line := range strings.SplitAfter(text, "\n") {
			if strings.HasPrefix(line, "  if (a === ") && strings.HasSuffix(line, "}\n") {
				run = append(run, line)
				continue
			}
			out = append(out, run...)
			out = append(out, run...)
			run = nil
			out = append(out, line)
		}
		return strings.Join(out, "")
	}
	names := map[string]string{"lib/deep/old.ts": f7}
	for i := 1; i <= 25; i++ {
		names[fmt.Sprintf("src/m%02d.ts", i)] = "export const seven = 1;\n"
	}
	var ten, tenAndFresh strings.Builder
	for i := 1; i <= 20; i++ {
		ten.WriteString(fn(fmt.Sprintf("c%02d", i), 8, 0))
	}
	tenAndFresh.WriteString(ten.String() + fn("fresh", 7, 0))
	cases := []struct {
		name             string
		base, now        map[string]string
		complex, leftOut []string
	}{
		{"the same name", map[string]string{"src/old.ts": f7}, map[string]string{"src/old.ts": f7, "src/new.ts": f7},
			nil, []string{"src/new.ts seven copied src/old.ts"}},
		{"another name, two long names", map[string]string{"src/old.ts": long(f7)}, map[string]string{"src/old.ts": long(f7), "src/new.ts": long(fn("renamed", 7, 0))},
			nil, []string{"src/new.ts renamed copied src/old.ts"}},
		{"another name, one long name", map[string]string{"src/old.ts": f7}, map[string]string{"src/old.ts": f7, "src/new.ts": fn("lonely", 7, 0)},
			[]string{"src/new.ts lonely c=28 n=7@8"}, nil},
		{"code of its own file", map[string]string{"src/a.ts": grow + "\n" + other}, map[string]string{"src/a.ts": grow[:ret] + nested + grow[ret:] + "\n" + other},
			nil, []string{"src/a.ts grow copied src/a.ts"}},
		{"a block of its own body", map[string]string{"src/a.ts": fn("grow", 5, 8)}, map[string]string{"src/a.ts": twice(fn("grow", 5, 8))},
			[]string{"src/a.ts grow c=23→31 n=5→5@6"}, nil},
	}
	for _, tc := range cases {
		got := functionsIn(t, conventions.Default(), tc.base, tc.now)
		if !slices.Equal(shown(got.Complex), tc.complex) || !slices.Equal(leftOut(got.LeftOut), tc.leftOut) {
			t.Errorf("Build(%s).Functions = complex %q, left out %q; want %q, %q", tc.name, shown(got.Complex), leftOut(got.LeftOut), tc.complex, tc.leftOut)
		}
		if tc.name == "another name, one long name" && len(got.Complex) == 1 && got.Complex[0].CopyFiles != 0 {
			t.Errorf("Build(%s).Functions.Complex[0].CopyFiles = %d, want 0", tc.name, got.Complex[0].CopyFiles)
		}
	}
	defaults := conventions.Default()
	defaults.Limits.FunctionNesting.Value = 99
	own := functionsIn(t, defaults, map[string]string{"src/a.ts": fn("grow", 5, 8)}, map[string]string{"src/a.ts": twice(fn("grow", 5, 8))})
	if !slices.Equal(shown(own.Complex), []string{"src/a.ts grow c=23→31 n=5→5@6"}) || len(own.LeftOut) != 0 {
		t.Errorf("Build(a block of its own body).Functions = complex %q, left out %q; want grow listed, nothing left out", shown(own.Complex), leftOut(own.LeftOut))
	}
	far := functionsIn(t, conventions.Default(), names, maps.Collect(func(yield func(string, string) bool) {
		for k, v := range names {
			if !yield(k, v) {
				return
			}
		}
		yield("src/new.ts", f7)
	}))
	if len(far.Complex) != 1 || far.Complex[0].CopyFiles != 20 {
		t.Errorf("Build(the name in 25 nearer files).Functions = %+v, want seven listed after 20 files compared", far)
	}
	many := functionsIn(t, conventions.Default(), map[string]string{"src/lib.ts": ten.String()}, map[string]string{"src/lib.ts": ten.String(), "src/new.ts": tenAndFresh.String()})
	// twenty copies spend the ten checks of each measure; fresh, ranked last in both, is never compared
	if len(many.Complex) != 0 || len(many.LeftOut) != 20 || many.Uncompared != 1 || many.ComplexCrossed != 1 || many.DeeperCrossed != 1 || len(many.Deeper) != 0 {
		t.Errorf("Build(twenty copies before one of its own).Functions = complex %q, %d left out, uncompared %d, crossed %d and %d, deeper %q; want none, 20, 1, 1, 1, none",
			shown(many.Complex), len(many.LeftOut), many.Uncompared, many.ComplexCrossed, many.DeeperCrossed, shown(many.Deeper))
	}
	var nine strings.Builder
	for _, name := range []string{"one", "two", "three", "four", "five", "six", "seven", "eight", "nine"} {
		nine.WriteString(fn(name, 7, 0))
	}
	// past the three listed per measure, a function counts as crossing, not as not compared: the checks were not spent
	full := functionsIn(t, conventions.Default(), nil, map[string]string{"src/new.ts": nine.String()})
	if len(full.Complex) != 3 || len(full.Deeper) != 3 || full.ComplexCrossed != 9 || full.Uncompared != 0 {
		t.Errorf("Build(nine new functions).Functions = complex %q, deeper %q, crossed %d, uncompared %d; want 3 and 3 listed, 9 crossed, 0 uncompared",
			shown(full.Complex), shown(full.Deeper), full.ComplexCrossed, full.Uncompared)
	}
}

func TestBuildKeepsNoFunctionText(t *testing.T) {
	secret := strings.Replace(snapshottest.Function("seven", 7, 0), "  return a;", "  const zzsecret = 'zzsecret text';\n  return a;", 1)
	got := functionsIn(t, conventions.Default(), map[string]string{"src/old.ts": snapshottest.Function("other", 1, 0)}, map[string]string{"src/a.ts": secret})
	if text := fmt.Sprintf("%+v", got); strings.Contains(text, "zzsecret") || len(got.Complex) != 1 {
		t.Errorf("Build(a function with a secret local).Functions = %s, want the function listed and none of its text", text)
	}
	// a name keeps no comment or literal written inside it
	for _, inside := range []string{"int Store:: /* zzsecret */ route(int a) {\n  return a;\n}\n", "int Store<\"zzsecret\">::route(int a) {\n  return a;\n}\n"} {
		for _, f := range functionsIn(t, everyFunction(), nil, map[string]string{"src/store.cpp": inside}).Complex {
			if strings.Contains(f.Name, "zzsecret") {
				t.Errorf("Build(%q).Functions names %q, want no text of a comment or literal", inside, f.Name)
			}
		}
	}
	// a qualified name may be split over lines; a name shows on one line
	split := "int Store::\n    route(int a) {\n  return a;\n}\n"
	for _, f := range functionsIn(t, everyFunction(), nil, map[string]string{"src/store.cpp": split}).Complex {
		if f.Name != "Store::route" {
			t.Errorf("Build(a qualified name split over two lines).Functions names %q, want \"Store::route\"", f.Name)
		}
	}
}

func TestBuildMeasuresNestedFunctionsInTimeThatGrowsWithTheFile(t *testing.T) {
	// each function measures its own code once: a file of functions declared inside each other costs what its size costs
	const nested = 4000
	var b strings.Builder
	for i := range nested {
		fmt.Fprintf(&b, "function f%d(a: number): number { if (a === %d) { a += 1; }\n", i, i)
	}
	b.WriteString(strings.Repeat("}\n", nested))
	done := make(chan snapshot.Functions, 1)
	go func() {
		done <- functionsIn(t, conventions.Default(), nil, map[string]string{"src/deep.ts": b.String()})
	}()
	select {
	case got := <-done:
		if got.Measured != nested {
			t.Errorf("Build(%d nested functions).Functions.Measured = %d, want %d", nested, got.Measured, nested)
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("Build(%d nested functions) did not answer within 20 s", nested)
	}
}

package snapshot_test

import (
	"cmp"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

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
		{"only the line ends changed", one(fn("seven", 7, 0)), one(strings.ReplaceAll(fn("seven", 7, 0), "\n", "\r\n")), nil, nil, nil, [2]int{0, 0}},
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
	rules.FunctionRules.ReasonNesting = 0
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
		"src/n.go":    {"Method c=0 n=0", "Plain c=0 n=0"},
		"src/n.py":    {"save c=0 n=0", "inner c=0 n=0", "fetch c=0 n=0"},
		"src/n.ts":    {"plain c=0 n=0", "arrow c=0 n=0", "constructor c=0 n=0", "save c=0 n=0", "field c=0 n=0", "size c=0 n=0"},
		"src/N.java":  {"N c=0 n=0", "size c=0 n=0"},
		"src/n.dart":  {"Store c=0 n=0", "size c=0 n=0", "save c=0 n=0", "top c=0 n=0"},
		"src/n.kt":    {"save c=0 n=0", "getter c=0 n=0", "top c=0 n=0", "weird name c=0 n=0"},
		"src/N.cs":    {"N c=0 n=0", "get c=0 n=0", "Save c=0 n=0", "Local c=0 n=0"},
		"src/n.swift": {"init c=0 n=0", "save c=0 n=0", "deinit c=0 n=0", "top c=0 n=0"},
		"src/n.rs":    {"save c=0 n=0", "top c=0 n=0", "inner c=0 n=0"},
	}
	for path, wanted := range want {
		content, err := os.ReadFile("testdata/functions/now/" + path)
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
	rules.FunctionRules.ReasonNesting = 99 // only a repeated block can be the reason
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
}

func TestBuildMeasuresOnlyProductCodeItCanRead(t *testing.T) {
	f := snapshottest.Function("seven", 7, 0)
	rust := string(must(os.ReadFile("testdata/functions/now/src/t.rs")))
	cases := []struct {
		path, content     string
		listed            bool
		minified, missing []string
		measured          int // -1: not checked
	}{
		{"src/workbench/a.ts", f, true, nil, nil, -1},
		{"src/bench/a.ts", f, false, nil, nil, -1},
		{"src/benchmarks/a.ts", f, false, nil, nil, -1},
		{"src/RoutingBenchmarks.ts", f, false, nil, nil, -1},
		{"src/benchmark_utils.ts", f, false, nil, nil, -1},
		{"src/fixtures/a.ts", f, false, nil, nil, -1},
		{"src/generated/a.ts", f, false, nil, nil, -1},
		{"packages/x/scripts/a.ts", f, false, nil, nil, -1},
		{"packages/x/docs/a.ts", f, false, nil, nil, -1},
		{"src/min.ts", strings.ReplaceAll(f, "\n", " "), false, []string{"src/min.ts"}, nil, -1},
		{"src/gen.ts", "// Code generated by x. DO NOT EDIT.\n" + f, false, nil, nil, -1},
		{"src/a.rs", rust, false, nil, nil, 1},
		{"src/a.c", "int f(void) { return 1; }\n", false, nil, []string{"c"}, -1},
	}
	for _, tc := range cases {
		got := functionsIn(t, conventions.Default(), nil, map[string]string{tc.path: tc.content})
		if (len(got.Complex) == 1) != tc.listed || !slices.Equal(got.Minified, tc.minified) || !slices.Equal(got.Unmeasured, tc.missing) ||
			tc.measured >= 0 && got.Measured != tc.measured {
			t.Errorf("Build(%s).Functions = %d listed, minified %v, unmeasured %v, measured %d; want listed %v, %v, %v, %d",
				tc.path, len(got.Complex), got.Minified, got.Unmeasured, got.Measured, tc.listed, tc.minified, tc.missing, tc.measured)
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

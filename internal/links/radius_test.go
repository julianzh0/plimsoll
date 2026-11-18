package links

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

// code writes, per language, a declaration of a function or type name whose body gives variant, a use of name, and
// a comment that mentions name. In the JVM languages and C# a name is a class, the only top-level unit.
type code struct {
	declare func(name string, variant int) string
	use     func(name string) string
	comment func(name string) string
}

func codeOf(language string) code {
	slash := func(name string) string { return "// " + name + " is mentioned here\n" }
	switch language {
	case "typescript":
		return code{func(n string, v int) string { return fmt.Sprintf("export function %s() { return %d; }\n", n, v) },
			func(n string) string { return fmt.Sprintf("export const use%s = %s();\n", n, n) }, slash}
	case "python":
		return code{func(n string, v int) string { return fmt.Sprintf("def %s():\n    return %d\n", n, v) },
			func(n string) string { return fmt.Sprintf("use_%s = %s()\n", n, n) },
			func(n string) string { return "# " + n + " is mentioned here\n" }}
	case "dart":
		return code{func(n string, v int) string { return fmt.Sprintf("int %s() {\n  return %d;\n}\n", n, v) },
			func(n string) string { return fmt.Sprintf("final use%s = %s();\n", n, n) }, slash}
	case "go":
		return code{func(n string, v int) string { return fmt.Sprintf("func %s() int { return %d }\n", n, v) },
			func(n string) string { return fmt.Sprintf("var _ = %s\n", n) }, slash}
	case "rust":
		return code{func(n string, v int) string { return fmt.Sprintf("pub fn %s() -> u8 { %d }\n", n, v) },
			func(n string) string { return fmt.Sprintf("pub fn use_%s() -> u8 { %s() }\n", n, n) }, slash}
	case "java", "csharp":
		return code{func(n string, v int) string { return fmt.Sprintf("class %s { int V() { return %d; } }\n", n, v) },
			func(n string) string { return fmt.Sprintf("class Use%s { int x = %s.V(); }\n", n, n) }, slash}
	case "kotlin":
		return code{func(n string, v int) string { return fmt.Sprintf("fun %s() = %d\n", n, v) },
			func(n string) string { return fmt.Sprintf("val use%s = %s()\n", n, n) }, slash}
	case "swift":
		return code{func(n string, v int) string { return fmt.Sprintf("func %s() -> Int { return %d }\n", n, v) },
			func(n string) string { return fmt.Sprintf("let use%s = %s()\n", n, n) }, slash}
	case "c", "cpp":
		return code{func(n string, v int) string { return fmt.Sprintf("int %s(void) { return %d; }\n", n, v) },
			func(n string) string { return fmt.Sprintf("int use_%s(void) { return %s(); }\n", n, n) }, slash}
	}
	return code{}
}

// radiusOf builds a snapshot and gives each changed file as "<path> <state>" and the untouched users' paths.
func radiusOf(t *testing.T, source snapshot.Source) (states, users []string) {
	t.Helper()
	r := snapshot.Build(source, conventions.Default(), snapshottest.Readings(t)).Radius
	for _, c := range r.Changed {
		states = append(states, fmt.Sprintf("%s %d %s", cmpOr(c.Path, c.BasePath), c.State, c.Reason))
	}
	for _, u := range r.Users {
		users = append(users, u.Path)
	}
	return states, users
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func TestTheBlastRadiusCountsTheFilesThatUseANameTheChangeAlteredOrRemoved(t *testing.T) {
	eachLanguage(t, func(t *testing.T, name string, c languageCase) {
		k := codeOf(name)
		a, b, d, e := c.product("a"), c.product("b"), c.product("d"), c.product("e")
		head := c.importing(a, nil)
		base := c.tree(map[string]string{
			a: head + k.declare("Run", 1) + k.declare("Other", 1),
			b: c.importing(b, []string{a}) + k.use("Run"),
			d: c.importing(d, []string{a}) + k.use("Other"),
			e: c.importing(e, []string{a}) + k.comment("Run"),
		})
		cases := []struct {
			label   string
			changed string
			state   snapshot.FileState
			users   []string
		}{
			{"a comment", head + k.comment("Run") + k.declare("Run", 1) + k.declare("Other", 1), snapshot.NothingAltered, nil},
			{"an added declaration", head + k.declare("Run", 1) + k.declare("Other", 1) + k.declare("Added", 1), snapshot.NothingAltered, nil},
			{"declarations reordered", head + k.declare("Other", 1) + k.declare("Run", 1), snapshot.NothingAltered, nil},
			{"Run's body", head + k.declare("Run", 2) + k.declare("Other", 1), snapshot.ByName, []string{b}},
			{"Run removed", head + k.declare("Other", 1), snapshot.ByName, []string{b}},
		}
		for _, cs := range cases {
			now := maps.Clone(base)
			now[a] = cs.changed
			states, users := radiusOf(t, snapshottest.Files(base, now))
			if len(states) != 1 || !strings.HasPrefix(states[0], fmt.Sprintf("%s %d ", a, cs.state)) || !slices.Equal(users, cs.users) {
				t.Errorf("%s: radius = %q, users %q; want %s in state %d, users %q", cs.label, states, users, a, cs.state, cs.users)
			}
		}
	})
}

func TestTheBlastRadiusCarriesNamesAndCountsWholeFilesWhereNamesCannotBeRead(t *testing.T) {
	eachLanguage(t, func(t *testing.T, name string, c languageCase) {
		k := codeOf(name)
		a, b, d, f, g := c.product("a"), c.product("b"), c.product("d"), c.product("f"), c.product("g")
		head := c.importing(a, nil)
		base := c.tree(map[string]string{
			a: head + k.declare("Run", 1),
			b: c.importing(b, []string{a}) + k.use("Run"),
			d: c.importing(d, []string{a}) + k.declare("Unrelated", 1),
			f: c.importing(f, []string{b}) + k.use("Run"),
			g: c.importing(g, []string{b}) + k.declare("Elsewhere", 1),
		})
		check := func(label string, now map[string]string, wantState snapshot.FileState, wantUsers []string) {
			t.Helper()
			states, users := radiusOf(t, snapshottest.Files(base, now))
			if len(states) != 1 || !strings.HasPrefix(states[0], fmt.Sprintf("%s %d ", a, wantState)) || !slices.Equal(users, wantUsers) {
				t.Errorf("%s: radius = %q, users %q; want %s in state %d, users %q", label, states, users, a, wantState, wantUsers)
			}
		}
		changedBody := maps.Clone(base)
		changedBody[a] = head + k.declare("Run", 2)
		// b uses Run and passes it on: f, which imports b and holds Run, counts; g, which imports b, does not
		check("Run's body", changedBody, snapshot.ByName, []string{b, f})

		broken := maps.Clone(base)
		broken[a] = head + k.declare("Run", 2) + "}}} (((\n"
		// its declarations cannot be read: every importer counts, none behind them
		check("a syntax error", broken, snapshot.AsAWhole, []string{b, d})

		brokenBoth, brokenComment := maps.Clone(base), maps.Clone(base)
		brokenBoth[a] = head + k.declare("Run", 1) + "}}} (((\n"
		brokenComment[a] = head + k.comment("Run") + k.declare("Run", 1) + "}}} (((\n"
		states, users := radiusOf(t, snapshottest.Files(brokenBoth, brokenComment))
		if len(states) != 1 || !strings.HasPrefix(states[0], fmt.Sprintf("%s %d ", a, snapshot.NothingAltered)) || len(users) != 0 {
			t.Errorf("a comment in a file the parser cannot read: radius = %q, users %q; want nothing altered", states, users)
		}

		deleted := maps.Clone(base)
		delete(deleted, a)
		// a is gone with its folder or path: the files that imported it count whole
		check("a deleted", deleted, snapshot.Deleted, []string{b, d})
	})
}

func TestTheBlastRadiusAsksTheFilesThatNeedNoImport(t *testing.T) {
	cases := []struct {
		language string
		base     map[string]string
		changed  string
		now      string
		users    []string
	}{
		{"go", map[string]string{"go.mod": "module example.com/m\n",
			"internal/a/a.go":     "package a\n\nfunc Run() int { return 1 }\n",
			"internal/a/peer.go":  "package a\n\nvar x = Run()\n",
			"internal/a/other.go": "package a\n\nvar y = 2\n"},
			"internal/a/a.go", "package a\n\nfunc Run() int { return 2 }\n", []string{"internal/a/peer.go"}},
		{"java", map[string]string{
			"src/main/java/com/acme/a/A.java":     "package com.acme.a;\n\npublic class A { int v() { return 1; } }\n",
			"src/main/java/com/acme/a/Peer.java":  "package com.acme.a;\n\nclass Peer { int x = new A().v(); }\n",
			"src/test/java/com/acme/a/ATest.java": "package com.acme.a;\n\nclass ATest { A a; }\n",
			"src/main/java/com/acme/b/Other.java": "package com.acme.b;\n\nclass Other { int A = 1; }\n"},
			"src/main/java/com/acme/a/A.java", "package com.acme.a;\n\npublic class A { int v() { return 2; } }\n",
			[]string{"src/main/java/com/acme/a/Peer.java", "src/test/java/com/acme/a/ATest.java"}},
		{"c", map[string]string{
			"src/codec.c": "#include \"codec.h\"\n\nint encode(int x) { return x; }\n",
			"src/codec.h": "#pragma once\nint encode(int x);\n",
			"src/main.c":  "#include \"codec.h\"\n\nint main(void) { return encode(1); }\n",
			"src/other.c": "#include \"codec.h\"\n\nint other(void) { return 0; }\n"},
			"src/codec.c", "#include \"codec.h\"\n\nint encode(int x) { return x + 1; }\n", []string{"src/codec.h", "src/main.c"}},
	}
	for _, c := range cases {
		t.Run(c.language, func(t *testing.T) {
			now := maps.Clone(c.base)
			now[c.changed] = c.now
			_, users := radiusOf(t, snapshottest.Files(c.base, now))
			if !slices.Equal(users, c.users) {
				t.Errorf("users = %q, want %q", users, c.users)
			}
		})
	}
}

func TestTheBlastRadiusIsNotAnalysedOrALowerBoundWhereChangedFilesCannotBeCounted(t *testing.T) {
	c := languageCases()["typescript"]
	k := codeOf("typescript")
	a, b, gen := c.product("a"), c.product("b"), c.product("gen")
	base := c.tree(map[string]string{
		a:   k.declare("Run", 1),
		b:   c.importing(b, []string{a, gen}) + k.use("Run") + k.use("Gen"),
		gen: "// Code generated by protoc. DO NOT EDIT.\n" + k.declare("Gen", 1),
	})
	cases := []struct {
		label string
		now   map[string]string
		state answer.RadiusState
		files int
	}{
		{"nothing changed", nil, answer.RadiusCounted, 0},
		{"a generated file alone", map[string]string{gen: "// Code generated by protoc. DO NOT EDIT.\n" + k.declare("Gen", 2)}, answer.RadiusNotAnalysed, 0},
		{"a generated file beside a counted one", map[string]string{gen: "// Code generated by protoc. DO NOT EDIT.\n" + k.declare("Gen", 2),
			a: k.declare("Run", 2)}, answer.RadiusAtLeast, 1},
		{"a counted file alone", map[string]string{a: k.declare("Run", 2)}, answer.RadiusCounted, 1},
	}
	for _, cs := range cases {
		now := maps.Clone(base)
		maps.Copy(now, cs.now)
		r := detect(t, snapshottest.Files(base, now)).Radius
		if r == nil || r.State != cs.state || r.Files != cs.files {
			t.Errorf("%s: radius = %+v, want %s with %d files", cs.label, r, cs.state, cs.files)
		}
	}
}

func TestTheBlastRadiusListsItsFilesUpToTheCap(t *testing.T) {
	c := languageCases()["typescript"]
	k := codeOf("typescript")
	a := c.product("a")
	base := map[string]string{a: k.declare("Run", 1)}
	var want []string
	for i := range 32 {
		user := c.product(fmt.Sprintf("u%02d", i))
		base[user] = c.importing(user, []string{a}) + k.use("Run")
		want = append(want, user)
	}
	base = c.tree(base)
	now := maps.Clone(base)
	now[a] = k.declare("Run", 2)
	f := detect(t, snapshottest.Files(base, now))
	r := f.Radius
	if r == nil || r.Files != 32 || !slices.Equal(r.Paths, want[:30]) || r.Omitted != 2 || r.ByName != 32 || r.Product != 32 {
		t.Fatalf("radius = %+v, want 32 by name, the first 30 paths and 2 omitted", r)
	}
	rows, _ := statisticNamed(f, "blast radius files")
	if !strings.HasSuffix(rows.Note, " and 2 more") || strings.Count(rows.Note, "`")/2 != 30 {
		t.Errorf("blast radius files = %q, want 30 paths and 2 more", rows.Note)
	}
	if row, _ := statisticNamed(f, "blast radius"); row.Value == nil || *row.Value != 32 {
		t.Errorf("blast radius row = %+v, want 32 files", row)
	}
}

func TestAChangedFileWhoseContentIsMissingLeavesTheRadiusNotAnalysed(t *testing.T) {
	c := languageCases()["typescript"]
	k := codeOf("typescript")
	a, b := c.product("a"), c.product("b")
	base := c.tree(map[string]string{a: k.declare("Run", 1), b: c.importing(b, []string{a}) + k.use("Run")})
	now := maps.Clone(base)
	now[a] = k.declare("Run", 2)
	source := snapshottest.Files(base, now)
	for i, e := range source.Before {
		if e.Path == a {
			source.Before[i].Content = nil // the partial clone lacks a's old content
		}
	}
	source.Missing = []string{a}
	if r := detect(t, source).Radius; r == nil || r.State != answer.RadiusNotAnalysed {
		t.Errorf("only a changed, its old content missing: radius = %+v, want not analysed", r)
	}
	other := maps.Clone(now)
	other[b] = c.importing(b, []string{a}) + k.use("Run") + k.declare("More", 1)
	source = snapshottest.Files(base, other)
	source.Missing = []string{c.product("unrelated")}
	if r := detect(t, source).Radius; r == nil || r.State != answer.RadiusCounted {
		t.Errorf("a missing path that is no tracked code file: radius = %+v, want counted", r)
	}
}

func TestTheStateOfAChangedFileFollowsWhatItsDeclarationsDid(t *testing.T) {
	cases := []struct {
		label, path, base, now string
		state                  snapshot.FileState
	}{
		{"a Rust test module added", "src/a.rs", "pub fn run() -> u8 { 1 }\n",
			"pub fn run() -> u8 { 1 }\n\n#[cfg(test)]\nmod tests {\n    #[test]\n    fn t() {}\n}\n", snapshot.NothingAltered},
		{"a Rust attribute added to a function", "src/a.rs", "pub fn run() -> u8 { 1 }\n", "#[inline]\npub fn run() -> u8 { 1 }\n", snapshot.ByName},
		{"a Python import added to a module", "src/pkg/a.py", "import os\n\ndef run():\n    return 1\n", "import os\nimport sys\n\ndef run():\n    return 1\n", snapshot.NothingAltered},
		{"a Python import added to a package's surface", "src/pkg/__init__.py", "from .a import run\n", "from .a import run\nfrom .b import other\n", snapshot.AsAWhole},
		{"a C++ function changed beside a syntax error elsewhere", "src/a.cc", "int run() { return 1; }\n\nint broken( {\n",
			"int run() { return 2; }\n\nint broken( {\n", snapshot.ByName},
		{"a C++ function changed where the syntax error is", "src/a.cc", "int run() { return 1; }\n\nint other() { return 1 $$ ; }\n",
			"int run() { return 1; }\n\nint other() { return 2 $$ ; }\n", snapshot.AsAWhole},
	}
	for _, c := range cases {
		states, _ := radiusOf(t, snapshottest.Files(map[string]string{c.path: c.base}, map[string]string{c.path: c.now}))
		if len(states) != 1 || !strings.HasPrefix(states[0], fmt.Sprintf("%s %d ", c.path, c.state)) {
			t.Errorf("%s: states = %q, want %s in state %d", c.label, states, c.path, c.state)
		}
	}
}

func TestTheBlastRadiusAddsNoNameItReadFromCode(t *testing.T) {
	eachLanguage(t, func(t *testing.T, name string, c languageCase) {
		k := codeOf(name)
		a, b := c.product("a"), c.product("b")
		const distinct = "Zqxwvut"
		base := c.tree(map[string]string{a: c.importing(a, nil) + k.declare(distinct, 1), b: c.importing(b, []string{a}) + k.use(distinct)})
		now := maps.Clone(base)
		now[a] = c.importing(a, nil) + k.declare(distinct, 2)
		f := detect(t, snapshottest.Files(base, now))
		if f.Radius == nil || f.Radius.ByName != 1 {
			t.Fatalf("radius = %+v, want the one user by name", f.Radius)
		}
		written, err := json.Marshal(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(written), distinct) {
			t.Errorf("the findings hold the altered name %s: %s", distinct, written)
		}
	})
}

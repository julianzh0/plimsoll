package links

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

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
		states = append(states, fmt.Sprintf("%s %d %s", cmp.Or(c.Path, c.BasePath), c.State, c.Reason))
	}
	for _, u := range r.Users {
		users = append(users, u.Path)
	}
	return states, users
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
	for _, test := range []string{"__tests__/t0.ts", "__tests__/t1.ts"} { // before the product files in path order
		base[test] = c.importing(test, []string{a}) + k.use("Run")
	}
	base = c.tree(base)
	now := maps.Clone(base)
	now[a] = k.declare("Run", 2)
	f := detect(t, snapshottest.Files(base, now))
	r := f.Radius
	limit := int(conventions.Default().Limits.RadiusPaths.Value)
	if r == nil || r.Files != 34 || !slices.Equal(r.Paths, want[:limit]) || r.Omitted != 34-limit || r.ByName != 34 || r.Product != 32 || r.Tests != 2 {
		t.Fatalf("radius = %+v, want 34 by name, the first %d product paths and the rest omitted", r, limit)
	}
	rows, _ := statisticNamed(f, "blast radius files")
	if !strings.HasSuffix(rows.Note, fmt.Sprintf(" and %d more (%d product, 2 test)", 34-limit, 32-limit)) || strings.Count(rows.Note, "`")/2 != limit {
		t.Errorf("blast radius files = %q, want %d product paths and the rest", rows.Note, limit)
	}
	if row, _ := statisticNamed(f, "blast radius"); !strings.HasPrefix(row.Note, "34 files (32 product, 2 test); ") {
		t.Errorf("blast radius row = %+v, want 34 files", row)
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
		// a Python import binds a name its importers can reach, in any module
		{"a Python import added to a module", "src/pkg/a.py", "import os\n\ndef run():\n    return 1\n", "import os\nimport sys\n\ndef run():\n    return 1\n", snapshot.AsAWhole},
		{"a Python binding re-exported from elsewhere", "src/pkg/api.py", "from .old import Run\n", "from .new import Run\n", snapshot.AsAWhole},
		{"a Python import added to a package's surface", "src/pkg/__init__.py", "from .a import run\n", "from .a import run\nfrom .b import other\n", snapshot.AsAWhole},
		{"a C++ function changed beside a syntax error elsewhere", "src/a.cc", "int run() { return 1; }\n\nint broken( {\n",
			"int run() { return 2; }\n\nint broken( {\n", snapshot.ByName},
		{"a C++ function changed where the syntax error is", "src/a.cc", "int run() { return 1; }\n\nint other() { return 1 $$ ; }\n",
			"int run() { return 1; }\n\nint other() { return 2 $$ ; }\n", snapshot.AsAWhole},
		{"a syntax error written into a changed function", "src/a.cc", "int run() { return 1; }\n",
			"int run() { return 2 $$ ; }\n", snapshot.AsAWhole},
		{"a TypeScript namespace renamed", "src/a.ts", "export namespace A {\n  export const x = 1;\n}\n",
			"export namespace B {\n  export const x = 1;\n}\n", snapshot.AsAWhole},
		{"a Go function whose name starts with a non-ASCII letter changed", "a.go", "package a\n\nfunc Über() int { return 1 }\n",
			"package a\n\nfunc Über() int { return 2 }\n", snapshot.ByName},
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

func TestAModuleDeclarationAloneMakesNoImporterOrUser(t *testing.T) {
	base := map[string]string{
		"Cargo.toml":  "[package]\nname = \"app\"\n",
		"src/lib.rs":  "mod a;\n",
		"src/a.rs":    "pub fn run() -> u8 { 1 }\n",
		"src/main.rs": "mod a;\npub use a::run;\n",
	}
	now := maps.Clone(base)
	now["src/a.rs"] = "pub fn run() -> u8 { 2 }\n"
	f := detect(t, snapshottest.Files(base, now))
	if f.Radius == nil || !slices.Equal(f.Radius.Paths, []string{"src/main.rs"}) {
		t.Errorf("radius = %+v, want main.rs, which re-exports run, and not lib.rs, which only declares the module", f.Radius)
	}
	if r, _ := statisticNamed(f, "importers of the change"); !strings.HasPrefix(r.Note, "1 file directly (1 product, 0 test), ") {
		t.Errorf("importers of the change = %q, want main.rs alone", r.Note)
	}
	if want := "`rust`: no importer and no user: a parent file that only declares a module (`mod x;`)"; !slices.Contains(f.Notes, want) {
		t.Errorf("notes %q, want %q", f.Notes, want)
	}
}

// A Swift target's files are each changed file's peers; the links of every file that imports the target reach all of
// them. Indexing those links once per imported folder keeps a large target fast: indexing per file and link took
// about 45 s for these 1,000 files and 100 importers.
func TestALargeSwiftTargetWithManyImportersIsQuick(t *testing.T) {
	const files, importers = 1000, 100
	base := map[string]string{"Package.swift": "let package = Package(name: \"app\", targets: [\n" +
		"    .target(name: \"Core\", path: \"Sources/Core\"),\n    .target(name: \"App\", path: \"Sources/App\"),\n])\n"}
	for i := range files {
		base[fmt.Sprintf("Sources/Core/F%d.swift", i)] = fmt.Sprintf("public func f%d() -> Int { return %d }\n", i, i)
	}
	var want []string
	for i := range importers {
		p := fmt.Sprintf("Sources/App/U%d.swift", i)
		base[p] = "import Core\n\nlet use = f0()\n"
		want = append(want, p)
	}
	slices.Sort(want)
	now := clone(base)
	now["Sources/Core/F0.swift"] = "public func f0() -> Int { return 1 }\n"
	start := time.Now()
	_, users := radiusOf(t, snapshottest.Files(base, now))
	if took := time.Since(start); took > 10*time.Second {
		t.Errorf("the blast radius of a 1,000-file target with 100 importers took %v, want a few seconds at most", took)
	}
	if !slices.Equal(users, want) {
		t.Errorf("users = %d files %q..., want the %d importers that call f0", len(users), users[:min(3, len(users))], importers)
	}
}

func TestAChangeWithoutCodeHasARadiusOfZero(t *testing.T) {
	base := map[string]string{"src/a.ts": "export const a = 1;\n", "README.md": "# a\n"}
	now := clone(base)
	now["README.md"] += "more\n"
	row, found := statisticNamed(detect(t, snapshottest.Files(base, now)), "blast radius")
	if want := "0 files; no code file changed"; !found || row.Note != want {
		t.Errorf("blast radius of a change without code = %+v (found %v), want %q", row, found, want)
	}
	partial := snapshottest.Files(base, now)
	partial.Missing = []string{"src/a.ts"} // an untouched code file whose content this clone lacks
	for i, e := range partial.Now {
		if e.Path == "src/a.ts" {
			partial.Now[i].Content = nil
		}
	}
	if row, _ := statisticNamed(detect(t, partial), "blast radius"); row.Note != "0 files found, an incomplete count; no code file changed" {
		t.Errorf("blast radius of a change without code in a partial clone = %q, want an incomplete count as the headline says", row.Note)
	}
}

// A radius of zero with a changed file not counted is no all-clear; the importers note names Rust's module rule only
// where Rust's links are used.
func TestAnIncompleteZeroSaysSoAndTheRustRuleOnlyWithRust(t *testing.T) {
	c := languageCases()["typescript"]
	k := codeOf("typescript")
	a, b, gone := c.product("a"), c.product("b"), c.product("gone")
	base := c.tree(map[string]string{a: k.declare("Run", 1), b: c.importing(b, []string{a}), gone: k.declare("Gone", 1)})
	now := maps.Clone(base)
	now[a] += k.comment("Run")
	now[gone] = k.declare("Gone", 2)
	source := snapshottest.Files(base, now)
	for i, e := range source.Before {
		if e.Path == gone {
			source.Before[i].Content = nil // the partial clone lacks gone's old content
		}
	}
	f := detect(t, source)
	if row, _ := statisticNamed(f, "blast radius"); !strings.HasPrefix(row.Note, "0 files found, an incomplete count; ") {
		t.Errorf("blast radius = %q, want an incomplete count, not at least 0", row.Note)
	}
	if slices.ContainsFunc(f.Notes, func(n string) bool { return strings.Contains(n, "Rust") }) {
		t.Errorf("notes of a TypeScript tree = %q, want no Rust rule", f.Notes)
	}
}

// usersOf gives the paths a change's radius lists, after the language gate.
func usersOf(t *testing.T, source snapshot.Source) []string {
	t.Helper()
	f := detect(t, source)
	if f.Radius == nil {
		t.Fatal("no radius")
	}
	return f.Radius.Paths
}

func TestNamesOfAnyLetterReachTheirUsers(t *testing.T) {
	for _, tc := range []struct{ label, name, declare, file, user string }{
		{"a non-ASCII letter inside", "Café", "func %s() int { return %d }\n", "internal/a/a.go", "var _ = a.%s()\n"},
		{"a non-ASCII first letter", "Über", "func %s() int { return %d }\n", "internal/a/a.go", "var _ = a.%s()\n"},
	} {
		base := map[string]string{"go.mod": "module " + goModule + "\n",
			tc.file:           "package a\n\n" + fmt.Sprintf(tc.declare, tc.name, 1),
			"internal/b/b.go": "package b\n\nimport \"" + goModule + "/internal/a\"\n\n" + fmt.Sprintf(tc.user, tc.name)}
		now := maps.Clone(base)
		now[tc.file] = "package a\n\n" + fmt.Sprintf(tc.declare, tc.name, 2)
		if users := usersOf(t, snapshottest.Files(base, now)); !slices.Equal(users, []string{"internal/b/b.go"}) {
			t.Errorf("%s: users = %q, want the file that calls %s", tc.label, users, tc.name)
		}
	}
	base := map[string]string{"src/a.ts": "export function $run() { return 1; }\n", "src/b.ts": "import { $run } from './a';\n$run();\n"}
	now := maps.Clone(base)
	now["src/a.ts"] = "export function $run() { return 2; }\n"
	if f := detect(t, snapshottest.Files(base, now)); f.Radius == nil || !slices.Equal(f.Radius.Paths, []string{"src/b.ts"}) || f.Radius.ByName != 1 {
		t.Errorf("a name with $: radius = %+v, want src/b.ts by name", f.Radius)
	}
	// A name the lexer ends at a non-ASCII space is the same word to the prefilter, warm or cold.
	base = map[string]string{"src/a.ts": "export function Run() { return 1; }\n", "src/b.ts": "import * as a from './a';\na.Run\u00a0();\n"}
	now = maps.Clone(base)
	now["src/a.ts"] = "export function Run() { return 2; }\n"
	if users := usersOf(t, snapshottest.Files(base, now)); !slices.Equal(users, []string{"src/b.ts"}) {
		t.Errorf("a name before a non-breaking space: users = %q, want the file that calls Run", users)
	}
}

// A user the gate leaves out for its own language although a used language's change reached it makes the number a
// lower bound, never an exact count.
func TestAUserTheGateLeavesOutMakesALowerBound(t *testing.T) {
	s := snapshot.Snapshot{Radius: snapshot.Radius{
		Changed: []snapshot.ChangedFile{{Path: "a.go", Language: "go", State: snapshot.ByName}},
		Users:   []snapshot.User{{Path: "src/u.ts", Role: snapshot.Product, ByName: []string{"go"}, Outside: []string{"go"}}},
	}}
	r, _, _ := blastRadius(s, []snapshot.LanguageImports{{Language: "go"}}, graph{known: map[string]bool{}})
	if r.State != answer.RadiusAtLeast || r.Files != 0 {
		t.Errorf("radius = %+v, want at least 0: the TypeScript user is out of sight", r)
	}
}

// A user reached by name only from a language whose links are not used, and as a whole from a used one, counts as a
// whole: the gated language decides nothing about it.
func TestAGatedLanguageDecidesNothingAboutAUser(t *testing.T) {
	s := snapshot.Snapshot{Radius: snapshot.Radius{
		Changed: []snapshot.ChangedFile{{Path: "a.go", Language: "go", State: snapshot.AsAWhole}, {Path: "src/b.ts", Language: "typescript", State: snapshot.ByName}},
		Users:   []snapshot.User{{Path: "u.go", Role: snapshot.Product, Whole: []string{"go"}, ByName: []string{"typescript"}}},
	}}
	r, _, _ := blastRadius(s, []snapshot.LanguageImports{{Language: "go"}}, graph{known: map[string]bool{"u.go": true}})
	if r.Files != 1 || r.AsWhole != 1 || r.ByName != 0 || r.Product != 1 {
		t.Errorf("radius = %+v, want one product file counted as a whole", r)
	}
}

// A product file that a used language reaches as a whole is product code even when the name it holds by name sits
// only in its tests.
func TestAUserReachedAsAWholeIsNoTestOnlyUser(t *testing.T) {
	s := snapshot.Snapshot{Radius: snapshot.Radius{
		Changed: []snapshot.ChangedFile{{Path: "a.go", Language: "go", State: snapshot.AsAWhole}, {Path: "b.go", Language: "go", State: snapshot.ByName}},
		Users:   []snapshot.User{{Path: "u.go", Role: snapshot.Product, Whole: []string{"go"}, ByName: []string{"go"}}},
	}}
	r, _, _ := blastRadius(s, []snapshot.LanguageImports{{Language: "go"}}, graph{known: map[string]bool{"u.go": true}})
	if r.Files != 1 || r.Product != 1 || r.Tests != 0 {
		t.Errorf("radius = %+v, want one product file", r)
	}
}

// The same name changed in two languages, one of them behind the gate: a file of the gated language that uses the
// name counts neither through its own language nor through the other one's seed.
func TestTheGateHoldsWhenTwoLanguagesChangeTheSameName(t *testing.T) {
	ts, k := languageCases()["typescript"], codeOf("typescript")
	base := ts.fan(8, ts.nowhere) // over the limit: TypeScript's links are not used
	ta, user := ts.product("ta"), ts.product("a")
	base[ta] += k.declare("Run", 1)
	base[user] += k.use("Run")
	base["go.mod"] = "module " + goModule + "\n"
	base["internal/a/a.go"] = "package a\n\nfunc Run() int { return 1 }\n"
	base["internal/b/b.go"] = "package b\n\nimport \"" + goModule + "/internal/a\"\n\nvar _ = a.Run()\n"
	now := maps.Clone(base)
	now[ta] = strings.Replace(now[ta], k.declare("Run", 1), k.declare("Run", 2), 1)
	now["internal/a/a.go"] = "package a\n\nfunc Run() int { return 2 }\n"
	if users := usersOf(t, snapshottest.Files(base, now)); !slices.Equal(users, []string{"internal/b/b.go"}) {
		t.Errorf("users = %q, want the Go user alone", users)
	}
	// The walk itself keeps the languages that reached a user: the TypeScript user was reached from TypeScript alone.
	s := snapshot.Build(snapshottest.Files(base, now), conventions.Default(), snapshottest.Readings(t))
	found := false
	for _, u := range s.Radius.Users {
		if u.Path == user {
			found = true
			if !slices.Equal(u.ByName, []string{"typescript"}) {
				t.Errorf("the TypeScript user was reached by name from %q, want typescript alone", u.ByName)
			}
		}
	}
	if !found {
		t.Errorf("the walk did not reach the TypeScript user %s", user)
	}
	for _, u := range s.Radius.Users {
		if u.Path == "internal/b/b.go" && !slices.Equal(u.ByName, []string{"go"}) {
			t.Errorf("the Go user was reached by name from %q, want go alone", u.ByName)
		}
	}
}

func TestACodeFileRenamedToAnotherSuffixIsGone(t *testing.T) {
	base := map[string]string{"src/a.ts": "export function run() { return 1; }\n", "src/b.ts": "import { run } from './a';\nrun();\n"}
	now := map[string]string{"src/a.txt": base["src/a.ts"], "src/b.ts": base["src/b.ts"]}
	source := snapshottest.Files(base, now)
	source.Changes = []snapshot.Change{{Path: "src/a.txt", From: "src/a.ts"}}
	states, users := radiusOf(t, source)
	if !slices.Equal(states, []string{fmt.Sprintf("src/a.ts %d ", snapshot.Deleted)}) || !slices.Equal(users, []string{"src/b.ts"}) {
		t.Errorf("states %q, users %q; want a.ts deleted and its importer counted", states, users)
	}
}

func TestAChangedFileTheRepositoryIgnoresIsNotCounted(t *testing.T) {
	base := map[string]string{".plimsoll.toml": "[ignore]\nfolders = [\"gen\"]\n", "gen/x.ts": "export function run() { return 1; }\n",
		"src/b.ts": "import { run } from '../gen/x';\nrun();\n"}
	now := maps.Clone(base)
	now["gen/x.ts"] = "export function run() { return 2; }\n"
	f := detect(t, snapshottest.Files(base, now))
	if f.Radius == nil || f.Radius.State != answer.RadiusNotAnalysed ||
		!slices.ContainsFunc(f.NotAnalysed, func(s string) bool { return strings.Contains(s, "(left out by the repository's own file): `gen/x.ts`") }) {
		t.Errorf("radius %+v, not analysed %q; want not analysed and the ignored file named", f.Radius, f.NotAnalysed)
	}
	if _, found := statisticNamed(f, "importers of the change"); found {
		t.Errorf("an ignored file's change printed importers of the change, want none")
	}
}

func TestASwiftTargetsFilesAreUsersWithoutAnImport(t *testing.T) {
	base := map[string]string{"Package.swift": "let package = Package(name: \"app\", targets: [\n    .target(name: \"App\", path: \"Sources/App\"),\n])\n",
		"Sources/App/Core/F.swift": "public func f0() -> Int { return 1 }\n", "Sources/App/UI/U.swift": "let use = f0()\n"}
	now := maps.Clone(base)
	now["Sources/App/Core/F.swift"] = "public func f0() -> Int { return 2 }\n"
	if users := usersOf(t, snapshottest.Files(base, now)); !slices.Equal(users, []string{"Sources/App/UI/U.swift"}) {
		t.Errorf("users = %q, want the file of the same target in another folder", users)
	}
}

func TestTheStructuredRadiusHoldsTheNumbersItsRowsSay(t *testing.T) {
	base := map[string]string{"src/a.ts": "export function run() { return 1; }\n", "src/b.ts": "import { run } from './a';\nexport const b = run();\n",
		"src/c.ts": "import { b } from './b';\nexport const c = b;\n", "tests/a.test.ts": "import { run } from '../src/a';\nrun();\n"}
	now := maps.Clone(base)
	now["src/a.ts"] = "export function run() { return 2; }\n"
	f := detect(t, snapshottest.Files(base, now))
	r := f.Radius
	want := answer.ChangedFiles{All: 1, ByName: 1}
	if r == nil || r.Changed != want || r.Direct != (answer.Importers{Files: 2, Product: 1, Tests: 1}) ||
		r.Total != (answer.Importers{Files: 3, Product: 2, Tests: 1}) || r.ProductCodeFiles != 3 ||
		!slices.Equal(r.Folders, []answer.FolderCount{{Folder: "src/", Files: 2}, {Folder: "tests/", Files: 1}}) {
		t.Fatalf("radius = %+v, want 1 changed by name, 2 direct (1 product, 1 test), 3 in total, 2 of 3 product files", r)
	}
	if row, _ := statisticNamed(f, "importers of the change"); !strings.HasPrefix(row.Note, "2 files directly (1 product, 1 test), 3 at any distance (2 product, 1 test); in `src/` 2, `tests/` 1; 2 of 3 product code files") {
		t.Errorf("importers row = %q, want the structured numbers", row.Note)
	}
}

func TestADeletedFileWithABrokenDeclarationCountsAsAWhole(t *testing.T) {
	base := map[string]string{"src/a.cc": "int run() { return 1 $$ ; }\n", "src/b.cc": "#include \"a.cc\"\n"}
	states, _ := radiusOf(t, snapshottest.Files(base, map[string]string{"src/b.cc": base["src/b.cc"]}))
	if want := fmt.Sprintf("src/a.cc %d %s", snapshot.Deleted, snapshot.ReasonSyntaxError); !slices.Contains(states, want) {
		t.Errorf("states = %q, want %q", states, want)
	}
}

func TestAChangedFileThatCouldNotBeReadIsNotCountedAndLowersTheBound(t *testing.T) {
	base := map[string]string{"src/a.ts": "export function run() { return 1; }\n", "src/b.ts": "export function other() { return 1; }\n"}
	now := maps.Clone(base)
	now["src/a.ts"] = "export function run() { return 2; }\n"
	now["src/b.ts"] = "export function other() { return 2; }\n"
	source := snapshottest.Files(base, now)
	for i, e := range source.Now {
		if e.Path == "src/a.ts" {
			source.Now[i].Content = nil // the read failed
		}
	}
	source.Unreadable = []string{"src/a.ts"}
	states, _ := radiusOf(t, source)
	if want := fmt.Sprintf("src/a.ts %d %s", snapshot.NotCounted, snapshot.ReasonUnreadable); !slices.Contains(states, want) {
		t.Errorf("states = %q, want %q", states, want)
	}
	if r := detect(t, source).Radius; r == nil || r.State != answer.RadiusAtLeast {
		t.Errorf("radius = %+v, want at least", r)
	}
}

func TestAnUntouchedFileThatCouldNotBeReadLowersTheBound(t *testing.T) {
	base := map[string]string{"src/a.ts": "export function run() { return 1; }\n", "src/u.ts": "import { run } from './a';\nrun();\n",
		"src/b.ts": "export const b = 1;\n"}
	now := maps.Clone(base)
	now["src/a.ts"] = "export function run() { return 2; }\n"
	source := snapshottest.Files(base, now)
	for i, e := range source.Now {
		if e.Path == "src/b.ts" {
			source.Now[i].Content = nil // the read failed; the file is untouched
		}
	}
	source.Unreadable = []string{"src/b.ts"}
	if r := detect(t, source).Radius; r == nil || r.State != answer.RadiusAtLeast || r.Changed.NotCounted != 0 {
		t.Errorf("radius = %+v, want at least, with no changed file left out", r)
	}
}

func TestAnIgnoredChangeBesideACountedOneSeedsNoUsers(t *testing.T) {
	base := map[string]string{".plimsoll.toml": "[ignore]\nfolders = [\"gen\"]\n", "gen/x.ts": "export function gen() { return 1; }\n",
		"src/a.ts": "export function run() { return 1; }\n", "src/b.ts": "import { run } from './a';\nrun();\n",
		"src/c.ts": "import { gen } from '../gen/x';\ngen();\n"}
	now := maps.Clone(base)
	now["gen/x.ts"] = "export function gen() { return 2; }\n"
	now["src/a.ts"] = "export function run() { return 2; }\n"
	f := detect(t, snapshottest.Files(base, now))
	if f.Radius == nil || !slices.Equal(f.Radius.Paths, []string{"src/b.ts"}) || f.Radius.State != answer.RadiusAtLeast {
		t.Errorf("radius = %+v, want src/b.ts alone, at least: the ignored file seeds no user", f.Radius)
	}
}

func TestARenameIntoAnotherLanguageIsOneFileGoneAndOneNew(t *testing.T) {
	base := map[string]string{"src/a.c": "int run(void) { return 1; }\n", "src/b.c": "#include \"a.c\"\nint use(void) { return run(); }\n"}
	now := map[string]string{"src/a.cpp": base["src/a.c"], "src/b.c": base["src/b.c"]}
	source := snapshottest.Files(base, now)
	source.Changes = []snapshot.Change{{Path: "src/a.cpp", From: "src/a.c"}}
	states, _ := radiusOf(t, source)
	if want := []string{fmt.Sprintf("src/a.c %d ", snapshot.Deleted), fmt.Sprintf("src/a.cpp %d ", snapshot.NothingAltered)}; !slices.Equal(states, want) {
		t.Errorf("states = %q, want %q", states, want)
	}
}

func TestANestedSwiftTargetIsNoPeerOfTheOuterOne(t *testing.T) {
	base := map[string]string{"Package.swift": "let package = Package(name: \"app\", targets: [\n    .target(name: \"App\", path: \"Sources/App\", exclude: [\"Inner\"]),\n    .target(name: \"Inner\", path: \"Sources/App/Inner\"),\n])\n",
		"Sources/App/F.swift": "public func f0() -> Int { return 1 }\n", "Sources/App/Inner/U.swift": "let use = f0()\n",
		"Sources/App/Core/V.swift": "let v = f0()\n"}
	now := maps.Clone(base)
	now["Sources/App/F.swift"] = "public func f0() -> Int { return 2 }\n"
	if users := usersOf(t, snapshottest.Files(base, now)); !slices.Equal(users, []string{"Sources/App/Core/V.swift"}) {
		t.Errorf("users = %q, want the outer target's file alone", users)
	}
}

func TestTheProductDenominatorDoesNotDependOnTheImporterRow(t *testing.T) {
	base := map[string]string{"src/a.ts": "export const a = 1;\n", "src/b.ts": "import { a } from './a';\nexport const b = a;\n"}
	now := maps.Clone(base)
	now["src/a.ts"] = "// a comment\nexport const a = 1;\n"
	f := detect(t, snapshottest.Files(base, now))
	if _, row := statisticNamed(f, "importers of the change"); row || f.Radius == nil || f.Radius.ProductCodeFiles != 2 {
		t.Errorf("importer row %v, radius %+v; want no row and 2 product code files", row, f.Radius)
	}
}

func TestTheRootTargetHoldsOnlyWhatNoDeeperTargetHolds(t *testing.T) {
	base := map[string]string{"Package.swift": "let package = Package(name: \"app\", targets: [\n    .target(name: \"App\", path: \".\"),\n    .target(name: \"A\", path: \"A\"),\n])\n",
		"A/F.swift": "public func f0() -> Int { return 1 }\n", "A/X.swift": "let x = f0()\n", "Main.swift": "let m = f0()\n"}
	now := maps.Clone(base)
	now["A/F.swift"] = "public func f0() -> Int { return 2 }\n"
	if users := usersOf(t, snapshottest.Files(base, now)); !slices.Equal(users, []string{"A/X.swift"}) {
		t.Errorf("users = %q, want the file of target A alone", users)
	}
}

// Content missing in a language the gate judged and left out hides no user that could count: the number stays exact.
func TestMissingContentOfALanguageLeftOutLeavesTheNumberExact(t *testing.T) {
	s := snapshot.Snapshot{Radius: snapshot.Radius{
		Changed:    []snapshot.ChangedFile{{Path: "a.go", Language: "go", State: snapshot.ByName}},
		MissingNow: []string{"csharp"},
	}}
	s.Now.Links.Languages = []snapshot.LanguageImports{{Language: "go"}, {Language: "csharp"}}
	if r, _, _ := blastRadius(s, []snapshot.LanguageImports{{Language: "go"}}, graph{known: map[string]bool{}}); r.State != answer.RadiusCounted {
		t.Errorf("radius = %+v, want counted", r)
	}
	s.Radius.MissingNow = []string{"go"}
	if r, _, _ := blastRadius(s, []snapshot.LanguageImports{{Language: "go"}}, graph{known: map[string]bool{}}); r.State != answer.RadiusAtLeast {
		t.Errorf("missing Go content: radius = %+v, want at least", r)
	}
}

// The importers of the change count every changed code file's importers, also those of a file the blast radius
// does not count.
func TestTheImportersOfTheChangeAreUnfiltered(t *testing.T) {
	base := map[string]string{"src/a.ts": "export const a = 1;\n", "src/b.ts": "import { a } from './a';\nexport const b = a;\n",
		"src/c.ts": "export function run() { return 1; }\n", "src/d.ts": "import { run } from './c';\nrun();\n"}
	now := maps.Clone(base)
	now["src/a.ts"] = "export const a = 2;\n"
	now["src/c.ts"] = "export function run() { return 2; }\n"
	source := snapshottest.Files(base, now)
	for i, e := range source.Before {
		if e.Path == "src/a.ts" {
			source.Before[i].Content = nil // the partial clone lacks a's old content: a is not counted
		}
	}
	source.Missing = []string{"src/a.ts"}
	f := detect(t, source)
	if f.Radius == nil || f.Radius.Changed.NotCounted != 1 || f.Radius.Direct.Files != 2 {
		t.Errorf("radius = %+v, want a not counted and both b and d direct importers", f.Radius)
	}
}

// Each state with its reason, for the ways a file is new, gone, left out, unchanged in code or changed outside its
// declarations.
func TestEachStateOfAChangedFileCarriesItsReason(t *testing.T) {
	const run = "export function run() { return 1; }\n"
	cases := []struct {
		label     string
		base, now map[string]string
		missing   string
		state     snapshot.FileState
		reason    string
	}{
		{"a new file", nil, map[string]string{"src/a.ts": run}, "", snapshot.NothingAltered, ""},
		{"a deleted file", map[string]string{"src/a.ts": run}, nil, "", snapshot.Deleted, ""},
		{"in a skipped folder", map[string]string{"vendor/x/a.go": "package x\n\nfunc F() int { return 1 }\n"},
			map[string]string{"vendor/x/a.go": "package x\n\nfunc F() int { return 2 }\n"}, "", snapshot.NotCounted, snapshot.ReasonSkipped},
		{"generated by its name", map[string]string{"src/A.g.cs": "class A { int F() { return 1; } }\n"},
			map[string]string{"src/A.g.cs": "class A { int F() { return 2; } }\n"}, "", snapshot.NotCounted, snapshot.ReasonGenerated},
		{"generated by its content", map[string]string{"a.go": "// Code generated by protoc. DO NOT EDIT.\n\npackage a\n\nfunc F() int { return 1 }\n"},
			map[string]string{"a.go": "// Code generated by protoc. DO NOT EDIT.\n\npackage a\n\nfunc F() int { return 2 }\n"}, "", snapshot.NotCounted, snapshot.ReasonGenerated},
		{"its old content missing", map[string]string{"src/a.ts": run}, map[string]string{"src/a.ts": "export function run() { return 2; }\n"},
			"src/a.ts", snapshot.NotCounted, snapshot.ReasonMissing},
		{"only its layout changed, where line breaks are no code", map[string]string{"src/a.c": "int run() { return 1; }\n"},
			map[string]string{"src/a.c": "int run()\n{\n    return 1;\n}\n"}, "", snapshot.NothingAltered, ""},
		{"only an import added", map[string]string{"src/a.ts": "import { b } from './b';\n" + run},
			map[string]string{"src/a.ts": "import { b } from './b';\nimport { c } from './c';\n" + run}, "", snapshot.NothingAltered, ""},
		{"a top-level statement changed", map[string]string{"src/a.ts": "console.log(1);\n" + run},
			map[string]string{"src/a.ts": "console.log(2);\n" + run}, "", snapshot.AsAWhole, snapshot.ReasonRemainder},
		{"a syntax error written into a changed function", map[string]string{"src/a.cc": "int run() { return 1; }\n"},
			map[string]string{"src/a.cc": "int run() { return 2 $$ ; }\n"}, "", snapshot.AsAWhole, snapshot.ReasonSyntaxError},
		{"a deleted file missing a token", map[string]string{"a.go": "package a\n\nfunc Run( int { return 1 }\n"}, nil, "",
			snapshot.Deleted, snapshot.ReasonSyntaxError},
		{"a declaration changed", map[string]string{"src/a.ts": run}, map[string]string{"src/a.ts": "export function run() { return 2; }\n"},
			"", snapshot.ByName, ""},
		{"its line breaks changed, where they are code", map[string]string{"src/a.ts": run},
			map[string]string{"src/a.ts": "export function run() {\n  return 1;\n}\n"}, "", snapshot.ByName, ""},
		{"its indentation changed, where it is code", map[string]string{"src/pkg/a.py": "def run(x):\n    if x:\n        x = 1\n    return x\n"},
			map[string]string{"src/pkg/a.py": "def run(x):\n    if x:\n        x = 1\n        return x\n"}, "", snapshot.ByName, ""},
		{"a member of a namespace changed", map[string]string{"src/a.ts": "export namespace N {\n  export const x = 1;\n  export const y = 1;\n}\n"},
			map[string]string{"src/a.ts": "export namespace N {\n  export const x = 2;\n  export const y = 1;\n}\n"}, "", snapshot.ByName, ""},
	}
	for _, c := range cases {
		source := snapshottest.Files(c.base, c.now)
		if c.missing != "" {
			for i, e := range source.Before {
				if e.Path == c.missing {
					source.Before[i].Content = nil
				}
			}
			source.Missing = []string{c.missing}
		}
		states, _ := radiusOf(t, source)
		p := slices.Collect(maps.Keys(c.now))
		if len(p) == 0 {
			p = slices.Collect(maps.Keys(c.base))
		}
		if want := fmt.Sprintf("%s %d %s", p[0], c.state, c.reason); !slices.Equal(states, []string{want}) {
			t.Errorf("%s: states = %q, want %q", c.label, states, want)
		}
	}
}

// How the walk reaches each user from each kind of changed file: as a whole or by name, as an untouched user or one the
// change touched.
func TestTheWalkSeedsEachKindOfChangedFile(t *testing.T) {
	const ts = "export function run() { return 1; }\n"
	const user = "import { run } from './a';\nrun();\n"
	goPkg := func(extra map[string]string) map[string]string {
		m := map[string]string{"go.mod": "module " + goModule + "\n",
			"internal/a/a.go":  "package a\n\nfunc Run() int { return 1 }\n",
			"internal/a/a2.go": "package a\n\nfunc Other() int { return 1 }\n",
			"internal/b/b.go":  "package b\n\nimport \"" + goModule + "/internal/a\"\n\nvar _ = a.Run()\n",
			"internal/c/c.go":  "package c\n\nimport \"" + goModule + "/internal/a\"\n\nvar _ = a.Other()\n"}
		maps.Copy(m, extra)
		return m
	}
	without := func(m map[string]string, p string) map[string]string { m = maps.Clone(m); delete(m, p); return m }
	describe := func(us []snapshot.User) []string {
		var d []string
		for _, u := range us {
			d = append(d, fmt.Sprintf("%s whole=%v byName=%v outside=%v", u.Path, u.Whole, u.ByName, u.Outside))
		}
		return d
	}
	renamed := snapshottest.Files(map[string]string{"src/a.ts": ts, "src/b.ts": user}, map[string]string{"src/a2.ts": ts, "src/b.ts": user})
	renamed.Changes = []snapshot.Change{{Path: "src/a2.ts", From: "src/a.ts"}}
	cases := []struct {
		label          string
		source         snapshot.Source
		users, touched []string
	}{
		{"a deleted file whose path its importers name", snapshottest.Files(map[string]string{"src/a.ts": ts, "src/b.ts": user},
			map[string]string{"src/b.ts": user}), []string{"src/b.ts whole=[typescript] byName=[] outside=[]"}, nil},
		{"a deleted file of a package that lives on", snapshottest.Files(goPkg(nil), without(goPkg(nil), "internal/a/a.go")),
			[]string{"internal/b/b.go whole=[] byName=[go] outside=[go]"}, nil},
		{"a deleted file with a broken declaration", snapshottest.Files(map[string]string{"src/a.cc": "int run() { return 1 $$ ; }\n",
			"src/a.h": "int run();\n", "src/c.cc": "#include \"a.h\"\nint x = 0;\n"},
			map[string]string{"src/a.h": "int run();\n", "src/c.cc": "#include \"a.h\"\nint x = 0;\n"}),
			[]string{"src/a.h whole=[cpp] byName=[] outside=[]", "src/c.cc whole=[cpp] byName=[] outside=[]"}, nil},
		{"a file renamed under importers that still name its old path", renamed, []string{"src/b.ts whole=[typescript] byName=[] outside=[]"}, nil},
		{"a user whose use stands in its tests alone", snapshottest.Files(map[string]string{"Cargo.toml": "[package]\nname = \"app\"\n",
			"src/lib.rs": "mod a;\nmod b;\n", "src/a.rs": "pub fn run() -> u8 { 1 }\n",
			"src/b.rs": "pub fn other() -> u8 { 1 }\n\n#[cfg(test)]\nmod tests {\n    use crate::a::run;\n    #[test]\n    fn t() { run(); }\n}\n"},
			map[string]string{"Cargo.toml": "[package]\nname = \"app\"\n", "src/lib.rs": "mod a;\nmod b;\n", "src/a.rs": "pub fn run() -> u8 { 2 }\n",
				"src/b.rs": "pub fn other() -> u8 { 1 }\n\n#[cfg(test)]\nmod tests {\n    use crate::a::run;\n    #[test]\n    fn t() { run(); }\n}\n"}),
			[]string{"src/b.rs whole=[] byName=[rust] outside=[]"}, nil},
		{"a user the change touched", snapshottest.Files(map[string]string{"src/a.ts": ts, "src/b.ts": user},
			map[string]string{"src/a.ts": "export function run() { return 2; }\n", "src/b.ts": user + "// seen\n"}),
			nil, []string{"src/b.ts whole=[] byName=[typescript] outside=[typescript]"}},
	}
	for _, c := range cases {
		r := snapshot.Build(c.source, conventions.Default(), snapshottest.Readings(t)).Radius
		if users, touched := describe(r.Users), describe(r.Touched); !slices.Equal(users, c.users) || !slices.Equal(touched, c.touched) {
			t.Errorf("%s:\nusers   %q\ntouched %q\nwant    %q and %q", c.label, users, touched, c.users, c.touched)
		}
	}
}

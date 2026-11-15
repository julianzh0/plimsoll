package snapshot_test

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/juzhxng/plimsoll/internal/conventions"
	"github.com/juzhxng/plimsoll/internal/snapshot"
	"github.com/juzhxng/plimsoll/internal/snapshot/snapshottest"
)

func TestBuildReadsSettingNamesNeverValues(t *testing.T) {
	source := snapshottest.Files(nil, map[string]string{".env.example": "PORT=8080\n# DEBUG=true\nlowercase=x\n\nPORT=9090\n"})
	s := snapshot.Build(source, conventions.Default())
	if s.Now.Settings == nil || !reflect.DeepEqual(s.Now.Settings.Names, []string{"PORT", "DEBUG", "PORT"}) {
		t.Fatalf("Build(.env.example).Now.Settings = %+v, want PORT, DEBUG, PORT", s.Now.Settings)
	}
	if printed := fmt.Sprintf("%+v", s); strings.Contains(printed, "8080") || strings.Contains(printed, "9090") {
		t.Errorf("Build(.env.example) holds a setting's value: %s", printed)
	}
	if got := snapshot.Build(snapshottest.Files(nil, map[string]string{"src/a.ts": "x\n"}), conventions.Default()).Now.Settings; got != nil {
		t.Errorf("Build(no .env.example).Now.Settings = %+v, want nil", got)
	}
	unreadable := snapshottest.Files(nil, map[string]string{".env.example": "PORT=1\n"})
	unreadable.Now[0].Content, unreadable.Now[0].Binary = nil, true
	broken := snapshot.Build(unreadable, conventions.Default())
	if broken.Now.Settings != nil || !slices.Contains(broken.Now.Broken, ".env.example") || !slices.Contains(broken.Unreadable, ".env.example") {
		t.Errorf("Build(unreadable .env.example) = settings %+v, broken %v, unreadable %v; want none, and it named twice", broken.Now.Settings, broken.Now.Broken, broken.Unreadable)
	}
}

func TestBuildReadsRuntimeDependencies(t *testing.T) {
	cases := []struct {
		name   string
		source snapshot.Source
		want   []snapshot.Manifest
	}{
		{"typescript", snapshottest.Load(t, "testdata/typescript"), []snapshot.Manifest{{Path: "package.json", Dependencies: []string{"express", "pg"}}}},
		{"go", snapshottest.Load(t, "testdata/go"), []snapshot.Manifest{{Path: "go.mod", Dependencies: []string{"github.com/a/b"}}}},
		{"mixed", snapshottest.Load(t, "testdata/mixed"), []snapshot.Manifest{{Path: "go.mod", Dependencies: []string{}}, {Path: "package.json", Dependencies: []string{}}}},
		{"unproven", snapshottest.Load(t, "testdata/unproven"), nil},
		{"python", snapshottest.Load(t, "testdata/python"), pyprojectNames("httpx", "pydantic-settings")},
		{"a nested manifest", snapshottest.Files(nil, map[string]string{"web/package.json": `{"dependencies": {"x": "1"}}`}), nil},
		{"a comment in a require block", snapshottest.Files(nil, map[string]string{"go.mod": "module m\n\nrequire (\n\t// pinned\n\tgithub.com/a/b v1.0.0\n)\n"}), []snapshot.Manifest{{Path: "go.mod", Dependencies: []string{"github.com/a/b"}}}},
		{"a one-line require", snapshottest.Files(nil, map[string]string{"go.mod": "module m\n\nrequire github.com/a/b v1.0.0\n"}), []snapshot.Manifest{{Path: "go.mod", Dependencies: []string{"github.com/a/b"}}}},
		{"pyproject", pyproject("[project]\ndependencies = [\"a\", 'b']\n"), pyprojectNames("a", "b")},
		{"pyproject with comments over lines", pyproject("[project] # the package\ndependencies = [\n  \"a\",  # why\n  \"b @ https://x.example/b#egg=b\",\n]\n"), pyprojectNames("a", "b")},
		{"pyproject extras, markers, dots", pyproject("[project]\ndependencies = [\"uvicorn[standard]>=0.34\", \"torch>=2 ; sys_platform == 'linux'\", \"zope.interface\"]\n"),
			pyprojectNames("torch", "uvicorn", "zope-interface")},
		{"pyproject optional dependencies left out", pyproject("[project]\ndependencies = [\"a\"]\n\n[project.optional-dependencies]\ndev = [\"b\"]\n"), pyprojectNames("a")},
		{"pyproject without dependencies", pyproject("[tool.x]\ndependencies = [\"z\"]\n\n[project]\nname = \"x\"\n"), pyprojectNames()},
		{"pyproject normalises names", pyproject("[project]\ndependencies = [\"Pydantic_Settings>=2\", \"httpx>=0.28\"]\n"), pyprojectNames("httpx", "pydantic-settings")},
		{"pyproject for tools only", pyproject("[tool.ruff]\nline-length = 120\n"), nil},
		{"pyproject with inline tables and a long string first", pyproject("[project]\nauthors = [{name = \"a\", email = \"b\"}]\ndescription = \"\"\"\nline\n\"\"\"\ndependencies = [\"httpx\"]\n"), pyprojectNames("httpx")},
		{"pyproject with a byte order mark", pyproject("\uFEFF[project]\ndependencies = [\"a\"]\n"), pyprojectNames("a")},
		{"a nested pyproject", snapshottest.Files(nil, map[string]string{"pkg/pyproject.toml": "[project]\ndependencies = [\"a\"]\n"}), nil},
		{"dart", snapshottest.Load(t, "testdata/dart"), pubspecNames("flutter", "http", "provider")},
		{"pubspec with quoted names and a nested git entry", pubspec("name: 'x'\ndependencies:\n    'a': ^1.0.0\n    b:\n      git:\n        url: https://x.example/b\n    c: any # why\nflutter:\n  assets: []\n"),
			pubspecNames("a", "b", "c")},
		{"pubspec without dependencies", pubspec("name: x\ndev_dependencies:\n  test: ^1.0.0\n"), pubspecNames()},
		{"pubspec without a name", pubspec("dependencies:\n  a: ^1.0.0\n"), nil},
		{"pubspec of a workspace without dependencies", pubspec("name: _ws\nworkspace:\n  - app\n"), nil},
	}
	for _, tc := range cases {
		if got := snapshot.Build(tc.source, conventions.Default()).Now.Manifests; !reflect.DeepEqual(got, tc.want) {
			t.Errorf("Build(%s).Now.Manifests = %#v, want %#v", tc.name, got, tc.want)
		}
	}
}

func TestBuildLeavesAnUnreadableManifestOut(t *testing.T) {
	for _, tc := range []struct{ file, content string }{
		{"package.json", "{not json"},
		{"pyproject.toml", "[project]\nname = \"x\"\ndynamic = [\"dependencies\"]\n"},
		{"pyproject.toml", "[project]\ndependencies = [\n  \"a\",\n"},
		{"pyproject.toml", "[project]\ndependencies = \"a\"\n"},
		{"pubspec.yaml", "name: x\ndependencies: {a: ^1.0.0}\n"},
	} {
		s := snapshot.Build(snapshottest.Files(nil, map[string]string{tc.file: tc.content}), conventions.Default())
		if len(s.Now.Manifests) != 0 || !reflect.DeepEqual(s.Unreadable, []string{tc.file}) {
			t.Errorf("Build(%s %q) = manifests %v, unreadable %v; want none and [%s]", tc.file, tc.content, s.Now.Manifests, s.Unreadable, tc.file)
		}
	}
}

func pyproject(content string) snapshot.Source {
	return snapshottest.Files(nil, map[string]string{"pyproject.toml": content})
}

func pubspec(content string) snapshot.Source {
	return snapshottest.Files(nil, map[string]string{"pubspec.yaml": content})
}

func pubspecNames(names ...string) []snapshot.Manifest {
	return []snapshot.Manifest{{Path: "pubspec.yaml", Dependencies: append([]string{}, names...)}}
}

func pyprojectNames(names ...string) []snapshot.Manifest {
	return []snapshot.Manifest{{Path: "pyproject.toml", Dependencies: append([]string{}, names...)}}
}

func TestBuildSumsTheFootprintByArea(t *testing.T) {
	now := map[string]string{
		"src/a.ts": "a\n", "tests/a.test.ts": "test(\"a\", () => {});\n", "docs/x.md": "x\n", "config.json": "{}\n",
	}
	base := map[string]string{"src/a.ts": "b\n", "src/old.ts": "old\n", "tests/a.test.ts": "x\n", "config.json": "[]\n"}
	changes := []snapshot.Change{
		{Path: "src/a.ts", Added: 120, Removed: 30}, {Path: "src/old.ts", Removed: 50},
		{Path: "tests/a.test.ts", Added: 300, Removed: 10}, {Path: "docs/x.md", Added: 40}, {Path: "config.json", Added: 5},
	}
	source := snapshottest.Files(base, now)
	source.Changes = changes
	source.Since = []string{"e", "d", "c", "b", "a"}
	ratio := 2.5
	want := snapshot.Footprint{
		Commits: 5, Files: 5, Product: snapshot.Lines{Added: 120, Removed: 80}, Test: snapshot.Lines{Added: 300, Removed: 10},
		Docs: snapshot.Lines{Added: 40}, TestPerProduct: &ratio,
	}
	if got := snapshot.Build(source, conventions.Default()).Footprint; !reflect.DeepEqual(got, want) {
		t.Fatalf("Build(changes).Footprint = %+v, want %+v", got, want)
	}
	source.Changes = changes[2:]
	if got := snapshot.Build(source, conventions.Default()).Footprint.TestPerProduct; got != nil {
		t.Errorf("Build(no product change).Footprint.TestPerProduct = %v, want nil", *got)
	}
}

func TestBuildFlagsUsableCommits(t *testing.T) {
	touching := func(n int) []snapshot.FileChange {
		files := make([]snapshot.FileChange, n)
		for i := range files {
			files[i].Path = fmt.Sprintf("src/f%d.ts", i)
		}
		return files
	}
	source := snapshot.Source{
		Since:   []string{"c1"},
		Commits: []snapshot.Commit{{ID: "c1", Files: touching(21)}, {ID: "c2"}, {ID: "c3", Files: touching(20)}},
	}
	history := snapshot.Build(source, conventions.Default()).History
	for i, want := range []struct{ since, usable bool }{{true, false}, {false, false}, {false, true}} {
		if got := history.Commits[i]; got.Since != want.since || got.Usable != want.usable {
			t.Errorf("Build(history).Commits[%d] = since %v, usable %v; want %v, %v", i, got.Since, got.Usable, want.since, want.usable)
		}
	}
	if history.Read != 3 || history.Usable != 1 {
		t.Errorf("Build(history) = read %d, usable %d; want 3, 1", history.Read, history.Usable)
	}
}

func TestBuildGivesEachHistoryPathItsArea(t *testing.T) {
	areas := map[string]snapshot.Area{
		"src/a.ts": snapshot.AreaProduct, "tests/a.test.ts": snapshot.AreaTests, "docs/a.md": snapshot.AreaDocs,
		"README.md": snapshot.AreaDocs, "package.json": snapshot.AreaOther, "scripts/x.ts": snapshot.AreaOther,
		"gone/old.ts":                 snapshot.AreaProduct,
		"docs/superpowers/plans/a.md": snapshot.AreaRecords, "projects/notes.md": snapshot.AreaRecords,
		"docs/qualification/run.json": snapshot.AreaRecords, "evidence/run.txt": snapshot.AreaRecords,
		"src/projects/list.ts": snapshot.AreaProduct,
	}
	for p, want := range areas {
		source := snapshot.Source{Commits: []snapshot.Commit{{ID: "x", Files: []snapshot.FileChange{{Path: p}}}}}
		if got := snapshot.Build(source, conventions.Default()).History.Commits[0].Files[0].Area; got != want {
			t.Errorf("area of %s = %v, want %v", p, got, want)
		}
	}
}

func TestWantsNamesWhatTheReaderReads(t *testing.T) {
	w := snapshot.WantsOf(conventions.Default())
	paths := map[string]bool{
		"src/a.ts": true, "README.md": false, "package.json": true, "web/package.json": false, ".env.example": true,
		"tsconfig.json": true, "web/tsconfig.json": false, "go.mod": true, "web/go.mod": false, "pyproject.toml": true, "web/pyproject.toml": false, "pytest.ini": false, "pytest.toml": false,
		"pubspec.yaml": true, "app/pubspec.yaml": true, "packages/core/pubspec.yaml": true, "node_modules/x/pubspec.yaml": false,
		"docs/.env.example": false, "node_modules/x/a.js": false, "assets/logo.bin": true,
		"docker-compose.yml": true, "deploy/compose.prod.yaml": true, "node_modules/x/compose.yml": false, "compose.txt": false,
	}
	for p, want := range paths {
		if got := w.Path(p); got != want {
			t.Errorf("Wants.Path(%q) = %v, want %v", p, got, want)
		}
	}
	if !w.Binary([]byte{'a', 0}) || w.Binary([]byte("ab")) || w.SniffBytes != 8192 {
		t.Errorf("Wants.Binary or SniffBytes wrong: %v, %v, %d", w.Binary([]byte{'a', 0}), w.Binary([]byte("ab")), w.SniffBytes)
	}
}

func TestTouchesCountUsableCommitsPerPath(t *testing.T) {
	var wide []string
	for i := range 21 {
		wide = append(wide, fmt.Sprintf("src/f%d.ts", i))
	}
	source := snapshot.Source{Since: []string{"c3"}, Commits: []snapshot.Commit{
		snapshottest.Commit("c3", "src/a.ts"), snapshottest.Commit("c2", "src/a.ts", "src/b.ts"),
		snapshottest.Commit("c1", append(wide, "src/a.ts")...), snapshottest.Commit("c0", "src/b.ts"),
	}}
	got := snapshot.Build(source, conventions.Default()).History.Touches()
	want := map[string]snapshot.Touches{"src/a.ts": {All: 2, AtBase: 1}, "src/b.ts": {All: 2, AtBase: 2}}
	if !maps.Equal(got, want) {
		t.Errorf("Touches() = %v, want %v", got, want)
	}
}

package snapshot_test

import (
	"fmt"
	"maps"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/juzhxng/plimsoll/internal/conventions"
	"github.com/juzhxng/plimsoll/internal/snapshot"
	"github.com/juzhxng/plimsoll/internal/snapshot/snapshottest"
)

type fileRow struct {
	path, language string
	role           snapshot.Role
	lines          int
}

func rows(files []snapshot.File) []fileRow {
	var got []fileRow
	for _, file := range files {
		got = append(got, fileRow{file.Path, file.Language, file.Role, file.Lines})
	}
	return got
}

func TestBuildClassifiesEachSampleLanguage(t *testing.T) {
	cases := map[string][]fileRow{
		"typescript": {
			{"scripts/seed.ts", "typescript", snapshot.Other, 3},
			{"src/index.ts", "typescript", snapshot.Product, 4},
			{"src/store.ts", "typescript", snapshot.Product, 6},
			{"src/util.spec.ts", "typescript", snapshot.Test, 5},
			{"tests/fakes/clock.ts", "typescript", snapshot.TestInfrastructure, 5},
			{"tests/store.test.ts", "typescript", snapshot.Test, 7},
		},
		"go": {
			{"cmd/app/main.go", "go", snapshot.Product, 5},
			{"internal/store/helpers_test.go", "go", snapshot.TestInfrastructure, 4},
			{"internal/store/store.go", "go", snapshot.Product, 6},
			{"internal/store/store_test.go", "go", snapshot.Test, 8},
		},
		"python-declared": {
			{"checks/helpers.py", "python", snapshot.TestInfrastructure, 2}, {"checks/test_flow.py", "python", snapshot.Test, 5},
			{"lib/core/__init__.py", "python", snapshot.Product, 1}, {"lib/core/model.py", "python", snapshot.Product, 2},
			{"src/app/__init__.py", "python", snapshot.Product, 1}, {"src/app/service.py", "python", snapshot.Product, 5},
			{"tests/test_service.py", "python", snapshot.Test, 5},
		},
		"python": {
			{"scripts/seed.py", "python", snapshot.Other, 3},
			{"src/app/__init__.py", "python", snapshot.Product, 1},
			{"src/app/config.py", "python", snapshot.Product, 4},
			{"src/app/store.py", "python", snapshot.Product, 6},
			{"tests/conftest.py", "python", snapshot.TestInfrastructure, 3},
			{"tests/fakes/clock.py", "python", snapshot.TestInfrastructure, 2},
			{"tests/test_store.py", "python", snapshot.Test, 5},
			{"typings/extlib/__init__.pyi", "python", snapshot.Product, 2},
		},
		"mixed": {
			{"service/main.go", "go", snapshot.Product, 5},
			{"service/main_test.go", "go", snapshot.Test, 6},
			{"web/src/app.test.ts", "typescript", snapshot.Test, 4},
			{"web/src/app.ts", "typescript", snapshot.Product, 3},
		},
		"dart": {
			{"integration_test/app_test.dart", "dart", snapshot.Test, 8},
			{"lib/config.dart", "dart", snapshot.Product, 4},
			{"lib/main.dart", "dart", snapshot.Product, 4},
			{"lib/notes.dart", "dart", snapshot.Product, 4},
			{"lib/src/app.dart", "dart", snapshot.Product, 8},
			{"lib/src/note.dart", "dart", snapshot.Product, 6},
			{"lib/src/store.dart", "dart", snapshot.Product, 7},
			{"test/fakes/fake_store.dart", "dart", snapshot.TestInfrastructure, 3},
			{"test/store_test.dart", "dart", snapshot.Test, 8},
		},
		"java": {
			{"src/main/java/com/acme/notes/App.java", "java", snapshot.Product, 14},
			{"src/main/java/com/acme/notes/model/Note.java", "java", snapshot.Product, 7},
			{"src/main/java/com/acme/notes/store/NoteStore.java", "java", snapshot.Product, 17},
			{"src/main/java/com/acme/notes/store/package-info.java", "java", snapshot.Product, 2},
			{"src/main/java/module-info.java", "java", snapshot.Product, 3},
			{"src/test/java/com/acme/notes/AppTest.java", "java", snapshot.Test, 13},
			{"src/test/java/com/acme/notes/store/FakeClock.java", "java", snapshot.TestInfrastructure, 7},
			{"src/test/java/com/acme/notes/store/NoteStoreTest.java", "java", snapshot.Test, 11},
		},
		"kotlin": {
			{"build.gradle.kts", "kotlin", snapshot.Product, 15},
			{"settings.gradle.kts", "kotlin", snapshot.Product, 1},
			{"src/androidTest/kotlin/com/acme/tasks/MainScreenTest.kt", "kotlin", snapshot.Test, 11},
			{"src/main/java/com/acme/tasks/legacy/Clock.java", "java", snapshot.Product, 9},
			{"src/main/kotlin/com/acme/tasks/Main.kt", "kotlin", snapshot.Product, 11},
			{"src/main/kotlin/com/acme/tasks/data/Loaders.kt", "kotlin", snapshot.Product, 6},
			{"src/main/kotlin/com/acme/tasks/data/TaskRepository.kt", "kotlin", snapshot.Product, 8},
			{"src/main/kotlin/com/acme/tasks/model/Task.kt", "kotlin", snapshot.Product, 7},
			{"src/test/kotlin/com/acme/tasks/data/TaskRepositoryTest.kt", "kotlin", snapshot.Test, 10},
			{"src/test/kotlin/com/acme/tasks/data/TestTaskDao.kt", "kotlin", snapshot.TestInfrastructure, 11},
		},
		"csharp": {
			{"Acme.Notes.Tests/NoteStoreTests.cs", "csharp", snapshot.Test, 17},
			{"Acme.Notes.Tests/Support/FixedClock.cs", "csharp", snapshot.TestInfrastructure, 9},
			{"Acme.Notes/GlobalUsings.cs", "csharp", snapshot.Product, 1},
			{"Acme.Notes/Model/Note.cs", "csharp", snapshot.Product, 3},
			{"Acme.Notes/Polyfills/IsExternalInit.cs", "csharp", snapshot.Product, 4},
			{"Acme.Notes/Program.cs", "csharp", snapshot.Product, 8},
			{"Acme.Notes/Store/Clock.cs", "csharp", snapshot.Product, 8},
			{"Acme.Notes/Store/NoteStore.cs", "csharp", snapshot.Product, 24},
		},
		"c": {
			{"include/tally.h", "c", snapshot.Product, 7},
			{"src/buffer.c", "c", snapshot.Product, 12},
			{"src/buffer.h", "c", snapshot.Product, 8},
			{"src/tally.c", "c", snapshot.Product, 18},
			{"tests/test_tally.c", "c", snapshot.TestInfrastructure, 9},
		},
		"cpp": {
			{"include/notes/note.h", "cpp", snapshot.Product, 9},
			{"include/notes/store.hpp", "cpp", snapshot.Product, 16},
			{"src/clock.h", "cpp", snapshot.Product, 5},
			{"src/main.cpp", "cpp", snapshot.Product, 10},
			{"src/store.cc", "cpp", snapshot.Product, 26},
			{"tests/store_test.cc", "cpp", snapshot.Test, 10},
			{"tests/support/fake_clock.h", "cpp", snapshot.TestInfrastructure, 5},
		},
		"swift": {
			{"Old/Legacy/Clock.swift", "swift", snapshot.Product, 4},
			{"Package.swift", "swift", snapshot.Product, 19},
			{"Sources/NotesCore/Config.swift", "swift", snapshot.Product, 9},
			{"Sources/NotesCore/Model/Note.swift", "swift", snapshot.Product, 3},
			{"Sources/NotesCore/Store.swift", "swift", snapshot.Product, 11},
			{"Sources/notes/main.swift", "swift", snapshot.Product, 10},
			{"Tests/NotesCoreTests/Fixtures.swift", "swift", snapshot.TestInfrastructure, 3},
			{"Tests/NotesCoreTests/StoreTests.swift", "swift", snapshot.Test, 8},
		},
		"rust": {
			{"build.rs", "rust", snapshot.Product, 3},
			{"crates/notes-store/src/disk.rs", "rust", snapshot.Product, 1},
			{"crates/notes-store/src/lib.rs", "rust", snapshot.Product, 3},
			{"src/cli.rs", "rust", snapshot.Product, 7},
			{"src/config.rs", "rust", snapshot.Product, 10},
			{"src/lib.rs", "rust", snapshot.Product, 14},
			{"src/main.rs", "rust", snapshot.Product, 11},
			{"src/model.rs", "rust", snapshot.Product, 26},
			{"src/store/memory.rs", "rust", snapshot.Product, 27},
			{"src/store/mod.rs", "rust", snapshot.Product, 11},
			{"tests/common/mod.rs", "rust", snapshot.TestInfrastructure, 5},
			{"tests/notes.rs", "rust", snapshot.Test, 9},
		},
		"unproven": {
			{"bin/run.sh", "", snapshot.Product, 4},
			{"tests/check.sh", "", snapshot.TestInfrastructure, 3},
		},
	}
	rules := conventions.Default()
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			s := snapshot.Build(snapshottest.Load(t, "testdata/"+name), rules, snapshottest.Readings(t))
			if got := rows(s.Now.Files); !reflect.DeepEqual(got, want) {
				t.Fatalf("Build(%s).Now.Files = %v, want %v", name, got, want)
			}
			for _, file := range s.Now.Files {
				if file.Limit != rules.Limits.FileLines {
					t.Errorf("Build(%s): %s has limit %+v, want the shared %+v", name, file.Path, file.Limit, rules.Limits.FileLines)
				}
			}
			if !reflect.DeepEqual(s.Before.Files, s.Now.Files) {
				t.Errorf("Build(%s).Before.Files = %v, want the same as now", name, rows(s.Before.Files))
			}
		})
	}
}

func TestBuildLeavesOutWhatIsNotCode(t *testing.T) {
	s := snapshot.Build(snapshottest.Load(t, "testdata/typescript"), conventions.Default(), snapshottest.Readings(t))
	files := map[string]bool{}
	for _, file := range s.Now.Files {
		files[file.Path] = true
	}
	paths := map[string]bool{}
	for _, p := range s.Now.Paths {
		paths[p] = true
	}
	for _, p := range []string{
		"README.md", "package.json", "package-lock.json", ".env.example", "assets/logo.bin", "dist/index.js",
		"node_modules/left-pad/index.js", "src/api.gen.ts",
	} {
		if files[p] {
			t.Errorf("Build(typescript) counts %s as code, want it left out", p)
		}
		if !paths[p] {
			t.Errorf("Build(typescript).Now.Paths lacks %s, want every tracked path", p)
		}
	}
}

func TestBuildNamesExtensionsWithoutAnEntry(t *testing.T) {
	rules := conventions.Default()
	unproven := snapshot.Build(snapshottest.Load(t, "testdata/unproven"), rules, snapshottest.Readings(t))
	if want := []snapshot.Extension{{Suffix: ".sh", Files: 2, Lines: 7}}; !reflect.DeepEqual(unproven.Unverified, want) {
		t.Errorf("Build(unproven).Unverified = %v, want %v", unproven.Unverified, want)
	}
	if got := snapshot.Build(snapshottest.Load(t, "testdata/typescript"), rules, snapshottest.Readings(t)).Unverified; len(got) != 0 {
		t.Errorf("Build(typescript).Unverified = %v, want none", got)
	}
}

func TestBuildCountsLinesOfAwkwardContent(t *testing.T) {
	cases := map[string]int{"a\nb\n": 2, "a\nb": 2, "": 0, "a\r\nb\r\n": 2, "\xff\xfe\n": 1}
	for content, want := range cases {
		s := snapshot.Build(snapshottest.Files(nil, map[string]string{"src/a.ts": content}), conventions.Default(), snapshottest.Readings(t))
		if len(s.Now.Files) != 1 || s.Now.Files[0].Lines != want {
			t.Errorf("Build(%q).Now.Files = %v, want one file of %d lines", content, rows(s.Now.Files), want)
		}
	}
}

func TestBuildCopiesAnUnchangedFileFromNow(t *testing.T) {
	source := snapshot.Source{
		Now: []snapshot.Entry{
			{Path: ".env.example", Content: []byte("PORT=1\n")},
			{Path: "package.json", Content: []byte(`{"dependencies": {"pg": "8"}}`)},
			{Path: "src/a.ts", Content: []byte("a\nb\nc\n")},
		},
		Before: []snapshot.Entry{{Path: ".env.example"}, {Path: "package.json"}, {Path: "src/a.ts"}},
	}
	s := snapshot.Build(source, conventions.Default(), snapshottest.Readings(t))
	if !reflect.DeepEqual(s.Before.Files, s.Now.Files) || len(s.Now.Files) != 1 {
		t.Fatalf("Build(unchanged).Before.Files = %v, want %v", rows(s.Before.Files), rows(s.Now.Files))
	}
	if !reflect.DeepEqual(s.Before.Settings, s.Now.Settings) || !reflect.DeepEqual(s.Before.Manifests, s.Now.Manifests) {
		t.Fatalf("Build(unchanged) base = %+v %+v, want the same settings and manifests as now", s.Before.Settings, s.Before.Manifests)
	}
}

func TestEveryLanguageEntryHasItsSample(t *testing.T) {
	rules := conventions.Default()
	for _, language := range rules.Languages {
		if _, err := os.Stat("testdata/" + language.Name + "/now"); err != nil {
			t.Errorf("language %s has no sample repository: %v", language.Name, err)
			continue
		}
		found := false
		for _, file := range snapshot.Build(snapshottest.Load(t, "testdata/"+language.Name), rules, snapshottest.Readings(t)).Now.Files {
			found = found || file.Language == language.Name
		}
		if !found {
			t.Errorf("the %s sample holds no file of its language", language.Name)
		}
	}
}

func TestBuildLeavesOutGeneratedFiles(t *testing.T) {
	cases := []struct {
		path, content string
		code          bool
	}{
		{"src/a.pb.go", "// Code generated by x. DO NOT EDIT.", false},
		{"src/a.pb.go", "package pb\n// Code generated by x. DO NOT EDIT.", false},
		{"src/Api.cs", "// <auto-generated/>\nnamespace A;\n", false},
		{"src/Info.cs", "//------\n// <auto-generated>\n//     This code was generated by a tool.\n// </auto-generated>\nnamespace A;\n", false},
		{"src/Attributes.cs", "// <autogenerated />\nnamespace A;\n", false},
		{"src/Form1.Designer.cs", "namespace A;\n", false},
		{"src/Views.g.cs", "namespace A;\n", false},
		{"src/Designer.cs", "namespace A;\n", true},
		{"src/Late.cs", "\n\n\n\n\n// <auto-generated/>\n", true},
	}
	for _, tc := range cases {
		s := snapshot.Build(snapshottest.Files(nil, map[string]string{tc.path: tc.content}), conventions.Default(), snapshottest.Readings(t))
		if got := len(s.Now.Files) == 1; got != tc.code {
			t.Errorf("Build(%s %q): a code file = %v, want %v", tc.path, tc.content, got, tc.code)
		}
	}
}

func TestBuildListsPlanningAndReviewFiles(t *testing.T) {
	planning := []string{
		".review-work/a/packet.md", "REQUIREMENTS.md", "docs/SPEC.md", "docs/api-review-packet-2.md", "docs/stories/US-1.md",
		"packages/api/docs/specs/x.md", "projects/notes.txt",
	}
	other := []string{
		"docs/guide.md", "docs/stories.md", "mydocs/stories/a.md", "docs/qualification/run.json", "evidence/a.txt",
		"node_modules/x/SPEC.md", "internal/x/testdata/s/REQUIREMENTS.md", "src/spec.md", "src/features/projects/list.md",
	}
	files := map[string]string{}
	for _, p := range append(slices.Clone(planning), other...) {
		files[p] = "x\n"
	}
	s := snapshot.Build(snapshottest.Files(nil, files), conventions.Default(), snapshottest.Readings(t))
	if !slices.Equal(s.Now.Planning, planning) {
		t.Errorf("Build.Now.Planning = %q, want %q", s.Now.Planning, planning)
	}
	if s.PlanningRule != conventions.Default().PlanningRule() {
		t.Errorf("Build.PlanningRule = %q, want the rules' own", s.PlanningRule)
	}
}

func TestBuildFlagsCodeFilesUnderRecordPaths(t *testing.T) {
	records := []string{"projects/tools/gen.ts", "docs/qualification/run.ts", "evidence/collect.sh", "x/docs/plans/draw.ts"}
	others := []string{"src/a.ts", "docs/guide/sample.ts", "scripts/evidence.ts", "src/evidence/check.ts", "src/projects/list.ts"}
	files := map[string]string{}
	for _, p := range append(slices.Clone(records), others...) {
		files[p] = "x\n"
	}
	for _, file := range snapshot.Build(snapshottest.Files(nil, files), conventions.Default(), snapshottest.Readings(t)).Now.Files {
		if want := slices.Contains(records, file.Path); file.Record != want {
			t.Errorf("Build: %s Record = %v, want %v", file.Path, file.Record, want)
		}
	}
}

func TestBuildCountsTestCodeInsideProductFiles(t *testing.T) {
	testLines := func(source snapshot.Source) map[string]int {
		found := map[string]int{}
		for _, file := range snapshot.Build(source, conventions.Default(), snapshottest.Readings(t)).Now.Files {
			if file.TestLines > 0 {
				found[file.Path] = file.TestLines
			}
		}
		return found
	}
	if got, want := testLines(snapshottest.Load(t, "testdata/rust")), map[string]int{"src/model.rs": 15, "src/store/memory.rs": 12}; !maps.Equal(got, want) {
		t.Errorf("the sample: test lines inside product files = %v, want %v", got, want)
	}
	for _, tc := range []struct {
		name, text string
		want       int
	}{
		{"a module with a string that holds a brace", "fn a() {}\n\n#[cfg(test)]\nmod tests {\n    const T: &str = \"\n}\n\";\n    #[test]\n    fn t() { assert!('}' != '{'); }\n}\nfn b() {}\n", 8},
		{"single statements", "#[cfg(test)]\nuse crate::testutil;\n#[cfg(test)]\nmod testutil;\nfn a() {}\n", 4},
		{"a field", "struct S {\n    #[cfg(test)]\n    extra: u8,\n    other: u8,\n}\n", 0},
		{"a function", "fn a() {}\n#[cfg(test)]\n#[test]\nfn t() {\n    assert!(true);\n}\n", 5},
		{"other conditions", "#[cfg(not(test))]\nfn a() {}\n#[cfg(all(test, unix))]\nfn b() {}\n", 0},
		{"test code inside test code", "#[cfg(test)]\nmod tests {\n    #[cfg(test)]\n    fn helper() {}\n}\n", 5},
		{"a block that does not close", "fn a() {}\n#[cfg(test)]\nmod tests {\n    fn t() {}\n", 3},
		{"two on one line", "#[cfg(test)] use a::b; #[cfg(test)] use c::d;\nfn a() {}\n", 1},
		{"a comment that names it", "// #[cfg(test)]\n/* #[cfg(test)] mod x { */\nfn a() {}\n", 0},
		{"a raw string that holds it", "const S: &str = r#\"\n#[cfg(test)]\nmod tests {\n}\n\"#;\n", 0},
	} {
		for _, text := range []string{tc.text, strings.ReplaceAll(tc.text, "\n", "\r\n")} {
			if got := testLines(snapshottest.Files(nil, map[string]string{"src/a.rs": text}))["src/a.rs"]; got != tc.want {
				t.Errorf("%s: test lines of %q = %d, want %d", tc.name, text, got, tc.want)
			}
		}
	}
	block := "#[cfg(test)]\nmod tests {\n}\n"
	if got := testLines(snapshottest.Files(nil, map[string]string{"tests/a.rs": block, "scripts/a.rs": block, "src/a.ts": block})); len(got) != 0 {
		t.Errorf("test code by its path, other code and another language: test lines %v, want none", got)
	}
}

func TestBuildGivesASharedSuffixToTheEntryWithMoreFilesOfItsOwn(t *testing.T) {
	// Two entries that both list .both: first also .one, second also .two and .too.
	rules := conventions.Default()
	rules.Languages = append(rules.Languages,
		conventions.Language{Name: "first", Extensions: []string{".one", ".both"}},
		conventions.Language{Name: "second", Extensions: []string{".two", ".too", ".both"}})
	cases := []struct {
		name  string
		files []string
		want  string // the language of a.both
	}{
		{"only the first has files of its own", []string{"a.both", "a.one", "b.one"}, "first"},
		{"only the second has", []string{"a.both", "a.two"}, "second"},
		{"each suffix of its own counts", []string{"a.both", "a.one", "a.two", "b.too"}, "second"},
		{"more of the first", []string{"a.both", "a.one", "b.one", "a.two"}, "first"},
		{"as many of each: the entry listed first", []string{"a.both", "a.one", "a.two"}, "first"},
		{"shared files alone", []string{"a.both", "b.both"}, "first"},
		{"files in a skipped folder count for nothing", []string{"a.both", "a.one", "vendor/x.two", "vendor/y.two"}, "first"},
	}
	for _, tc := range cases {
		files := map[string]string{}
		for _, p := range tc.files {
			files[p] = "x\n"
		}
		s := snapshot.Build(snapshottest.Files(files, files), rules, snapshottest.Readings(t))
		for _, side := range []snapshot.Tree{s.Now, s.Before} {
			for _, file := range side.Files {
				if file.Path == "a.both" && file.Language != tc.want {
					t.Errorf("%s: the language of a.both = %q, want %q", tc.name, file.Language, tc.want)
				}
			}
		}
	}
	// The other files keep their entries, and the rules a caller passes are not altered.
	mixed := snapshot.Build(snapshottest.Files(nil, map[string]string{"a.both": "x\n", "a.two": "x\n", "b.one": "x\n", "c.go": "package c\n", "d.two": "x\n"}), rules, snapshottest.Readings(t))
	want := []fileRow{{"a.both", "second", snapshot.Product, 1}, {"a.two", "second", snapshot.Product, 1}, {"b.one", "first", snapshot.Product, 1},
		{"c.go", "go", snapshot.Product, 1}, {"d.two", "second", snapshot.Product, 1}}
	if got := rows(mixed.Now.Files); !reflect.DeepEqual(got, want) {
		t.Errorf("Build(mixed).Now.Files = %v, want %v", got, want)
	}
	if first, _ := rules.LanguageOf("a.both"); first.Name != "first" {
		t.Errorf("the rules passed to Build name %q for a.both afterwards, want them unaltered", first.Name)
	}
}

func TestBuildListsAFileTheParserCannotRead(t *testing.T) {
	var big strings.Builder
	big.WriteString("package main\n")
	for i := range 150_000 { // measured: the Go grammar needs more than its 128 MiB for this shape from 100,000 lines
		fmt.Fprintf(&big, "var v%d = %d\n", i, i)
	}
	s := snapshot.Build(snapshottest.Files(nil, map[string]string{
		"go.mod":         "module example.com/m\n",
		"store/store.go": "package store\n",
		"main.go":        "package main\n\nimport \"example.com/m/store\"\n",
		"big.go":         big.String(),
	}), conventions.Default(), snapshottest.Readings(t))
	if !slices.Equal(s.Unparsed, []string{"big.go"}) {
		t.Errorf("Build(a file over the cap).Unparsed = %v, want [big.go]", s.Unparsed)
	}
	if !slices.ContainsFunc(s.Now.Links.List, func(l snapshot.Link) bool { return l.From == "main.go" && l.To == "store" }) {
		t.Errorf("Build(a file over the cap) links = %+v, want main.go -> store", s.Now.Links.List)
	}
	if i := slices.IndexFunc(s.Now.Files, func(f snapshot.File) bool { return f.Path == "big.go" }); i < 0 || s.Now.Files[i].Lines != 150_001 {
		t.Errorf("Build(a file over the cap) files = %d, want big.go counted with 150,001 lines", len(s.Now.Files))
	}
}

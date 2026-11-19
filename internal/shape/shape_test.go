package shape

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

// detect runs the detector and checks what every mark must carry: an identifier, a change, a subject, a unit,
// the source of its limit, a way to verify it, and exact confidence.
func detect(t *testing.T, source snapshot.Source, rules conventions.Rules) answer.Findings {
	t.Helper()
	findings := Detector{}.Detect(snapshot.Build(source, rules, snapshottest.Readings(t)))
	for _, m := range findings.Marks {
		if m.ID == "" || m.Change == "" || m.Subject.Kind == "" || m.Measured.Unit == "" || m.Measured.LimitSource == "" ||
			m.Hint == "" || m.Confidence != wantedConfidence(m) {
			t.Errorf("mark %+v lacks a field it must carry", m)
		}
	}
	return findings
}

func statistic(findings answer.Findings, name, subject string) (answer.Statistic, bool) {
	for _, s := range findings.Statistics {
		if s.Name == name && s.Subject == subject {
			return s, true
		}
	}
	return answer.Statistic{}, false
}

func value(t *testing.T, findings answer.Findings, name, subject string) float64 {
	t.Helper()
	s, found := statistic(findings, name, subject)
	if !found || s.Value == nil {
		t.Fatalf("statistic %q %q is missing in %+v", name, subject, findings.Statistics)
	}
	return *s.Value
}

func marksOf(findings answer.Findings, rule string) []answer.Mark {
	var found []answer.Mark
	for _, m := range findings.Marks {
		if strings.HasPrefix(m.ID, "shape."+rule+":") {
			found = append(found, m)
		}
	}
	return found
}

func TestDetectOnEachLanguageSample(t *testing.T) {
	cases := []struct {
		name                 string
		product, test, other float64
		ratio                float64
		largest              []string
		dependencies         map[string]string
		notAnalysed          []string
	}{
		{"typescript", 10, 17, 3, 1.7, []string{"tests/store.test.ts", "src/store.ts", "src/util.spec.ts"},
			map[string]string{"package.json": "express, pg"}, nil},
		{"go", 11, 12, 0, 1.1, []string{"internal/store/store_test.go", "internal/store/store.go", "cmd/app/main.go"},
			map[string]string{"go.mod": "github.com/a/b"}, []string{"no `.env.example`, so no settings were counted or compared"}},
		{"python", 13, 10, 3, 0.8, []string{"src/app/store.py", "tests/test_store.py", "src/app/config.py"},
			map[string]string{"pyproject.toml": "httpx, pydantic-settings"}, nil},
		{"dart", 33, 19, 0, 0.6, []string{"integration_test/app_test.dart", "lib/src/app.dart", "test/store_test.dart"}, map[string]string{"pubspec.yaml": "flutter, http, provider"}, nil},
		{"java", 43, 31, 0, 0.7, []string{"src/main/java/com/acme/notes/store/NoteStore.java", "src/main/java/com/acme/notes/App.java", "src/test/java/com/acme/notes/AppTest.java"},
			map[string]string{"pom.xml": "com.fasterxml.jackson.core:jackson-databind, org.postgresql:postgresql"}, []string{"no `.env.example`, so no settings were counted or compared"}},
		{"kotlin", 57, 32, 0, 0.6, []string{"build.gradle.kts", "src/androidTest/kotlin/com/acme/tasks/MainScreenTest.kt", "src/main/kotlin/com/acme/tasks/Main.kt"},
			map[string]string{"build.gradle.kts": ":shared, io.ktor:ktor-bom, io.ktor:ktor-client-core, libs.kotlinx.coroutines.core, org.slf4j:slf4j-simple"},
			[]string{"no `.env.example`, so no settings were counted or compared"}},
		{"csharp", 48, 26, 0, 0.5, []string{"Acme.Notes/Store/NoteStore.cs", "Acme.Notes.Tests/NoteStoreTests.cs", "Acme.Notes.Tests/Support/FixedClock.cs"},
			map[string]string{"Acme.Notes/Acme.Notes.csproj": "Microsoft.Extensions.Configuration, Npgsql, System.Text.Json"},
			[]string{"no `.env.example`, so no settings were counted or compared"}},
		{"c", 45, 9, 0, 0.2, []string{"src/tally.c", "src/buffer.c", "tests/test_tally.c"}, nil, []string{
			"no `.env.example`, so no settings were counted or compared", "no known dependency manifest, at the root or as a project file outside test and other code, so no dependency count",
		}},
		{"cpp", 66, 15, 0, 0.2, []string{"src/store.cc", "include/notes/store.hpp", "src/main.cpp"},
			map[string]string{"vcpkg.json": "fmt, sqlite3"}, []string{"no `.env.example`, so no settings were counted or compared"}},
		{"swift", 56, 11, 0, 0.2, []string{"Package.swift", "Sources/NotesCore/Store.swift", "Sources/notes/main.swift"},
			map[string]string{"Package.swift": "swift-argument-parser, swift-log"}, []string{"no `.env.example`, so no settings were counted or compared"}},
		{"rust", 86, 41, 0, 0.5, []string{"src/store/memory.rs", "src/model.rs", "src/lib.rs"},
			map[string]string{"Cargo.toml": "anyhow, clap, libc, log, notes-store, serde"}, []string{"no `.env.example`, so no settings were counted or compared"}},
		{"mixed", 8, 10, 0, 1.3, []string{"service/main_test.go", "service/main.go", "web/src/app.test.ts"},
			map[string]string{"go.mod": "", "package.json": ""}, []string{"no `.env.example`, so no settings were counted or compared"}},
		{"python-declared", 9, 12, 0, 1.3, []string{"checks/test_flow.py", "src/app/service.py", "tests/test_service.py"},
			map[string]string{"pyproject.toml": ""}, []string{"no `.env.example`, so no settings were counted or compared"}},
		{"unproven", 4, 3, 0, 0.8, []string{"bin/run.sh", "tests/check.sh"}, nil, []string{
			"no `.env.example`, so no settings were counted or compared", "no known dependency manifest, at the root or as a project file outside test and other code, so no dependency count",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := snapshottest.Load(t, "../snapshot/testdata/"+tc.name)
			findings := detect(t, source, conventions.Default())
			// A manifest new since the base names its dependencies; an unchanged one only counts them.
			added := source
			added.Before = slices.DeleteFunc(slices.Clone(source.Before), func(e snapshot.Entry) bool { _, named := tc.dependencies[e.Path]; return named })
			withNames := detect(t, added, conventions.Default())
			got := []float64{value(t, findings, "product lines", ""), value(t, findings, "test lines", ""), value(t, findings, "other code lines", ""), value(t, findings, "test ratio", "")}
			if want := []float64{tc.product, tc.test, tc.other, tc.ratio}; !slices.Equal(got, want) {
				t.Errorf("Detect(%s) product, test, other, ratio = %v, want %v", tc.name, got, want)
			}
			var largest []string
			for _, s := range findings.Statistics {
				if s.Name == "largest code file" {
					largest = append(largest, s.Subject)
				}
			}
			if !slices.Equal(largest, tc.largest) {
				t.Errorf("Detect(%s) largest code files = %v, want %v", tc.name, largest, tc.largest)
			}
			for manifest, names := range tc.dependencies {
				if s, found := statistic(withNames, "runtime dependencies", manifest); !found || s.Note != names {
					t.Errorf("Detect(%s, %s new) dependencies = %+v, want names %q", tc.name, manifest, s, names)
				}
				if s, found := statistic(findings, "runtime dependencies", manifest); !found || s.Note != "" {
					t.Errorf("Detect(%s, %s unchanged) dependencies = %+v, want the count without names", tc.name, manifest, s)
				}
			}
			for _, want := range append(tc.notAnalysed, "no commit in the history, so no churn") {
				if !slices.Contains(findings.NotAnalysed, want) {
					t.Errorf("Detect(%s).NotAnalysed = %q, want it to hold %q", tc.name, findings.NotAnalysed, want)
				}
			}
			if _, found := statistic(findings, "churn: product", ""); found || len(findings.Marks) != 0 {
				t.Errorf("Detect(%s) = churn row %v, marks %v; want neither", tc.name, found, findings.Marks)
			}
			total := 0
			for _, file := range snapshot.Build(source, conventions.Default(), snapshottest.Readings(t)).Now.Files {
				total += file.Lines
			}
			if sum := tc.product + tc.test + tc.other; sum != float64(total) {
				t.Errorf("Detect(%s): product + test + other = %v, want all %d code lines", tc.name, sum, total)
			}
			declaredNote := "test folders declared in `pyproject.toml` (`[tool.pytest.ini_options]` `testpaths`): `tests/`, `checks/`"
			if got := len(findings.Notes) > 1 && findings.Notes[1] == declaredNote; got != (tc.name == "python-declared") {
				t.Errorf("Detect(%s).Notes = %q; the declared note second: %v", tc.name, findings.Notes, got)
			}
		})
	}
}

// declaredRatioSource declares checks/ a test folder now only; one rule then classifies both sides.
func declaredRatioSource() snapshot.Source {
	base := map[string]string{"src/a.ts": snapshottest.Lines(10), "checks/h.ts": snapshottest.Lines(30)}
	now := map[string]string{"src/a.ts": snapshottest.Lines(10), "checks/h.ts": snapshottest.Lines(30), "pyproject.toml": "[tool.pytest.ini_options]\ntestpaths = [\"checks\"]\n"}
	return snapshottest.Files(base, now)
}

func ratioSource(testBefore, testNow int) snapshot.Source {
	return snapshottest.Files(
		map[string]string{"src/a.ts": snapshottest.Lines(10), "tests/a.ts": snapshottest.Lines(testBefore)},
		map[string]string{"src/a.ts": snapshottest.Lines(10), "tests/a.ts": snapshottest.Lines(testNow)},
	)
}

func TestRatioMarks(t *testing.T) {
	cases := []struct {
		before, now int
		want        answer.Change
	}{{19, 21, answer.New}, {21, 22, answer.Grown}, {21, 21, answer.Standing}, {23, 19, answer.Resolved}, {20, 20, ""}, {37, 38, answer.Grown}}
	for _, tc := range cases {
		marks := marksOf(detect(t, ratioSource(tc.before, tc.now), conventions.Default()), "test-ratio")
		if tc.want == "" {
			if len(marks) != 0 {
				t.Errorf("ratio %d -> %d: marks %+v, want none", tc.before, tc.now, marks)
			}
			continue
		}
		if len(marks) != 1 || marks[0].Change != tc.want || marks[0].Measured.Value != float64(tc.now)/10 ||
			*marks[0].Measured.Before != float64(tc.before)/10 || *marks[0].Measured.Limit != 2.0 {
			t.Errorf("ratio %d -> %d: marks %+v, want one %s mark", tc.before, tc.now, marks, tc.want)
		}
	}
	if marks := marksOf(detect(t, declaredRatioSource(), conventions.Default()), "test-ratio"); len(marks) != 1 || marks[0].Change != answer.Standing ||
		marks[0].Measured.Value != 3.0 || *marks[0].Measured.Before != 3.0 {
		t.Errorf("a folder declared now only: marks %+v, want one standing ratio of 3.0 on both sides", marks)
	}
	ownBase := map[string]string{"src/a.ts": snapshottest.Lines(10), "checks/h.ts": snapshottest.Lines(30)}
	ownNow := map[string]string{"src/a.ts": snapshottest.Lines(10), "checks/h.ts": snapshottest.Lines(30), ".plimsoll.toml": "[layout]\ntest_folders = [\"checks\"]\n"}
	ownFound := detect(t, snapshottest.Files(ownBase, ownNow), conventions.Default())
	if marks := marksOf(ownFound, "test-ratio"); len(marks) != 1 || marks[0].Change != answer.Standing || marks[0].Measured.Value != 3.0 || *marks[0].Measured.Before != 3.0 ||
		!slices.Contains(ownFound.Notes, "test folders declared in `.plimsoll.toml` (`[layout]` `test_folders`): `checks/`") {
		t.Errorf("a folder the own file declares now only: marks %+v, notes %q; want one standing ratio of 3.0 and the folder named", marks, ownFound.Notes)
	}
	ownHeld := snapshottest.Files(nil, map[string]string{".plimsoll.toml": "[layout]\ntest_folders = [\"src\"]\n", "src/a.ts": "x\n"})
	if f := detect(t, ownHeld, conventions.Default()); !slices.Contains(f.NotAnalysed, "`.plimsoll.toml` `test_folders` names `src/`, which is or holds a package root; only the shared rules apply inside it") {
		t.Errorf("a held folder of the own file: NotAnalysed = %q, want the held sentence", f.NotAnalysed)
	}
	held := snapshottest.Files(nil, map[string]string{"pyproject.toml": "[tool.pytest.ini_options]\ntestpaths = [\"src\"]\n", "src/a.ts": "x\n"})
	if f := detect(t, held, conventions.Default()); len(f.NotAnalysed) == 0 ||
		f.NotAnalysed[0] != "`pyproject.toml` `testpaths` names `src/`, which is or holds a package root; only the shared rules apply inside it" {
		t.Errorf("a held test folder: NotAnalysed = %q, want the held sentence first", f.NotAnalysed)
	}
}

func TestFileSizeMarks(t *testing.T) {
	cases := []struct {
		name      string
		base, now map[string]string
		changes   []snapshot.Change
		want      answer.Change
		path      string
		value     float64
	}{
		{"crossed", map[string]string{"src/a.ts": snapshottest.Lines(980)}, map[string]string{"src/a.ts": snapshottest.Lines(1120)}, nil, answer.New, "src/a.ts", 1120},
		{"new and over", nil, map[string]string{"src/a.ts": snapshottest.Lines(1100)}, nil, answer.New, "src/a.ts", 1100},
		{"grew", map[string]string{"src/a.ts": snapshottest.Lines(1100)}, map[string]string{"src/a.ts": snapshottest.Lines(1101)}, nil, answer.Grown, "src/a.ts", 1101},
		{"unchanged", map[string]string{"src/a.ts": snapshottest.Lines(1100)}, map[string]string{"src/a.ts": snapshottest.Lines(1100)}, nil, answer.Standing, "src/a.ts", 1100},
		{"shrunk", map[string]string{"src/a.ts": snapshottest.Lines(1100)}, map[string]string{"src/a.ts": snapshottest.Lines(1050)}, nil, answer.Standing, "src/a.ts", 1050},
		{"under again", map[string]string{"src/a.ts": snapshottest.Lines(1100)}, map[string]string{"src/a.ts": snapshottest.Lines(900)}, nil, answer.Resolved, "src/a.ts", 900},
		{"deleted", map[string]string{"src/a.ts": snapshottest.Lines(1100)}, map[string]string{}, nil, answer.Resolved, "src/a.ts", 0},
		{"moved", map[string]string{"src/old.ts": snapshottest.Lines(1100)}, map[string]string{"src/new.ts": snapshottest.Lines(1100)},
			[]snapshot.Change{{Path: "src/new.ts", From: "src/old.ts"}}, answer.Standing, "src/new.ts", 1100},
		{"exactly the limit", map[string]string{"src/a.ts": snapshottest.Lines(1000)}, map[string]string{"src/a.ts": snapshottest.Lines(1000)}, nil, "", "", 0},
		{"a file in a language plimsoll does not read", map[string]string{"src/resources/flow.groovy": snapshottest.Lines(980)}, map[string]string{"src/resources/flow.groovy": snapshottest.Lines(1120)}, nil, "", "", 0},
		{"a test file", map[string]string{"tests/a.test.ts": snapshottest.Lines(980)}, map[string]string{"tests/a.test.ts": snapshottest.Lines(1120)}, nil, answer.New, "tests/a.test.ts", 1120},
	}
	for _, tc := range cases {
		source := snapshottest.Files(tc.base, tc.now)
		if tc.changes != nil {
			source.Changes = tc.changes
		}
		marks := marksOf(detect(t, source, conventions.Default()), "file-size")
		if tc.want == "" {
			if len(marks) != 0 {
				t.Errorf("%s: marks %+v, want none", tc.name, marks)
			}
			continue
		}
		if len(marks) != 1 || marks[0].Change != tc.want || marks[0].Subject.Paths[0] != tc.path || marks[0].Measured.Value != tc.value {
			t.Errorf("%s: marks %+v, want one %s mark on %s with value %v", tc.name, marks, tc.want, tc.path, tc.value)
		}
		if tc.name == "new and over" && marks[0].Measured.Before != nil {
			t.Errorf("%s: before = %v, want none", tc.name, *marks[0].Measured.Before)
		}
	}
}

func TestAdditionMarks(t *testing.T) {
	manifest := func(names ...string) string {
		var entries []string
		for _, name := range names {
			entries = append(entries, fmt.Sprintf("%q: \"1\"", name))
		}
		return `{"dependencies": {` + strings.Join(entries, ", ") + `}}`
	}
	findings := detect(t, snapshottest.Files(
		map[string]string{"package.json": manifest("a", "b"), ".env.example": "ALPHA=1\n"},
		map[string]string{"package.json": manifest("a", "c"), ".env.example": "ALPHA=1\nBETA=2\nGAMMA=3\n"},
	), conventions.Default())
	dependencies, settings := marksOf(findings, "dependencies"), marksOf(findings, "settings")
	if len(dependencies) != 1 || !slices.Equal(dependencies[0].Subject.Names, []string{"c"}) || dependencies[0].Measured.Value != 2 || *dependencies[0].Measured.Before != 2 || dependencies[0].Change != answer.New ||
		dependencies[0].Hint != "compare the runtime dependencies in `package.json` at the base and now. Gone since the base: `b`." {
		t.Errorf("dependency marks = %+v, want one new mark naming c, 2 now and 2 before, b gone", dependencies)
	}
	if len(settings) != 1 || !slices.Equal(settings[0].Subject.Names, []string{"BETA", "GAMMA"}) || settings[0].Measured.Value != 3 || *settings[0].Measured.Before != 1 || settings[0].Change != answer.New {
		t.Errorf("settings marks = %+v, want one new mark naming BETA and GAMMA, 3 now and 1 before", settings)
	}
	if hint := dependencies[0].Hint; strings.Contains(hint, "lockfile") {
		t.Errorf("no lockfile tracked: hint %q, want no lockfile remark", hint)
	}
	for _, tc := range []struct {
		name, lock, want string
	}{
		{"a lockfile without the new name", `{"packages": {"node_modules/a": {}}}`, " Not in the lockfile `package-lock.json`: `c`."},
		{"a lockfile that holds it", `{"packages": {"node_modules/a": {}, "node_modules/c": {}}}`, ""},
		{"a lockfile that holds a longer name only", `{"packages": {"node_modules/a": {}, "node_modules/cc": {}}}`, " Not in the lockfile `package-lock.json`: `c`."},
	} {
		locked := detect(t, snapshottest.Files(
			map[string]string{"package.json": manifest("a", "b"), "package-lock.json": tc.lock},
			map[string]string{"package.json": manifest("a", "c"), "package-lock.json": tc.lock},
		), conventions.Default())
		marks := marksOf(locked, "dependencies")
		if len(marks) != 1 || !strings.HasSuffix(marks[0].Hint, "at the base and now. Gone since the base: `b`."+tc.want) {
			t.Errorf("%s: dependency marks %+v, want the hint to end with %q", tc.name, marks, tc.want)
		}
	}
	same := map[string]string{"package.json": manifest("a"), ".env.example": "ALPHA=1\n"}
	if marks := detect(t, snapshottest.Files(same, same), conventions.Default()).Marks; len(marks) != 0 {
		t.Errorf("nothing added: marks %+v, want none", marks)
	}
	broken := detect(t, snapshottest.Files(map[string]string{"package.json": `{"dependencies": {"a": "1",}}`}, map[string]string{"package.json": manifest("a")}), conventions.Default())
	if marks := marksOf(broken, "dependencies"); len(marks) != 0 || !slices.Contains(broken.NotAnalysed, "`package.json` could not be read at the base, so no dependencies were compared") {
		t.Errorf("a manifest broken at the base: marks %+v, not analysed %q; want no mark and the entry", marks, broken.NotAnalysed)
	}
	unreadableNow := snapshottest.Files(nil, map[string]string{".env.example": "ALPHA=1\n"})
	unreadableNow.Now[0].Content, unreadableNow.Now[0].Binary = nil, true
	if f := detect(t, unreadableNow, conventions.Default()); !slices.Contains(f.NotAnalysed, "`.env.example` could not be read, so no settings count") ||
		slices.Contains(f.NotAnalysed, "no `.env.example`, so no settings were counted or compared") {
		t.Errorf("an unreadable .env.example: not analysed %q, want it named as not read", f.NotAnalysed)
	} else if _, found := statistic(f, "settings", ""); found {
		t.Errorf("an unreadable .env.example: a settings row exists, want none")
	}
	unreadableBase := snapshottest.Files(map[string]string{".env.example": "ALPHA=1\n"}, map[string]string{".env.example": "ALPHA=1\nBETA=2\n"})
	unreadableBase.Before[0].Content, unreadableBase.Before[0].Binary = nil, true
	f := detect(t, unreadableBase, conventions.Default())
	row, _ := statistic(f, "settings", "")
	if marks := marksOf(f, "settings"); len(marks) != 0 || row.Value == nil || *row.Value != 2 || row.Before != nil ||
		!slices.Contains(f.NotAnalysed, "`.env.example` could not be read at the base, so no settings were compared") {
		t.Errorf("unreadable at the base: marks %v, row %+v, not analysed %q", marks, row, f.NotAnalysed)
	}
	added := marksOf(detect(t, snapshottest.Files(nil, map[string]string{"package.json": manifest("x", "y")}), conventions.Default()), "dependencies")
	if len(added) != 1 || !slices.Equal(added[0].Subject.Names, []string{"x", "y"}) || added[0].Measured.Before != nil {
		t.Errorf("a new manifest: marks %+v, want one naming all its dependencies and no value before", added)
	}
}

func TestARepositorysOwnLineLimitAppliesToEveryFile(t *testing.T) {
	files := map[string]string{"src/a.go": snapshottest.Lines(900), "src/a.ts": snapshottest.Lines(900)}
	if marks := marksOf(detect(t, snapshottest.Files(nil, files), conventions.Default()), "file-size"); len(marks) != 0 {
		t.Errorf("default rules: marks %+v, want none", marks)
	}
	files[".plimsoll.toml"] = "[limits]\nfile_lines = 800\n"
	marks := marksOf(detect(t, snapshottest.Files(nil, files), conventions.Default()), "file-size")
	if len(marks) != 2 || *marks[0].Measured.Limit != 800 || marks[0].Measured.LimitSource != "set by `.plimsoll.toml`" {
		t.Errorf("with .plimsoll.toml: marks %+v, want two with limit 800 set by `.plimsoll.toml`", marks)
	}
}

func TestSplitShowsWhenTheRatioIsOverItsLimit(t *testing.T) {
	split := func(files map[string]string) answer.Findings {
		files["src/a.ts"] = snapshottest.Lines(10)
		return detect(t, snapshottest.Files(files, files), conventions.Default())
	}
	defining := "test(\"x\", () => {});\n" + snapshottest.Lines(9)
	withInfrastructure := split(map[string]string{
		"tests/a.test.ts": defining, "tests/support/a.ts": snapshottest.Lines(3), "tests/support/b.ts": snapshottest.Lines(4),
		"tests/support/c.ts": snapshottest.Lines(2), "tests/support/d.ts": snapshottest.Lines(5), "tests/support/e.ts": snapshottest.Lines(1),
	})
	var largest []string
	for _, s := range withInfrastructure.Statistics {
		if s.Name == "largest test infrastructure file" {
			largest = append(largest, s.Subject)
		}
	}
	if value(t, withInfrastructure, "tests", "") != 10 || value(t, withInfrastructure, "test infrastructure", "") != 15 ||
		!slices.Equal(largest, []string{"tests/support/d.ts", "tests/support/b.ts", "tests/support/a.ts"}) {
		t.Errorf("ratio 2.5 with infrastructure: statistics %+v", withInfrastructure.Statistics)
	}
	onlyTests := split(map[string]string{"tests/a.test.ts": defining, "tests/b.test.ts": defining + snapshottest.Lines(5)})
	if _, found := statistic(onlyTests, "test infrastructure", ""); found || value(t, onlyTests, "tests", "") != 25 {
		t.Errorf("ratio 2.5 without infrastructure: statistics %+v, want a tests row only", onlyTests.Statistics)
	}
	under := split(map[string]string{"tests/a.test.ts": defining + snapshottest.Lines(5)})
	if _, found := statistic(under, "tests", ""); found {
		t.Errorf("ratio 1.5: statistics %+v, want no split", under.Statistics)
	}
}

func TestUnchangedOverLimitsGivesOnlyStanding(t *testing.T) {
	files := map[string]string{"src/a.ts": snapshottest.Lines(1600), "tests/a.ts": snapshottest.Lines(1000), "tests/b.ts": snapshottest.Lines(1000), "tests/c.ts": snapshottest.Lines(1000), "tests/d.ts": snapshottest.Lines(1000)}
	marks := detect(t, snapshottest.Files(files, files), conventions.Default()).Marks
	if len(marks) != 2 || marks[0].Change != answer.Standing || marks[1].Change != answer.Standing {
		t.Errorf("unchanged over the limits: marks %+v, want two standing", marks)
	}
}

func TestChurnCoversTheLast150Commits(t *testing.T) {
	commits := func(n, since int) snapshot.Source {
		source := snapshot.Source{}
		for i := range n {
			id := fmt.Sprintf("c%03d", i+1)
			source.Commits = append(source.Commits, snapshot.Commit{ID: id, Files: []snapshot.FileChange{{Path: "src/a.ts", Added: i + 1}}})
			if i < since {
				source.Since = append(source.Since, id)
			}
		}
		return source
	}
	if one := detect(t, commits(1, 0), conventions.Default()); !slices.Contains(one.Notes, "churn over the 1 non-merge commit (fewer than 150)") {
		t.Errorf("1 commit: notes %q, want the singular note", one.Notes)
	}
	few := detect(t, commits(37, 0), conventions.Default())
	if !slices.Contains(few.Notes, "churn over all 37 non-merge commits (fewer than 150)") {
		t.Errorf("37 commits: notes %q, want the fewer-than-150 note", few.Notes)
	}
	many := detect(t, commits(160, 5), conventions.Default())
	churn, _ := statistic(many, "churn: product", "")
	sum := func(from, to int) float64 { return float64((from + to) * (to - from + 1) / 2) }
	if *churn.Value != sum(1, 150) || *churn.Before != sum(6, 155) || !slices.Contains(many.Notes, "churn over the last 150 non-merge commits") {
		t.Errorf("160 commits, 5 since the base: churn %v before %v, notes %q; want %v, %v", *churn.Value, *churn.Before, many.Notes, sum(1, 150), sum(6, 155))
	}
}

// rustLines is a Rust file of product lines followed by a test module of so many lines.
func rustLines(product, test int) string {
	text := strings.Repeat("fn p() {}\n", product)
	if test > 0 {
		text += "#[cfg(test)]\nmod tests {\n" + strings.Repeat("    // t\n", test-3) + "}\n"
	}
	return text
}

func TestTestCodeInsideProductFilesCountsAsTestLines(t *testing.T) {
	base := map[string]string{"src/a.rs": rustLines(10, 5), "src/b.rs": rustLines(4, 0)}
	now := map[string]string{"src/a.rs": rustLines(12, 9), "src/b.rs": rustLines(4, 0)}
	source := snapshottest.Files(base, now)
	source.Changes = []snapshot.Change{{Path: "src/a.rs", Added: 6}}
	findings := detect(t, source, conventions.Default())
	for name, want := range map[string][2]float64{"product lines": {16, 14}, "test lines": {9, 5}, "test ratio": {0.6, 0.4}} {
		if s, _ := statistic(findings, name, ""); s.Value == nil || s.Before == nil || *s.Value != want[0] || *s.Before != want[1] {
			t.Errorf("%s = %+v, want %v now and %v at the base", name, s, want[0], want[1])
		}
	}
	if got := value(t, findings, "largest code file", "src/a.rs"); got != 21 {
		t.Errorf("largest code file src/a.rs = %v lines, want the whole file's 21", got)
	}
	note := "`rust`: test code inside 1 product file counts as test lines, not as product lines: 9 lines"
	footprint := "the footprint counts, in 1 changed product file that holds test code, only the growth of that test code " +
		"as test lines and every other changed line as a product line"
	if !slices.Contains(findings.Notes, note) || !slices.Contains(findings.NotAnalysed, footprint) {
		t.Errorf("notes %q, not analysed %q; want %q and %q", findings.Notes, findings.NotAnalysed, note, footprint)
	}
	if same := detect(t, snapshottest.Files(now, now), conventions.Default()); !slices.Contains(same.Notes, note) || slices.Contains(same.NotAnalysed, footprint) {
		t.Errorf("an unchanged tree: notes %q, not analysed %q; want the note and no footprint sentence", same.Notes, same.NotAnalysed)
	}
	big := detect(t, snapshottest.Files(nil, map[string]string{"src/big.rs": rustLines(401, 600)}), conventions.Default())
	if marks := marksOf(big, "file-size"); len(marks) != 1 || marks[0].Measured.Value != 1001 {
		t.Errorf("a file of 1,001 lines, 600 of them test code: size marks %+v, want one with the whole 1,001", marks)
	}
	plain := detect(t, ratioSource(10, 10), conventions.Default())
	for _, line := range append(slices.Clone(plain.Notes), plain.NotAnalysed...) {
		if strings.Contains(line, "test code inside") || strings.Contains(line, "footprint") {
			t.Errorf("a tree without such code: %q", line)
		}
	}
}

// wantedConfidence is candidate for the function rules, exact for every other.
func wantedConfidence(m answer.Mark) answer.Confidence {
	if rule := answer.RuleOf(m.ID); rule == ruleComplexity || rule == ruleNesting {
		return answer.Candidate
	}
	return answer.Exact
}

func TestShapeNamesTheDecidingRulesThatRan(t *testing.T) {
	manifest := func(dep string) string { return `{"dependencies": {"` + dep + `": "1"}}` }
	source := snapshottest.Files(map[string]string{"package.json": manifest("a"), "src/a.ts": "export const a = 1;\n"},
		map[string]string{"package.json": manifest("a"), "src/a.ts": "export const a = 2;\n"})
	read := detect(t, source, conventions.Default())
	if want := []string{"shape.test-ratio", "shape.function-complexity", "shape.function-duplicate", "shape.dependencies"}; !slices.Equal(read.RanRules, want) {
		t.Errorf("a read manifest and changed product code: ran %q, want %q", read.RanRules, want)
	}
	source.Unavailable = []string{"src/old.ts"}
	if lacking := detect(t, source, conventions.Default()); slices.Contains(lacking.RanRules, "shape.function-duplicate") {
		t.Errorf("a changed code file without its base content: ran %q, want no duplicate rule", lacking.RanRules)
	}
	broken := detect(t, snapshottest.Files(map[string]string{"package.json": manifest("a")}, map[string]string{"package.json": `{"dependencies": {`}), conventions.Default())
	if slices.Contains(broken.RanRules, "shape.dependencies") {
		t.Errorf("a manifest unreadable now: ran %q, want no dependencies rule", broken.RanRules)
	}
	none := detect(t, snapshottest.Files(map[string]string{"README.md": "a\n"}, map[string]string{"README.md": "b\n"}), conventions.Default())
	if len(none.RanRules) != 0 {
		t.Errorf("no manifest, no code: ran %q, want none", none.RanRules)
	}
}

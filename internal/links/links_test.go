package links

import (
	"fmt"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

// eachLanguage runs a test body for every language entry with its case.
func eachLanguage(t *testing.T, body func(t *testing.T, name string, c languageCase)) {
	t.Helper()
	for _, language := range conventions.Default().Languages {
		c, known := languageCases()[language.Name]
		if !known {
			t.Errorf("language %s has no links case", language.Name)
			continue
		}
		c.byFolder, c.noRings = language.Imports.Target == conventions.FolderTarget, language.Imports.NoRings
		c.sourceSets = language.Imports.Prefix.Format == conventions.SourceRoots
		c.notSeen = language.Imports.NotSeen
		t.Run(language.Name, func(t *testing.T) { body(t, language.Name, c) })
	}
}

// tree is a language's setup files plus the given ones.
func (c languageCase) tree(files map[string]string) map[string]string {
	all := map[string]string{}
	maps.Copy(all, c.setup)
	maps.Copy(all, files)
	return all
}

// fan is a tree in which product file a imports resolved product files, and adds the given specs.
func (c languageCase) fan(resolved int, specs ...string) map[string]string {
	files := map[string]string{}
	var targets []string
	for i := range resolved {
		target := c.product("t" + string(rune('a'+i)))
		targets = append(targets, target)
		files[target] = c.importing(target, nil)
	}
	files[c.product("a")] = c.importing(c.product("a"), targets, specs...)
	return c.tree(files)
}

// detect builds the snapshot, runs the detector and checks what every mark must carry.
func detect(t *testing.T, source snapshot.Source) answer.Findings {
	t.Helper()
	f := Detector{}.Detect(snapshot.Build(source, conventions.Default(), snapshottest.Readings(t)))
	for _, m := range f.Marks {
		if m.ID == "" || m.Change == "" || m.Subject.Kind == "" || m.Measured.Unit == "" || m.Measured.LimitSource == "" ||
			m.Hint == "" || m.Confidence != answer.Candidate {
			t.Errorf("mark %+v lacks a field it must carry", m)
		}
	}
	return f
}

func same(files map[string]string) snapshot.Source { return snapshottest.Files(files, files) }

const wantLinksNote = "links: imports read from the code; an import is a link only when it names exactly one tracked file " +
	"of the repository, or the folders of one package where a language imports folders; outside packages are not compared with local names"

const wantFolderNote = "an import of a folder reaches the folder's files, and a package that lies in several folders is reached in each; " +
	"use between files of one folder or one package needs no import and is not seen"

func TestLinksCountImportsPerAnalysedLanguage(t *testing.T) {
	eachLanguage(t, func(t *testing.T, name string, c languageCase) {
		f := detect(t, same(c.fan(9, c.outside...)))
		value, before := 9.0, 9.0
		want := answer.Statistic{Name: "imports followed", Subject: name, Value: &value, Before: &before, Unit: "imports",
			Note: "of 9 own: 0 ambiguous, 0 not readable, 0 not found; 3 outside"}
		if len(f.Statistics) == 0 || !sameStatistic(f.Statistics[0], want) {
			t.Errorf("statistics = %+v, want first %+v", f.Statistics, want)
		}
		if len(f.Notes) == 0 || f.Notes[0] != wantLinksNote || len(f.NotAnalysed) != 0 {
			t.Errorf("notes %q, not analysed %q; want the links note first and nothing not analysed", f.Notes, f.NotAnalysed)
		}
		if got, want := slices.Contains(f.Notes, wantFolderNote), c.byFolder; got != want {
			t.Errorf("the folder note present = %v, want %v", got, want)
		}
		sourceSetsNote := "`" + name + "`: a package's folders under a test folder directly in `src/` are reached only from files under such a folder"
		if got, want := slices.Contains(f.Notes, sourceSetsNote), c.sourceSets; got != want {
			t.Errorf("the source-set note present = %v, want %v", got, want)
		}
		notSeenNote := "`" + name + "`: not seen: " + c.notSeen
		if got, want := slices.Contains(f.Notes, notSeenNote), c.notSeen != ""; got != want {
			t.Errorf("the not-seen note present = %v, want %v", got, want)
		}
		for _, note := range []string{
			"`python`: its own packages are looked for at the root and under `src/`; an import of a package elsewhere counts as outside",
			"`python`: `__init__.py` and `__main__.py` run without an import of them; a new one is not listed as a file without an importer",
		} {
			if got, want := slices.Contains(f.Notes, note), name == "python"; got != want {
				t.Errorf("note %q present = %v, want %v", note, got, want)
			}
		}
		fresh := detect(t, snapshottest.Files(map[string]string{"README.md": "x\n"}, c.fan(9, c.outside...)))
		if len(fresh.Statistics) == 0 || fresh.Statistics[0].Before != nil {
			t.Errorf("a language new since the base: statistics %+v, want no value at the base", fresh.Statistics)
		}
	})
}

func sameStatistic(a, b answer.Statistic) bool {
	value := func(v *float64) any {
		if v == nil {
			return nil
		}
		return *v
	}
	return a.Name == b.Name && a.Subject == b.Subject && value(a.Value) == value(b.Value) && value(a.Before) == value(b.Before) &&
		a.Unit == b.Unit && a.Note == b.Note
}

func TestALanguageOverTheLimitIsNotAnalysed(t *testing.T) {
	eachLanguage(t, func(t *testing.T, name string, c languageCase) {
		over := c.fan(8, c.nowhere)
		sentence := "`" + name + "`: 1 of 9 own imports could not be followed now (0 ambiguous, 0 not readable, 1 not found), " +
			"over the limit of 10% (links checked against go list and dependency-cruiser, 2026-11); no link numbers for its files"
		f := detect(t, same(over))
		if len(f.Statistics) != 0 || len(f.Marks) != 0 || !slices.Equal(f.NotAnalysed, []string{sentence}) {
			t.Errorf("over the limit: statistics %+v, marks %+v, not analysed %q; want only %q", f.Statistics, f.Marks, f.NotAnalysed, sentence)
		}
		atBase := detect(t, snapshottest.Files(over, c.fan(8)))
		if want := strings.Replace(sentence, "now", "at the base", 1); !slices.Equal(atBase.NotAnalysed, []string{want}) {
			t.Errorf("over the limit at the base only: not analysed %q, want %q", atBase.NotAnalysed, want)
		}
		if at := detect(t, same(c.fan(9, c.nowhere))); len(at.NotAnalysed) != 0 || len(at.Statistics) == 0 {
			t.Errorf("exactly at the limit: not analysed %q, statistics %+v; want analysed", at.NotAnalysed, at.Statistics)
		}
	})
	ts, golang := languageCases()["typescript"], languageCases()["go"]
	mixed := golang.fan(9)
	maps.Copy(mixed, ts.fan(8, ts.nowhere))
	f := detect(t, same(mixed))
	if len(f.NotAnalysed) != 1 || !strings.HasPrefix(f.NotAnalysed[0], "`typescript`: ") ||
		len(f.Statistics) == 0 || f.Statistics[0].Subject != "go" {
		t.Errorf("typescript over beside a sound go tree: not analysed %q, statistics %+v", f.NotAnalysed, f.Statistics)
	}
	cs := languageCases()["csharp"]
	global := cs.fan(8)
	global["src/Acme/GlobalUsings.cs"] = "global using Acme.ta;\nglobal using System.Linq;\n"
	wantGlobal := "`csharp`: 1 of 9 own imports could not be followed now (0 ambiguous, 1 not readable, 0 not found), " +
		"over the limit of 10% (links checked against go list and dependency-cruiser, 2026-11); no link numbers for its files"
	if f := detect(t, same(global)); len(f.Statistics) != 0 || !slices.Equal(f.NotAnalysed, []string{wantGlobal}) {
		t.Errorf("a global using of own code: statistics %+v, not analysed %q; want only %q", f.Statistics, f.NotAnalysed, wantGlobal)
	}
}

func TestADeclaredRootIsNamedAndSearched(t *testing.T) {
	declared := detect(t, same(map[string]string{"pyproject.toml": "[tool.pytest.ini_options]\npythonpath = [\"lib\"]\n", "lib/core/__init__.py": "",
		"src/app/__init__.py": "", "src/app/a.py": "import core\n"}))
	for _, note := range []string{
		"`python`: roots declared in `pyproject.toml` (`[tool.pytest.ini_options]` `pythonpath`): `lib/`",
		"`python`: its own packages are looked for at the root, under `src/` and under `lib/`; an import of a package elsewhere counts as outside",
	} {
		if !slices.Contains(declared.Notes, note) {
			t.Errorf("a declared root: notes %q, want %q", declared.Notes, note)
		}
	}
	if row, _ := statisticNamed(declared, "imports followed"); row.Value == nil || *row.Value != 1 {
		t.Errorf("a declared root: imports followed = %+v, want 1", row)
	}
}

func TestALanguageWithoutItsPrefixFile(t *testing.T) {
	golang, ts := languageCases()["go"], languageCases()["typescript"]
	loose := map[string]string{"lib/app/a.py": "import app.b\n", "lib/app/b.py": "x = 1\n"}
	noRoot := map[string]string{"app/A.java": "import com.acme.b.B;\n", "app/com/acme/b/B.java": "package com.acme.b;\n"}
	scripts := map[string]string{"build.gradle.kts": "import org.gradle.api.JavaVersion\n", "settings.gradle.kts": "rootProject.name = \"x\"\n"}
	bare := map[string]string{"src/A.cs": "using Acme.B;\n", "src/B.cs": "class B {}\n"}
	with := func(files map[string]string, path, content string) map[string]string {
		changed := clone(files)
		if content == "" {
			delete(changed, path)
		} else {
			changed[path] = content
		}
		return changed
	}
	cases := []struct {
		name      string
		base, now map[string]string
		sentence  string
		numbers   bool
	}{
		{"go without go.mod", with(golang.fan(2), "go.mod", ""), with(golang.fan(2), "go.mod", ""),
			"`go`: no `go.mod` at the root, so its own imports cannot be told from outside ones; no link numbers for its files", false},
		{"go.mod added since the base", with(golang.fan(2), "go.mod", ""), golang.fan(2),
			"`go`: no `go.mod` at the root at the base, so its own imports cannot be told from outside ones; no link numbers for its files", false},
		{"go.mod without a module line", with(golang.fan(2), "go.mod", "go 1.27\n"), with(golang.fan(2), "go.mod", "go 1.27\n"),
			"`go`: `go.mod` could not be read, so its own imports cannot be told from outside ones; no link numbers for its files", false},
		{"an extending tsconfig.json", with(ts.fan(2), "tsconfig.json", `{"extends": "./base.json"}`), with(ts.fan(2), "tsconfig.json", `{"extends": "./base.json"}`),
			"`typescript`: `tsconfig.json` extends another file; aliases declared there count as outside", true},
		{"an unreadable tsconfig.json", with(ts.fan(2), "tsconfig.json", "{not json"), with(ts.fan(2), "tsconfig.json", "{not json"),
			"`typescript`: `tsconfig.json` could not be read; its aliases count as outside", true},
		{"python without a package of its own", loose, loose,
			"`python`: no package of its own at the root or under `src/`, so its own imports cannot be told from outside ones; no link numbers for its files", false},
		{"python with a declared root and no package", with(loose, "pyproject.toml", "[tool.pytest.ini_options]\npythonpath = [\"lib\"]\n"), with(loose, "pyproject.toml", "[tool.pytest.ini_options]\npythonpath = [\"lib\"]\n"),
			"`python`: no package of its own at the root, under `src/` or under `lib/`, so its own imports cannot be told from outside ones; no link numbers for its files", false},
		{"java without a source root", noRoot, noRoot,
			"`java`: no package of its own under `src/*/java/` or under `src/*/kotlin/`, so its own imports cannot be told from outside ones; no link numbers for its files", false},
		{"csharp without a declared namespace", bare, bare,
			"`csharp`: no package of its own declared by its files, so its own imports cannot be told from outside ones; no link numbers for its files", false},
		{"kotlin build scripts alone", scripts, scripts,
			"`kotlin`: no package of its own under `src/*/java/` or under `src/*/kotlin/`, so its own imports cannot be told from outside ones; no link numbers for its files", false},
		{"a module added since the base", loose, with(loose, "main.py", "x = 1\n"),
			"`python`: no package of its own at the root or under `src/` at the base, so its own imports cannot be told from outside ones; no link numbers for its files", false},
	}
	for _, tc := range cases {
		f := detect(t, snapshottest.Files(tc.base, tc.now))
		if !slices.Equal(f.NotAnalysed, []string{tc.sentence}) || (len(f.Statistics) > 0) != tc.numbers {
			t.Errorf("%s: not analysed %q, statistics %+v; want %q and numbers %v", tc.name, f.NotAnalysed, f.Statistics, tc.sentence, tc.numbers)
		}
	}
	python := languageCases()["python"]
	outside := with(python.fan(2), "pyproject.toml", "[tool.pytest.ini_options]\npythonpath = [\"../x\"]\n")
	if f := detect(t, same(outside)); !slices.Equal(f.NotAnalysed, []string{"`python`: `pyproject.toml` `pythonpath`: 1 value is absolute, leaves the repository or holds a pattern; it is not used"}) || len(f.Statistics) == 0 {
		t.Errorf("a root outside the repository: not analysed %q, statistics %d", f.NotAnalysed, len(f.Statistics))
	}
}

func TestLinksRunWithoutALanguageEntry(t *testing.T) {
	f := detect(t, same(map[string]string{"bin/a.sh": "echo a\n", "bin/b.sh": "echo b\n"}))
	if !slices.Equal(f.NotAnalysed, []string{"no file of a language whose imports are read"}) ||
		len(f.Statistics) != 0 || len(f.Notes) != 0 || len(f.Marks) != 0 {
		t.Errorf("shell scripts only: %+v, want one not-analysed entry and nothing else", f)
	}
}

// imports is a tree in which product file a imports the given targets, each of which exists.
func (c languageCase) imports(a string, targets ...string) map[string]string {
	files := map[string]string{}
	for _, target := range targets {
		files[target] = c.importing(target, nil)
		if strings.Contains(target, "test.") || strings.HasSuffix(target, "_test.go") {
			files[target] = c.testBody + "\n"
		}
	}
	if a != "" {
		files[a] = c.importing(a, targets)
	}
	return c.tree(files)
}

func marksOf(f answer.Findings, rule string) []answer.Mark {
	var found []answer.Mark
	for _, m := range f.Marks {
		if answer.RuleOf(m.ID) == rule {
			found = append(found, m)
		}
	}
	return found
}

func TestTestImportMarksInEachLanguage(t *testing.T) {
	support := map[string]string{"src/a.ts": "import \"@t/db\";\n", "tests/support/db.ts": "export const db = 1;\n"}
	if marks := marksOf(detect(t, snapshottest.Files(aliased(false, support), aliased(true, support))), "test-import"); len(marks) != 1 ||
		marks[0].ID != "links.test-import:src/a.ts" || marks[0].Change != answer.Touched || marks[0].Measured.Value != 1 ||
		marks[0].Measured.Before == nil || *marks[0].Measured.Before != 0 {
		t.Errorf("an import of test code only made readable: marks %+v, want one touched, 1 now and 0 at the base", marks)
	}
	eachLanguage(t, func(t *testing.T, _ string, c languageCase) {
		a, b, db, clock := c.product("a"), c.product("b"), c.support("db"), c.support("clock")
		one := 1.0
		cases := []struct {
			name      string
			base, now map[string]string
			change    answer.Change
			value     float64
			before    *float64
		}{
			{"starts to import", c.imports(a, b), c.imports(a, b, db), answer.New, 1, new(0.0)},
			{"a new file", c.imports("", b), c.imports(a, db), answer.New, 1, nil},
			{"imports more", c.imports(a, db), c.imports(a, db, clock), answer.Grown, 2, &one},
			{"unchanged", c.imports(a, db), c.imports(a, db), answer.Standing, 1, &one},
			{"stops", c.imports(a, db), c.imports(a, b), answer.Resolved, 0, &one},
		}
		z := strings.Replace(a, "a", "z", 1)
		renamed := snapshottest.Files(c.imports(a, db), c.imports(z, db))
		renamed.Changes = []snapshot.Change{{Path: z, From: a}}
		if marks := marksOf(detect(t, renamed), "test-import"); len(marks) != 1 || marks[0].ID != "links.test-import:"+z ||
			marks[0].Change != answer.Standing || marks[0].Measured.Before == nil || *marks[0].Measured.Before != 1 {
			t.Errorf("a renamed file that imports test code: marks %+v, want it standing under its new name", marks)
		}
		gone := snapshottest.Files(c.imports(a, db), c.imports("", db))
		goneMarks := marksOf(detect(t, gone), "test-import")
		if len(goneMarks) != 1 || goneMarks[0].ID != "links.test-import:"+a || goneMarks[0].Change != answer.Resolved {
			t.Errorf("a product file gone since the base: marks %+v, want it resolved under its base path", goneMarks)
		}
		s := snapshot.Build(gone, conventions.Default(), snapshottest.Readings(t))
		if got := (Detector{}).Explain(s, goneMarks[0]); len(got) == 0 || !strings.HasPrefix(got[0], "at the base `"+a+"` line ") {
			t.Errorf("Explain of a gone file = %q, want its base import first", got)
		}
		for _, tc := range cases {
			marks := marksOf(detect(t, snapshottest.Files(tc.base, tc.now)), "test-import")
			if len(marks) != 1 {
				t.Errorf("%s: test-import marks = %+v, want one", tc.name, marks)
				continue
			}
			m := marks[0]
			beforeOK := tc.before == nil && m.Measured.Before == nil || tc.before != nil && m.Measured.Before != nil && *m.Measured.Before == *tc.before
			if m.ID != "links.test-import:"+a || m.Change != tc.change || m.Measured.Value != tc.value || !beforeOK ||
				!slices.Equal(m.Subject.Paths, []string{a}) || m.Subject.Kind != "file" || m.Measured.Limit != nil ||
				m.Measured.Unit != "imports of test code" || m.Measured.LimitSource != "product code imports no test code" ||
				m.Hint != "found by reading the import statements; `explain` names each import by file and line." {
				t.Errorf("%s: mark %+v %+v", tc.name, m, m.Measured)
			}
		}
		scripts := c.imports("", db)
		script := "scripts/x" + path.Ext(a)
		if path.Ext(a) == ".go" {
			script = "scripts/x/x.go"
		}
		scripts[script] = c.importing(script, []string{db})
		if marks := marksOf(detect(t, same(scripts)), "test-import"); len(marks) != 0 {
			t.Errorf("other code importing test code: marks %+v, want none", marks)
		}
	})
	ts := languageCases()["typescript"]
	typeOnly := ts.imports("", ts.support("db"))
	typeOnly[ts.product("a")] = `import type { T } from "../tests/support/db.ts";` + "\n"
	if marks := marksOf(detect(t, snapshottest.Files(ts.tree(nil), typeOnly)), "test-import"); len(marks) != 1 || marks[0].Measured.Value != 1 {
		t.Errorf("a type-only import of test code: marks %+v, want one with value 1", marks)
	}
	testFile := ts.imports(ts.product("a"), ts.test("b"))
	if marks := marksOf(detect(t, snapshottest.Files(ts.tree(nil), testFile)), "test-import"); len(marks) != 1 {
		t.Errorf("product code importing a file that defines tests: marks %+v, want one", marks)
	}
}

// ring builds a typescript tree from edges "a>b", one import per line, every named file tracked.
func ring(edges ...string) map[string]string {
	ts := languageCases()["typescript"]
	targets := map[string][]string{}
	for _, edge := range edges {
		from, to, _ := strings.Cut(edge, ">")
		targets[ts.product(from)] = append(targets[ts.product(from)], ts.product(to))
		if _, known := targets[ts.product(to)]; !known {
			targets[ts.product(to)] = nil
		}
	}
	files := map[string]string{}
	for from, to := range targets {
		files[from] = ts.importing(from, to)
	}
	return files
}

func statisticNamed(f answer.Findings, statName string) (answer.Statistic, bool) {
	for _, s := range f.Statistics {
		if s.Name == statName {
			return s, true
		}
	}
	return answer.Statistic{}, false
}

// aliased is a TypeScript tree whose files reach others by aliases that only its tsconfig.json makes readable.
func aliased(withConfig bool, files map[string]string) map[string]string {
	tree := maps.Clone(files)
	if withConfig {
		tree["tsconfig.json"] = `{"compilerOptions":{"paths":{"@app/*":["src/app/*"],"@t/*":["tests/support/*"]}}}` + "\n"
	}
	return tree
}

func TestCycleMarks(t *testing.T) {
	three, two := 3.0, 2.0
	readable := map[string]string{"src/app/a.ts": "import \"@app/b\";\n", "src/app/b.ts": "import \"./a.ts\";\n"}
	cases := []struct {
		name      string
		base, now map[string]string
		id        string
		change    answer.Change
		value     float64
		before    *float64
	}{
		{"a new ring", ring("a>b", "b>c"), ring("a>b", "b>c", "c>a"), "src/a.ts", answer.New, 3, nil},
		{"a ring that grew", ring("b>c", "c>d", "d>b"), ring("b>c", "c>d", "d>b", "b>a", "a>c"), "src/a.ts", answer.Grown, 4, &three},
		{"an unchanged ring", ring("a>b", "b>c", "c>a"), ring("a>b", "b>c", "c>a"), "src/a.ts", answer.Standing, 3, &three},
		{"a broken ring", ring("a>b", "b>c", "c>a"), ring("a>b", "b>c"), "src/a.ts", answer.Resolved, 0, &three},
		{"two rings joined", ring("a>b", "b>a", "c>d", "d>c"), ring("a>b", "b>a", "c>d", "d>c", "b>c", "d>a"), "src/a.ts", answer.Grown, 4, &two},
		{"a ring only made readable", aliased(false, readable), aliased(true, readable), "src/app/a.ts", answer.Touched, 2, nil},
	}
	for _, tc := range cases {
		marks := marksOf(detect(t, snapshottest.Files(tc.base, tc.now)), ruleCycle)
		if len(marks) != 1 {
			t.Errorf("%s: cycle marks = %+v, want one", tc.name, marks)
			continue
		}
		m := marks[0]
		beforeOK := tc.before == nil && m.Measured.Before == nil || tc.before != nil && m.Measured.Before != nil && *m.Measured.Before == *tc.before
		if m.ID != "links.cycle:"+tc.id || m.Change != tc.change || m.Measured.Value != tc.value || !beforeOK ||
			m.Subject.Kind != "files" || m.Measured.Unit != "files" || m.Measured.Limit != nil ||
			m.Measured.LimitSource != "files do not import each other in a ring" || !strings.HasSuffix(m.Hint, hint) {
			t.Errorf("%s: mark %+v %+v", tc.name, m, m.Measured)
		}
	}
	newRing := marksOf(detect(t, snapshottest.Files(ring("a>b", "b>c"), ring("a>b", "b>c", "c>a"))), ruleCycle)[0]
	if !slices.Equal(newRing.Subject.Paths, []string{"src/a.ts", "src/b.ts", "src/c.ts"}) {
		t.Errorf("a new ring's subject = %q, want its three files sorted", newRing.Subject.Paths)
	}
	if want := "closed by `src/c.ts` line 1 importing `src/a.ts`, added by this change; " + hint; newRing.Hint != want {
		t.Errorf("a new ring's hint = %q, want %q", newRing.Hint, want)
	}

	renamed := snapshottest.Files(ring("a>b", "b>a"), ring("z>b", "b>z"))
	renamed.Changes = []snapshot.Change{{Path: "src/b.ts", Added: 1, Removed: 1}, {Path: "src/z.ts", From: "src/a.ts"}}
	if marks := marksOf(detect(t, renamed), ruleCycle); len(marks) != 1 || marks[0].ID != "links.cycle:src/b.ts" ||
		marks[0].Change != answer.Standing || marks[0].Measured.Value != 2 {
		t.Errorf("a ring with a renamed member: marks %+v, want it standing as links.cycle:src/b.ts", marks)
	}

	ts := languageCases()["typescript"]
	typeRing := map[string]string{
		ts.product("a"): `import type { B } from "./b.ts";` + "\n",
		ts.product("b"): `import { a } from "./a.ts";` + "\n",
	}
	f := detect(t, same(typeRing))
	rings, _ := statisticNamed(f, "import rings")
	typeOnly, _ := statisticNamed(f, "files in a ring only through type-only imports")
	if len(marksOf(f, ruleCycle)) != 0 || rings.Value == nil || *rings.Value != 0 || typeOnly.Value == nil || *typeOnly.Value != 2 {
		t.Errorf("a ring through a type-only import: marks %+v, rings %+v, type-only %+v", f.Marks, rings, typeOnly)
	}
	if self := detect(t, same(map[string]string{"src/self.ts": `import "./self.ts";` + "\n"})); len(marksOf(self, ruleCycle)) != 0 {
		t.Errorf("a file that imports itself: marks %+v, want none", self.Marks)
	}

	f = detect(t, snapshottest.Files(ring("a>b", "b>c"), ring("a>b", "b>c", "c>a")))
	rings, _ = statisticNamed(f, "import rings")
	typeOnly, _ = statisticNamed(f, "files in a ring only through type-only imports")
	if rings.Value == nil || *rings.Value != 1 || rings.Before == nil || *rings.Before != 0 || rings.Unit != "rings" || rings.Note != "3 files" ||
		typeOnly.Value == nil || *typeOnly.Value != 0 || typeOnly.Before == nil || *typeOnly.Before != 0 || typeOnly.Unit != "files" {
		t.Errorf("rows for a new ring: %+v and %+v", rings, typeOnly)
	}
	if !slices.Contains(f.Notes, "rings: among imports that are not type-only") {
		t.Errorf("notes = %q, want the rings note", f.Notes)
	}
}

func TestRingsAreNotLookedForWhereTheEntrySaysSo(t *testing.T) {
	eachLanguage(t, func(t *testing.T, name string, c languageCase) {
		if c.noRings == "" {
			return
		}
		a, b := c.product("a"), c.product("b")
		f := detect(t, same(c.tree(map[string]string{a: c.importing(a, []string{b}), b: c.importing(b, []string{a})})))
		_, rings := statisticNamed(f, "import rings")
		if len(marksOf(f, ruleCycle)) != 0 || rings || len(f.NotAnalysed) != 0 ||
			!slices.Contains(f.Notes, "`"+name+"`: import rings are not looked for: "+c.noRings) ||
			slices.Contains(f.Notes, "rings: among imports that are not type-only") {
			t.Errorf("%s: marks %+v, rows %+v, notes %q, not analysed %q", name, f.Marks, f.Statistics, f.Notes, f.NotAnalysed)
		}
	})
}

func clone(files map[string]string) map[string]string {
	copied := map[string]string{}
	maps.Copy(copied, files)
	return copied
}

// testFile is a test file of the language that imports the targets.
func (c languageCase) testFile(at string, targets ...string) string {
	return c.importing(at, targets) + c.testBody + "\n"
}

// importersBase is the base tree of the importer tests: b, c, the test of b and a second test file import a; u imports
// old; p imports ten further files, so one import that leads nowhere stays under the guard's limit.
func (c languageCase) importersBase() map[string]string {
	a, old := c.product("a"), c.product("old")
	files := map[string]string{
		a: c.importing(a, nil), old: c.importing(old, nil),
		c.product("b"): c.importing(c.product("b"), []string{a}), c.product("c"): c.importing(c.product("c"), []string{a}),
		c.test("b"): c.testFile(c.test("b"), a), c.test("t2"): c.testFile(c.test("t2"), a),
		c.product("u"): c.importing(c.product("u"), []string{old}),
	}
	var padding []string
	for i := range 10 {
		target := c.product("p" + string(rune('a'+i)))
		files[target] = c.importing(target, nil)
		padding = append(padding, target)
	}
	files[c.product("p")] = c.importing(c.product("p"), padding)
	return c.tree(files)
}

func TestImportersOfTheChangeCountDirectAndAtAnyDistance(t *testing.T) {
	eachLanguage(t, func(t *testing.T, name string, c languageCase) {
		a := c.product("a")
		base := c.importersBase()
		now := clone(base)
		now[a] += "// changed\n"
		now[c.product("c")] += "// changed\n"
		now[c.product("d")] = c.importing(c.product("d"), []string{a})
		delete(now, c.product("old"))
		withNew := clone(now)
		withNew[c.product("e")] = c.importing(c.product("e"), []string{c.product("d")})
		withNewBase := clone(base)
		withNewBase[c.product("e")] = withNew[c.product("e")]
		for _, s := range detect(t, snapshottest.Files(withNewBase, withNew)).Statistics {
			if s.Name == "most direct importers" && strings.HasPrefix(c.product("d"), strings.TrimSuffix(s.Subject, "/")) && s.Before != nil {
				t.Errorf("a file new since the base has a most-direct-importers row with a value at the base: %+v", s)
			}
		}
		chained, chainedNow := clone(base), clone(now)
		chained[c.product("e")] = c.importing(c.product("e"), []string{c.product("b")})
		chainedNow[c.product("e")] = chained[c.product("e")]
		r, _ := statisticNamed(detect(t, snapshottest.Files(chained, chainedNow)), "importers of the change")
		// a, c and d changed and old deleted: b, two tests and u (which imported old) directly, e behind b
		want := regexp.MustCompile(`^4 files directly, 5 at any distance; 4 changed code files; directly 2 product, 2 test; ` +
			`at any distance 3 product, 2 test; in [^;]+; 3 of 1[78] product code files; 2 more importers are part of the change$`)
		if r.Value != nil || !want.MatchString(r.Note) {
			t.Errorf("importers of the change = %+v, want %s", r, want)
		}
		editedB := clone(chained)
		editedB[a] += "// changed\n"
		editedB[c.product("b")] += "// changed\n"
		renamedB := clone(chained)
		delete(renamedB, c.product("b"))
		renamedB[c.product("b2")] = chained[c.product("b")]
		renamedB[a] += "// changed\n"
		moved := snapshottest.Files(chained, renamedB)
		moved.Changes = append(moved.Changes, snapshot.Change{Path: c.product("b2"), From: c.product("b")})
		// e imports b: a changed file, or a path the rename removed; either way it counts directly beside c and the tests
		for label, changed := range map[string]snapshot.Source{"a and b changed": snapshottest.Files(chained, editedB), "a changed and b renamed": moved} {
			if r, _ := statisticNamed(detect(t, changed), "importers of the change"); !strings.HasPrefix(r.Note, "4 files directly, 4 at any distance; 2 changed code files; ") {
				t.Errorf("%s: importers of the change = %q, want c, two tests and e directly", label, r.Note)
			}
		}
		// old deleted and its importer u renamed by the same change: u counts as part of the change under its path now
		renamedU := clone(now)
		delete(renamedU, c.product("u"))
		renamedU[c.product("u2")] = base[c.product("u")]
		withRenamedU := snapshottest.Files(base, renamedU)
		withRenamedU.Changes = append(withRenamedU.Changes, snapshot.Change{Path: c.product("u2"), From: c.product("u")})
		if r, _ := statisticNamed(detect(t, withRenamedU), "importers of the change"); !strings.HasPrefix(r.Note, "3 files directly, ") ||
			!strings.HasSuffix(r.Note, "; 3 more importers are part of the change") {
			t.Errorf("old deleted and u renamed: importers of the change = %q, want b and two tests directly, u among the change", r.Note)
		}
		f := detect(t, snapshottest.Files(base, now))
		var most []answer.Statistic
		for _, s := range f.Statistics {
			if s.Name == "most direct importers" {
				most = append(most, s)
			}
		}
		wantSubject := a
		if c.byFolder {
			wantSubject = path.Dir(a) + "/"
		}
		if len(most) != 1 || !sameStatistic(most[0], answer.Statistic{Name: "most direct importers", Subject: wantSubject, Note: "3 files; 1 product, 2 test"}) {
			t.Errorf("most direct importers = %+v, want one row for %s", most, a)
		}
		if !slices.Contains(f.Notes, importersNote) {
			t.Errorf("notes = %q, want the importers note", f.Notes)
		}
		unchanged, _ := statisticNamed(detect(t, same(base)), "importers of the change")
		if !sameStatistic(unchanged, answer.Statistic{Name: "importers of the change", Note: "0 files directly, 0 at any distance; 0 changed code files"}) {
			t.Errorf("an unchanged tree: importers of the change = %+v", unchanged)
		}
		if name == "go" {
			testOnly := clone(base)
			testOnly[c.test("b")] += "// changed\n"
			if r, _ := statisticNamed(detect(t, snapshottest.Files(base, testOnly)), "importers of the change"); r.Value != nil || r.Note != "0 files directly, 0 at any distance; 1 changed code file" {
				t.Errorf("a changed Go test file alone: importers of the change = %+v, want 0 and one changed code file", r)
			}
		}
		if name == "typescript" {
			withIndex, nowIndex := clone(base), clone(now)
			withIndex["src/index.ts"] = "export * from \"./a.ts\";\n"
			nowIndex["src/index.ts"] = withIndex["src/index.ts"]
			r, _ := statisticNamed(detect(t, snapshottest.Files(withIndex, nowIndex)), "importers of the change")
			if r.Value != nil || !strings.HasPrefix(r.Note, "5 files directly, ") {
				t.Errorf("an untouched index file among the importers: importers of the change = %+v", r)
			}
		}
		changedMany := clone(base)
		for _, target := range []string{"pa", "pb", "pc", "pd"} {
			changedMany[c.product(target)] += "// changed\n"
		}
		var rows []string
		for _, s := range detect(t, snapshottest.Files(base, changedMany)).Statistics {
			if s.Name == "most direct importers" {
				rows = append(rows, s.Subject)
			}
		}
		subject := func(file string) string {
			if c.byFolder {
				return path.Dir(file) + "/"
			}
			return file
		}
		if want := []string{subject(c.product("pa")), subject(c.product("pb")), subject(c.product("pc"))}; !slices.Equal(rows, want) {
			t.Errorf("four changed files with one importer each: most direct importers = %q, want %q", rows, want)
		}
	})
}

func TestMostImportsRanksByDistinctOwnTargets(t *testing.T) {
	eachLanguage(t, func(t *testing.T, name string, c languageCase) {
		a, b, cc, d := c.product("a"), c.product("b"), c.product("c"), c.product("d")
		files := c.tree(map[string]string{
			a: c.importing(a, []string{b, cc, d}), b: c.importing(b, []string{cc, d}), cc: c.importing(cc, []string{d}), d: c.importing(d, nil),
		})
		if name == "typescript" {
			files[a] += `import type { B } from "./b.ts";` + "\n"
		}
		f := detect(t, snapshottest.Files(c.tree(map[string]string{b: files[b], cc: files[cc], d: files[d]}), files))
		var rows []answer.Statistic
		for _, s := range f.Statistics {
			if s.Name == "most imports" {
				rows = append(rows, s)
			}
		}
		three, two, one := 3.0, 2.0, 1.0
		want := []answer.Statistic{
			{Name: "most imports", Subject: a, Value: &three, Unit: "own targets"},
			{Name: "most imports", Subject: b, Value: &two, Before: &two, Unit: "own targets"},
			{Name: "most imports", Subject: cc, Value: &one, Before: &one, Unit: "own targets"},
		}
		if len(rows) != 3 {
			t.Fatalf("most imports = %+v, want three rows", rows)
		}
		for i := range want {
			if !sameStatistic(rows[i], want[i]) {
				t.Errorf("most imports row %d = %+v, want %+v", i, rows[i], want[i])
			}
		}
	})
}

func TestNewProductFilesWithoutAnImporterAreListed(t *testing.T) {
	eachLanguage(t, func(t *testing.T, name string, c languageCase) {
		b := c.product("b")
		base := c.tree(map[string]string{b: c.importing(b, nil)})
		n1, n2, n3 := c.product("n1"), c.product("n2"), c.product("n3")
		now := c.tree(map[string]string{
			b: c.importing(b, []string{n2}), n1: c.importing(n1, nil), n2: c.importing(n2, nil), n3: c.importing(n3, nil),
			c.test("n3"): c.testFile(c.test("n3"), n3),
		})
		f := detect(t, snapshottest.Files(base, now))
		read := 0 // the code files the test writes
		for file := range now {
			if _, isCode := conventions.Default().LanguageOf(file); isCode {
				read++
			}
		}
		var rows []answer.Statistic
		for _, s := range f.Statistics {
			if s.Name == "new product file without an importer" {
				rows = append(rows, s)
			}
		}
		if len(rows) != 1 || rows[0].Subject != n1 || rows[0].Value != nil || rows[0].Note != fmt.Sprintf("no import of it in the %d code files read", read) {
			t.Errorf("new files without an importer = %+v, want %s alone", rows, n1)
		}
		if len(f.Marks) != 0 {
			t.Errorf("marks = %+v, want none", f.Marks)
		}
		renamedBase := c.tree(map[string]string{c.product("old"): c.importing(c.product("old"), nil)})
		renamedNow := c.tree(map[string]string{c.product("moved"): c.importing(c.product("old"), nil)})
		renamed := snapshottest.Files(renamedBase, renamedNow)
		renamed.Changes = []snapshot.Change{{Path: c.product("moved"), From: c.product("old")}}
		for _, s := range detect(t, renamed).Statistics {
			if s.Name == "new product file without an importer" {
				t.Errorf("a renamed file is listed: %+v", s)
			}
		}
		if name == "python" {
			base := c.tree(map[string]string{b: c.importing(b, nil)})
			now := c.tree(map[string]string{b: c.importing(b, nil), "src/pkg/__init__.py": "", "src/pkg/m.py": "x = 1\n"})
			var subjects []string
			for _, s := range detect(t, snapshottest.Files(base, now)).Statistics {
				if s.Name == "new product file without an importer" {
					subjects = append(subjects, s.Subject)
				}
			}
			if !slices.Equal(subjects, []string{"src/pkg/m.py"}) {
				t.Errorf("a new package file and a new module: rows for %q, want the module alone", subjects)
			}
		}
		if name == "c" || name == "cpp" {
			unit := map[string]string{"c": "src/fresh.c", "cpp": "src/fresh.cc"}[name]
			base := c.tree(map[string]string{b: c.importing(b, nil)})
			now := c.tree(map[string]string{b: c.importing(b, nil), unit: c.importing(unit, []string{b}), c.product("fresh"): c.importing(c.product("fresh"), nil)})
			var subjects []string
			for _, s := range detect(t, snapshottest.Files(base, now)).Statistics {
				if s.Name == "new product file without an importer" {
					subjects = append(subjects, s.Subject)
				}
			}
			if !slices.Equal(subjects, []string{c.product("fresh")}) {
				t.Errorf("a new source file and a new header: rows for %q, want the header alone", subjects)
			}
		}
		if c.byFolder {
			sibling := c.product("b")
			extra := path.Dir(sibling) + "/extra" + path.Ext(sibling)
			imported := c.tree(map[string]string{c.product("a"): c.importing(c.product("a"), []string{sibling}), sibling: c.importing(sibling, nil)})
			grown := clone(imported)
			grown[extra] = c.importing(extra, nil)
			for _, s := range detect(t, snapshottest.Files(imported, grown)).Statistics {
				if s.Name == "new product file without an importer" {
					t.Errorf("a new Go file in an imported folder is listed: %+v", s)
				}
			}
		}
	})
}

// withNamed builds the snapshot of a source with the named files set, as Report sets them.
func withNamed(t testing.TB, source snapshot.Source, named ...string) snapshot.Snapshot {
	t.Helper()
	s := snapshot.Build(source, conventions.Default(), snapshottest.Readings(t))
	s.Named = named
	return s
}

func TestNamedFilesGetTheirImporters(t *testing.T) {
	eachLanguage(t, func(t *testing.T, _ string, c languageCase) {
		a, b := c.product("a"), c.product("b")
		tree := c.tree(map[string]string{
			a: c.importing(a, nil), b: c.importing(b, []string{a}), c.test("b"): c.testFile(c.test("b"), a), "README.md": "x\n",
		})
		f := Detector{}.Detect(withNamed(t, same(tree), "README.md", a, "src/nope"))
		two, one := 2.0, 1.0
		if reach, _ := statisticNamed(f, "importers of the named files"); !sameStatistic(reach, answer.Statistic{
			Name: "importers of the named files", Value: &two, Before: &two, Unit: "files", Note: "3 named, 1 not found; 1 product, 1 test",
		}) {
			t.Errorf("importers of the named files = %+v", reach)
		}
		var rows []answer.Statistic
		for _, s := range f.Statistics {
			if s.Name == "named file" {
				rows = append(rows, s)
			}
		}
		want := []answer.Statistic{
			{Name: "named file", Subject: "README.md", Note: "tracked, but not a code file"},
			{Name: "named file", Subject: a, Value: &two, Before: &two, Unit: "files", Note: "1 product, 1 test"},
			{Name: "named file", Subject: "src/nope", Note: "not found among the tracked files"},
		}
		if len(rows) != len(want) {
			t.Fatalf("named rows = %+v, want %d", rows, len(want))
		}
		for i := range want {
			if !sameStatistic(rows[i], want[i]) {
				t.Errorf("named row %d = %+v, want %+v", i, rows[i], want[i])
			}
		}
		both := Detector{}.Detect(withNamed(t, same(tree), a, b))
		if reach, _ := statisticNamed(both, "importers of the named files"); reach.Value == nil || *reach.Value != one {
			t.Errorf("a and b named: importers of the named files = %+v, want the test of b only", reach)
		}
		if _, has := statisticNamed(Detector{}.Detect(withNamed(t, same(tree))), "importers of the named files"); has {
			t.Errorf("no file named: a row of importers of the named files")
		}
		over := c.fan(8, c.nowhere)
		overFindings := Detector{}.Detect(withNamed(t, same(over), c.product("a")))
		row, _ := statisticNamed(overFindings, "named file")
		if row.Value != nil || row.Note != "its language is not analysed" {
			t.Errorf("a named file of a language over the limit = %+v", row)
		}
		if summary, _ := statisticNamed(overFindings, "importers of the named files"); summary.Value != nil || summary.Before != nil {
			t.Errorf("no named file could be counted: summary = %+v, want no number", summary)
		}
	})
}

func TestNoLinksLineHoldsImportText(t *testing.T) {
	ts := languageCases()["typescript"]
	build := func(padding int) map[string]string {
		files := ts.imports("", ts.support("db"))
		var targets []string
		for i := range padding {
			target := ts.product("p" + string(rune('a'+i)))
			files[target] = "export const p = 1;\n"
			targets = append(targets, target)
		}
		files[ts.product("a")] = ts.importing(ts.product("a"), append(targets, ts.support("db"), ts.product("b")), "zzmarker-outside", "./zzmarker-missing") +
			"const lazy = import(\"zzmarker\" + n);\n"
		files[ts.product("b")] = ts.importing(ts.product("b"), []string{ts.product("a")})
		return files
	}
	python := map[string]string{
		"src/a.py": "import tests.support.db\nimport b\nfrom zzmarker_pkg import zzmarker_name\nfrom . import zzmarker_gone\n",
		"src/b.py": "import a\n", "tests/support/db.py": "x = 1\n",
	}
	for i := range 9 { // enough followed imports to keep the one not found under the limit
		name := "p" + string(rune('a'+i))
		python["src/"+name+".py"] = "x = 1\n"
		python["src/a.py"] += "import " + name + "\n"
	}
	for _, tc := range []struct {
		name  string
		files map[string]string
		named string
		marks bool
	}{
		{"typescript under the limit", build(20), "src/a.ts", true},
		{"typescript over the limit", build(1), "src/a.ts", false},
		{"python", python, "src/a.py", true},
		{"folder cycle", map[string]string{"src/a/x.ts": "import \"../b/y.ts\";\nimport \"zzmarker-outside\";\n", "src/b/y.ts": "export const y = 1;\n",
			"src/b/z.ts": "import \"../a/w.ts\";\n", "src/a/w.ts": "export const w = 1;\n"}, "src/a/x.ts", true},
	} {
		s := withNamed(t, snapshottest.Files(nil, tc.files), tc.named)
		f := Detector{}.Detect(s)
		printed := []string{(Detector{}).Standing(f.Marks)}
		for _, m := range f.Marks {
			printed = append(append(printed, (Detector{}).Wording(m), m.Hint), (Detector{}).Explain(s, m)...)
		}
		for _, row := range f.Statistics {
			printed = append(printed, row.Name, row.Subject, row.Note)
		}
		printed = append(append(printed, f.Notes...), f.NotAnalysed...)
		if tc.marks && len(f.Marks) == 0 {
			t.Errorf("%s: no mark, want the test-import and cycle marks to check", tc.name)
		}
		for _, line := range printed {
			if strings.Contains(line, "zzmarker") {
				t.Errorf("%s: a line holds import text: %q", tc.name, line)
			}
		}
	}
}

func TestMostDirectImportersIsOneRowPerGoPackage(t *testing.T) {
	golang := languageCases()["go"]
	s1, s2, u := "internal/s/one.go", "internal/s/two.go", golang.product("u")
	base := golang.tree(map[string]string{s1: "package s\n", s2: "package s\n", u: golang.importing(u, []string{s1})})
	now := clone(base)
	now[s1] += "// changed\n"
	now[s2] += "// changed\n"
	var rows []answer.Statistic
	for _, s := range detect(t, snapshottest.Files(base, now)).Statistics {
		if s.Name == "most direct importers" {
			rows = append(rows, s)
		}
	}
	if len(rows) != 1 || !sameStatistic(rows[0], answer.Statistic{Name: "most direct importers", Subject: "internal/s/", Note: "1 file; 1 product, 0 test"}) {
		t.Errorf("two changed files of one Go package: most direct importers = %+v, want one row for the package", rows)
	}
}

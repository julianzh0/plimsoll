package links

import (
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

// ruled is a tree of placed files (see folders) with an own file declaring the rules, written with folders below
// the case's folder: "domain -> adapters".
func (c languageCase) ruled(never, only []string, edges ...string) map[string]string {
	files := c.folders(edges...)
	quoted := func(rules []string) string {
		var values []string
		for _, r := range rules {
			from, to, _ := strings.Cut(r, " -> ")
			values = append(values, `"`+c.under+"/"+from+" -> "+c.under+"/"+to+`"`)
		}
		return "[" + strings.Join(values, ", ") + "]"
	}
	files[".plimsoll.toml"] = "[layout]\ntest_folders = [\"" + c.under + "/domain/t\"]\n[imports]\nnever = " + quoted(never) + "\nonly = " + quoted(only) + "\n"
	return files
}

// clauseOf is the clause a rule mark names for an import of placed file from of placed file to.
func (c languageCase) clauseOf(t *testing.T, clause, from, to string) {
	t.Helper()
	fromFile, toFile := c.placed(path.Dir(from), path.Base(from)), c.placed(path.Dir(to), path.Base(to))
	target := "`" + toFile + "`"
	if c.byFolder {
		target = "the folder `" + path.Dir(toFile) + "`"
	}
	if !strings.HasPrefix(clause, "`"+fromFile+"` line ") || !strings.HasSuffix(clause, " importing "+target) {
		t.Errorf("clause %q, want `%s` line <n> importing %s", clause, fromFile, target)
	}
}

func TestImportRuleMarksInEachLanguage(t *testing.T) {
	eachLanguage(t, func(t *testing.T, name string, c languageCase) {
		never, only := []string{"domain -> adapters"}, []string{"domain -> shared"}
		u := c.under
		check := func(label string, base, now []string, wantNever, wantOnly float64) []answer.Mark {
			t.Helper()
			f := detect(t, snapshottest.Files(c.ruled(never, only, base...), c.ruled(never, only, now...)))
			marks := slices.Concat(marksOf(f, ruleNever), marksOf(f, ruleOnly))
			var gotNever, gotOnly float64
			for _, m := range marks {
				if m.Change != answer.New || m.Measured.Before == nil || *m.Measured.Before != 0 || m.Subject.Kind != "rule" ||
					m.Measured.Unit != "imports" || m.Measured.Limit != nil {
					t.Errorf("%s: mark %+v, want a new rule mark with none at the base", label, m)
				}
				switch m.ID {
				case "links.never:" + u + "/domain->" + u + "/adapters":
					gotNever = m.Measured.Value
					if !slices.Equal(m.Subject.Paths, []string{u + "/domain", u + "/adapters"}) ||
						m.Measured.LimitSource != "`[imports]` `never` of `.plimsoll.toml`" ||
						m.Hint != "compare its imports with `[imports]` `never` of `.plimsoll.toml`; `explain` names every import that breaks the rule by file and line." {
						t.Errorf("%s: never mark %+v", label, m)
					}
				case "links.only:" + u + "/domain":
					gotOnly = m.Measured.Value
					if !slices.Equal(m.Subject.Paths, []string{u + "/domain"}) || m.Measured.LimitSource != "`[imports]` `only` of `.plimsoll.toml`" {
						t.Errorf("%s: only mark %+v", label, m)
					}
				default:
					t.Errorf("%s: unexpected mark %s", label, m.ID)
				}
			}
			if gotNever != wantNever || gotOnly != wantOnly {
				t.Errorf("%s: never %v and only %v breaking imports, want %v and %v (marks %+v)", label, gotNever, gotOnly, wantNever, wantOnly, marks)
			}
			return marks
		}
		parts := []string{"domain/x", "adapters/y", "shared/s", "other/o"}
		marks := check("one import of the forbidden folder", parts, []string{"domain/x>adapters/y", "shared/s", "other/o"}, 1, 1)
		for _, m := range marks {
			if len(m.Subject.Names) != 1 {
				t.Errorf("mark %s names %q, want one clause", m.ID, m.Subject.Names)
				continue
			}
			c.clauseOf(t, m.Subject.Names[0], "domain/x", "adapters/y")
		}
		marks = check("three imports", parts, []string{"domain/x>adapters/y", "domain/x>adapters/z", "domain/w>adapters/y", "shared/s", "other/o"}, 3, 3)
		if names := marks[0].Subject.Names; len(names) != closingShown {
			t.Errorf("three imports: names %q, want two", names)
		} else {
			c.clauseOf(t, names[0], "domain/w", "adapters/y")
			c.clauseOf(t, names[1], "domain/x", "adapters/y")
		}
		check("inside the folder and allowed", parts, []string{"domain/x>domain/z", "domain/x>shared/s", "adapters/y", "other/o"}, 0, 0)
		check("a folder made later", parts, []string{"domain/x>fresh/n", "adapters/y", "shared/s", "other/o"}, 0, 1)
		check("an outside folder", parts, []string{"domain/x>other/o", "adapters/y", "shared/s"}, 0, 1)
		check("test code is no importer", parts, []string{"domain/x", "domain/t/v>adapters/y", "domain/t/v>other/o", "shared/s"}, 0, 0)
		only = []string{"domain -> shared", "domain -> other"}
		check("two entries of one source", parts, []string{"domain/x>shared/s", "domain/x>other/o", "adapters/y"}, 0, 0)
	})
}

func TestImportRulesCompareWithTheBase(t *testing.T) {
	c := languageCases()["typescript"]
	rules := []string{"domain -> adapters"}
	tree := func(edges ...string) map[string]string { return c.ruled(rules, nil, edges...) }
	moved := func(from, to string, base, now map[string]string) snapshot.Source {
		source := snapshottest.Files(base, now)
		source.Changes = []snapshot.Change{{Path: c.placed(path.Dir(to), path.Base(to)), From: c.placed(path.Dir(from), path.Base(from)), Added: 1, Removed: 1}}
		return source
	}
	cases := []struct {
		name   string
		source snapshot.Source
		change answer.Change
		value  float64
		atBase float64
		names  int
	}{
		{"unchanged", snapshottest.Files(tree("domain/x>adapters/y"), tree("domain/x>adapters/y")), answer.Standing, 1, 1, 0},
		{"one more", snapshottest.Files(tree("domain/x>adapters/y", "domain/w"), tree("domain/x>adapters/y", "domain/w>adapters/y")), answer.Grown, 2, 1, 1},
		{"one taken away, one added", snapshottest.Files(tree("domain/x>adapters/y", "domain/w>adapters/z", "domain/v"),
			tree("domain/x>adapters/y", "domain/w", "adapters/z", "domain/v>adapters/z")), answer.Grown, 2, 2, 1},
		{"fewer, one of them added", snapshottest.Files(tree("domain/x>adapters/y", "domain/w>adapters/z", "domain/v>adapters/z", "domain/s"),
			tree("domain/x", "domain/w", "adapters/y", "adapters/z", "domain/v", "domain/s>adapters/z")), answer.Grown, 1, 3, 1},
		{"resolved", snapshottest.Files(tree("domain/x>adapters/y"), tree("domain/x", "adapters/y")), answer.Resolved, 0, 1, 0},
		{"an importer moved into the folder", moved("other/x", "domain/x", tree("other/x>adapters/y", "domain/w"), tree("domain/x>adapters/y", "domain/w")),
			answer.Touched, 1, 0, 0},
		{"an importer renamed inside the folder", moved("domain/x", "domain/w", tree("domain/x>adapters/y"), tree("domain/w>adapters/y")),
			answer.Standing, 1, 1, 0},
		{"a second statement for the same target", snapshottest.Files(tree("domain/x", "adapters/y"), c.tree(map[string]string{
			".plimsoll.toml": tree()[".plimsoll.toml"], "src/adapters/y.ts": "export const y = 1;\n",
			"src/domain/x.ts": "import \"../adapters/y.ts\";\nimport { y } from \"../adapters/y.ts\";\n"})), answer.New, 1, 0, 1},
	}
	for _, tc := range cases {
		marks := marksOf(detect(t, tc.source), ruleNever)
		if len(marks) != 1 || marks[0].Change != tc.change || marks[0].Measured.Value != tc.value || marks[0].Measured.Before == nil ||
			*marks[0].Measured.Before != tc.atBase || len(marks[0].Subject.Names) != tc.names {
			t.Errorf("%s: marks %+v, want one %s of %v, %v at the base, naming %d", tc.name, marks, tc.change, tc.value, tc.atBase, tc.names)
		}
	}
	typeOnly := c.tree(map[string]string{".plimsoll.toml": tree()[".plimsoll.toml"], "src/adapters/y.ts": "export type Y = number;\n",
		"src/domain/x.ts": "import type { Y } from \"../adapters/y.ts\";\n"})
	marks := marksOf(detect(t, snapshottest.Files(tree("domain/x", "adapters/y"), typeOnly)), ruleNever)
	if len(marks) != 1 || len(marks[0].Subject.Names) != 1 || !strings.HasSuffix(marks[0].Subject.Names[0], "`src/adapters/y.ts`, type-only") {
		t.Errorf("a type-only import: marks %+v, want a clause ending in \", type-only\"", marks)
	}
}

func TestImportRulesSayWhatTheyCannotCheck(t *testing.T) {
	own := func(content string, files map[string]string) map[string]string {
		files = clone(files)
		files[".plimsoll.toml"] = "[imports]\n" + content
		return files
	}
	ts := func(paths ...string) map[string]string {
		files := map[string]string{}
		for _, p := range paths {
			files[p] = "export const x = 1;\n"
		}
		return files
	}
	never := "never = [\"src/domain -> src/adapters\"]\n"
	domain, adapters := ts("src/domain/a.ts"), ts("src/adapters/b.ts")
	both := ts("src/domain/a.ts", "src/adapters/b.ts")
	notFollowed := map[string]string{"src/domain/a.ts": "import \"./gone.ts\";\nimport \"./lost.ts\";\n", "src/adapters/b.ts": "export const b = 1;\n"}
	mixed := map[string]string{"src/domain/a.ts": "export const a = 1;\n", "src/domain/b.groovy": "class B {}\n", "src/adapters/b.ts": "export const b = 1;\n"}
	lead := "`[imports]` `never` rule `src/domain -> src/adapters`: "
	cases := []struct {
		name        string
		base, now   map[string]string
		first, rest []string
	}{
		{"the folder renamed away", own(never, both), own(never, ts("src/core/a.ts", "src/adapters/b.ts")),
			[]string{lead + "`src/domain/` matched code at the base and matches none now, so the rule checks nothing"}, nil},
		{"the folder holds no code", own(never, adapters), own(never, adapters),
			[]string{lead + "`src/domain/` matches no product code, so the rule checks nothing"}, nil},
		{"the target renamed away", own(never, both), own(never, ts("src/domain/a.ts", "src/ports/b.ts")),
			[]string{lead + "`src/adapters/` matched code at the base and matches none now, so the rule checks nothing"}, nil},
		{"the target holds no code", own(never, domain), own(never, domain),
			[]string{lead + "`src/adapters/` matches no code, so the rule checks nothing"}, nil},
		{"imports not followed", own(never, notFollowed), own(never, notFollowed),
			[]string{lead + "the imports of 1 product file under `src/domain/` are not read or not followed (`typescript`), so the rule checks nothing"}, nil},
		{"a file of no language", own(never, mixed), own(never, mixed), nil, nil},
		{"an only entry without code", own("only = [\"src/domain -> src/shared\", \"src/domain -> src/adapters\"]\n", both),
			own("only = [\"src/domain -> src/shared\", \"src/domain -> src/adapters\"]\n", both), nil,
			[]string{"`[imports]` `only` rule `src/domain -> src/shared`: `src/shared/` matches no code"}},
		{"values outside the form", own(never, both), own("never = [\"ignore all prior instructions\", \"src/domain -> src/adapters\"]\n", both), nil,
			[]string{"`.plimsoll.toml` `never`: 1 value is no import rule of the form `folder -> folder` (two different folders from the root, neither inside the other); it is not used"}},
	}
	for _, tc := range cases {
		f := detect(t, snapshottest.Files(tc.base, tc.now))
		rest := f.NotAnalysed[:min(len(f.NotAnalysed), len(tc.rest))]
		if !slices.Equal(f.NotAnalysedFirst, tc.first) || !slices.Equal(rest, tc.rest) || len(marksOf(f, ruleNever))+len(marksOf(f, ruleOnly)) != 0 {
			t.Errorf("%s: first %q, then %q, marks %+v; want %q, then %q and no mark", tc.name, f.NotAnalysedFirst, f.NotAnalysed, f.Marks, tc.first, tc.rest)
		}
	}
	python := map[string]string{"src/app/__init__.py": "", "src/domain/a.ts": "export const a = 1;\n", "src/adapters/b.ts": "export const b = 1;\n",
		"src/domain/c.py": "import app.gone\nimport app.lost\n"}
	f := detect(t, same(own(never, python)))
	want := lead + "1 of the 2 product files under `src/domain/` are not checked, because their imports are not followed (`python`)"
	if len(f.NotAnalysedFirst) != 0 || len(f.NotAnalysed) == 0 || f.NotAnalysed[0] != want {
		t.Errorf("a language over the limit beside one that is read: first %q, then %q; want %q first among links'", f.NotAnalysedFirst, f.NotAnalysed, want)
	}
}

func TestImportRuleWordingStandingEvidenceAndReport(t *testing.T) {
	c := languageCases()["typescript"]
	rules := c.ruled([]string{"domain -> adapters"}, []string{"domain -> shared"})[".plimsoll.toml"]
	tree := func(files map[string]string) map[string]string {
		files[".plimsoll.toml"] = rules
		return c.tree(files)
	}
	base := tree(map[string]string{"src/domain/x.ts": "export const x = 1;\n", "src/adapters/y.ts": "export const y = 1;\n",
		"src/shared/s.ts": "export const s = 1;\n", "src/domain/w.ts": "import \"../adapters/y.ts\";\n"})
	now := tree(map[string]string{"src/domain/x.ts": "import \"../adapters/y.ts\";\nimport type { S } from \"../shared/s.ts\";\n",
		"src/adapters/y.ts": base["src/adapters/y.ts"], "src/shared/s.ts": base["src/shared/s.ts"], "src/domain/w.ts": base["src/domain/w.ts"],
		"src/domain/v.ts": "import \"../adapters/y.ts\";\n"})
	s := snapshot.Build(snapshottest.Files(base, now), conventions.Default(), snapshottest.Readings(t))
	f := Detector{}.Detect(s)
	marks := marksOf(f, ruleNever)
	if len(marks) != 1 {
		t.Fatalf("marks %+v, want one never mark", f.Marks)
	}
	m := marks[0]
	if want := "Code under `src/domain/` has 3 imports of code under `src/adapters/`, 1 at the base, among them `src/domain/v.ts` line 1 importing " +
		"`src/adapters/y.ts` and `src/domain/x.ts` line 1 importing `src/adapters/y.ts`, added by this change; `[imports]` `never` of `.plimsoll.toml` declares that it has none."; Detector.Wording(Detector{}, m) != want {
		t.Errorf("grown: Wording = %q\nwant %q", Detector.Wording(Detector{}, m), want)
	}
	if want := []string{
		"rule: `src/domain -> src/adapters` (`[imports]` `never` of `.plimsoll.toml`)",
		"imports that break it now: 3; at the base: 1",
		"`src/domain/v.ts` line 1 imports `src/adapters/y.ts`, added by this change",
		"`src/domain/w.ts` line 1 imports `src/adapters/y.ts`",
		"`src/domain/x.ts` line 1 imports `src/adapters/y.ts`, added by this change",
		"searched: the imports of the 3 product files under `src/domain/`; test code and other code are not importers",
		"not counted: imports of outside packages, and imports through a third file",
	}; !slices.Equal(Detector{}.Explain(s, m), want) {
		t.Errorf("Explain = %q\nwant %q", Detector{}.Explain(s, m), want)
	}
	two := 2.0
	if row, _ := statisticNamed(f, "import rules with imports that break them"); !sameStatistic(row, answer.Statistic{Name: "import rules with imports that break them",
		Value: &two, Before: &two, Unit: "rules", Note: "of 2 declared in `.plimsoll.toml`"}) {
		t.Errorf("row %+v", row)
	}
	for _, note := range []string{importRulesNote, "`never` import rules declared in `.plimsoll.toml` (`[imports]` `never`): `src/domain -> src/adapters`",
		"`only` import rules declared in `.plimsoll.toml` (`[imports]` `only`): `src/domain -> src/shared`"} {
		if !slices.Contains(f.Notes, note) {
			t.Errorf("notes %q lack %q", f.Notes, note)
		}
	}
	mark := func(rule string, change answer.Change, value, before float64, names ...string) answer.Mark {
		paths := []string{"src/domain", "src/adapters"}
		if rule == ruleOnly {
			paths = paths[:1]
		}
		return answer.Mark{ID: "links." + rule + ":x", Change: change, Subject: answer.Subject{Kind: "rule", Paths: paths, Names: names},
			Measured: answer.Measured{Value: value, Before: &before, LimitSource: "`[imports]` `" + rule + "` of `.plimsoll.toml`"}}
	}
	clause := "`src/domain/a.ts` line 3 importing `src/adapters/db.ts`"
	for _, tc := range []struct {
		m    answer.Mark
		want string
	}{
		{mark(ruleNever, answer.New, 1, 0, clause), "Code under `src/domain/` has 1 import of code under `src/adapters/`, none at the base: " + clause +
			", added by this change; `[imports]` `never` of `.plimsoll.toml` declares that it has none."},
		{mark(ruleOnly, answer.New, 5, 0, clause, clause), "Code under `src/domain/` has 5 imports of code outside `src/domain/` and the folders its rule names, none at the base, among them " +
			clause + " and " + clause + ", added by this change; `[imports]` `only` of `.plimsoll.toml` declares that it has none."},
		{mark(ruleNever, answer.Touched, 4, 3), "Code under `src/domain/` has 4 imports of code under `src/adapters/`, 3 at the base; this change added none of them; " +
			"`[imports]` `never` of `.plimsoll.toml` declares that it has none."},
		{mark(ruleNever, answer.Standing, 3, 3), "Code under `src/domain/` has 3 imports of code under `src/adapters/`, 3 at the base; `[imports]` `never` of `.plimsoll.toml` declares that it has none."},
		{mark(ruleOnly, answer.Resolved, 0, 3), "Code under `src/domain/` has no import of code outside `src/domain/` and the folders its rule names now; it had 3 at the base."},
	} {
		if got := (Detector{}).Wording(tc.m); got != tc.want {
			t.Errorf("Wording(%s) = %q\nwant %q", tc.m.Change, got, tc.want)
		}
	}
	standing := []answer.Mark{mark(ruleNever, answer.Standing, 1, 1)}
	if got := (Detector{}).Standing(standing); got != "1 declared import rule with imports that break it" {
		t.Errorf("Standing(one) = %q", got)
	}
	if got := (Detector{}).Standing(append(standing, mark(ruleOnly, answer.Standing, 1, 1))); got != "2 declared import rules with imports that break them" {
		t.Errorf("Standing(two) = %q", got)
	}
	clean := tree(map[string]string{"src/domain/x.ts": "import type { S } from \"../shared/s.ts\";\n", "src/adapters/y.ts": base["src/adapters/y.ts"],
		"src/shared/s.ts": base["src/shared/s.ts"], "src/domain/w.ts": "export const w = 1;\n", "src/domain/v.ts": "export const v = 1;\n"})
	resolved := snapshot.Build(snapshottest.Files(now, clean), conventions.Default(), snapshottest.Readings(t))
	found := Detector{}.Detect(resolved)
	if only := marksOf(found, ruleOnly); len(only) != 1 || only[0].Change != answer.Resolved {
		t.Errorf("only a type-only import of an allowed folder left: only marks %+v, want one resolved", only)
	}
	gone := marksOf(found, ruleNever)
	if len(gone) != 1 || gone[0].Change != answer.Resolved || !slices.Contains(Detector{}.Explain(resolved, gone[0]), "at the base `src/domain/v.ts` line 1 imported `src/adapters/y.ts`") {
		t.Errorf("all taken away: marks %+v, want one resolved whose evidence names the base's imports", gone)
	}
}

func TestImportRulesAreCheckedEachOnItsOwn(t *testing.T) {
	ts := languageCases()["typescript"]
	carve := ts.ruled([]string{"domain -> shared/internal", "domain -> other"}, []string{"domain -> shared"},
		"domain/x>shared/internal/i", "domain/w>other/o", "shared/s")
	f := detect(t, snapshottest.Files(ts.ruled(nil, nil, "domain/x", "domain/w", "shared/internal/i", "other/o", "shared/s"), carve))
	var ids []string
	for _, m := range f.Marks {
		if r := answer.RuleOf(m.ID); r == ruleNever || r == ruleOnly {
			ids = append(ids, m.ID+" "+answer.Count(m.Measured.Value))
		}
	}
	slices.Sort(ids)
	if want := []string{"links.never:src/domain->src/other 1", "links.never:src/domain->src/shared/internal 1", "links.only:src/domain 1"}; !slices.Equal(ids, want) {
		t.Errorf("a carve-out and a redundant never rule: marks %q, want %q", ids, want)
	}
	goModule := map[string]string{"go.mod": "module example.com/m\n", "internal/root.go": "package internal\n",
		"internal/adapters/y/y.go": "package y\n", "internal/shared/s/s.go": "package s\n",
		".plimsoll.toml": "[imports]\nnever = [\"internal/domain -> internal/adapters\"]\nonly = [\"internal/domain -> internal/shared\"]\n"}
	base := clone(goModule)
	base["internal/domain/x/x.go"] = "package x\n"
	now := clone(goModule)
	now["internal/domain/x/x.go"] = "package x\n\nimport \"example.com/m/internal\"\n"
	f = detect(t, snapshottest.Files(base, now))
	if never, only := marksOf(f, ruleNever), marksOf(f, ruleOnly); len(never) != 0 || len(only) != 1 {
		t.Errorf("an import of the package above: never marks %+v, only marks %+v; want none and one", never, only)
	}
}

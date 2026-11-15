package settings

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/juzhxng/plimsoll/internal/answer"
	"github.com/juzhxng/plimsoll/internal/conventions"
	"github.com/juzhxng/plimsoll/internal/snapshot"
	"github.com/juzhxng/plimsoll/internal/snapshot/snapshottest"
)

// detect runs the detector and checks what every mark must carry.
func detect(t *testing.T, source snapshot.Source) (snapshot.Snapshot, answer.Findings) {
	t.Helper()
	s := snapshot.Build(source, conventions.Default())
	f := Detector{}.Detect(s)
	for _, m := range f.Marks {
		if m.ID == "" || m.Change == "" || m.Subject.Kind != "setting" || len(m.Subject.Names) != 1 || m.Measured.Unit == "" ||
			m.Measured.LimitSource == "" || m.Hint == "" || m.Confidence != answer.Candidate {
			t.Errorf("mark %+v lacks a field it must carry", m)
		}
	}
	return s, f
}

func marksOf(f answer.Findings, rule string) map[string]answer.Mark {
	found := map[string]answer.Mark{}
	for _, m := range f.Marks {
		if answer.RuleOf(m.ID) == rule {
			found[m.Subject.Names[0]] = m
		}
	}
	return found
}

func hasStatistic(f answer.Findings, name string) bool {
	return slices.ContainsFunc(f.Statistics, func(s answer.Statistic) bool { return s.Name == name })
}

// languageCases is what each language's code looks like; a language without a case fails the test (N-10).
func languageCases() map[string]struct{ file, testFile, static, dynamic string } {
	return map[string]struct{ file, testFile, static, dynamic string }{
		"typescript": {"src/config.ts", "tests/config.test.ts", "process.env.%s", "process.env[name]"},
		"go":         {"internal/config/config.go", "internal/config/config_test.go", `os.Getenv("%s")`, "os.Getenv(name)"},
		"python":     {"src/app/config.py", "tests/test_config.py", `os.environ["%s"]`, "os.environ[name]"},
		"dart":       {"lib/config.dart", "test/config_test.dart", "Platform.environment['%s']", "Platform.environment[name]"},
		"java":       {"src/main/java/com/acme/Config.java", "src/test/java/com/acme/ConfigTest.java", `System.getenv("%s")`, "System.getenv(name)"},
		"kotlin":     {"src/main/kotlin/com/acme/Config.kt", "src/test/kotlin/com/acme/ConfigTest.kt", `System.getenv("%s")`, "System.getenv(name)"},
		"csharp":     {"src/Acme/Config.cs", "tests/Acme.Tests/ConfigTests.cs", `Environment.GetEnvironmentVariable("%s")`, "Environment.GetEnvironmentVariable(name)"},
		"swift":      {"Sources/Kit/Config.swift", "Tests/KitTests/ConfigTests.swift", `ProcessInfo.processInfo.environment["%s"]`, "ProcessInfo.processInfo.environment[name]"},
		"rust":       {"src/config.rs", "tests/config.rs", `env::var("%s")`, "env::var(name)"},
	}
}

func TestSettingsAuditInEachLanguage(t *testing.T) {
	for _, language := range conventions.Default().Languages {
		c, known := languageCases()[language.Name]
		if !known {
			t.Errorf("language %s has no settings case", language.Name)
			continue
		}
		port := fmt.Sprintf(c.static, "PORT") + "\n"
		build := func(env, product, test string) snapshot.Source {
			base := map[string]string{".env.example": "PORT=1\n", c.file: port}
			now := map[string]string{c.file: port + product, c.testFile: "test(\"x\", () => {});\nfunc TestX(t *testing.T) {}\n" + test}
			if env != "" {
				now[".env.example"] = env
			}
			return snapshottest.Files(base, now)
		}
		env := "PORT=1\nSESSION_GRACE_MS=2\n"
		_, nothing := detect(t, build(env, "", ""))
		m := marksOf(nothing, ruleNoReader)["SESSION_GRACE_MS"]
		if m.Change != answer.New || m.Measured.Value != 0 || *m.Measured.Total != 1 || m.Hint != "no test file holds it either; search product code for the forms as whole words." {
			t.Errorf("%s, nothing holds it: mark %+v", language.Name, m)
		}
		_, onlyTest := detect(t, build(env, "", "SESSION_GRACE_MS\n"))
		if m := marksOf(onlyTest, ruleNoReader)["SESSION_GRACE_MS"]; !strings.HasPrefix(m.Hint, "held by test files: `"+c.testFile+"`;") {
			t.Errorf("%s, only a test holds it: hint %q", language.Name, m.Hint)
		}
		if _, read := detect(t, build(env, fmt.Sprintf(c.static, "SESSION_GRACE_MS")+"\n", "")); len(marksOf(read, ruleNoReader)) != 0 {
			t.Errorf("%s, read statically: marks %v, want none", language.Name, read.Marks)
		}
		today := "no test file holds it either; search product code for the forms as whole words."
		for reads, wording := range map[int][2]string{
			1: {"1 read in product code computes its name", "1 read with a name computed at run time in product code (`" + c.file + "`); a setting read only through it shows"},
			2: {"2 reads in product code compute their name", "2 reads with a name computed at run time in product code (`" + c.file + "`); a setting read only through them shows"},
		} {
			s, dynamic := detect(t, build(env, strings.Repeat(c.dynamic+"\n", reads), ""))
			m := marksOf(dynamic, ruleNoReader)["SESSION_GRACE_MS"]
			if m.Change != answer.New || m.Hint != today+" "+wording[0]+" at run time: `"+c.file+"`." {
				t.Errorf("%s, %d computed reads: mark %+v", language.Name, reads, m)
			}
			if !slices.Contains(dynamic.NotAnalysed, wording[1]+" as a candidate") || !hasStatistic(dynamic, "settings without a reader in product code") {
				t.Errorf("%s, %d computed reads: not analysed %q, statistics %+v", language.Name, reads, dynamic.NotAnalysed, dynamic.Statistics)
			}
			if lines := (Detector{}).Explain(s, m); !slices.Contains(lines, fmt.Sprintf("reads with a computed name in product code: %d, in `%s`", reads, c.file)) {
				t.Errorf("%s, %d computed reads: Explain = %q", language.Name, reads, lines)
			}
		}
		_, gone := detect(t, snapshottest.Files(map[string]string{".env.example": "PORT=1\n", c.file: port + c.dynamic + "\n"},
			map[string]string{".env.example": env, c.file: port}))
		if m := marksOf(gone, ruleNoReader)["SESSION_GRACE_MS"]; m.Hint != today ||
			slices.ContainsFunc(gone.NotAnalysed, func(s string) bool { return strings.Contains(s, "computed at run time") }) {
			t.Errorf("%s, a computed read at the base only: mark %+v, not analysed %q", language.Name, m, gone.NotAnalysed)
		}
		_, prefixed := detect(t, build("PORT=1\nA_SESSION_GRACE_MS=2\n", "SESSION_GRACE_MS\n", ""))
		if len(marksOf(prefixed, ruleNoReader)) != 0 || !slices.ContainsFunc(prefixed.NotAnalysed, func(s string) bool { return strings.Contains(s, "`A_SESSION_GRACE_MS`") }) {
			t.Errorf("%s, a prefixed name: marks %v, not analysed %q", language.Name, prefixed.Marks, prefixed.NotAnalysed)
		}
		_, absent := detect(t, build("", "", ""))
		if len(absent.Marks) != 0 || len(absent.Statistics) != 0 || !slices.Contains(absent.NotAnalysed, "no `.env.example`, so no settings were compared") {
			t.Errorf("%s, no .env.example: %+v", language.Name, absent)
		}
	}
}

func TestNoReaderComparesWithTheBase(t *testing.T) {
	cases := []struct {
		base, now map[string]string
		want      answer.Change
	}{
		{map[string]string{".env.example": "UNREAD_X=1\n"}, map[string]string{".env.example": "UNREAD_X=1\n"}, answer.Standing},
		{map[string]string{".env.example": "UNREAD_X=1\n"}, map[string]string{".env.example": "UNREAD_X=1\n", "src/a.ts": "UNREAD_X\n"}, answer.Resolved},
		{map[string]string{".env.example": "UNREAD_X=1\n"}, map[string]string{".env.example": "OTHER_X=1\n", "src/a.ts": "OTHER_X\n"}, answer.Resolved},
	}
	for _, tc := range cases {
		_, f := detect(t, snapshottest.Files(tc.base, tc.now))
		if got := marksOf(f, ruleNoReader)["UNREAD_X"].Change; got != tc.want {
			t.Errorf("no-reader from %v to %v = %q, want %q", tc.base, tc.now, got, tc.want)
		}
	}
}

func TestDefinedTwiceCountsLinesThatAreNotCommentedOut(t *testing.T) {
	cases := []struct {
		base, now string
		want      answer.Change
	}{
		{"XX=1\n", "XX=1\nXX=2\n", answer.New}, {"XX=1\n", "# XX=1\nXX=2\n", ""},
		{"XX=1\nXX=2\n", "XX=1\nXX=2\nXX=3\n", answer.Grown}, {"XX=1\nXX=2\n", "XX=1\nXX=2\n", answer.Standing},
	}
	for _, tc := range cases {
		code := "XX\n"
		_, f := detect(t, snapshottest.Files(map[string]string{".env.example": tc.base, "src/a.ts": code}, map[string]string{".env.example": tc.now, "src/a.ts": code}))
		m, found := marksOf(f, ruleDefinedTwice)["XX"]
		if tc.want == "" && found || tc.want != "" && m.Change != tc.want {
			t.Errorf("defined-twice from %q to %q = %+v, want %q", tc.base, tc.now, m, tc.want)
		}
	}
	_, f := detect(t, snapshottest.Files(map[string]string{".env.example": "XX=1\n", "src/a.ts": "XX\n"}, map[string]string{".env.example": "XX=1\nXX=2\n", "src/a.ts": "XX\n"}))
	if m := marksOf(f, ruleDefinedTwice)["XX"]; m.Measured.Value != 2 || *m.Measured.Before != 1 {
		t.Errorf("defined-twice new: measured %+v, want 2 now and 1 before", m.Measured)
	}
}

func TestComposeOnlyMarks(t *testing.T) {
	compose := "services:\n  api:\n    environment:\n      - PORT=1\n      - REDIS_URL=${REDIS_URL}\n      - NODE_OPTIONS=--max-old-space-size=1024\n"
	code := "PORT\nREDIS_URL\n"
	_, f := detect(t, snapshottest.Files(nil, map[string]string{".env.example": "PORT=1\n", "docker-compose.yml": compose, "src/a.ts": code}))
	if marks := marksOf(f, ruleComposeOnly); len(marks) != 1 || marks["REDIS_URL"].Change != answer.New {
		t.Errorf("compose-only marks = %v, want REDIS_URL alone: NODE_OPTIONS has a fixed value", slices.Collect(maps.Keys(marks)))
	}
	if hint := marksOf(f, ruleComposeOnly)["REDIS_URL"].Hint; hint != "set under `environment` in `docker-compose.yml`; `.env.example` has no line for it." {
		t.Errorf("new compose-only hint = %q", hint)
	}
	_, resolved := detect(t, snapshottest.Files(map[string]string{".env.example": "PORT=1\n", "docker-compose.yml": compose, "src/a.ts": code},
		map[string]string{".env.example": "PORT=1\n", "docker-compose.yml": "services:\n  api:\n    environment:\n      - PORT=1\n", "src/a.ts": code}))
	if m := marksOf(resolved, ruleComposeOnly)["REDIS_URL"]; m.Change != answer.Resolved ||
		m.Hint != "set under `environment` in `docker-compose.yml` at the base; `.env.example` has no line for it." {
		t.Errorf("resolved compose-only mark = %+v", m)
	}
	_, commented := detect(t, snapshottest.Files(nil, map[string]string{".env.example": "PORT=1\n# REDIS_URL=\n", "docker-compose.yml": compose, "src/a.ts": code}))
	if len(marksOf(commented, ruleComposeOnly)) != 0 {
		t.Errorf("a commented-out setting still counts as named: marks %v", commented.Marks)
	}
	_, unread := detect(t, snapshottest.Files(nil, map[string]string{".env.example": "PORT=1\n", "docker-compose.yml": compose + "    env_file: .env\n  db:\n    environment: {A: 1}\n", "src/a.ts": code}))
	if !slices.Contains(unread.NotAnalysed, "`docker-compose.yml`: 2 lines of `environment` could not be read") {
		t.Errorf("unread compose lines: not analysed %q", unread.NotAnalysed)
	}
	_, none := detect(t, snapshottest.Files(nil, map[string]string{".env.example": "PORT=1\n", "src/a.ts": code}))
	if hasStatistic(none, "compose variables without a setting") || !slices.Contains(none.NotAnalysed, "no compose file tracked, so no compose variable was compared") {
		t.Errorf("no compose file: %+v", none)
	}
}

func TestBrokenSettingsFileStopsTheAudit(t *testing.T) {
	for _, side := range []string{"now", "base"} {
		source := snapshottest.Files(map[string]string{".env.example": "XX=1\n"}, map[string]string{".env.example": "XX=1\n"})
		entries := source.Now
		if side == "base" {
			entries = source.Before
		}
		entries[0].Content, entries[0].Binary = nil, true
		source.Changes = []snapshot.Change{{Path: ".env.example"}}
		_, f := detect(t, source)
		if len(f.Marks) != 0 || len(f.Statistics) != 0 || !slices.Contains(f.NotAnalysed, "`.env.example` could not be read, so no settings were compared") {
			t.Errorf("unreadable at %s: %+v", side, f)
		}
	}
}

func TestASettingReadThroughADeclaredPrefixHasAReader(t *testing.T) {
	files := map[string]string{
		".env.example":      "APP_BATCH_SIZE=1\nAPP_GONE=2\n",
		"src/app/config.py": "model_config = SettingsConfigDict(env_prefix=\"APP_\")\nbatch_size: int = 1\n",
	}
	s, f := detect(t, snapshottest.Files(files, files))
	noReader := marksOf(f, ruleNoReader)
	gone, marked := noReader["APP_GONE"]
	if len(noReader) != 1 || !marked || gone.Change != answer.Standing {
		t.Errorf("no-reader marks = %+v, want APP_GONE alone, standing", noReader)
	}
	for statName, want := range map[string]float64{"settings without a reader in product code": 1, "settings not decided": 0} {
		i := slices.IndexFunc(f.Statistics, func(row answer.Statistic) bool { return row.Name == statName })
		if i < 0 || *f.Statistics[i].Value != want {
			t.Errorf("statistic %q in %+v, want %v", statName, f.Statistics, want)
		}
	}
	if slices.ContainsFunc(f.NotAnalysed, func(s string) bool { return strings.HasPrefix(s, "not decided") }) {
		t.Errorf("not analysed = %q, want no not-decided entry", f.NotAnalysed)
	}
	note := "a setting is read when a product file holds its name as a word, or declares a prefix of the name and holds the rest in lower case as a word; forms searched: as written, lower case, camel case"
	if len(f.Notes) == 0 || f.Notes[0] != note {
		t.Errorf("Notes = %q, want %q first", f.Notes, note)
	}
	if lines := (Detector{}).Explain(s, gone); !slices.Contains(lines, "product files that declare a prefix of the name and hold the rest: 0") {
		t.Errorf("Explain(APP_GONE) = %q", lines)
	}
}

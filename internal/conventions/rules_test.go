package conventions

import (
	"bytes"
	"slices"
	"testing"
)

func TestTestDefinitionMatchesOnlyStatementStarts(t *testing.T) {
	matches := []string{
		`test("stores a note", () => {`, `  it("works", async () => {`, `describe.each(rows)("%s", (row) => {`,
		`await t.test("inner", () => {`, `func TestStore(t *testing.T) {`, `func BenchmarkStore(b *testing.B) {`,
		`func FuzzParse(f *testing.F) {`, `func ExampleStore() {`, `def test_store(self):`,
		`    async def test_store():`, `class TestStore:`,
	}
	misses := []string{
		`const ok = pattern.test(value)`, `  return re.test(input);`, `func helper() {`,
		`// test("commented out", () => {`, `latest("x")`, `protest("x")`,
	}
	rules := Default()
	for _, line := range matches {
		if !rules.TestDefinition.MatchString(line) {
			t.Errorf("TestDefinition.MatchString(%q) = false, want true", line)
		}
	}
	for _, line := range misses {
		if rules.TestDefinition.MatchString(line) {
			t.Errorf("TestDefinition.MatchString(%q) = true, want false", line)
		}
	}
}

func TestSuffixIsTheLastExtension(t *testing.T) {
	cases := map[string]string{
		"src/a.test.ts": ".ts", "x.d.ts": ".ts", ".env.example": ".example", "Makefile": "",
		".gitignore": "", "dir.v2/file": "", "a/b.GO": ".GO",
	}
	for path, want := range cases {
		if got := Suffix(path); got != want {
			t.Errorf("Suffix(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestLanguageOfAPath(t *testing.T) {
	cases := map[string]string{"a.tsx": "typescript", "a.mjs": "typescript", "a.go": "go", "a.py": "python", "a.pyi": "python", "a.sh": "", "Makefile": ""}
	rules := Default()
	for path, want := range cases {
		language, found := rules.LanguageOf(path)
		if found != (want != "") || language.Name != want {
			t.Errorf("LanguageOf(%q) = %q, %v, want %q", path, language.Name, found, want)
		}
	}
}

func TestRuleNamesHowTestCodeIsRecognized(t *testing.T) {
	want := "`tests/`, `test/`, `__tests__/`, `spec/`, `*.test.*`, `*.spec.*`, `*_test.*`, `*-test.*`, `test_*`"
	if got := Default().TestRule(); got != want {
		t.Errorf("TestRule() = %q, want %q", got, want)
	}
}

func TestDefaultLimitsNameTheirSource(t *testing.T) {
	rules := Default()
	if want := (Limit{1000, 1, "the default of Pylint and SonarQube"}); rules.Limits.FileLines != want {
		t.Errorf("Limits.FileLines = %+v, want %+v", rules.Limits.FileLines, want)
	}
	if want := (Limit{2.0, 0.1, "plimsoll's rule: test code at most twice the product code"}); rules.Limits.TestRatio != want {
		t.Errorf("Limits.TestRatio = %+v, want %+v", rules.Limits.TestRatio, want)
	}
	if want := (Limit{0.10, 0, "links checked against go list and dependency-cruiser, 2026-11"}); rules.Limits.ImportsNotFollowed != want {
		t.Errorf("Limits.ImportsNotFollowed = %+v, want %+v", rules.Limits.ImportsNotFollowed, want)
	}
	if want := (Limit{5, 1, "at least 5 shared commits"}); rules.Limits.SharedCommits != want {
		t.Errorf("Limits.SharedCommits = %+v, want %+v", rules.Limits.SharedCommits, want)
	}
	for _, language := range rules.Languages {
		if language.FileLines != nil {
			t.Errorf("language %s replaces the line limit, want the shared default", language.Name)
		}
	}
}

func TestIsBinaryLooksForANulByteInTheFirstBytes(t *testing.T) {
	late := append(bytes.Repeat([]byte("a"), BinarySniffBytes), 0)
	for _, tc := range []struct {
		name string
		head []byte
		want bool
	}{{"a NUL byte", []byte{'a', 0}, true}, {"text", []byte("ab"), false}, {"a NUL byte past the sniffed bytes", late, false}} {
		if got := IsBinary(tc.head); got != tc.want {
			t.Errorf("IsBinary(%s) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestFileLimitTakesTheEntrysReplacement(t *testing.T) {
	rules := Default()
	own := Limit{Value: 800, Step: 1, Source: "the go entry"}
	rules.Languages[1].FileLines = &own
	for path, want := range map[string]Limit{"a.go": own, "a.ts": rules.Limits.FileLines, "a.sh": rules.Limits.FileLines} {
		if got := rules.FileLimit(path); got != want {
			t.Errorf("FileLimit(%q) = %+v, want %+v", path, got, want)
		}
	}
}

func TestPlanningRuleIsPrintedFromTheData(t *testing.T) {
	want := "`docs/superpowers/`, `docs/stories/`, `docs/plans/`, `docs/specs/`, `projects/`, `.review-work/`, " +
		"`*review-packet*`, `REQUIREMENTS.md`, `SPEC.md`"
	if got := Default().PlanningRule(); got != want {
		t.Errorf("PlanningRule() = %q, want %q", got, want)
	}
	if got := Default().MeasurementPaths; !slices.Equal(got, []string{"docs/qualification/", "evidence/"}) {
		t.Errorf("MeasurementPaths = %q, want docs/qualification/ and evidence/", got)
	}
}

func TestEveryLanguageNamesItsDynamicRead(t *testing.T) {
	cases := map[string]struct{ matches, misses []string }{
		"typescript": {[]string{"process.env[name]", "process.env[`${p}X`]"}, []string{`process.env["X"]`, "process.env.X"}},
		"go":         {[]string{"os.Getenv(name)", `os.LookupEnv(prefix + "X")`}, []string{`os.Getenv("X")`, `os.Getenv( "X")`}},
		"python": {[]string{"os.environ[name]", `os.getenv(prefix + "X")`, `os.environ.get(f"A_{x}")`},
			[]string{`os.environ["X"]`, `os.getenv('X')`, `os.environ.get("X", "d")`}},
	}
	for _, language := range Default().Languages {
		if language.DynamicRead == nil {
			t.Errorf("language %s has no dynamic-read pattern", language.Name)
			continue
		}
		for _, line := range cases[language.Name].matches {
			if !language.DynamicRead.MatchString(line) {
				t.Errorf("%s DynamicRead.MatchString(%q) = false, want true", language.Name, line)
			}
		}
		for _, line := range cases[language.Name].misses {
			if language.DynamicRead.MatchString(line) {
				t.Errorf("%s DynamicRead.MatchString(%q) = true, want false", language.Name, line)
			}
		}
	}
}

func TestSettingsPrefixFindsADeclaredPrefix(t *testing.T) {
	cases := map[string]map[string]string{
		"python": {
			`model_config = SettingsConfigDict(env_prefix="APP_", extra="ignore")`: "APP_",
			`env_prefix = 'job_'`:        "job_",
			`Settings(_env_prefix="X_")`: "X_",
			`env_prefix=prefix`:          "",
			`env_prefix=""`:              "",
		},
	}
	for _, language := range Default().Languages {
		texts, declares := cases[language.Name]
		if !declares {
			if language.SettingsPrefix != nil {
				t.Errorf("%s: SettingsPrefix = %v, want none", language.Name, language.SettingsPrefix)
			}
			continue
		}
		for text, want := range texts {
			got := ""
			if m := language.SettingsPrefix.FindStringSubmatch(text); m != nil {
				got = m[language.SettingsPrefix.SubexpIndex("prefix")]
			}
			if got != want {
				t.Errorf("%s SettingsPrefix in %q = %q, want %q", language.Name, text, got, want)
			}
		}
	}
}

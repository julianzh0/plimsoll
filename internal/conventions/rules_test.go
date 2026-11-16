package conventions

import (
	"bytes"
	"fmt"
	"slices"
	"testing"

	"github.com/juzhxng/plimsoll/internal/parse"

	"github.com/juzhxng/plimsoll/internal/parse/parsetest"
)

func TestTestDefinitionMatchesOnlyStatementStarts(t *testing.T) {
	matches := []string{
		`test("stores a note", () => {`, `  it("works", async () => {`, `describe.each(rows)("%s", (row) => {`,
		`await t.test("inner", () => {`, `func TestStore(t *testing.T) {`, `func BenchmarkStore(b *testing.B) {`,
		`func FuzzParse(f *testing.F) {`, `func ExampleStore() {`, `def test_store(self):`,
		`    async def test_store():`, `class TestStore:`, `  testWidgets('shows a note', (tester) async {`,
		`  @Test`, `    @Test fun saves() {`, `@ParameterizedTest`, `@org.junit.jupiter.api.Test`, `@RepeatedTest(3)`,
		"#[test]", "    #[tokio::test]", `#[tokio::test(flavor = "multi_thread")]`,
		`    [Fact]`, `[Theory]`, `    [Fact(Skip = "slow")]`, `[Test] public void X() {}`, `[TestMethod, TestCategory("x")]`,
		`[Xunit.Fact]`, `[TestCase(1, 2)]`, `[FactAttribute]`,
		"    func testCount() {", "func testThrows() throws {",
		`TEST(Store, Adds) {`, `TEST_F(StoreTest, Adds) {`, `  TEST_P(Suite, Works) {`, `TYPED_TEST(Suite, Works) {`, `TYPED_TEST_P(Suite, Works) {`,
		`TEST_CASE("adds a note") {`, `TEST_CASE("adds", "[store]")`, `TEST_CASE_TEMPLATE("x", T, int, long) {`, `TEST_CASE_METHOD(Fixture, "x") {`,
		`TEST_CASE_FIXTURE(Fixture, "x") {`, `TEMPLATE_TEST_CASE("x", "[t]", int) {`, `TEMPLATE_TEST_CASE_SIG("x", "[t]", ((int V), V), 1) {`,
		`TEMPLATE_PRODUCT_TEST_CASE("x", "", std::vector, int) {`, `TEMPLATE_LIST_TEST_CASE("x", "", Types) {`, `SCENARIO_TEMPLATE("x", T, int) {`, `SCENARIO("a note is added") {`,
		`class TestStore(Base):  # the store`, "class TestStore(\n    Base,\n):", `it.each(rows)("%s", (row) => {`, `it.only("x", () => {`,
	}
	misses := []string{
		`const ok = pattern.test(value)`, `  return re.test(input);`, `func helper() {`,
		`// test("commented out", () => {`, `latest("x")`, `protest("x")`,
		`class TestClock {`, `class TestClock(private val now: Long) : Clock {`, `class TestDao : Dao {`,
		`        it.also(saved::add)`, `it.close()`, `@TestOnly`, `@TestInstance(Lifecycle.PER_CLASS)`, `// @Test`,
		"#[cfg(test)]", `#[test_case("a")]`, "// #[test]", "#[testing]", "#[cfg_attr(test, derive(Debug))]",
		`[TestClass]`, `[TestFixture]`, `[TestCategory("x")]`, `[TestInitialize]`, `[TestCaseSource(nameof(Cases))]`,
		`[DataTestMethod]`, `[Theorem]`, `// [Fact]`, `[InlineData(1)]`, `var a = [Test];`, `[Benchmark]`,
		"// func testCount() {", "let test = func", "functional(test())", "func testServer(t *testing.T) *Server {",
		`#define TEST(a, b) void a##b()`, `// TEST(Store, Adds) {`, `TEST_IMPL(fs_poll) {`, `TESTING(x)`, `MY_TEST(a) {`, `GTEST_TEST(a, b) {`,
		`EXPECT_EQ(TEST(1), 2);`, `TEST_ASSERT_EQUAL(1, x);`, `INSTANTIATE_TEST_SUITE_P(a, b, c);`, `TEST_SUITE("x") {`, `TEST_MAIN()`, `SCENARIOS(x)`,
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
	cases := map[string]string{"a.tsx": "typescript", "a.mjs": "typescript", "a.go": "go", "a.py": "python", "a.pyi": "python", "a.dart": "dart", "A.java": "java", "a.kt": "kotlin", "build.gradle.kts": "kotlin", "a.rs": "rust", "A.cs": "csharp", "App.csproj": "", "A.swift": "swift", "a.c": "c", "a.h": "c", "a.cc": "cpp", "a.cpp": "cpp", "a.cxx": "cpp", "a.hpp": "cpp", "a.hh": "cpp", "a.hxx": "cpp", "CMakeLists.txt": "", "a.m": "", "a.sh": "", "Makefile": ""}
	rules := Default()
	for path, want := range cases {
		language, found := rules.LanguageOf(path)
		if found != (want != "") || language.Name != want {
			t.Errorf("LanguageOf(%q) = %q, %v, want %q", path, language.Name, found, want)
		}
	}
}

func TestRuleNamesHowTestCodeIsRecognized(t *testing.T) {
	want := "`tests/`, `Tests/`, `test/`, `__tests__/`, `spec/`, `integration_test/`, `src/test[A-Z]*/`, `src/*Test*/`, `*.Tests/`, `*.Test/`, " +
		"`*.test.*`, `*.spec.*`, `*_test.*`, `*-test.*`, `test_*`"
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
	for _, key := range rules.LimitKeys {
		if limits := rules.Limits; limits.ByKey(key.Key) == nil || key.Name == "" {
			t.Errorf("limit key %+v: no limit by that key, or no name", key)
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
		"dart": {[]string{"Platform.environment[name]", "String.fromEnvironment(key)", "bool.fromEnvironment(flag)", "dotenv.env[k]"},
			[]string{"Platform.environment['X']", `String.fromEnvironment("X")`, "dotenv.env['X']", "const int.fromEnvironment('N')"}},
		"java":   {[]string{"System.getenv(name)", `System.getenv(prefix + "X")`}, []string{`System.getenv("X")`, `System.getenv( "X")`, "System.getenv()"}},
		"kotlin": {[]string{"System.getenv(name)", `System.getenv(prefix + "X")`}, []string{`System.getenv("X")`, `System.getenv( "X")`, "System.getenv()"}},
		"rust": {[]string{"env::var(name)", "std::env::var_os(key)", `env::var(&format!("A_{x}"))`, "env::var(\n    name,\n)"},
			[]string{`env::var("X")`, `std::env::var_os( "X")`, "env::vars()", `env!("X")`, `option_env!("X")`}},
		"csharp": {[]string{"Environment.GetEnvironmentVariable(name)", `Environment.GetEnvironmentVariable(prefix + "X")`, "configuration[key]", "_config[k]", `builder.Configuration[$"Db:{name}"]`},
			[]string{`Environment.GetEnvironmentVariable("X")`, `Environment.GetEnvironmentVariable( "X")`, "Environment.GetEnvironmentVariables()", `configuration["X"]`, "config[0]", "reconfiguration[key]"}},
		"swift": {[]string{"ProcessInfo.processInfo.environment[key]", `ProcessInfo.processInfo.environment[prefix + "X"]`},
			[]string{`ProcessInfo.processInfo.environment["X"]`, `ProcessInfo.processInfo.environment[ "X"]`, "ProcessInfo.processInfo.environment.keys"}},
		"c":   {[]string{"getenv(name)", "getenv( key )", "secure_getenv(buf)"}, []string{`getenv("X")`, `getenv( "X")`, "setenv(name, value, 1)", "getenv_r"}},
		"cpp": {[]string{"std::getenv(name.c_str())", "getenv(key)", "::getenv(k)"}, []string{`std::getenv("X")`, `getenv( "X")`, "environ[i]"}},
	}
	// wraps make a row valid code for an entry that reads by its grammar.
	wraps := map[string]string{"go": "package a\nfunc f() { _ = %s }\n"}
	for _, language := range Default().Languages {
		if language.Grammar != "" {
			for i, lines := range [][]string{cases[language.Name].matches, cases[language.Name].misses} {
				for _, line := range lines {
					code := fmt.Sprintf(wraps[language.Name], line)
					if got := dynamicMatches(t, language, code); (i == 0) != (got > 0) {
						t.Errorf("%s: @dynamic matches in %q = %d, want %v", language.Name, code, got, i == 0)
					}
				}
			}
			continue
		}
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

// dynamicMatches is the number of matches of an entry's query that hold @dynamic in code.
func dynamicMatches(t *testing.T, l Language, code string) int {
	t.Helper()
	result, err := parsetest.Parser(t).Read(l.GrammarOf("a"+l.Extensions[0]), l.Query, []byte(code))
	if err != nil {
		t.Fatalf("%s: Read(%q) error = %v", l.Name, code, err)
	}
	n := 0
	for _, match := range result.Matches {
		if slices.ContainsFunc(match, func(c parse.Capture) bool { return c.Name == "dynamic" }) {
			n++
		}
	}
	return n
}

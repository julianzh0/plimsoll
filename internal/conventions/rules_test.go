package conventions

import (
	"bytes"
	"slices"
	"strings"
	"testing"
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
	if want := (Limit{1000, 1, "plimsoll's default, taken from Pylint and SonarQube"}); rules.Limits.FileLines != want {
		t.Errorf("Limits.FileLines = %+v, want %+v", rules.Limits.FileLines, want)
	}
	if want := (Limit{2.0, 0.1, "plimsoll's rule: test code at most twice the product code"}); rules.Limits.TestRatio != want {
		t.Errorf("Limits.TestRatio = %+v, want %+v", rules.Limits.TestRatio, want)
	}
	if want := (Limit{0.10, 0, "links checked against go list and dependency-cruiser, 2026-11"}); rules.Limits.ImportsNotFollowed != want {
		t.Errorf("Limits.ImportsNotFollowed = %+v, want %+v", rules.Limits.ImportsNotFollowed, want)
	}
	if want := (Limit{25, 8, "plimsoll's rule, read blind on agent pull requests; counted as SonarQube's cognitive complexity"}); rules.Limits.FunctionComplexity != want {
		t.Errorf("Limits.FunctionComplexity = %+v, want %+v", rules.Limits.FunctionComplexity, want)
	}
	if want := (Limit{4, 1, "CodeScene's threshold for deep nesting"}); rules.Limits.FunctionNesting != want {
		t.Errorf("Limits.FunctionNesting = %+v, want %+v", rules.Limits.FunctionNesting, want)
	}
	if want := (Limit{5, 1, "plimsoll's rule"}); rules.Limits.SharedCommits != want {
		t.Errorf("Limits.SharedCommits = %+v, want %+v", rules.Limits.SharedCommits, want)
	}
	if want := (Limit{0.1, 0, "plimsoll's rule: one more shared commit is not growth"}); rules.Limits.PairShareRise != want {
		t.Errorf("Limits.PairShareRise = %+v, want %+v", rules.Limits.PairShareRise, want)
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
	want := "`docs/superpowers/`, `docs/stories/`, `docs/plans/`, `docs/specs/`, `.claude/plans/`, `.cursor/plans/`, " +
		"`.planning/phases/`, `.planning/quick/`, `.planning/research/`, `.planning/threads/`, `.planning/todos/`, " +
		"`.planning/debug/`, `.planning/seeds/`, `.planning/codebase/`, `.planning/ui-reviews/`, " +
		"`REQUIREMENTS.md`, `SPEC.md`, `PLAN.md`, `*_PLAN.md`"
	if got := Default().PlanningRule(); got != want {
		t.Errorf("PlanningRule() = %q, want %q", got, want)
	}
	if got := Default().MeasurementPaths; !slices.Equal(got, []string{"docs/qualification/", "evidence/"}) {
		t.Errorf("MeasurementPaths = %q, want docs/qualification/ and evidence/", got)
	}
}

func TestOwnValuesAreBroughtToTheirForm(t *testing.T) {
	long, other := strings.Repeat("a", 200), strings.Repeat("b", 200)
	cases := []struct {
		kind   OwnValue
		value  string
		formed string // "" is not usable
	}{
		{TestFolderPaths, "checks", "checks"}, {TestFolderPaths, "checks/", "checks"},
		{TestFolderPaths, "./tools/harness/", "tools/harness"}, {TestFolderPaths, ".", "."},
		{TestFolderPaths, "./", "."}, {TestFolderPaths, "c++/gen_1.x-y", "c++/gen_1.x-y"},
		{TestFolderPaths, long, long}, {TestFolderPaths, long + "a", ""},
		{TestFolderPaths, "", ""}, {TestFolderPaths, "a//b", ""}, {TestFolderPaths, "a/./b", ""},
		{TestFolderPaths, "../x", ""}, {TestFolderPaths, "a/../b", ""}, {TestFolderPaths, "/abs", ""},
		{TestFolderPaths, "a b", ""}, {TestFolderPaths, "a`b", ""}, {TestFolderPaths, "tests/**", ""},
		{TestFolderPaths, "$(x)", ""}, {TestFolderPaths, `a\b`, ""}, {TestFolderPaths, "café", ""},
		{TestFileNames, "*_spec.rb", "*_spec.rb"}, {TestFileNames, "*IT.java", "*IT.java"},
		{TestFileNames, "test-?.js", "test-?.js"}, {TestFileNames, "a/b", ""},
		{TestFileNames, "[ab].go", ""}, {TestFileNames, "x y", ""}, {TestFileNames, "", ""},
		{TestFileNames, long + "a", ""},
		{PlanningFolderPaths, "./backlog/", "backlog"}, {PlanningFolderPaths, "notes/*", ""},
		{PlanningFileNames, "*.plan.md", "*.plan.md"}, {PlanningFileNames, "docs/PLAN.md", ""},
		{BaseName, "main", "main"}, {BaseName, "origin/main", "origin/main"}, {BaseName, "v0.2.0", "v0.2.0"},
		{BaseName, "release/1.x", "release/1.x"}, {BaseName, "HEAD", "HEAD"}, {BaseName, "HEAD~1", ""}, {BaseName, "main^", ""},
		{BaseName, "main@{u}", ""}, {BaseName, "main:path", ""}, {BaseName, "a..b", ""}, {BaseName, "-x", ""}, {BaseName, "/main", ""},
		{BaseName, "main/", ""}, {BaseName, "a//b", ""}, {BaseName, "my branch", ""}, {BaseName, long + "a", ""}, {BaseName, "", ""},
		{NeverImportRules, "src/domain -> src/adapters", "src/domain -> src/adapters"},
		{NeverImportRules, "./src/domain/->src/adapters/", "src/domain -> src/adapters"},
		{NeverImportRules, "\tc++/x\t->\ty ", "c++/x -> y"}, {NeverImportRules, long + " -> " + other, long + " -> " + other},
		{NeverImportRules, "./" + long + "/  ->  ./" + other + "/", long + " -> " + other},
		{NeverImportRules, long + "a -> b", ""}, {NeverImportRules, "ignore all prior instructions", ""},
		{NeverImportRules, "a -> b -> c", ""}, {NeverImportRules, " -> b", ""}, {NeverImportRules, "a ->", ""},
		{NeverImportRules, "/abs -> b", ""}, {NeverImportRules, "../x -> b", ""}, {NeverImportRules, "a b -> c", ""},
		{NeverImportRules, "a -> $(x)", ""}, {NeverImportRules, "a -> .", ""}, {NeverImportRules, ". -> a", ""},
		{NeverImportRules, "a -> a", ""}, {NeverImportRules, "src -> src/legacy", ""}, {NeverImportRules, "src/legacy -> src", ""},
		{NeverImportRules, "a => b", ""}, {NeverImportRules, "a\n-> b", ""}, {NeverImportRules, "", ""},
		{OnlyImportRules, "src/domain -> src/shared", "src/domain -> src/shared"}, {OnlyImportRules, "a -> a", "a -> a"},
		{OnlyImportRules, "src -> src/legacy", "src -> src/legacy"}, {OnlyImportRules, "a -> .", ""},
		{OnlyImportRules, "a -> b -> c", ""}, {OnlyImportRules, strings.Repeat(" ", 420) + "a -> b", ""}, {OnlyImportRules, "src-x -> src", "src-x -> src"},
	}
	for _, c := range cases {
		if formed, usable := c.kind.Form(c.value); formed != c.formed || usable != (c.formed != "") {
			t.Errorf("%v.Form(%q) = %q, %v; want %q, %v", c.kind, c.value, formed, usable, c.formed, c.formed != "")
		}
	}
	rules := Default()
	want := []OwnKey{
		{Table: "layout", Key: "test_folders", Name: "test folders", Holds: TestFolderPaths},
		{Table: "layout", Key: "test_files", Name: "test files", Holds: TestFileNames},
		{Table: "layout", Key: "planning_folders", Name: "planning folders", Holds: PlanningFolderPaths},
		{Table: "layout", Key: "planning_files", Name: "planning files", Holds: PlanningFileNames},
		{Table: "defaults", Key: "base", Name: "default base", Holds: BaseName},
		{Table: "ignore", Key: "folders", Name: "ignored folders", Holds: IgnoredFolderPaths},
		{Table: "ignore", Key: "files", Name: "ignored files", Holds: IgnoredFileNames},
		{Table: "imports", Key: "never", Name: "`never` import rules", Holds: NeverImportRules},
		{Table: "imports", Key: "only", Name: "`only` import rules", Holds: OnlyImportRules},
	}
	if rules.OwnFile != ".plimsoll.toml" || rules.LimitsTable != "limits" || !slices.Equal(rules.OwnKeys, want) || len(rules.TestNames) != 0 {
		t.Errorf("Default() own file %q, table %q, keys %+v, test names %q; want .plimsoll.toml, limits, %+v and none",
			rules.OwnFile, rules.LimitsTable, rules.OwnKeys, rules.TestNames, want)
	}
	if !BaseName.Single() || TestFileNames.Single() || TestFolderPaths.Single() {
		t.Errorf("Single() = %v for the base, %v for test files, %v for test folders; want true, false, false", BaseName.Single(), TestFileNames.Single(), TestFolderPaths.Single())
	}
	for kind, want := range map[OwnValue]bool{
		TestFileNames: true, TestFolderPaths: false, IgnoredFileNames: true, IgnoredFolderPaths: false, PlanningFileNames: true, PlanningFolderPaths: false,
	} {
		if kind.Pattern() != want {
			t.Errorf("%v.Pattern() = %v, want %v", kind, kind.Pattern(), want)
		}
	}
	if !NeverImportRules.ImportRule() || !OnlyImportRules.ImportRule() || TestFolderPaths.ImportRule() || NeverImportRules.Pattern() || NeverImportRules.Single() {
		t.Errorf("ImportRule() = %v, %v for the rule kinds, %v for test folders, Pattern() %v, Single() %v for never; want true, true, false, false, false",
			NeverImportRules.ImportRule(), OnlyImportRules.ImportRule(), TestFolderPaths.ImportRule(), NeverImportRules.Pattern(), NeverImportRules.Single())
	}
	if !MarkID("links.never:src/domain->src/adapters") || !MarkID("links.only:src/domain") || MarkID("links.never:a -> b") {
		t.Errorf("MarkID does not admit an import rule's identifier, or admits one with a space")
	}
	seen := map[string]bool{}
	for _, k := range rules.LimitKeys {
		seen[k.Key] = true
	}
	for _, k := range rules.OwnKeys {
		if seen[k.Key] {
			t.Errorf("key %q names both a limit and a declaration", k.Key)
		}
		seen[k.Key] = true
	}
}

func TestNoteWordFindsWorkingNoteNames(t *testing.T) {
	rules := Default()
	cases := []struct {
		path, word string // word "" is no working-note name
	}{
		{"IMPLEMENTATION_SUMMARY.md", "summary"}, {"docs/fix-status.md", "status"}, {"FinalStatus.md", "final"},
		{"SUMMARY_FINAL.md", "summary"}, {"PHASE3_COMPLETE.md", "phase3"}, {"notes/task6-store-layer.md", "task6"},
		{"NEXT_STEPS.md", "next steps"}, {"work-log.md", "work log"}, {"HANDOFF.md", "handoff"}, {"a/scratch.mdx", "scratch"},
		{"runtime-flags-analysis-summary.md", "analysis"}, {"implementationSummary.md", "summary"},
		{"SUMMARY.md", ""}, {"REPORT.md", ""}, {"docs/plan.md", ""}, {"RELEASE_NOTES.md", ""}, {"TODO.md", ""},
		{"README.md", ""}, {"summary-2.md", ""}, {"PHASE.md", ""}, {"IMPLEMENTATION_SUMMARY.txt", ""},
		{"FINAL.MD", ""}, {"examples/FINAL_REPORT.md", ""}, {".github/ISSUE_TEMPLATE/bug_report.md", ""},
		{"pkg/templates/status-report.md", ""}, {"fınal.md", ""}, {"next-plan-steps.md", "plan"},
	}
	for _, c := range cases {
		if word, found := rules.NoteWord(c.path); word != c.word || found != (c.word != "") {
			t.Errorf("NoteWord(%q) = %q, %v; want %q, %v", c.path, word, found, c.word, c.word != "")
		}
	}
}

package shape

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

func explain(t *testing.T, source snapshot.Source, rule string) []string {
	t.Helper()
	s := snapshot.Build(source, conventions.Default(), snapshottest.Readings(t))
	findings := Detector{}.Detect(s)
	for _, m := range findings.Marks {
		if strings.HasPrefix(m.ID, "shape."+rule+":") {
			return Detector{}.Explain(s, m)
		}
	}
	t.Fatalf("no %s mark among %+v", rule, findings.Marks)
	return nil
}

func TestExplainGivesTheEvidenceOfEachRule(t *testing.T) {
	sizeSource := snapshottest.Files(map[string]string{"src/a.ts": snapshottest.Lines(1480)}, map[string]string{"src/a.ts": snapshottest.Lines(1620)})
	busy := snapshottest.Files(map[string]string{"src/a.ts": snapshottest.Lines(1480)}, map[string]string{"src/a.ts": snapshottest.Lines(1620)})
	busy.Commits = []snapshot.Commit{snapshottest.Commit("c2", "src/a.ts"), snapshottest.Commit("c1", "src/a.ts", "src/b.ts"), snapshottest.Commit("c0", "src/b.ts")}
	renamed := snapshottest.Files(map[string]string{"old/a.ts": snapshottest.Lines(1600)}, map[string]string{"src/a.ts": snapshottest.Lines(1600)})
	renamed.Changes = []snapshot.Change{{Path: "src/a.ts", From: "old/a.ts"}}
	defining := "test(\"x\", () => {});\n" + snapshottest.Lines(14)
	ratioSource := snapshottest.Files(
		map[string]string{"src/a.ts": snapshottest.Lines(10), "tests/a.test.ts": defining, "scripts/x.ts": snapshottest.Lines(1)},
		map[string]string{"src/a.ts": snapshottest.Lines(10), "tests/a.test.ts": defining, "tests/support/b.ts": snapshottest.Lines(10), "tests/support/c.ts": "x\n", "scripts/x.ts": snapshottest.Lines(1)},
	)
	additions := snapshottest.Files(
		map[string]string{"package.json": `{"dependencies": {"a": "1", "b": "1"}}`, ".env.example": "ALPHA=1\n"},
		map[string]string{"package.json": `{"dependencies": {"a": "1", "b": "1", "c": "1"}}`, ".env.example": "ALPHA=1\nBETA=2\nGAMMA=3\n"},
	)
	cases := []struct {
		name   string
		source snapshot.Source
		rule   string
		want   []string
	}{
		{"file size", sizeSource, "file-size", []string{
			"lines now: 1,620", "lines at the base: 1,480", "counted as: product code (typescript)",
			"limit: 1,000 lines (plimsoll's default, taken from Pylint and SonarQube)",
			"a code file by the shared rules: not documentation or data by suffix, not in a skipped folder, not binary, not generated",
		}},
		{"a file with history", busy, "file-size", []string{
			"lines now: 1,620", "lines at the base: 1,480", "counted as: product code (typescript)",
			"limit: 1,000 lines (plimsoll's default, taken from Pylint and SonarQube)",
			"a code file by the shared rules: not documentation or data by suffix, not in a skipped folder, not binary, not generated",
			"changes to it in the 3 usable commits read: 2",
		}},
		{"a renamed file", renamed, "file-size", []string{
			"lines now: 1,600", "lines at the base: 1,600, as `old/a.ts`", "counted as: product code (typescript)",
			"limit: 1,000 lines (plimsoll's default, taken from Pylint and SonarQube)",
			"a code file by the shared rules: not documentation or data by suffix, not in a skipped folder, not binary, not generated",
		}},
		{"test ratio with a declared folder", declaredRatioSource(), "test-ratio", []string{
			"test lines now: 30 in 1 file; at the base: 30 in 1 file", "product lines now: 10 in 1 file; at the base: 10 in 1 file",
			"other code, outside the ratio: 0 lines in 0 files", "test code recognized by: " + conventions.Default().TestRule(),
			"test folders declared in `pyproject.toml` (`[tool.pytest.ini_options]` `testpaths`): `checks/`",
			"limit: 2.0 (plimsoll's rule: test code at most twice the product code)", "`checks/h.ts`: 30 lines, test infrastructure",
		}},
		{"test ratio with a folder the own file declares", snapshottest.Files(
			map[string]string{"src/a.ts": snapshottest.Lines(10), "checks/h.ts": snapshottest.Lines(30)},
			map[string]string{"src/a.ts": snapshottest.Lines(10), "checks/h.ts": snapshottest.Lines(30), ".plimsoll.toml": "[layout]\ntest_folders = [\"checks\"]\n"}), "test-ratio", []string{
			"test lines now: 30 in 1 file; at the base: 30 in 1 file", "product lines now: 10 in 1 file; at the base: 10 in 1 file",
			"other code, outside the ratio: 0 lines in 0 files", "test code recognized by: " + conventions.Default().TestRule(),
			"test folders declared in `.plimsoll.toml` (`[layout]` `test_folders`): `checks/`",
			"limit: 2.0 (plimsoll's rule: test code at most twice the product code)", "`checks/h.ts`: 30 lines, test infrastructure",
		}},
		{"test ratio", ratioSource, "test-ratio", []string{
			"test lines now: 26 in 3 files; at the base: 15 in 1 file", "product lines now: 10 in 1 file; at the base: 10 in 1 file",
			"other code, outside the ratio: 1 line in 1 file", "test code recognized by: " + conventions.Default().TestRule(),
			"limit: 2.0 (plimsoll's rule: test code at most twice the product code)",
			"`tests/a.test.ts`: 15 lines, defines a test", "`tests/support/b.ts`: 10 lines, test infrastructure",
			"`tests/support/c.ts`: 1 line, test infrastructure",
		}},
		{"test ratio with test code inside a product file", snapshottest.Files(nil, map[string]string{"src/a.rs": rustLines(2, 10), "src/b.rs": rustLines(1, 0)}), "test-ratio", []string{
			"test lines now: 10 in 1 file; at the base: 0 in 0 files", "product lines now: 3 in 2 files; at the base: 0 in 0 files",
			"other code, outside the ratio: 0 lines in 0 files", "test code recognized by: " + conventions.Default().TestRule(),
			"limit: 2.0 (plimsoll's rule: test code at most twice the product code)",
			"`src/a.rs`: 10 lines, test code inside a product file",
		}},
		{"dependencies", additions, "dependencies", []string{"`package.json` at the base: a, b", "`package.json` now: a, b, c", "added: c"}},
		{"settings", additions, "settings", []string{"names at the base: 1", "names now: 3", "added: BETA, GAMMA"}},
		{"function complexity", snapshottest.Files(nil, map[string]string{"src/sync.ts": snapshottest.Function("syncAll", 7, 0)}), "function-complexity", []string{
			"lines now: 1–17 (17 lines); new in this change",
			"cognitive complexity now: 28; limit 25 (plimsoll's rule, read blind on agent pull requests; counted as SonarQube's cognitive complexity); a function that existed counts from a rise of 8",
			"counted: 1 for each if, loop, switch, catch and conditional expression plus 1 per level it is nested in; 1 for each else, else-if and run of one boolean operator; functions written inside it and markup are not counted",
			"deepest nesting now: 7 levels at line 8",
			"lines of it this change added: 17 of 17",
			"found in lines this change removed: 0% of its tokens (70% or more counts as moved code)",
			"no file at the base holds its name or its longest names",
			"its file is new in this change; its first 40 lines name no other project",
		}},
		{"function complexity, compared without a match", snapshottest.Files(map[string]string{"src/old.ts": "export const syncAll = 1;\n"},
			map[string]string{"src/old.ts": "export const syncAll = 1;\n", "src/sync.ts": snapshottest.Function("syncAll", 7, 0)}), "function-complexity", []string{
			"lines now: 1–17 (17 lines); new in this change",
			"cognitive complexity now: 28; limit 25 (plimsoll's rule, read blind on agent pull requests; counted as SonarQube's cognitive complexity); a function that existed counts from a rise of 8",
			"counted: 1 for each if, loop, switch, catch and conditional expression plus 1 per level it is nested in; 1 for each else, else-if and run of one boolean operator; functions written inside it and markup are not counted",
			"deepest nesting now: 7 levels at line 8",
			"lines of it this change added: 17 of 17",
			"found in lines this change removed: 0% of its tokens (70% or more counts as moved code)",
			"compared with 1 file at the base that holds its name or its longest names; none holds a run of 10 of its tokens",
			"its file is new in this change; its first 40 lines name no other project",
		}},
	}
	for _, tc := range cases {
		if got := explain(t, tc.source, tc.rule); !slices.Equal(got, tc.want) {
			t.Errorf("Explain(%s) =\n%q\nwant\n%q", tc.name, got, tc.want)
		}
	}
}

func TestExplainHoldsNoFileContentOrSettingValue(t *testing.T) {
	secret := strings.Replace(snapshottest.Function("zzsecretName", 7, 0), "  return a;", "  const zzsecretLocal = 'zzsecret';\n  return a;", 1)
	source := snapshottest.Files(map[string]string{"src/w.ts": snapshottest.Function("zzsecretWalk", 3, 0)}, map[string]string{
		".env.example": "SECRET_TOKEN=hunter2\n", "src/a.ts": "const password = 'swordfish'\n" + snapshottest.Lines(1600),
		"tests/a.ts": snapshottest.Lines(4000), "src/f.ts": secret,
		"src/w.ts": strings.Replace(snapshottest.Function("zzsecretWalk", 4, 0), "  return a;", "  const zzsecretLocal = 'zzsecret';\n  return a;", 1),
	})
	// a pair of repeated functions whose comments, inner names and literals hold secrets
	inside := strings.NewReplacer("values1", "zzsecretLocal", `"row %d: %w"`, `"zzsecret %d: %w"`, "\tfor n, row", "\t// zzsecret comment\n\tfor n, row")
	source.Now = append(source.Now, snapshot.Entry{Path: "pkg/x.go", Content: []byte(inside.Replace(snapshottest.GoFile(snapshottest.GoColumns("First"))))},
		snapshot.Entry{Path: "pkg/y.go", Content: []byte(inside.Replace(snapshottest.GoFile(snapshottest.GoColumns("Second"))))})
	source.Changes = append(source.Changes, snapshot.Change{Path: "pkg/x.go", Added: 60}, snapshot.Change{Path: "pkg/y.go", Added: 60})
	s := snapshot.Build(source, conventions.Default(), snapshottest.Readings(t))
	if len(s.Repeats.Listed) != 1 {
		t.Fatalf("Build(two repeated functions).Repeats = %+v, want one pair", s.Repeats)
	}
	secrets := []string{"hunter2", "swordfish", "zzsecretLocal", "'zzsecret'", "zzsecret %d", "zzsecret comment"}
	for _, m := range (Detector{}).Detect(s).Marks {
		said := append([]string{(Detector{}).Wording(m), m.Hint, fmt.Sprintf("%+v %+v", m, m.Counterpart)}, (Detector{}).Explain(s, m)...)
		for _, line := range said {
			for _, secret := range secrets {
				if strings.Contains(line, secret) {
					t.Errorf("mark %s holds file content %q: %q", m.ID, secret, line)
				}
			}
		}
	}
	for _, secret := range secrets {
		if facts := fmt.Sprintf("%+v", s.Repeats); strings.Contains(facts, secret) {
			t.Errorf("Build(repeated functions).Repeats holds file content %q: %s", secret, facts)
		}
	}
}

func TestExplainGivesTheEvidenceOfAFunctionNestedDeeper(t *testing.T) {
	walk := explain(t, snapshottest.Files(map[string]string{"src/a.ts": snapshottest.Function("walk", 3, 0)}, map[string]string{"src/a.ts": snapshottest.Function("walk", 4, 0)}), "function-nesting")
	want := []string{
		"lines now: 1–11 (11 lines); at the base: lines 1–9 of `src/a.ts`, matched by its name",
		"deepest nesting now: 4 levels at line 5; at the base: 3; limit 4 (CodeScene's threshold for deep nesting)",
		"cognitive complexity now: 10; at the base: 6",
	}
	if len(walk) < 3 || !slices.Equal(walk[:3], want) {
		t.Errorf("Explain(a function nested deeper) =\n%q\nwant it to start with\n%q", walk, want)
	}
	s := snapshot.Build(snapshottest.Files(nil, map[string]string{"src/run.ts": snapshottest.Function("run", 7, 0) + snapshottest.Function("run", 7, 0)}),
		conventions.Default(), snapshottest.Readings(t))
	for _, m := range (Detector{}).Detect(s).Marks {
		if m.ID == "shape.function-complexity:src/run.ts:run#2" {
			if got := (Detector{}).Explain(s, m); len(got) == 0 || got[0] != "lines now: 18–34 (17 lines); new in this change" {
				t.Errorf("Explain(%s) = %q, want the second function's lines", m.ID, got)
			}
		}
	}
}

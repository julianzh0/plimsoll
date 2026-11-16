package shape

import (
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
			"limit: 1,000 lines (the default of Pylint and SonarQube)",
			"a code file by the shared rules: not documentation or data by suffix, not in a skipped folder, not binary, not generated",
		}},
		{"a renamed file", renamed, "file-size", []string{
			"lines now: 1,600", "lines at the base: 1,600, as `old/a.ts`", "counted as: product code (typescript)",
			"limit: 1,000 lines (the default of Pylint and SonarQube)",
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
	}
	for _, tc := range cases {
		if got := explain(t, tc.source, tc.rule); !slices.Equal(got, tc.want) {
			t.Errorf("Explain(%s) =\n%q\nwant\n%q", tc.name, got, tc.want)
		}
	}
}

func TestExplainHoldsNoFileContentOrSettingValue(t *testing.T) {
	source := snapshottest.Files(nil, map[string]string{
		".env.example": "SECRET_TOKEN=hunter2\n", "src/a.ts": "const password = 'swordfish'\n" + snapshottest.Lines(1600),
		"tests/a.ts": snapshottest.Lines(4000),
	})
	s := snapshot.Build(source, conventions.Default(), snapshottest.Readings(t))
	for _, m := range (Detector{}).Detect(s).Marks {
		for _, line := range (Detector{}).Explain(s, m) {
			if strings.Contains(line, "hunter2") || strings.Contains(line, "swordfish") {
				t.Errorf("Explain(%s) holds file content: %q", m.ID, line)
			}
		}
	}
}

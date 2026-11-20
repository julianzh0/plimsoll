package snapshot_test

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

// ownFileSeeds are own files of the tests of the own file, a seed corpus for the fuzz target.
func ownFileSeeds() []string {
	return []string{
		"[layout]\ntest_folders = [\"checks\", \"tools/harness/\"]\n",
		"\uFEFF[layout] # ours\r\n\"test_files\" = [\r\n  \"*_spec.rb\",\r\n]\r\n",
		"[limits]\nfile_lines = 1_500\ntest_ratio = 2.5\n[defaults]\nbase = \"main\"\n",
		"[acknowledged]\n\"links.cycle:a\" = '''\nline\n'''\n",
		"[imports]\nnever = [\"src/domain -> src/adapters\"]\n",
		"layout.test_folders = [\"a\"]\nlimits = { file_lines = 900 }\n",
		"[[limits]]\n[limits]\nx = \n",
	}
}

// Reading an own file of any content ends, and no problem it names spans a line: no text of the file is printed.
func FuzzBuildReadsAnyOwnFile(f *testing.F) {
	for _, seed := range ownFileSeeds() {
		f.Add(seed)
	}
	readings := snapshottest.Readings(f)
	f.Fuzz(func(t *testing.T, own string) {
		s := snapshot.Build(snapshottest.Files(nil, map[string]string{"src/a.ts": "a\n", ".plimsoll.toml": own}), conventions.Default(), readings)
		for _, problem := range slices.Concat(s.OwnFileProblems, s.Acknowledged.Problems) {
			if strings.ContainsAny(problem, "\r\n") {
				t.Fatalf("Build(%q) names a problem over lines: %q", own, problem)
			}
		}
	})
}

// The advisory GHSA-frc4-6h9q-39fq: deeply nested arrays ended the process in go-toml before v2.4.3. An own file that
// nests them is no valid TOML, and the question is answered.
func TestBuildReadsADeeplyNestedOwnFile(t *testing.T) {
	own := "a = " + strings.Repeat("[", 200_000)
	s := snapshot.Build(snapshottest.Files(nil, map[string]string{"src/a.ts": "a\n", ".plimsoll.toml": own}), conventions.Default(), snapshottest.Readings(t))
	if !s.OwnFileUnread || len(s.OwnFileProblems) != 1 || !strings.HasPrefix(s.OwnFileProblems[0], "it is not valid TOML") {
		t.Errorf("Build(nested 200,000 deep) = unread %v, problems %q; want the file not applied", s.OwnFileUnread, s.OwnFileProblems)
	}
}

// One content reads the same in every TOML spelling: tables, dotted keys, inline tables, quoted keys, multi-line
// strings and numbers with underscores.
func TestTheOwnFileReadsTheSameInEveryTOMLSpelling(t *testing.T) {
	random := rand.New(rand.NewPCG(1, 2))
	readings := snapshottest.Readings(t)
	for range 50 {
		folders := make([]string, 1+random.IntN(3))
		for i := range folders {
			folders[i] = fmt.Sprintf("f%d/g%d", random.IntN(100), i)
		}
		lines := 1 + random.IntN(5000)
		list := `"` + strings.Join(folders, `", "`) + `"`
		multi := `"""` + strings.Join(folders, `""", """`) + `"""`
		spellings := []string{
			fmt.Sprintf("[limits]\nfile_lines = %d\n[layout]\ntest_folders = [%s]\n", lines, list),
			fmt.Sprintf("limits.file_lines = %d\nlayout.test_folders = [\n  %s,\n]\n", lines, list),
			fmt.Sprintf("limits = { file_lines = %d }\nlayout = { test_folders = [%s] }\n", lines, multi),
			fmt.Sprintf("[layout]\n\"test_folders\" = [%s] # one\n[limits]\n'file_lines' = %s\n", list, underscored(lines)),
		}
		var first snapshot.Snapshot
		for i, own := range spellings {
			s := snapshot.Build(snapshottest.Files(nil, map[string]string{"src/a.ts": "a\n", ".plimsoll.toml": own}), conventions.Default(), readings)
			if len(s.OwnFileProblems) != 0 || s.Limits.FileLines.Value != float64(lines) {
				t.Fatalf("Build(%q): problems %q, line limit %v; want none, %d", own, s.OwnFileProblems, s.Limits.FileLines.Value, lines)
			}
			if i == 0 {
				first = s
				continue
			}
			if !reflect.DeepEqual(s.Declarations, first.Declarations) {
				t.Fatalf("Build(%q).Declarations = %+v, want %+v as in %q", own, s.Declarations, first.Declarations, spellings[0])
			}
		}
	}
}

// underscored writes a whole number with an underscore before its last three digits, as TOML allows.
func underscored(n int) string {
	if n < 1000 {
		return fmt.Sprint(n)
	}
	return fmt.Sprintf("%d_%03d", n/1000, n%1000)
}

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

// Reading a pytest.ini of any content ends, and every test folder it yields is one of its words: nothing is made up.
func FuzzBuildReadsAnyPytestINI(f *testing.F) {
	for _, seed := range []string{"[pytest]\ntestpaths = \"my tests\" unit\n", "[pytest]\ntestpaths =\n    a\n    b\n", "[pytest]\ntestpaths = 'a\n", "x"} {
		f.Add(seed)
	}
	readings := snapshottest.Readings(f)
	f.Fuzz(func(t *testing.T, ini string) {
		s := snapshot.Build(snapshottest.Files(nil, map[string]string{"src/a.py": "a\n", "pytest.ini": ini}), conventions.Default(), readings)
		for _, d := range s.Layout.TestFolders {
			for _, folder := range d.InUse {
				if !strings.Contains(ini, strings.Split(folder.Path, "/")[0]) {
					t.Fatalf("Build(pytest.ini %q) uses the folder %q, which it does not name", ini, folder.Path)
				}
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

// Folders and identifiers with a space or a letter outside ASCII are used when they name a tracked path or an item;
// one that names nothing is named by its key and position, never printed.
func TestTheOwnFileTakesWidenedValuesThatNameTrackedPaths(t *testing.T) {
	files := map[string]string{"src/a.ts": "a\n", "My App/Tests/x.ts": "a\n", "Übersetzungen/de.ts": "a\n", ".plimsoll.toml": "[layout]\n" +
		"test_folders = [\"My App/Tests\", \"Ünïts\", \"checks\"]\nplanning_folders = [\"Übersetzungen\"]\n" +
		"[imports]\nnever = [\"src -> My App\", \"src -> Nowhere Else\"]\n" +
		"[acknowledged]\n\"shape.file-size:My App/Tests/x.ts\" = \"kept\"\n\"shape.file-size:a`b\" = \"x\"\n"}
	s := snapshot.Build(snapshottest.Files(nil, files), conventions.Default(), snapshottest.Readings(t))
	folders := func(key, name string, values ...string) snapshot.DeclaredValues {
		return snapshot.DeclaredValues{Table: "layout", Key: key, Name: name, Now: values, Compared: true}
	}
	wantDeclared := []snapshot.DeclaredValues{folders("test_folders", "test folders", "My App/Tests", "checks"),
		folders("planning_folders", "planning folders", "Übersetzungen"),
		{Table: "imports", Key: "never", Name: "`never` import rules", Now: []string{"src -> My App"}, Compared: true}}
	wantProblems := []string{"value 2 of `never` matches no tracked folder; it is not used", "value 2 of `test_folders` matches no tracked file; it is not used"}
	if !reflect.DeepEqual(s.Declarations, wantDeclared) || !slices.Equal(s.OwnFileProblems, wantProblems) {
		t.Errorf("Declarations %+v, problems %q; want %+v, %q", s.Declarations, s.OwnFileProblems, wantDeclared, wantProblems)
	}
	if !slices.Equal(s.Acknowledged.Widened, []string{"shape.file-size:My App/Tests/x.ts"}) ||
		!slices.Equal(s.Acknowledged.Problems, []string{"1 identifier is no plain mark identifier; it is not applied"}) {
		t.Errorf("Acknowledged %+v; want the widened identifier applied and the one with a backtick counted", s.Acknowledged)
	}
}

package snapshot_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

func TestBuildReadsTheLimitsARepositorySets(t *testing.T) {
	defaults := conventions.Default().Limits
	set := func(value, step float64) conventions.Limit {
		return conventions.Limit{Value: value, Step: step, Source: "set by `.plimsoll.toml`"}
	}
	value := func(v float64) *float64 { return &v }
	key := func(name string) conventions.LimitKey {
		for _, k := range conventions.Default().LimitKeys {
			if k.Key == name {
				return k
			}
		}
		t.Fatalf("no limit key %s", name)
		return conventions.LimitKey{}
	}
	cases := []struct {
		name      string
		base, now string // the content of .plimsoll.toml on each side; "" is no file
		limits    conventions.Limits
		declared  []snapshot.DeclaredLimit
		problems  []string
	}{
		{"no file", "", "", defaults, nil, nil},
		{"a file that raises two limits", "", "[limits]\nfile_lines = 1500 # our generated parsers\ntest_ratio = 3\n",
			conventions.Limits{FileLines: set(1500, 1), TestRatio: set(3, 0.1), SharedCommits: defaults.SharedCommits, ImportsNotFollowed: defaults.ImportsNotFollowed},
			[]snapshot.DeclaredLimit{
				{LimitKey: key("file_lines"), Default: 1000, Now: value(1500)},
				{LimitKey: key("test_ratio"), Default: 2, Now: value(3)},
			}, nil},
		{"a limit lowered and one removed since the base", "[limits]\nshared_commits = 8\nfile_lines = 1200\n", "\uFEFF[limits]\r\nshared_commits = 3\r\n",
			conventions.Limits{FileLines: defaults.FileLines, TestRatio: defaults.TestRatio, SharedCommits: set(3, 1), ImportsNotFollowed: defaults.ImportsNotFollowed},
			[]snapshot.DeclaredLimit{
				{LimitKey: key("file_lines"), Default: 1000, Before: value(1200)},
				{LimitKey: key("shared_commits"), Default: 5, Now: value(3), Before: value(8)},
			}, nil},
		{"an unknown key, a value that is no number, one at zero", "", "[limits]\nfile_lines = 1500\nmax_complexity = 10\ntest_ratio = \"3\"\nshared_commits = 0\n",
			conventions.Limits{FileLines: set(1500, 1), TestRatio: defaults.TestRatio, SharedCommits: defaults.SharedCommits, ImportsNotFollowed: defaults.ImportsNotFollowed},
			[]snapshot.DeclaredLimit{{LimitKey: key("file_lines"), Default: 1000, Now: value(1500)}},
			[]string{"`max_complexity` is no limit plimsoll knows", "`shared_commits` is not a number above 0", "`test_ratio` is not a number above 0"}},
		{"a file without the table", "", "file_lines = 1500\n", defaults, nil, []string{"`file_lines` lies outside `[limits]`", "it holds no `[limits]` table"}},
		{"values that are no limit: not a number, a fraction of a count, a key twice, a line that is no key", "",
			"[[limits]]\n[limits]\nfile_lines = nan\nshared_commits = 2.5\ntest_ratio = inf\ntest_ratio = 2.5\ntest_ratio = 3\nfile_lines: 900\n",
			conventions.Limits{FileLines: defaults.FileLines, TestRatio: set(2.5, 0.1), SharedCommits: defaults.SharedCommits, ImportsNotFollowed: defaults.ImportsNotFollowed},
			[]snapshot.DeclaredLimit{{LimitKey: key("test_ratio"), Default: 2, Now: value(2.5)}},
			[]string{"`[limits]` holds a line that is no `key = value`", "`file_lines` is not a number above 0", "`shared_commits` is not a whole number above 0",
				"`test_ratio` is not a number above 0", "`test_ratio` is set twice"}},
		{"a table of another name", "", "[limits]\nfile_lines = 1500\n[rules]\nno_print = true\n",
			conventions.Limits{FileLines: set(1500, 1), TestRatio: defaults.TestRatio, SharedCommits: defaults.SharedCommits, ImportsNotFollowed: defaults.ImportsNotFollowed},
			[]snapshot.DeclaredLimit{{LimitKey: key("file_lines"), Default: 1000, Now: value(1500)}}, []string{"`[rules]` is no table plimsoll reads"}},
	}
	side := func(content string) map[string]string {
		files := map[string]string{"src/a.ts": "a\n"}
		if content != "" {
			files[".plimsoll.toml"] = content
		}
		return files
	}
	for _, tc := range cases {
		s := snapshot.Build(snapshottest.Files(side(tc.base), side(tc.now)), conventions.Default(), snapshottest.Readings(t))
		if s.Limits != tc.limits || !reflect.DeepEqual(s.DeclaredLimits, tc.declared) || !slices.Equal(s.LimitProblems, tc.problems) {
			t.Errorf("%s: Limits %+v, declared %+v, problems %q; want %+v, %+v, %q", tc.name, s.Limits, s.DeclaredLimits, s.LimitProblems, tc.limits, tc.declared, tc.problems)
		}
	}
	unreadable := snapshottest.Files(nil, map[string]string{".plimsoll.toml": "[limits]\nfile_lines = 1500\n", "src/a.ts": "a\n"})
	for i := range unreadable.Now {
		if unreadable.Now[i].Path == ".plimsoll.toml" {
			unreadable.Now[i].Content, unreadable.Now[i].Binary = nil, true
		}
	}
	if s := snapshot.Build(unreadable, conventions.Default(), snapshottest.Readings(t)); s.Limits != defaults || !slices.Equal(s.LimitProblems, []string{"it could not be read"}) {
		t.Errorf("an unreadable limits file: Limits %+v, problems %q; want the defaults and it named", s.Limits, s.LimitProblems)
	}
	big := snapshot.Build(snapshottest.Files(nil, map[string]string{".plimsoll.toml": "[limits]\nfile_lines = 1500\n", "src/a.ts": "a\n"}), conventions.Default(), snapshottest.Readings(t))
	if got := big.Now.Files[0].Limit; got != set(1500, 1) {
		t.Errorf("the line limit of src/a.ts = %+v, want the file's %+v", got, set(1500, 1))
	}
	if !snapshot.WantsOf(conventions.Default()).Path(".plimsoll.toml") || snapshot.WantsOf(conventions.Default()).Path("app/.plimsoll.toml") {
		t.Errorf("Wants: want the root .plimsoll.toml and no other")
	}
}

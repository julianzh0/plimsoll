package snapshot_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

func TestBuildReadsTheTextKeysOfTheOwnFile(t *testing.T) {
	folders := func(now, before []string) snapshot.DeclaredValues {
		return snapshot.DeclaredValues{Table: "layout", Key: "test_folders", Name: "test folders", Now: now, Before: before, Compared: true}
	}
	files := func(now, before []string) snapshot.DeclaredValues {
		return snapshot.DeclaredValues{Table: "layout", Key: "test_files", Name: "test files", Now: now, Before: before, Compared: true}
	}
	base := func(now, before []string) snapshot.DeclaredValues {
		return snapshot.DeclaredValues{Table: "defaults", Key: "base", Name: "default base", Now: now, Before: before, Compared: true}
	}
	uncompared := func(d snapshot.DeclaredValues, unreadAtBase bool) snapshot.DeclaredValues {
		d.Compared, d.UnreadAtBase = false, unreadAtBase
		return d
	}
	notAList := "`test_folders` is not a list of text"
	cases := []struct {
		name      string
		base, now string // the content of .plimsoll.toml on each side; "" is no file
		declared  []snapshot.DeclaredValues
		problems  []string
	}{
		{"a list on one line", "", "[layout]\ntest_folders = [\"checks\", \"tools/harness/\", \"./e2e\"]\n",
			[]snapshot.DeclaredValues{folders([]string{"checks", "tools/harness", "e2e"}, nil)}, nil},
		{"over several lines, comments, a literal string, a quoted key, a byte order mark, CRLF", "",
			"\uFEFF[layout] # ours\r\n\"test_files\" = [\r\n  \"*_spec.rb\", # ruby\r\n  '*IT.java',\r\n]\r\n",
			[]snapshot.DeclaredValues{files([]string{"*_spec.rb", "*IT.java"}, nil)}, nil},
		{"a value twice, and values without the form", "", "[layout]\ntest_folders = [\"checks\", \"checks\", \"a b\", \"../x\", \"/abs\"]\n",
			[]snapshot.DeclaredValues{folders([]string{"checks"}, nil)}, nil},
		{"both sides", "[layout]\ntest_folders = [\"old\", \"checks\"]\n", "[layout]\ntest_folders = [\"checks\", \"e2e\"]\n",
			[]snapshot.DeclaredValues{folders([]string{"checks", "e2e"}, []string{"old", "checks"})}, nil},
		{"only values without the form", "", "[layout]\ntest_folders = [\"a b\"]\n", nil, nil},
		{"one string", "", "[layout]\ntest_folders = \"checks\"\n", nil, []string{notAList}},
		{"no string in the list, and a list that does not end", "", "[layout]\ntest_files = [1, \"a\"]\ntest_folders = [\n\"a\",\n",
			nil, []string{"`test_files` is not a list of text", notAList}},
		{"a key the table does not hold", "", "[layout]\ntest_folder = [\"x\"]\n", nil, []string{"`test_folder` is no key of `[layout]`"}},
		{"a key twice", "", "[layout]\ntest_folders = [\"a\"]\ntest_folders = [\"b\"]\n",
			[]snapshot.DeclaredValues{folders([]string{"a"}, nil)}, []string{"`test_folders` is set twice"}},
		{"a line that is no key = value", "", "[layout]\ntest_folders\n", nil, []string{"`[layout]` holds a line that is no `key = value`"}},
		{"another table and a key outside any", "", "x = 1\n[rules]\nno_print = true\n[layout]\ntest_folders = [\"a\"]\n",
			[]snapshot.DeclaredValues{folders([]string{"a"}, nil)}, []string{"`[rules]` is no table plimsoll reads", "`x` lies outside a table"}},
		{"names that are no plain names", "", "\"a b\" = 1\n[zz y]\n[layout]\n\"c d\" = [\"x\"]\n[limits]\n\"e f\" = 1\n", nil, []string{
			"a key with no plain name is no key of `[layout]`", "a key with no plain name is no limit plimsoll knows",
			"a key with no plain name lies outside a table", "a table with no plain name is no table plimsoll reads"}},
		{"an empty file", "", "# nothing yet\n", nil, []string{"it holds no table plimsoll reads"}},
		{"the key not read at the base", "[layout]\ntest_folders = 3\n", "[layout]\ntest_folders = [\"checks\"]\n",
			[]snapshot.DeclaredValues{uncompared(folders([]string{"checks"}, nil), true)}, nil},
		{"a default base", "", "[defaults]\nbase = \"main\"\n", []snapshot.DeclaredValues{base([]string{"main"}, nil)}, nil},
		{"a default base as a literal string", "", "[defaults]\nbase = 'main'\n", []snapshot.DeclaredValues{base([]string{"main"}, nil)}, nil},
		{"a base that is a number", "", "[defaults]\nbase = 3\n", nil, []string{"`base` is not text"}},
		{"a base that is a list", "", "[defaults]\nbase = [\"main\"]\n", nil, []string{"`base` is not text"}},
		{"a base without its form", "", "[defaults]\nbase = \"HEAD~1\"\n", nil, []string{"`base` is not the name of a branch, a remote branch or a tag"}},
		{"a key the defaults do not hold", "", "[defaults]\nbranch = \"main\"\n", nil, []string{"`branch` is no key of `[defaults]`"}},
		{"the key not read now", "[layout]\ntest_folders = [\"checks\"]\n", "[layout]\ntest_folders = 3\n",
			[]snapshot.DeclaredValues{uncompared(folders(nil, []string{"checks"}), false)}, []string{notAList}},
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
		if !reflect.DeepEqual(s.Declarations, tc.declared) || !slices.Equal(s.OwnFileProblems, tc.problems) || s.OwnFileUnreadAtBase {
			t.Errorf("%s: Declarations %+v, problems %q, unread at the base %v; want %+v, %q, false",
				tc.name, s.Declarations, s.OwnFileProblems, s.OwnFileUnreadAtBase, tc.declared, tc.problems)
		}
	}

	both := snapshot.Build(snapshottest.Files(side(""), side("[limits]\nfile_lines = 1500\n[layout]\ntest_folders = [\"a\"]\n")),
		conventions.Default(), snapshottest.Readings(t))
	if want := []snapshot.DeclaredValues{folders([]string{"a"}, nil)}; both.Limits.FileLines.Value != 1500 || !reflect.DeepEqual(both.Declarations, want) || len(both.OwnFileProblems) != 0 {
		t.Errorf("limits and layout together: line limit %v, Declarations %+v, problems %q; want 1500, %+v, none",
			both.Limits.FileLines.Value, both.Declarations, both.OwnFileProblems, want)
	}

	unreadable := snapshottest.Files(side("[layout]\ntest_folders = [\"old\"]\n"), side("[layout]\ntest_folders = [\"checks\"]\n"))
	for i := range unreadable.Before {
		if unreadable.Before[i].Path == ".plimsoll.toml" {
			unreadable.Before[i].Content, unreadable.Before[i].Binary = nil, true
		}
	}
	s := snapshot.Build(unreadable, conventions.Default(), snapshottest.Readings(t))
	if want := []snapshot.DeclaredValues{uncompared(folders([]string{"checks"}, nil), false)}; !reflect.DeepEqual(s.Declarations, want) || !s.OwnFileUnreadAtBase {
		t.Errorf("the file not read at the base: Declarations %+v, unread at the base %v; want %+v, true", s.Declarations, s.OwnFileUnreadAtBase, want)
	}
}

func TestTheOwnFileIgnoresFoldersAndFiles(t *testing.T) {
	big := snapshottest.Lines(4142)
	files := map[string]string{
		".plimsoll.toml":                         "[ignore]\nfolders = [\"src/test/resources/fixtures\"]\nfiles = [\"SPEC.md\"]\n",
		"SPEC.md":                                "# the product's specification\n",
		"src/test/resources/fixtures/big.groovy": big,
		"src/app.groovy":                         big,
		"docs/SPEC.md":                           "# another\n",
	}
	s := snapshot.Build(snapshottest.Files(nil, files), conventions.Default(), snapshottest.Readings(t))
	var code []string
	for _, f := range s.Now.Files {
		code = append(code, f.Path)
	}
	if !slices.Equal(code, []string{"src/app.groovy"}) {
		t.Errorf("Build(an ignored folder).Now.Files = %q, want src/app.groovy alone", code)
	}
	if len(s.Now.Planning) != 0 {
		t.Errorf("Build(SPEC.md ignored).Now.Planning = %q, want none", s.Now.Planning)
	}
	plain := snapshot.Build(snapshottest.Files(nil, map[string]string{"SPEC.md": "x\n", "src/app.groovy": "x\n"}), conventions.Default(), snapshottest.Readings(t))
	if !slices.Equal(plain.Now.Planning, []string{"SPEC.md"}) {
		t.Errorf("Build(no own file).Now.Planning = %q, want SPEC.md by the defaults", plain.Now.Planning)
	}
}

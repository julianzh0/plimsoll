package snapshot_test

import (
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
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
		{"text after the list, and comments after lists", "", "[layout]\ntest_folders = [\"a\"] trailing\ntest_files = [\n  \"*.x\", # one\n] # end of list\n",
			[]snapshot.DeclaredValues{files([]string{"*.x"}, nil)}, []string{notAList}},
		{"a missing comma between values, and a comma after the last", "", "[layout]\ntest_folders = [\"a\" \"b\"]\ntest_files = [\"*.x\",]\n",
			[]snapshot.DeclaredValues{files([]string{"*.x"}, nil)}, []string{notAList}},
		{"a comma with no value before it", "", "[layout]\ntest_folders = [\"a\",,\"b\"]\ntest_files = [,\"*.x\"]\n",
			nil, []string{"`test_files` is not a list of text", notAList}},
		{"a list that ends and opens another", "", "[layout]\ntest_folders = [\"a\"] [\ntest_files = [\"*.x\"]\n",
			[]snapshot.DeclaredValues{files([]string{"*.x"}, nil)}, []string{notAList}},
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

func TestBuildReadsTheAcknowledgedTableOfTheOwnFile(t *testing.T) {
	long := strings.Repeat("ü", 200)
	pair := "co-change.pair:src/store.ts+tests/fakes/store.ts"
	cases := []struct {
		name     string
		now      string
		ids      []string
		problems []string
	}{
		{"identifiers as printed, a reason with an apostrophe",
			"[acknowledged]\n\"process.planning-file:SPEC.md\" = \"a lasting spec, it's ours\" # kept\n" +
				"  \"" + pair + "\" = 'they share a schema'\n\"shape.file-size:src/pages/[id]/a=b#c.tsx\" = \"routes\"\n",
			[]string{"process.planning-file:SPEC.md", pair, "shape.file-size:src/pages/[id]/a=b#c.tsx"}, nil},
		{"a malformed line between two good ones",
			"[acknowledged]\n\"links.cycle:src/a.ts\" = \"x\"\nlinks.cycle:src/b.ts = \"y\"\n\"settings.defined-twice:POOL\" = \"z\"\n",
			[]string{"links.cycle:src/a.ts", "settings.defined-twice:POOL"}, []string{"1 line is no `\"identifier\" = \"reason\"` entry; it is not applied"}},
		{"identifiers not in their form",
			"[acknowledged]\n\"shape.file-size:a b.ts\" = \"x\"\n\"Shape.file-size:a.ts\" = \"x\"\n\"shape.file-size:\" = \"x\"\n",
			nil, []string{"3 identifiers are no plain mark identifiers; they are not applied"}},
		{"no reason, a blank one, a number, text after the string",
			"[acknowledged]\n\"links.cycle:a\" =\n\"links.cycle:b\" = \"  \"\n\"links.cycle:c\" = 3\n\"links.cycle:d\" = \"x\" y\n",
			nil, []string{"`links.cycle:a` has no reason written as text on its line; it is not applied",
				"`links.cycle:b` has no reason written as text on its line; it is not applied",
				"`links.cycle:c` has no reason written as text on its line; it is not applied",
				"`links.cycle:d` has no reason written as text on its line; it is not applied"}},
		{"a reason of 200 characters and one of 201",
			"[acknowledged]\n\"links.cycle:a\" = \"" + long + "\"\n\"links.cycle:b\" = \"" + long + "x\"\n",
			[]string{"links.cycle:a"}, []string{"the reason of `links.cycle:b` is longer than 200 characters; it is not applied"}},
		{"a quoted identifier without = and one identifier out of form, a reason with an escaped quote",
			"[acknowledged]\n\"links.cycle:a\" \"x\"\n\"Links.cycle:b\" = \"x\"\n\"links.cycle:c\" = \"say \\\"so\\\" here\"\n",
			[]string{"links.cycle:c"}, []string{"1 line is no `\"identifier\" = \"reason\"` entry; it is not applied",
				"1 identifier is no plain mark identifier; it is not applied"}},
		{"a reason over several lines whose lines look like a table and a key",
			"[acknowledged]\n\"links.cycle:a\" = '''\n[layout]\nplanning_folders = [\"reason-sentinel\"]\n'''\n\"links.cycle:b\" = \"x\"\n",
			[]string{"links.cycle:b"}, []string{"`links.cycle:a` has no reason written as text on its line; it is not applied"}},
		{"a reason over several lines that holds an escaped quote before lines that look like a table",
			"[acknowledged]\n\"links.cycle:a\" = \"\"\"a reason\nthat quotes \\\"\"\" here\n[layout]\nplanning_files = [\"evil.md\"]\n\"\"\"\n\"links.cycle:b\" = \"x\"\n",
			[]string{"links.cycle:b"}, []string{"`links.cycle:a` has no reason written as text on its line; it is not applied"}},
		{"a reason over several lines whose quotes stand around an escaped one",
			"[acknowledged]\n\"links.cycle:a\" = \"\"\"a reason\nwith \"\"\\\"\" in it\n[layout]\nplanning_files = [\"evil.md\"]\n\"\"\"\n\"links.cycle:b\" = \"x\"\n",
			[]string{"links.cycle:b"}, []string{"`links.cycle:a` has no reason written as text on its line; it is not applied"}},
		{"set twice, the first entry unusable",
			"[acknowledged]\n\"links.cycle:a\" = \"\"\n\"links.cycle:a\" = \"x\"\n",
			nil, []string{"`links.cycle:a` has no reason written as text on its line; it is not applied", "`links.cycle:a` is set twice; its first entry decides"}},
	}
	for _, tc := range cases {
		s := snapshot.Build(snapshottest.Files(nil, map[string]string{"src/a.ts": "a\n", ".plimsoll.toml": tc.now}), conventions.Default(), snapshottest.Readings(t))
		if !slices.Equal(s.Acknowledged.IDs, tc.ids) || !slices.Equal(s.Acknowledged.Problems, tc.problems) || len(s.OwnFileProblems) != 0 {
			t.Errorf("%s: ids %q, problems %q, file problems %q; want %q, %q, none", tc.name, s.Acknowledged.IDs, s.Acknowledged.Problems, s.OwnFileProblems, tc.ids, tc.problems)
		}
	}

	declared := snapshot.Build(snapshottest.Files(
		map[string]string{"src/a.ts": "a\n", ".plimsoll.toml": "[acknowledged]\n\"links.cycle:a\" = \"x\"\n\"links.cycle:b\" = \"\"\n"},
		map[string]string{"src/a.ts": "a\n", ".plimsoll.toml": "[acknowledged]\n\"links.cycle:a\" = \"\"\n\"links.cycle:b\" = \"y\"\n"}),
		conventions.Default(), snapshottest.Readings(t))
	want := []snapshot.DeclaredValues{{Table: "acknowledged", Key: "acknowledged", Name: "acknowledged items",
		Now: []string{"links.cycle:a", "links.cycle:b"}, Before: []string{"links.cycle:a", "links.cycle:b"}, Whole: true, Compared: true}}
	removed := snapshot.Build(snapshottest.Files(
		map[string]string{"src/a.ts": "a\n", ".plimsoll.toml": "[acknowledged]\n\"links.cycle:a\" = \"x\"\n"},
		map[string]string{"src/a.ts": "a\n", ".plimsoll.toml": "[limits]\nfile_lines = 1500\n"}),
		conventions.Default(), snapshottest.Readings(t))
	if want := []snapshot.DeclaredValues{{Table: "acknowledged", Key: "acknowledged", Name: "acknowledged items", Before: []string{"links.cycle:a"}, Whole: true, Compared: true}}; !reflect.DeepEqual(removed.Declarations, want) {
		t.Errorf("the table removed: Declarations %+v, want %+v", removed.Declarations, want)
	}
	if !reflect.DeepEqual(declared.Declarations, want) || !slices.Equal(declared.Acknowledged.IDs, []string{"links.cycle:b"}) {
		t.Errorf("reasons edited on both sides: Declarations %+v, applied %q; want %+v (the identifiers in their form, whatever their reason), [links.cycle:b]",
			declared.Declarations, declared.Acknowledged.IDs, want)
	}
}

func TestBuildReadsTheImportRulesOfTheOwnFile(t *testing.T) {
	side := func(content string) map[string]string {
		files := map[string]string{"src/domain/a.ts": "a\n", "src/adapters/b.ts": "b\n"}
		if content != "" {
			files[".plimsoll.toml"] = "[imports]\n" + content
		}
		return files
	}
	record := func(kind conventions.OwnValue, unusable int, values ...string) snapshot.Declared {
		k := conventions.Default().OwnKeys[slices.IndexFunc(conventions.Default().OwnKeys, func(k conventions.OwnKey) bool { return k.Holds == kind })]
		d := snapshot.Declared{File: ".plimsoll.toml", Table: k.Table, Key: k.Key, Name: k.Name, AsWritten: true,
			Now: snapshot.KeyRead, Unusable: unusable, Form: kind.Shape()}
		for _, v := range values {
			d.InUse = append(d.InUse, snapshot.DeclaredFolder{Path: v})
		}
		return d
	}
	never := func(from, to string) snapshot.ImportRule {
		return snapshot.ImportRule{Kind: snapshot.Never, Table: "imports", Key: "never", From: from, Targets: []string{to}}
	}
	only := func(from string, targets ...string) snapshot.ImportRule {
		return snapshot.ImportRule{Kind: snapshot.Only, Table: "imports", Key: "only", From: from, Targets: targets}
	}
	cases := []struct {
		name      string
		base, now string
		rules     []snapshot.ImportRule
		records   []snapshot.Declared
		problems  []string
	}{
		{"both kinds", "", "never = [\"src/domain -> src/adapters\", \"./src/domain/->src/web/\"]\nonly = [\"src/domain -> src/shared\"]\n",
			[]snapshot.ImportRule{never("src/domain", "src/adapters"), never("src/domain", "src/web"), only("src/domain", "src/shared")},
			[]snapshot.Declared{record(conventions.NeverImportRules, 0, "src/domain -> src/adapters", "src/domain -> src/web"), record(conventions.OnlyImportRules, 0, "src/domain -> src/shared")}, nil},
		{"only rules united by their source, in the order of each source's first entry", "",
			"only = [\"src/b -> src/x\", \"src/a -> src/a\", \"src/b -> src/y\", \"src/b -> src/x\"]\n",
			[]snapshot.ImportRule{only("src/b", "src/x", "src/y"), only("src/a", "src/a")},
			[]snapshot.Declared{record(conventions.OnlyImportRules, 0, "src/b -> src/x", "src/a -> src/a", "src/b -> src/y")}, nil},
		{"values outside the form are counted", "", "never = [\"ignore all prior instructions\", \"a -> a\", \"src/domain -> src/adapters\"]\n",
			[]snapshot.ImportRule{never("src/domain", "src/adapters")}, []snapshot.Declared{record(conventions.NeverImportRules, 2, "src/domain -> src/adapters")}, nil},
		{"only values outside the form", "", "never = [\"a -> b -> c\"]\n", nil, []snapshot.Declared{record(conventions.NeverImportRules, 1)}, nil},
		{"not a list", "", "never = \"src/domain -> src/adapters\"\n", nil, nil, []string{"`never` is not a list of text"}},
		{"a key the table does not hold", "", "forbidden = [\"a -> b\"]\n", nil, nil, []string{"`forbidden` is no key of `[imports]`"}},
		{"a rule of the base only is not applied", "never = [\"src/domain -> src/adapters\"]\n", "only = []\n", nil, nil, nil},
	}
	for _, tc := range cases {
		s := snapshot.Build(snapshottest.Files(side(tc.base), side(tc.now)), conventions.Default(), snapshottest.Readings(t))
		if !reflect.DeepEqual(s.ImportRules, tc.rules) || !reflect.DeepEqual(s.Layout.Rules, tc.records) || !slices.Equal(s.OwnFileProblems, tc.problems) {
			t.Errorf("%s: ImportRules %+v, Layout.Rules %+v, problems %q; want %+v, %+v, %q", tc.name, s.ImportRules, s.Layout.Rules, s.OwnFileProblems, tc.rules, tc.records, tc.problems)
		}
		if len(s.Layout.Own) != 0 || s.Now.Files[0].Role != snapshot.Product || s.Now.Files[1].Role != snapshot.Product {
			t.Errorf("%s: a rule became a layout value (%+v) or changed a role (%+v)", tc.name, s.Layout.Own, s.Now.Files)
		}
	}
	changed := snapshot.Build(snapshottest.Files(side("never = [\"src/domain -> src/adapters\"]\n"), side("never = [\"./src/domain/ -> src/web\"]\n")),
		conventions.Default(), snapshottest.Readings(t))
	want := []snapshot.DeclaredValues{{Table: "imports", Key: "never", Name: "`never` import rules", Now: []string{"src/domain -> src/web"},
		Before: []string{"src/domain -> src/adapters"}, Compared: true}}
	if !reflect.DeepEqual(changed.Declarations, want) {
		t.Errorf("a rule changed: Declarations %+v, want %+v", changed.Declarations, want)
	}
}

func TestBuildReadsTheTestsTableOfTheOwnFile(t *testing.T) {
	long := strings.Repeat("k", 201)
	cases := []struct {
		name     string
		file     string
		kinds    []snapshot.TestKind
		problems []string
	}{
		{"kinds in file order, a list over lines with a comment, a quoted name",
			"[tests]\nintegration = [\"**/*_integration_test.go\",\n  \"tests/integration/**\", # the slow ones\n]\n\"e2e\" = ['e2e/**']\nunit = [\"**/*_test.go\", \"tests/**\"]\n",
			[]snapshot.TestKind{{Name: "integration", Patterns: []string{"**/*_integration_test.go", "tests/integration/**"}},
				{Name: "e2e", Patterns: []string{"e2e/**"}}, {Name: "unit", Patterns: []string{"**/*_test.go", "tests/**"}}}, nil},
		{"names outside their form, quoted or bare",
			"[tests]\n\"a=b\" = [\"x/**\"]\n'a\"b' = [\"x/**\"]\n\" unit \" = [\"x/**\"]\na\"b = [\"x/**\"]\napi.integration = [\"x/**\"]\n" + long + " = [\"x/**\"]\n",
			nil, []string{"`[tests]`: 6 kind names are no plain names; they are not applied"}},
		{"a line that opens a brace does not swallow the next entry",
			"[tests]\nbad = {a = 1\nunit = [\"tests/**\"]\n",
			[]snapshot.TestKind{{Name: "unit", Patterns: []string{"tests/**"}}}, []string{"`[tests]` `bad` is not a list of text"}},
		{"a name set twice, a value that is no list, a line that is no entry, an empty list",
			"[tests]\nunit = [\"a/**\"]\nunit = [\"b/**\"]\ne2e = \"e2e/**\"\njust text\nnone = []\n",
			[]snapshot.TestKind{{Name: "unit", Patterns: []string{"a/**"}}},
			[]string{"`[tests]` `e2e` is not a list of text", "`[tests]` `unit` is set twice; its first entry decides",
				"`[tests]`: 1 line is no `name = [\"pattern\", …]` entry; it is not applied"}},
		{"patterns outside their form are counted per kind",
			"[tests]\nunit = [\"tests/**\", \"src/**.ts\", \"e2e/\", \"tests/**\"]\n",
			[]snapshot.TestKind{{Name: "unit", Patterns: []string{"tests/**"}, Unusable: 2}}, nil},
	}
	for _, tc := range cases {
		s := snapshot.Build(snapshottest.Files(nil, map[string]string{"src/a.ts": "a\n", ".plimsoll.toml": tc.file}), conventions.Default(), snapshottest.Readings(t))
		var kinds []snapshot.TestKind
		for _, k := range s.TestKinds.Kinds {
			kinds = append(kinds, snapshot.TestKind{Name: k.Name, Patterns: k.Patterns, Unusable: k.Unusable})
		}
		if !reflect.DeepEqual(kinds, tc.kinds) || !slices.Equal(s.OwnFileProblems, tc.problems) {
			t.Errorf("%s: kinds %+v, problems %q; want %+v, %q", tc.name, kinds, s.OwnFileProblems, tc.kinds, tc.problems)
		}
	}

	ordered := snapshot.Build(snapshottest.Files(
		map[string]string{"src/a.ts": "a\n", ".plimsoll.toml": "[tests]\nunit = [\"tests/**\", \"**/*_test.go\"]\ne2e = [\"e2e/**\"]\n"},
		map[string]string{"src/a.ts": "a\n", ".plimsoll.toml": "[tests]\ne2e = [\"e2e/**\"]\nunit = [\"**/*_test.go\", \"tests/**\"]\nsmoke = [\"smoke/**\"]\n"}),
		conventions.Default(), snapshottest.Readings(t))
	want := []snapshot.DeclaredValues{{Table: "tests", Key: "tests", Name: "test kinds",
		Now:    []string{"e2e: e2e/**", "unit: **/*_test.go", "unit: tests/**", "smoke: smoke/**"},
		Before: []string{"unit: **/*_test.go", "unit: tests/**", "e2e: e2e/**"}, Whole: true, Compared: true, OrderChanged: true}}
	if !reflect.DeepEqual(ordered.Declarations, want) {
		t.Errorf("kinds reordered and one added: Declarations %+v, want %+v", ordered.Declarations, want)
	}
	if only := snapshot.Build(snapshottest.Files(nil, map[string]string{"src/a.ts": "a\n", ".plimsoll.toml": "[tests]\nunit = [\"tests/**\"]\n"}),
		conventions.Default(), snapshottest.Readings(t)); len(only.OwnFileProblems) != 0 {
		t.Errorf("a file holding only [tests]: problems %q, want none", only.OwnFileProblems)
	}
}

// Property: a tests table written in any of TOML's ways (quoted or bare names, either quote, lists over lines, comments)
// reads back as the kinds it was written from, in order.
func TestTheTestsTableReadsBackWhatWasWritten(t *testing.T) {
	random := rand.New(rand.NewPCG(70, 7))
	word := func(alphabet string) string {
		b := make([]byte, 1+random.IntN(8))
		for i := range b {
			b[i] = alphabet[random.IntN(len(alphabet))]
		}
		return string(b)
	}
	for range 300 {
		var file strings.Builder
		file.WriteString("[tests]\n")
		var want []snapshot.TestKind
		seen := map[string]bool{}
		for range 1 + random.IntN(5) {
			name := word("abzAZ09_-")
			if seen[name] {
				continue
			}
			seen[name] = true
			kind := snapshot.TestKind{Name: name}
			for range 1 + random.IntN(4) {
				segments := make([]string, 1+random.IntN(4))
				for i := range segments {
					segments[i] = []string{"**", "*", "a" + word("abc._+-"), "*" + word("abc") + "?"}[random.IntN(4)]
				}
				kind.Patterns = appendOnce(kind.Patterns, strings.Join(segments, "/"))
			}
			quote := []string{"", "\"", "'"}[random.IntN(3)]
			file.WriteString(quote + name + quote + " = [")
			for i, p := range kind.Patterns {
				if i > 0 {
					file.WriteString([]string{", ", ",\n  ", ", # note\n"}[random.IntN(3)])
				}
				q := []string{"\"", "'"}[random.IntN(2)]
				file.WriteString(q + p + q)
			}
			file.WriteString([]string{"]\n", ",\n]\n", "] # done\n"}[random.IntN(3)])
			want = append(want, kind)
		}
		s := snapshot.Build(snapshottest.Files(nil, map[string]string{"src/a.ts": "a\n", ".plimsoll.toml": file.String()}), conventions.Default(), snapshottest.Readings(t))
		var got []snapshot.TestKind
		for _, k := range s.TestKinds.Kinds {
			got = append(got, snapshot.TestKind{Name: k.Name, Patterns: k.Patterns})
		}
		if !reflect.DeepEqual(got, want) || len(s.OwnFileProblems) != 0 {
			t.Fatalf("reading\n%s\ngave %+v, problems %q; want %+v", file.String(), got, s.OwnFileProblems, want)
		}
	}
}

func appendOnce(values []string, value string) []string {
	if slices.Contains(values, value) {
		return values
	}
	return append(values, value)
}

func FuzzTheOwnFile(f *testing.F) {
	for _, seed := range []string{
		"[tests]\nintegration = [\"**/*_integration_test.go\",\n  \"tests/integration/**\", # x\n]\n\"e2e\" = ['e2e/**']\n",
		"[tests]\nbad = {a = 1\nunit = [\"tests/**\"]\n", "[tests]\n\"a=b\" = [\"x/**\"]\n'a\"b' = [\"x/**\"]\n",
		"[layout]\ntest_folders = [\"checks\"]\n[acknowledged]\n\"links.cycle:a\" = \"x\"\n[imports]\nnever = [\"a -> b\"]\n",
		"[limits]\nfile_lines = 1500\n[defaults]\nbase = \"main\"\n",
	} {
		f.Add(seed)
	}
	readings := snapshottest.Readings(f)
	f.Fuzz(func(t *testing.T, own string) {
		s := snapshot.Build(snapshottest.Files(nil, map[string]string{"tests/a_test.go": "package a\n", ".plimsoll.toml": own}), conventions.Default(), readings)
		for _, k := range s.TestKinds.Kinds {
			if !conventions.KindName(k.Name) {
				t.Fatalf("kind name %q is outside its form", k.Name)
			}
			for _, p := range k.Patterns {
				if _, usable := conventions.NewPathPattern(p); !usable {
					t.Fatalf("pattern %q of %q is outside its form", p, k.Name)
				}
			}
		}
	})
}

package snapshot_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

func layoutOf(t testing.TB, base, now map[string]string) snapshot.Layout {
	t.Helper()
	return snapshot.Build(snapshottest.Files(base, now), conventions.Default(), snapshottest.Readings(t)).Layout
}

// declaring is a tree with a pyproject.toml and each file holding one line.
func declaring(pyproject string, files ...string) map[string]string {
	tree := map[string]string{"pyproject.toml": pyproject}
	for _, f := range files {
		tree[f] = "x = 1\n"
	}
	return tree
}

const pytestTable = "[tool.pytest.ini_options]\n"

// declared is a record of the python entry's pyproject.toml, read on both sides.
func declared(table, key string, inUse ...string) snapshot.Declared {
	d := snapshot.Declared{
		Language: "python", File: "pyproject.toml", Table: table, Key: key, Tool: "pytest",
		Now: snapshot.KeyRead, AtBase: snapshot.KeyRead,
	}
	for _, p := range inUse {
		d.InUse = append(d.InUse, snapshot.DeclaredFolder{Path: p})
	}
	return d
}

// owning is a tree whose .plimsoll.toml holds a [layout] table, and each other file one line.
func owning(layout string, files ...string) map[string]string {
	tree := map[string]string{".plimsoll.toml": "[layout]\n" + layout}
	for _, f := range files {
		tree[f] = "x = 1\n"
	}
	return tree
}

// own is a record of a key of the own file, read now.
func own(key, name string, asWritten bool, inUse ...string) snapshot.Declared {
	d := snapshot.Declared{File: ".plimsoll.toml", Table: "layout", Key: key, Name: name, AsWritten: asWritten, Now: snapshot.KeyRead}
	for _, p := range inUse {
		d.InUse = append(d.InUse, snapshot.DeclaredFolder{Path: p})
	}
	return d
}

func testpaths(inUse ...string) snapshot.Declared {
	return declared("tool.pytest.ini_options", "testpaths", inUse...)
}

func TestBuildReadsWhatAProjectDeclares(t *testing.T) {
	both := func(tree map[string]string) snapshot.Layout { return layoutOf(t, tree, tree) }
	one := func(d snapshot.Declared) snapshot.Layout { return snapshot.Layout{TestFolders: []snapshot.Declared{d}} }

	edge := testpaths("checks")
	edge.Held = []snapshot.HeldFolder{{Path: ".", Why: snapshot.HeldRepositoryRoot}, {Path: "src", Why: snapshot.HeldPackageRoot}, {Path: "pkg", Why: snapshot.HeldTestFolders}}
	edge.Unusable = 3
	roots := declared("tool.pytest.ini_options", "pythonpath", "checks", "src", ".")
	roots.Unusable = 1
	unreadable := testpaths()
	unreadable.Now, unreadable.AtBase = snapshot.KeyUnreadable, snapshot.KeyUnreadable
	fixed := testpaths("checks")
	fixed.AtBase = snapshot.KeyUnreadable
	moved := testpaths("checks")
	moved.InUse = append(moved.InUse, snapshot.DeclaredFolder{Path: "old", BaseOnly: true})
	shadowed := testpaths()
	shadowed.ShadowedBy = "pytest.ini"
	notRead := testpaths()
	notRead.Now, notRead.AtBase = snapshot.KeyFormNotRead, snapshot.KeyFormNotRead

	cases := []struct {
		name string
		got  snapshot.Layout
		want snapshot.Layout
	}{
		{"no pyproject.toml", both(map[string]string{"a.py": "x = 1\n"}), snapshot.Layout{}},
		{"a project table only", both(declaring("[project]\nname = \"x\"\n")), snapshot.Layout{}},
		{"test folders and roots", both(declaring(pytestTable+"testpaths = [\"tests\", \"checks\"]\npythonpath = [\"lib\"]\n", "tests/test_a.py", "checks/h.py", "lib/core/__init__.py")),
			snapshot.Layout{TestFolders: []snapshot.Declared{testpaths("tests", "checks")}, Roots: []snapshot.Declared{declared("tool.pytest.ini_options", "pythonpath", "lib")}}},
		{"one string", both(declaring(pytestTable+"testpaths = \"tests checks\"\n", "tests/test_a.py", "checks/h.py")), one(testpaths("tests", "checks"))},
		{"the tool.pytest table", both(declaring("[tool.pytest]\ntestpaths = [\"checks\"]\n", "checks/h.py")),
			one(declared("tool.pytest", "testpaths", "checks"))},
		{"both tables", both(declaring(pytestTable+"testpaths = [\"tests\"]\n\n[tool.pytest]\ntestpaths = [\"checks\"]\n", "tests/test_a.py", "checks/h.py")),
			snapshot.Layout{TestFolders: []snapshot.Declared{testpaths("tests"), declared("tool.pytest", "testpaths", "checks")}}},
		{"held and unusable values", both(declaring(pytestTable+"testpaths = [\".\", \"src\", \"pkg\", \"checks/\", \"./checks\", \"/abs\", \"../out\", \"tests/**\", \"nowhere\"]\n",
			"src/app/__init__.py", "pkg/core.py", "pkg/tests/test_core.py", "checks/h.py")), one(edge)},
		{"a shared name at the root", both(declaring(pytestTable+"testpaths = [\"tests\"]\n", "tests/unit/tests/test_x.py")), one(testpaths("tests"))},
		{"roots", both(declaring(pytestTable+"testpaths = [\"checks\"]\npythonpath = [\"checks\", \"src\", \".\", \"../x\"]\n", "checks/h.py", "src/app/__init__.py")),
			snapshot.Layout{TestFolders: []snapshot.Declared{testpaths("checks")}, Roots: []snapshot.Declared{roots}}},
		{"no string in the array", both(declaring(pytestTable + "testpaths = [\"tests\", 3]\n")), one(unreadable)},
		{"an array that does not end", both(declaring(pytestTable + "testpaths = [\n  \"tests\",\n")), one(unreadable)},
		{"a number", both(declaring(pytestTable + "testpaths = 3\n")), one(unreadable)},
		{"unreadable at the base", layoutOf(t, declaring(pytestTable+"testpaths = [3]\n", "checks/h.py"), declaring(pytestTable+"testpaths = [\"checks\"]\n", "checks/h.py")), one(fixed)},
		{"declared at the base only", layoutOf(t, declaring(pytestTable+"testpaths = [\"old\"]\n", "old/h.py"), declaring(pytestTable+"testpaths = [\"checks\"]\n", "checks/h.py")), one(moved)},
		{"a dotted key outside the table", both(declaring("tool.pytest.ini_options.testpaths = [\"checks\"]\n", "checks/h.py")), one(notRead)},
		{"a dotted key in tool.pytest", both(declaring("[tool.pytest]\nini_options.testpaths = [\"checks\"]\n", "checks/h.py")), one(notRead)},
		{"an inline table", both(declaring("[tool.pytest]\nini_options = { testpaths = [\"checks\"] }\n", "checks/h.py")), one(notRead)},
		{"a tracked pytest.ini", both(declaring(pytestTable+"testpaths = [\"checks\"]\n", "checks/h.py", "pytest.ini")),
			snapshot.Layout{TestFolders: []snapshot.Declared{shadowed}, Shadowing: []string{"pytest.ini"}}},
		{"an indented header", both(declaring("  [tool.pytest.ini_options]\ntestpaths = [\"checks\"]\n", "checks/h.py")), one(testpaths("checks"))},
		{"an array of tables is no table", both(declaring("[[tool.pytest.ini_options]]\ntestpaths = [\"checks\"]\n", "checks/h.py")), snapshot.Layout{}},
		{"a spaced and quoted header", both(declaring("[ tool.\"pytest\".ini_options ]\ntestpaths = [\"checks\"]\n", "checks/h.py")), one(testpaths("checks"))},
		{"shadowing files without a table", both(declaring("[project]\n", "pytest.ini", "pytest.toml")), snapshot.Layout{Shadowing: []string{"pytest.toml", "pytest.ini"}}},
		{"pytest.ini at the base only", layoutOf(t, declaring(pytestTable+"testpaths = [\"checks\"]\n", "checks/h.py", "pytest.ini"), declaring(pytestTable+"testpaths = [\"checks\"]\n", "checks/h.py")),
			one(testpaths("checks"))},
		{"a byte order mark", both(declaring("\uFEFF"+pytestTable+"testpaths = [\"checks\"]\n", "checks/h.py")), one(testpaths("checks"))},
		{"CRLF line ends", both(declaring("[tool.pytest.ini_options]\r\ntestpaths = [\"checks\"]\r\n", "checks/h.py")), one(testpaths("checks"))},
		{"comments", both(declaring("[tool.pytest.ini_options] # pytest\ntestpaths = [\n  \"checks\", # why\n]\n", "checks/h.py")), one(testpaths("checks"))},
		{"a quoted key", both(declaring(pytestTable+"\"testpaths\" = [\"checks\"]\n", "checks/h.py")), one(testpaths("checks"))},
		{"an empty array", both(declaring(pytestTable + "testpaths = []\n")), one(testpaths())},
		{"an empty string", both(declaring(pytestTable + "testpaths = \"\"\n")), one(testpaths())},
		{"another table", both(declaring("[tool.other]\ntestpaths = [\"checks\"]\n", "checks/h.py")), snapshot.Layout{}},
		{"a nested pyproject.toml", both(map[string]string{"pkg/pyproject.toml": pytestTable + "testpaths = [\"checks\"]\n", "checks/h.py": "x\n"}), snapshot.Layout{}},
	}
	for _, tc := range cases {
		if !reflect.DeepEqual(tc.got, tc.want) {
			t.Errorf("%s: Build.Layout =\n%+v\nwant\n%+v", tc.name, tc.got, tc.want)
		}
	}
	ownFolders, ownFiles := own("test_folders", "test folders", false, "tests"), own("test_files", "test files", true)
	ownFolders.Held = []snapshot.HeldFolder{{Path: ".", Why: snapshot.HeldRepositoryRoot}, {Path: "src", Why: snapshot.HeldPackageRoot}}
	ownFolders.Unmatched, ownFolders.Unusable = []string{"nowhere", "Checks"}, 1
	ownFiles.Held = []snapshot.HeldFolder{{Path: "*", Why: snapshot.HeldEveryName}, {Path: "**", Why: snapshot.HeldEveryName}}
	ownFiles.Unmatched, ownFiles.Unusable = []string{"*_nope.rb", "checks"}, 1
	pytestAndOwn := owning("test_folders = [\"checks\"]\n", "checks/h.py", "tests/test_a.py")
	pytestAndOwn["pyproject.toml"] = pytestTable + "testpaths = [\"tests\"]\n"
	ownCases := []struct {
		name string
		got  snapshot.Layout
		want snapshot.Layout
	}{
		{"own: in use", both(owning("test_folders = [\"checks\", \"tools/harness\"]\ntest_files = [\"*.check.ts\"]\n", "checks/h.py", "tools/harness/run.py", "src/a.check.ts")),
			snapshot.Layout{Own: []snapshot.Declared{own("test_folders", "test folders", false, "checks", "tools/harness"), own("test_files", "test files", true, "*.check.ts")}}},
		{"own: held, unmatched, unusable", both(owning("test_folders = [\".\", \"src\", \"nowhere\", \"Checks\", \"a b\", \"tests\"]\ntest_files = [\"*\", \"**\", \"*_nope.rb\", \"checks\", \"a/b\"]\n",
			"src/app/__init__.py", "tests/test_a.py", "checks/h.py")),
			snapshot.Layout{Own: []snapshot.Declared{ownFolders, ownFiles}}},
		{"own: no guard for a folder holding test folders", both(owning("test_folders = [\"pkg\"]\n", "pkg/core.py", "pkg/tests/test_core.py")),
			snapshot.Layout{Own: []snapshot.Declared{own("test_folders", "test folders", false, "pkg")}}},
		{"own: tracked at the base only", layoutOf(t, map[string]string{"checks/h.py": "x = 1\n"}, owning("test_folders = [\"checks\"]\n", "src/a.py")),
			snapshot.Layout{Own: []snapshot.Declared{own("test_folders", "test folders", false, "checks")}}},
		{"own: an empty list", both(owning("test_folders = []\n", "src/a.py")), snapshot.Layout{}},
		{"own: beside pytest", both(pytestAndOwn),
			snapshot.Layout{TestFolders: []snapshot.Declared{testpaths("tests")}, Own: []snapshot.Declared{own("test_folders", "test folders", false, "checks")}}},
	}
	for _, tc := range ownCases {
		if !reflect.DeepEqual(tc.got, tc.want) {
			t.Errorf("%s: Build.Layout =\n%+v\nwant\n%+v", tc.name, tc.got, tc.want)
		}
	}
	source := snapshot.Source{Now: []snapshot.Entry{{Path: "pyproject.toml"}}}
	if got := snapshot.Build(source, conventions.Default(), snapshottest.Readings(t)).Layout; !reflect.DeepEqual(got, snapshot.Layout{}) {
		t.Errorf("a pyproject.toml whose content was not read: Build.Layout = %+v, want none", got)
	}
}

// roleOf is the role of a file on a side; ok is false when it is no code file.
func roleOf(tree snapshot.Tree, p string) (snapshot.Role, bool) {
	for _, f := range tree.Files {
		if f.Path == p {
			return f.Role, true
		}
	}
	return 0, false
}

func TestBuildCountsADeclaredTestFolderAsTestCode(t *testing.T) {
	declare := func(values string) string { return "[tool.pytest.ini_options]\ntestpaths = [" + values + "]\n" }
	none := snapshot.Role(0)
	cases := []struct {
		name  string
		tree  map[string]string
		roles map[string]snapshot.Role // none: no code file
	}{
		{"a declared folder", declaring(declare(`"checks"`), "checks/h.py", "checks-old/h.py", "src/a.py"),
			map[string]snapshot.Role{"checks/h.py": snapshot.TestInfrastructure, "checks-old/h.py": snapshot.Product, "src/a.py": snapshot.Product}},
		{"a nested folder", declaring(declare(`"src/app/selftest"`), "src/app/selftest/util.py", "src/app/core.py"),
			map[string]snapshot.Role{"src/app/selftest/util.py": snapshot.TestInfrastructure, "src/app/core.py": snapshot.Product}},
		{"held values", func() map[string]string {
			tree := declaring(declare(`".", "src", "pkg"`), "src/app/x.py", "pkg/core.py")
			tree["pkg/tests/test_core.py"] = "def test_x(): pass\n"
			return tree
		}(), map[string]snapshot.Role{"src/app/x.py": snapshot.Product, "pkg/core.py": snapshot.Product, "pkg/tests/test_core.py": snapshot.Test}},
		{"every language", declaring(declare(`"checks"`), "checks/helper.ts"), map[string]snapshot.Role{"checks/helper.ts": snapshot.TestInfrastructure}},
		{"under other code", declaring(declare(`"scripts/checks"`), "scripts/checks/x.py", "scripts/seed.py"),
			map[string]snapshot.Role{"scripts/checks/x.py": snapshot.TestInfrastructure, "scripts/seed.py": snapshot.Other}},
		{"under a skipped folder", declaring(declare(`"build/checks"`), "build/checks/x.py"), map[string]snapshot.Role{"build/checks/x.py": none}},
		{"own: a declared folder", owning("test_folders = [\"checks\"]\n", "checks/h.py", "checks-old/h.py", "lib/checks/h.py", "src/a.py"),
			map[string]snapshot.Role{"checks/h.py": snapshot.TestInfrastructure, "checks-old/h.py": snapshot.Product, "lib/checks/h.py": snapshot.Product, "src/a.py": snapshot.Product}},
		{"own: declared file names", owning("test_files = [\"*.check.ts\"]\n", "src/a.check.ts", "src/b.ts", "scripts/x.check.ts", "build/y.check.ts"),
			map[string]snapshot.Role{"src/a.check.ts": snapshot.TestInfrastructure, "src/b.ts": snapshot.Product, "scripts/x.check.ts": snapshot.TestInfrastructure, "build/y.check.ts": none}},
		{"own: held folders", owning("test_folders = [\"src\", \".\"]\n", "src/app/x.py"), map[string]snapshot.Role{"src/app/x.py": snapshot.Product}},
	}
	for _, tc := range cases {
		s := snapshot.Build(snapshottest.Files(tc.tree, tc.tree), conventions.Default(), snapshottest.Readings(t))
		for p, want := range tc.roles {
			got, isCode := roleOf(s.Now, p)
			if want == none && isCode || want != none && got != want {
				t.Errorf("%s: role of %s = %v (code %v), want %v", tc.name, p, got, isCode, want)
			}
		}
	}

	history := snapshottest.Files(nil, declaring(declare(`"checks"`), "checks/h.py"))
	history.Commits = []snapshot.Commit{{ID: "x", Files: []snapshot.FileChange{{Path: "checks/h.py"}}}}
	if got := snapshot.Build(history, conventions.Default(), snapshottest.Readings(t)).History.Commits[0].Files[0].Area; got != snapshot.AreaTests {
		t.Errorf("a commit touching a declared folder: area %v, want tests", got)
	}
	ownHistory := snapshottest.Files(nil, owning("test_files = [\"*.check.ts\"]\n", "src/a.check.ts"))
	ownHistory.Commits = []snapshot.Commit{{ID: "x", Files: []snapshot.FileChange{{Path: "src/a.check.ts"}}}}
	if got := snapshot.Build(ownHistory, conventions.Default(), snapshottest.Readings(t)).History.Commits[0].Files[0].Area; got != snapshot.AreaTests {
		t.Errorf("a commit touching a declared test file name: area %v, want tests", got)
	}
	for name, sides := range map[string][2]map[string]string{
		"declared now only":                 {{"checks/h.py": "x = 1\n"}, declaring(declare(`"checks"`), "checks/h.py")},
		"declared in the own file now only": {{"checks/h.py": "x = 1\n"}, owning("test_folders = [\"checks\"]\n", "checks/h.py")},
		"declared at the base only":         {declaring(declare(`"checks"`), "checks/h.py"), declaring("[project]\n", "checks/h.py")},
	} {
		s := snapshot.Build(snapshottest.Files(sides[0], sides[1]), conventions.Default(), snapshottest.Readings(t))
		before, _ := roleOf(s.Before, "checks/h.py")
		now, _ := roleOf(s.Now, "checks/h.py")
		if before != snapshot.TestInfrastructure || now != snapshot.TestInfrastructure {
			t.Errorf("%s: checks/h.py at the base %v, now %v; want test infrastructure on both sides", name, before, now)
		}
	}

	rules := conventions.Default()
	snapshot.Build(snapshottest.Files(nil, declaring(declare(`"checks"`)+"pythonpath = [\"lib\"]\n", "checks/h.py", "lib/x/__init__.py", "src/a.py")), rules, snapshottest.Readings(t))
	snapshot.Build(snapshottest.Files(nil, owning("test_folders = [\"checks\"]\ntest_files = [\"*.check.ts\"]\n", "checks/h.py", "src/a.check.ts")), rules, snapshottest.Readings(t))
	second := snapshot.Build(snapshottest.Files(nil, map[string]string{"checks/h.py": "x = 1\n", "src/a.py": "x = 1\n"}), rules, snapshottest.Readings(t))
	if got, _ := roleOf(second.Now, "checks/h.py"); got != snapshot.Product || !reflect.DeepEqual(second.Layout, snapshot.Layout{}) ||
		!slices.Equal(second.Now.Links.Languages[0].Roots, []string{".", "src"}) {
		t.Errorf("a second build with the same rules: checks/h.py %v, layout %+v, links %+v", got, second.Layout, second.Now.Links.Languages)
	}
	python, _ := rules.LanguageOf("a.py")
	if len(rules.TestPaths) != 0 || len(rules.TestNames) != 0 || !slices.Equal(python.Imports.Prefix.Roots, []string{".", "src"}) {
		t.Errorf("the rules after the builds: TestPaths %q, TestNames %q, python roots %q; want them as given", rules.TestPaths, rules.TestNames, python.Imports.Prefix.Roots)
	}
}

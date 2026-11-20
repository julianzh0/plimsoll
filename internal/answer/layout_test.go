package answer

import (
	"slices"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

// testpaths is a testpaths record of pyproject.toml, read on both sides, with the folders in use.
func testpaths(inUse ...string) snapshot.Declared {
	d := snapshot.Declared{
		Language: "python", File: "pyproject.toml", Table: "tool.pytest.ini_options", Key: "testpaths", Tool: "pytest",
		Now: snapshot.KeyRead, AtBase: snapshot.KeyRead,
	}
	for _, p := range inUse {
		d.InUse = append(d.InUse, snapshot.DeclaredFolder{Path: p})
	}
	return d
}

func TestLayoutSaysWhatIsDeclaredAndWhatIsNotUsed(t *testing.T) {
	roots := func(d snapshot.Declared) snapshot.Declared {
		d.Key = "pythonpath"
		return d
	}
	withState := func(now, base snapshot.KeyState) snapshot.Declared {
		d := testpaths()
		d.Now, d.AtBase = now, base
		return d
	}
	held := func(why snapshot.HeldReason, p string) snapshot.Declared {
		d := testpaths()
		d.Held = []snapshot.HeldFolder{{Path: p, Why: why}}
		return d
	}
	unusable := func(n int) snapshot.Declared {
		d := testpaths()
		d.Unusable = n
		return d
	}
	ownRecord := func(key, name string, asWritten bool, inUse ...string) snapshot.Declared {
		d := snapshot.Declared{File: ".plimsoll.toml", Table: "layout", Key: key, Name: name, AsWritten: asWritten, Now: snapshot.KeyRead}
		for _, p := range inUse {
			d.InUse = append(d.InUse, snapshot.DeclaredFolder{Path: p})
		}
		return d
	}
	ownFolders := func(inUse ...string) snapshot.Declared {
		return ownRecord("test_folders", "test folders", false, inUse...)
	}
	ownFiles := func(inUse ...string) snapshot.Declared { return ownRecord("test_files", "test files", true, inUse...) }
	ownWith := func(d snapshot.Declared, change func(*snapshot.Declared)) snapshot.Layout {
		change(&d)
		return snapshot.Layout{Own: []snapshot.Declared{d}}
	}
	moved := testpaths("tests")
	moved.InUse = append(moved.InUse, snapshot.DeclaredFolder{Path: "old", BaseOnly: true})
	type lines struct{ testNotes, testGaps, rootNotes, rootGaps, ownNotes, ownGaps []string }
	cases := []struct {
		name   string
		layout snapshot.Layout
		want   lines
	}{
		{"a note", snapshot.Layout{TestFolders: []snapshot.Declared{testpaths("tests", "checks")}},
			lines{testNotes: []string{"test folders declared in `pyproject.toml` (`[tool.pytest.ini_options]` `testpaths`): `tests/`, `checks/`"}}},
		{"at the base only", snapshot.Layout{TestFolders: []snapshot.Declared{moved}},
			lines{testNotes: []string{"test folders declared in `pyproject.toml` (`[tool.pytest.ini_options]` `testpaths`): `tests/`, `old/` (at the base only)"}}},
		{"a roots note", snapshot.Layout{Roots: []snapshot.Declared{roots(testpaths("src", "."))}},
			lines{rootNotes: []string{"`python`: roots declared in `pyproject.toml` (`[tool.pytest.ini_options]` `pythonpath`): `src/`, the repository root"}}},
		{"unreadable", snapshot.Layout{TestFolders: []snapshot.Declared{withState(snapshot.KeyUnreadable, 0)}},
			lines{testGaps: []string{"`pyproject.toml` `testpaths` could not be read, so the test folders declared there are not used"}}},
		{"unreadable, the base read", snapshot.Layout{TestFolders: []snapshot.Declared{withState(snapshot.KeyUnreadable, snapshot.KeyRead)}},
			lines{testGaps: []string{"`pyproject.toml` `testpaths` could not be read; the test folders declared at the base are used for both sides"}}},
		{"unreadable at the base", snapshot.Layout{TestFolders: []snapshot.Declared{withState(snapshot.KeyRead, snapshot.KeyUnreadable)}},
			lines{testGaps: []string{"`pyproject.toml` `testpaths` could not be read at the base; the test folders declared now are used for both sides"}}},
		{"held: the root", snapshot.Layout{TestFolders: []snapshot.Declared{held(snapshot.HeldRepositoryRoot, ".")}},
			lines{testGaps: []string{"`pyproject.toml` `testpaths` names the repository root; only the shared rules apply"}}},
		{"held: a package root", snapshot.Layout{TestFolders: []snapshot.Declared{held(snapshot.HeldPackageRoot, "src")}},
			lines{testGaps: []string{"`pyproject.toml` `testpaths` names `src/`, which is or holds a package root; only the shared rules apply inside it"}}},
		{"held: test folders", snapshot.Layout{TestFolders: []snapshot.Declared{held(snapshot.HeldTestFolders, "pkg")}},
			lines{testGaps: []string{"`pyproject.toml` `testpaths` names `pkg/`, which holds test folders of its own: it is where tests are collected, so only the shared rules apply inside it"}}},
		{"two unusable", snapshot.Layout{TestFolders: []snapshot.Declared{unusable(2)}},
			lines{testGaps: []string{"`pyproject.toml` `testpaths`: 2 values are absolute, leave the repository or hold a pattern; they are not used"}}},
		{"one unusable root", snapshot.Layout{Roots: []snapshot.Declared{roots(unusable(1))}},
			lines{rootGaps: []string{"`python`: `pyproject.toml` `pythonpath`: 1 value is absolute, leaves the repository or holds a pattern; it is not used"}}},
		{"nothing to say", snapshot.Layout{TestFolders: []snapshot.Declared{testpaths()}}, lines{}},
		{"shadowing files", func() snapshot.Layout {
			d := testpaths()
			d.ShadowedBy = "pytest.toml"
			return snapshot.Layout{TestFolders: []snapshot.Declared{d}}
		}(), lines{testGaps: []string{"`testpaths` in `pyproject.toml` is not used, because pytest reads `pytest.toml` instead"}}},
		{"own: folders in use", snapshot.Layout{Own: []snapshot.Declared{ownFolders("checks", "tools/harness")}},
			lines{ownNotes: []string{"test folders declared in `.plimsoll.toml` (`[layout]` `test_folders`): `checks/`, `tools/harness/`"}}},
		{"own: names in use", snapshot.Layout{Own: []snapshot.Declared{ownFiles("*_spec.rb", "*IT.java")}},
			lines{ownNotes: []string{"test files declared in `.plimsoll.toml` (`[layout]` `test_files`): `*_spec.rb`, `*IT.java`"}}},
		{"own: held root", ownWith(ownFolders(), func(d *snapshot.Declared) {
			d.Held = []snapshot.HeldFolder{{Path: ".", Why: snapshot.HeldRepositoryRoot}}
		}),
			lines{ownGaps: []string{"`.plimsoll.toml` `test_folders` names the repository root; only the shared rules apply"}}},
		{"own: held package root", ownWith(ownFolders(), func(d *snapshot.Declared) {
			d.Held = []snapshot.HeldFolder{{Path: "src", Why: snapshot.HeldPackageRoot}}
		}),
			lines{ownGaps: []string{"`.plimsoll.toml` `test_folders` names `src/`, which is or holds a package root; only the shared rules apply inside it"}}},
		{"own: held every name", ownWith(ownFiles(), func(d *snapshot.Declared) { d.Held = []snapshot.HeldFolder{{Path: "*", Why: snapshot.HeldEveryName}} }),
			lines{ownGaps: []string{"`.plimsoll.toml` `test_files` names every file; only the shared rules apply"}}},
		{"own: one unmatched folder", ownWith(ownFolders(), func(d *snapshot.Declared) { d.Unmatched = []string{"chekcs"} }),
			lines{ownGaps: []string{"`.plimsoll.toml` `test_folders`: `chekcs/` matches no tracked file; it is not used"}}},
		{"own: two unmatched names", ownWith(ownFiles(), func(d *snapshot.Declared) { d.Unmatched = []string{"*_a.rb", "*_b.rb"} }),
			lines{ownGaps: []string{"`.plimsoll.toml` `test_files`: `*_a.rb`, `*_b.rb` match no tracked file; they are not used"}}},
		{"own: two unusable folders", ownWith(ownFolders(), func(d *snapshot.Declared) { d.Unusable = 2 }),
			lines{ownGaps: []string{"`.plimsoll.toml` `test_folders`: 2 values are no plain paths from the root; they are not used"}}},
		{"own: one unusable folder", ownWith(ownFolders(), func(d *snapshot.Declared) { d.Unusable = 1 }),
			lines{ownGaps: []string{"`.plimsoll.toml` `test_folders`: 1 value is no plain path from the root; it is not used"}}},
		{"own: one unusable name", ownWith(ownFiles(), func(d *snapshot.Declared) { d.Unusable = 1 }),
			lines{ownGaps: []string{"`.plimsoll.toml` `test_files`: 1 value is no plain file-name pattern; it is not used"}}},
		{"own: import rules in use, and two outside their form", ownWith(ownRecord("never", "`never` import rules", true, "src/domain -> src/adapters"), func(d *snapshot.Declared) {
			d.Table, d.Unusable, d.Form = "imports", 2, conventions.NeverImportRules.Shape()
		}),
			lines{ownNotes: []string{"`never` import rules declared in `.plimsoll.toml` (`[imports]` `never`): `src/domain -> src/adapters`"},
				ownGaps: []string{"`.plimsoll.toml` `never`: 2 values are no import rule of the form `folder -> folder` (two different folders from the root, neither inside the other); they are not used"}}},
		{"a shadowed root key", func() snapshot.Layout {
			d := roots(testpaths())
			d.ShadowedBy = "pytest.ini"
			return snapshot.Layout{Roots: []snapshot.Declared{d}}
		}(), lines{rootGaps: []string{"`python`: `pythonpath` in `pyproject.toml` is not used, because pytest reads `pytest.ini` instead"}}},
	}
	for _, tc := range cases {
		own := tc.layout.Own
		got := lines{TestFolderNotes(tc.layout), TestFoldersNotUsed(tc.layout), RootNotes(tc.layout), RootsNotUsed(tc.layout), OwnNotes(own), OwnNotUsed(own)}
		if !slices.Equal(got.testNotes, tc.want.testNotes) || !slices.Equal(got.testGaps, tc.want.testGaps) ||
			!slices.Equal(got.rootNotes, tc.want.rootNotes) || !slices.Equal(got.rootGaps, tc.want.rootGaps) ||
			!slices.Equal(got.ownNotes, tc.want.ownNotes) || !slices.Equal(got.ownGaps, tc.want.ownGaps) {
			t.Errorf("%s: lines =\n%q\nwant\n%q", tc.name, got, tc.want)
		}
	}
}

func TestLayoutLinesHoldNoValueFromOutsideTheRepository(t *testing.T) {
	tree := map[string]string{
		"pyproject.toml": "[tool.pytest.ini_options]\n" + "testpaths = [\"/zzmarker/abs\", \"../zzmarker-out\", \"zzmarker-nowhere\", \"tests/zz*marker\"]\n" +
			"pythonpath = [\"/zzmarker/root\", \"zzmarker-none\"]\n",
		"src/app/__init__.py": "",
	}
	layout := snapshot.Build(snapshottest.Files(nil, tree), conventions.Default(), snapshottest.Readings(t)).Layout
	all := slices.Concat(TestFolderNotes(layout), TestFoldersNotUsed(layout), RootNotes(layout), RootsNotUsed(layout))
	for _, line := range all {
		if strings.Contains(line, "zzmarker") {
			t.Errorf("a layout line holds a declared value: %q", line)
		}
	}
	if !slices.Contains(TestFoldersNotUsed(layout), "`pyproject.toml` `testpaths`: 3 values are absolute, leave the repository or hold a pattern; they are not used") ||
		!slices.Contains(RootsNotUsed(layout), "`python`: `pyproject.toml` `pythonpath`: 1 value is absolute, leaves the repository or holds a pattern; it is not used") {
		t.Errorf("layout lines = %q, want the two count sentences", all)
	}
}

package snapshot_test

import (
	"maps"
	"testing"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

// addedLinks builds a snapshot from two sides and returns, per link of now as "<from> > <to>", whether the change
// wrote its import. changes, when given, replace the changes the two sides differ by.
func addedLinks(t *testing.T, base, now map[string]string, changes ...snapshot.Change) map[string]bool {
	t.Helper()
	source := snapshottest.Files(base, now)
	if len(changes) > 0 {
		source.Changes = changes
	}
	s := snapshot.Build(source, conventions.Default(), snapshottest.Readings(t))
	for _, link := range s.Before.Links.List {
		if link.Added {
			t.Errorf("Build(%v -> %v): a link of the base %s > %s is Added, want none", base, now, link.From, link.To)
		}
	}
	got := map[string]bool{}
	for _, link := range s.Now.Links.List {
		got[link.From+" > "+link.To] = link.Added
	}
	return got
}

func TestBuildMarksTheImportsAChangeWrote(t *testing.T) {
	cases := []struct {
		name      string
		base, now map[string]string
		changes   []snapshot.Change
		want      map[string]bool
	}{
		{
			name: "a new file",
			base: map[string]string{"src/b.ts": "export const b = 1;\n"},
			now:  map[string]string{"src/b.ts": "export const b = 1;\n", "src/a.ts": "import \"./b.ts\";\n"},
			want: map[string]bool{"src/a.ts > src/b.ts": true},
		},
		{
			name: "a new statement",
			base: map[string]string{"a.ts": "import \"./b.ts\";\n", "b.ts": "export const b = 1;\n", "c.ts": "export const c = 1;\n"},
			now:  map[string]string{"a.ts": "import \"./b.ts\";\nimport \"./c.ts\";\n", "b.ts": "export const b = 1;\n", "c.ts": "export const c = 1;\n"},
			want: map[string]bool{"a.ts > b.ts": false, "a.ts > c.ts": true},
		},
		{
			name: "an unchanged file beside a changed one",
			base: map[string]string{"d.ts": "import \"./b.ts\";\n", "b.ts": "export const b = 1;\n", "e.ts": "export const e = 1;\n"},
			now:  map[string]string{"d.ts": "import \"./b.ts\";\n", "b.ts": "export const b = 1;\n", "e.ts": "export const e = 2;\n"},
			want: map[string]bool{"d.ts > b.ts": false},
		},
		{
			name: "readable only now",
			base: map[string]string{"src/app/a.ts": "import \"@app/b\";\n", "src/app/b.ts": "export const b = 1;\n"},
			now: map[string]string{
				"src/app/a.ts":  "// the app\nimport \"@app/b\";\n",
				"src/app/b.ts":  "export const b = 1;\n",
				"tsconfig.json": `{"compilerOptions":{"paths":{"@app/*":["src/app/*"]}}}` + "\n",
			},
			want: map[string]bool{"src/app/a.ts > src/app/b.ts": false},
		},
		{
			name:    "a moved file",
			base:    map[string]string{"src/a.ts": "import \"./b.ts\";\n", "src/b.ts": "export const b = 1;\n"},
			now:     map[string]string{"src/sub/a.ts": "import \"../b.ts\";\n", "src/b.ts": "export const b = 1;\n"},
			changes: []snapshot.Change{{Path: "src/sub/a.ts", From: "src/a.ts"}},
			want:    map[string]bool{"src/sub/a.ts > src/b.ts": false},
		},
		{
			name:    "a renamed target",
			base:    map[string]string{"src/a.ts": "import \"./b.ts\";\n", "src/b.ts": "export const b = 1;\n"},
			now:     map[string]string{"src/a.ts": "import \"./c.ts\";\n", "src/c.ts": "export const b = 1;\n"},
			changes: []snapshot.Change{{Path: "src/c.ts", From: "src/b.ts"}, {Path: "src/a.ts", Added: 1, Removed: 1}},
			want:    map[string]bool{"src/a.ts > src/c.ts": false},
		},
		{
			name: "one more name",
			base: map[string]string{"m.py": "from pkg import b\n", "pkg/__init__.py": "", "pkg/b.py": "B = 1\n", "pkg/c.py": "C = 1\n"},
			now:  map[string]string{"m.py": "from pkg import b, c\n", "pkg/__init__.py": "", "pkg/b.py": "B = 1\n", "pkg/c.py": "C = 1\n"},
			want: map[string]bool{"m.py > pkg/b.py": false, "m.py > pkg/c.py": true},
		},
		{
			name: "names in another order, on another line",
			base: map[string]string{"m.py": "from pkg import b, c\n", "pkg/__init__.py": "", "pkg/b.py": "B = 1\n", "pkg/c.py": "C = 1\n"},
			now:  map[string]string{"m.py": "\nfrom pkg import c, b\n", "pkg/__init__.py": "", "pkg/b.py": "B = 1\n", "pkg/c.py": "C = 1\n"},
			want: map[string]bool{"m.py > pkg/b.py": false, "m.py > pkg/c.py": false},
		},
		{
			name: "type-only turned ordinary",
			base: map[string]string{"a.ts": "import type { B } from \"./b.ts\";\n", "b.ts": "export type B = number;\n"},
			now:  map[string]string{"a.ts": "import { B } from \"./b.ts\";\n", "b.ts": "export type B = number;\n"},
			want: map[string]bool{"a.ts > b.ts": true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := addedLinks(t, tc.base, tc.now, tc.changes...); !maps.Equal(got, tc.want) {
				t.Errorf("Build(%v -> %v) Added = %v, want %v", tc.base, tc.now, got, tc.want)
			}
		})
	}
}

func TestBuildAddsNoImportWhoseBaseContentIsMissing(t *testing.T) {
	base := map[string]string{"src/a.ts": "import \"./b.ts\";\nexport const a = 1;\n", "src/b.ts": "import \"./a.ts\";\n"}
	now := map[string]string{"src/a.ts": "import \"./b.ts\";\nexport const a = 2;\n", "src/b.ts": base["src/b.ts"]}
	source := snapshottest.Files(base, now)
	for i := range source.Before {
		if source.Before[i].Path == "src/a.ts" {
			source.Before[i].Content = nil
		}
	}
	for _, link := range snapshot.Build(source, conventions.Default(), snapshottest.Readings(t)).Now.Links.List {
		if link.Added {
			t.Errorf("Build(base content of src/a.ts missing): %s > %s is Added, want no import of a file read on one side only", link.From, link.To)
		}
	}
}

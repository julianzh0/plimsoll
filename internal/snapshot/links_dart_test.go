package snapshot_test

import (
	"reflect"
	"testing"

	"github.com/julianzh0/plimsoll/internal/snapshot"
)

func TestBuildFollowsImportsIntoDartWorkspaceMembers(t *testing.T) {
	got := linksOf(t, map[string]string{
		"pubspec.yaml":                  "name: _ws\nworkspace:\n  - app # the app\n  - 'packages/core'\n  - packages/gone\n",
		"app/pubspec.yaml":              "name: app\nresolution: workspace\n",
		"packages/core/pubspec.yaml":    "name: core\nresolution: workspace\n",
		"app/lib/main.dart":             "import 'package:core/core.dart';\nimport 'package:app/src/x.dart';\nimport 'package:http/http.dart';\n",
		"app/lib/src/x.dart":            "const x = 1;\n",
		"packages/core/lib/core.dart":   "const c = 1;\n",
		"packages/other/pubspec.yaml":   "name: other\n",
		"packages/other/lib/other.dart": "import 'package:core/core.dart';\n",
	})
	// other is no member and depends on core by no path: its import is outside, as for http.
	want := []snapshot.Link{link("app/lib/main.dart", 1, "packages/core/lib/core.dart"), link("app/lib/main.dart", 2, "app/lib/src/x.dart")}
	if !reflect.DeepEqual(got.List, want) || got.Languages[0].Outside != 2 || got.Languages[0].NotFound != 0 {
		t.Errorf("links = %+v, languages %+v; want the members' packages as own and http and other's import outside", got.List, got.Languages)
	}
	// A self-import reads the importer's own package, not a fixture of its name.
	self := linksOf(t, map[string]string{
		"fixtures/foo/pubspec.yaml": "name: foo\n", "fixtures/foo/lib/b.dart": "",
		"packages/foo/pubspec.yaml": "name: foo\n", "packages/foo/lib/b.dart": "", "packages/foo/lib/a.dart": "import 'package:foo/b.dart';\n",
	})
	if want := []snapshot.Link{link("packages/foo/lib/a.dart", 1, "packages/foo/lib/b.dart")}; !reflect.DeepEqual(self.List, want) {
		t.Errorf("a self-import beside a fixture of the same name: links = %+v, want %+v", self.List, want)
	}
	// A glob names members; a path dependency is own outside a workspace.
	globbed := linksOf(t, map[string]string{
		"pubspec.yaml": "name: _ws\nworkspace:\n  - packages/*\n", "packages/a/pubspec.yaml": "name: a\n",
		"packages/b/pubspec.yaml": "name: b\n", "packages/a/lib/a.dart": "import 'package:b/b.dart';\n", "packages/b/lib/b.dart": "",
		"tool/pubspec.yaml": "name: tool\ndependencies:\n  b:\n    path: ../packages/b\n", "tool/lib/t.dart": "import 'package:b/b.dart';\n",
	})
	if want := []snapshot.Link{link("packages/a/lib/a.dart", 1, "packages/b/lib/b.dart"), link("tool/lib/t.dart", 1, "packages/b/lib/b.dart")}; !reflect.DeepEqual(globbed.List, want) {
		t.Errorf("a glob member and a path dependency: links = %+v, want %+v", globbed.List, want)
	}
	flow := linksOf(t, map[string]string{
		"packages/b/pubspec.yaml": "name: b\n", "packages/b/lib/b.dart": "",
		"util/pubspec.yaml":         "name: util\r\ndependencies: {b: {path: ../packages/b}, c: {path: \"../local libs/c\"}, d: {git: {url: x, path: d}}}\r\n",
		"util/lib/u.dart":           "import 'package:b/b.dart';\nimport 'package:c/c.dart';\nimport 'package:d/d.dart';\nimport 'package:e/e.dart';\n",
		"local libs/c/pubspec.yaml": "name: c\n", "local libs/c/lib/c.dart": "", "util/d/pubspec.yaml": "name: d\n", "util/d/lib/d.dart": "",
		"tool/pubspec.yaml":   "name: tool\ndependencies:\n  e:\n    git:\n      url: x\n      path: ../util/e # in the repository\n",
		"util/e/pubspec.yaml": "name: e\n", "util/e/lib/e.dart": "", "tool/lib/t.dart": "import 'package:e/e.dart';\n",
	})
	if want := []snapshot.Link{link("util/lib/u.dart", 1, "packages/b/lib/b.dart"), link("util/lib/u.dart", 2, "local libs/c/lib/c.dart")}; !reflect.DeepEqual(flow.List, want) {
		t.Errorf("path dependencies in a flow map on CRLF lines, quoted with a space; a git entry's path is no local one: links = %+v, want %+v", flow.List, want)
	}
}

package snapshot_test

import (
	"maps"
	"reflect"
	"testing"

	"github.com/julianzh0/plimsoll/internal/snapshot"
)

// Go modules: a file's own module is the nearest go.mod; another tracked module is own through a workspace that uses
// both, or a replacement of a module the importer requires; a nested go.mod makes its folder a module of its own.
func TestBuildFollowsGoImportsAcrossModules(t *testing.T) {
	lib := map[string]string{"lib/go.mod": "module example.com/lib\n", "lib/x/x.go": "package x\n"}
	app := func(mod string) map[string]string {
		files := maps.Clone(lib)
		files["app/go.mod"] = mod
		files["app/main.go"] = "package main\n\nimport \"example.com/lib/x\"\n"
		return files
	}
	reaches := []snapshot.Link{folderLink("app/main.go", 3, "lib/x", "lib/x/x.go")}
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  []snapshot.Link
	}{
		{"no workspace, no replacement", app("module example.com/app\n\nrequire example.com/lib v1.0.0\n"), nil},
		{"a replacement of a required module", app("module example.com/app\n\nrequire example.com/lib v1.0.0\n\nreplace example.com/lib => ../lib\n"), reaches},
		{"a replacement of the required version", app("module example.com/app\n\nrequire (\n\texample.com/lib v1.0.0\n)\n\nreplace example.com/lib v1.0.0 => ../lib\n"), reaches},
		{"a replacement of another version", app("module example.com/app\n\nrequire example.com/lib v1.0.0\n\nreplace example.com/lib v2.0.0 => ../lib\n"), nil},
		{"a replacement of a module not required", app("module example.com/app\n\nreplace example.com/lib => ../lib\n"), nil},
		{"a workspace that uses both", func() map[string]string {
			files := app("module example.com/app\n")
			files["go.work"] = "go 1.22\n\nuse (\n\t./app\n\t./lib\n)\n"
			return files
		}(), reaches},
		{"a workspace that does not use the importer", func() map[string]string {
			files := app("module example.com/app\n\nrequire example.com/lib v1.0.0\n")
			files["go.work"] = "go 1.22\n\nuse ./lib\n\nreplace example.com/lib => ./lib\n"
			return files
		}(), nil},
		{"a workspace's replacement before the importer's", func() map[string]string {
			files := app("module example.com/app\n\nrequire example.com/lib v1.0.0\n\nreplace example.com/lib => ../elsewhere\n")
			files["go.work"] = "go 1.22\n\nuse ./app\n\nreplace example.com/lib => ./lib\n"
			return files
		}(), reaches},
		{"a workspace that uses its own folder", func() map[string]string {
			files := maps.Clone(lib)
			files["go.mod"], files["main.go"] = "module example.com/app\n", "package main\n\nimport \"example.com/lib/x\"\n"
			files["go.work"] = "go 1.22\n\nuse .\nuse ./lib\n"
			return files
		}(), []snapshot.Link{folderLink("main.go", 3, "lib/x", "lib/x/x.go")}},
		{"a replacement written with backslashes", app("module example.com/app\n\nrequire example.com/lib v1.0.0\n\nreplace example.com/lib => ..\\lib\n"), reaches},
		{"a workspace's replacement by another module before the importer's local one", func() map[string]string {
			files := app("module example.com/app\n\nrequire example.com/lib v1.0.0\n\nreplace example.com/lib => ../lib\n")
			files["go.work"] = "go 1.22\n\nuse ./app\n\nreplace example.com/lib => example.com/fork v1.2.0\n"
			return files
		}(), nil},
		{"two modules of one path: the importer's own", func() map[string]string {
			files := app("module example.com/lib\n")
			files["app/main.go"], files["app/x/x.go"] = "package main\n\nimport \"example.com/lib/x\"\n", "package x\n"
			return files
		}(), []snapshot.Link{folderLink("app/main.go", 3, "app/x", "app/x/x.go")}},
		{"two modules of one path: the importer's own, though a copy's folder comes first", map[string]string{
			"a-copy/go.mod": "module example.com/lib\n", "a-copy/x/x.go": "package x\n",
			"b/go.mod": "module example.com/lib\n", "b/x/x.go": "package x\n", "b/main.go": "package main\n\nimport \"example.com/lib/x\"\n",
		}, []snapshot.Link{folderLink("b/main.go", 3, "b/x", "b/x/x.go")}},
		{"two modules of one path: the importer's own, though it reaches a copy listed first", map[string]string{
			"a/go.mod": "module example.com/m\n", "a/x/x.go": "package x\n",
			"z/go.mod": "module example.com/m\n\nrequire example.com/m v1.0.0\n\nreplace example.com/m => ../a\n",
			"z/x/x.go": "package x\n", "z/main.go": "package main\n\nimport \"example.com/m/x\"\n",
		}, []snapshot.Link{folderLink("z/main.go", 3, "z/x", "z/x/x.go")}},
		{"a longer module path the importer does not reach hides nothing", map[string]string{
			"go.mod": "module example.com/root\n", "sub/x/x.go": "package x\n", "main.go": "package main\n\nimport \"example.com/root/sub/x\"\n",
			"testdata/fixture/go.mod": "module example.com/root/sub\n", "testdata/fixture/x/x.go": "package x\n",
		}, []snapshot.Link{folderLink("main.go", 3, "sub/x", "sub/x/x.go")}},
		{"a module path is no prefix of a longer name", func() map[string]string {
			files := app("module example.com/app\n\nrequire example.com/lib v1.0.0\n\nreplace example.com/lib => ../lib\n")
			files["app/main.go"], files["lib/rary/x/x.go"] = "package main\n\nimport \"example.com/library/x\"\n", "package x\n"
			return files
		}(), nil},
		{"an empty require block on one line", app("module example.com/app\n\nrequire ( )\n\nrequire example.com/lib v1.0.0\n\nreplace example.com/lib => ../lib\n"), reaches},
		{"a nested module is no part of the module above", map[string]string{
			"go.mod": "module example.com/root\n", "main.go": "package main\n\nimport \"example.com/root/tools/y\"\n",
			"tools/go.mod": "module example.com/tools\n", "tools/y/y.go": "package y\n",
		}, nil},
	} {
		if got := linksOf(t, tc.files).List; !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: List = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

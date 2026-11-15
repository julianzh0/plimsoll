package snapshot_test

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/juzhxng/plimsoll/internal/conventions"
	"github.com/juzhxng/plimsoll/internal/snapshot"
	"github.com/juzhxng/plimsoll/internal/snapshot/snapshottest"
)

func linksOf(files map[string]string) snapshot.Links {
	return snapshot.Build(snapshottest.Files(nil, files), conventions.Default()).Now.Links
}

func withCRLF(files map[string]string) map[string]string {
	converted := map[string]string{}
	for p, content := range files {
		converted[p] = strings.ReplaceAll(content, "\n", "\r\n")
	}
	return converted
}

func link(from string, line int, to string) snapshot.Link {
	return snapshot.Link{From: from, Line: line, To: to, Files: []string{to}}
}

func TestBuildSortsTypeScriptImportsIntoKinds(t *testing.T) {
	importer := strings.Join([]string{
		`import { b } from "./b.ts";`, `import { c } from "./c.js";`, `import { d } from "./d";`,
		`import { f } from "./feature";`, `import data from "./data.json";`, `import { x } from "./x";`,
		`import { g } from "./gone";`, `import store from "store";`, `import fs from "node:fs";`,
		`import type { T } from "./t.ts";`, `const lazy = await import(name);`,
		"const text = `", `import z from "./nowhere.ts";`, "`;",
	}, "\n") + "\n"
	files := map[string]string{"src/a.ts": importer, "src/data.json": "{}\n"}
	for _, p := range []string{"src/b.ts", "src/c.ts", "src/d.ts", "src/feature/index.ts", "src/x.ts", "src/x/index.ts", "src/t.ts", "src/store.ts"} {
		files[p] = "export const v = 1;\n"
	}
	got := linksOf(files)
	wantLanguages := []snapshot.LanguageImports{{
		Language: "typescript", Resolved: 6, Outside: 2, Ambiguous: 1, Unreadable: 1, NotFound: 2,
		PrefixFile: "tsconfig.json", Prefix: snapshot.PrefixAbsent,
	}}
	typeOnly := link("src/a.ts", 10, "src/t.ts")
	typeOnly.TypeOnly = true
	wantList := []snapshot.Link{
		link("src/a.ts", 1, "src/b.ts"), link("src/a.ts", 2, "src/c.ts"), link("src/a.ts", 3, "src/d.ts"),
		link("src/a.ts", 4, "src/feature/index.ts"), typeOnly,
	}
	if !reflect.DeepEqual(got.Languages, wantLanguages) {
		t.Errorf("Languages = %+v, want %+v", got.Languages, wantLanguages)
	}
	if !reflect.DeepEqual(got.List, wantList) {
		t.Errorf("List = %+v, want %+v", got.List, wantList)
	}
	if crlf := linksOf(withCRLF(files)); !reflect.DeepEqual(crlf, got) {
		t.Errorf("CRLF line ends: Links = %+v, want %+v", crlf, got)
	}
	bom := map[string]string{}
	maps.Copy(bom, files)
	bom["src/a.ts"] = "\uFEFF" + importer
	if withBOM := linksOf(bom); !reflect.DeepEqual(withBOM, got) {
		t.Errorf("a byte order mark: Links = %+v, want %+v", withBOM, got)
	}

	index := linksOf(map[string]string{"src/feature/b.ts": `import { i } from ".";` + "\n", "src/feature/index.ts": "export const i = 1;\n"})
	if want := []snapshot.Link{link("src/feature/b.ts", 1, "src/feature/index.ts")}; !reflect.DeepEqual(index.List, want) {
		t.Errorf(`import "." = %+v, want the folder's index file`, index.List)
	}
	if out := linksOf(map[string]string{"src/a.ts": `import "../../out.ts";` + "\n"}); out.Languages[0].NotFound != 1 || len(out.List) != 0 {
		t.Errorf("a path that leaves the repository = %+v, want not found", out)
	}
	if self := linksOf(map[string]string{"src/self.ts": `import "./self.ts";` + "\n"}); self.Languages[0].Resolved != 1 || len(self.List) != 0 {
		t.Errorf("a file that imports itself = %+v, want resolved and no link", self)
	}
}

func TestBuildResolvesAliasesFromTheRootConfig(t *testing.T) {
	config := func(extra, paths string) string {
		return "{\n  // aliases\n" + extra + "  \"compilerOptions\": {\n    \"baseUrl\": \"./src\",\n    \"paths\": {\n" + paths + "    },\n  },\n}\n"
	}
	aliases := "      \"@app/*\": [\"app/*\"],\n      \"@app/special\": [\"app/special/entry.ts\"],\n      \"*\": [\"shared/*\"],\n" +
		"      \"types\": [\"../node_modules/@types/x/index.d.ts\"],\n"
	tree := func(tsconfig string) map[string]string {
		main := ""
		for _, spec := range []string{"@app/util", "@app/special", "@app/missing", "lodash", "local", "types"} {
			main += `import x from "` + spec + `";` + "\n"
		}
		return map[string]string{
			"tsconfig.json": tsconfig, "src/main.ts": main, "src/app/util.ts": "x\n", "src/app/special/entry.ts": "x\n",
			"src/app/special.ts": "x\n", "src/shared/local.ts": "x\n",
		}
	}
	got := linksOf(tree(config("", aliases)))
	wantList := []snapshot.Link{link("src/main.ts", 1, "src/app/util.ts"), link("src/main.ts", 2, "src/app/special/entry.ts"), link("src/main.ts", 5, "src/shared/local.ts")}
	if l := got.Languages[0]; !reflect.DeepEqual(got.List, wantList) || l.Resolved != 3 || l.NotFound != 1 || l.Outside != 2 || l.Prefix != snapshot.PrefixRead {
		t.Errorf("aliases: Links = %+v, want three links, 3 resolved, 1 not found, 2 outside, the prefix read", got)
	}
	extends := linksOf(tree(config("  \"extends\": \"./base.json\",\n", aliases)))
	if extends.Languages[0].Prefix != snapshot.PrefixExtends || !reflect.DeepEqual(extends.List, wantList) {
		t.Errorf("a config that extends another: %+v, want PrefixExtends and the same links", extends)
	}
	broken := linksOf(tree("{not json"))
	if l := broken.Languages[0]; l.Prefix != snapshot.PrefixUnreadable || l.Outside != 6 {
		t.Errorf("an unreadable config: %+v, want PrefixUnreadable and every alias outside", l)
	}
	if shape := linksOf(tree(config("", "      \"@app/*\": \"app/*\",\n"))); shape.Languages[0].Prefix != snapshot.PrefixUnreadable {
		t.Errorf("paths that are not lists: Prefix = %v, want PrefixUnreadable", shape.Languages[0].Prefix)
	}
	if bom := linksOf(tree("\uFEFF" + config("", aliases))); bom.Languages[0].Prefix != snapshot.PrefixRead {
		t.Errorf("a config with a byte order mark: Prefix = %v, want PrefixRead", bom.Languages[0].Prefix)
	}
}

func TestBuildListsFilesThatOnlyPassImportsOn(t *testing.T) {
	got := linksOf(map[string]string{
		"src/index.ts": "export * from \"./a.ts\";\nexport { b } from \"./b.ts\";\n",
		"src/mixed.ts": "export * from \"./a.ts\";\nexport const own = 1;\n",
		"src/empty.ts": "", "src/a.ts": "export const a = 1;\n", "src/b.ts": "export const b = 1;\n",
	})
	if !reflect.DeepEqual(got.PassOn, []string{"src/index.ts"}) {
		t.Errorf("PassOn = %q, want src/index.ts alone", got.PassOn)
	}
	from := 0
	for _, l := range got.List {
		if l.From == "src/index.ts" {
			from++
		}
	}
	if from != 2 {
		t.Errorf("links from src/index.ts = %d, want 2", from)
	}
}

func goTree() map[string]string {
	return map[string]string{
		"go.mod": "module example.com/m\n\ngo 1.27\n",
		"main.go": "package main\n\nimport (\n\t\"errors\"\n\t\"fmt\"\n\t\"example.com/m/internal/store\"\n\t\"example.com/m/internal/gen\"\n" +
			"\t\"example.com/m/internal/onlytests\"\n\t\"example.com/other/store\"\n)\n\nfunc main() {}\n\nvar t = `\nimport \"example.com/m/internal/late\"\n`\n",
		"errors/errors.go": "package errors\n", "store/store.go": "package store\n", "internal/late/late.go": "package late\n",
		"internal/store/store.go":        "package store\n\nimport \"example.com/m\"\n",
		"internal/store/store_test.go":   "package store_test\n\nimport (\n\t\"testing\"\n\t\"example.com/m/internal/store\"\n)\n",
		"internal/store/helpers_test.go": "package store\n",
		"internal/onlytests/a_test.go":   "package onlytests\n",
		"internal/gen/x.pb.go":           "// Code generated by x. DO NOT EDIT.\npackage gen\n",
	}
}

func TestBuildFollowsGoImportsToFolders(t *testing.T) {
	got := linksOf(goTree())
	wantLanguages := []snapshot.LanguageImports{{
		Language: "go", Resolved: 4, Outside: 4, NotFound: 1, PrefixFile: "go.mod", Prefix: snapshot.PrefixRead,
		PrefixRequired: true, Folders: true, NoRings: "the Go compiler rejects import cycles",
	}}
	folder := func(from string, line int, to string, files ...string) snapshot.Link {
		return snapshot.Link{From: from, Line: line, To: to, Folder: true, Files: files}
	}
	wantList := []snapshot.Link{
		folder("internal/store/store.go", 3, ".", "main.go"),
		folder("internal/store/store_test.go", 5, "internal/store", "internal/store/store.go"),
		folder("main.go", 6, "internal/store", "internal/store/store.go"),
	}
	if !reflect.DeepEqual(got.Languages, wantLanguages) {
		t.Errorf("Languages = %+v, want %+v", got.Languages, wantLanguages)
	}
	if !reflect.DeepEqual(got.List, wantList) {
		t.Errorf("List = %+v, want %+v", got.List, wantList)
	}
	if crlf := linksOf(withCRLF(goTree())); !reflect.DeepEqual(crlf, got) {
		t.Errorf("CRLF line ends: Links = %+v, want %+v", crlf, got)
	}
	without := goTree()
	delete(without, "go.mod")
	if l := linksOf(without).Languages[0]; l.Prefix != snapshot.PrefixAbsent || l.Outside != 9 || l.Resolved != 0 {
		t.Errorf("without go.mod: %+v, want PrefixAbsent and every import outside", l)
	}
	noModule := goTree()
	noModule["go.mod"] = "go 1.27\n"
	if l := linksOf(noModule).Languages[0]; l.Prefix != snapshot.PrefixUnreadable {
		t.Errorf("go.mod without a module line: Prefix = %v, want PrefixUnreadable", l.Prefix)
	}
	quoted := goTree()
	quoted["go.mod"] = "module \"example.com/m\" // note\n"
	if q := linksOf(quoted); q.Languages[0].Prefix != snapshot.PrefixRead || !reflect.DeepEqual(q.List, wantList) {
		t.Errorf("a quoted module line with a comment: %+v, want the same links", q)
	}
}

func TestBuildCountsEachLanguageApart(t *testing.T) {
	files := goTree()
	files["src/a.ts"] = `import { b } from "./b.ts";` + "\n"
	files["src/b.ts"] = "export const b = 1;\n"
	files["src/x.py"] = "x = 1\n"
	got := linksOf(files)
	if len(got.Languages) != 3 || got.Languages[0].Language != "typescript" || got.Languages[1].Language != "go" || got.Languages[2].Language != "python" {
		t.Errorf("Languages = %+v, want typescript, go, python: the entries' order", got.Languages)
	}
	if shell := linksOf(map[string]string{"bin/a.sh": "echo a\n"}); len(shell.Languages) != 0 || len(shell.List) != 0 {
		t.Errorf("a shell script only: Links = %+v, want none", shell)
	}
}

func TestBuildResolvesEachSideAgainstItsOwnFiles(t *testing.T) {
	build := func(base, now map[string]string) snapshot.Snapshot {
		return snapshot.Build(snapshottest.Files(base, now), conventions.Default())
	}
	tree := map[string]string{"src/a.ts": `import x from "./b.ts";` + "\n", "src/b.ts": "export const b = 1;\n"}
	same := build(tree, tree)
	if !reflect.DeepEqual(same.Before.Links, same.Now.Links) || len(same.Before.Links.List) != 1 {
		t.Errorf("an unchanged tree: Before %+v, Now %+v, want the same one link", same.Before.Links, same.Now.Links)
	}
	deleted := build(tree, map[string]string{"src/a.ts": tree["src/a.ts"]})
	if !reflect.DeepEqual(deleted.Before.Links.List, []snapshot.Link{link("src/a.ts", 1, "src/b.ts")}) ||
		len(deleted.Now.Links.List) != 0 || deleted.Now.Links.Languages[0].NotFound != 1 {
		t.Errorf("a target deleted since the base: Before %+v, Now %+v", deleted.Before.Links, deleted.Now.Links)
	}
	both := map[string]string{"src/b.ts": "export const b = 1;\n", "src/c.ts": "export const c = 1;\n"}
	base, now := map[string]string{"src/a.ts": `import x from "./b.ts";` + "\n"}, map[string]string{"src/a.ts": `import x from "./c.ts";` + "\n"}
	for p, content := range both {
		base[p], now[p] = content, content
	}
	moved := build(base, now)
	if !reflect.DeepEqual(moved.Before.Links.List, []snapshot.Link{link("src/a.ts", 1, "src/b.ts")}) ||
		!reflect.DeepEqual(moved.Now.Links.List, []snapshot.Link{link("src/a.ts", 1, "src/c.ts")}) {
		t.Errorf("a changed import: Before %+v, Now %+v", moved.Before.Links.List, moved.Now.Links.List)
	}
	withoutModule := goTree()
	delete(withoutModule, "go.mod")
	module := build(withoutModule, goTree())
	if module.Before.Links.Languages[0].Prefix != snapshot.PrefixAbsent || module.Now.Links.Languages[0].Prefix != snapshot.PrefixRead {
		t.Errorf("go.mod added since the base: Before %v, Now %v", module.Before.Links.Languages[0].Prefix, module.Now.Links.Languages[0].Prefix)
	}
}

func TestBuildKeepsNoImportText(t *testing.T) {
	tree := map[string]string{
		"src/a.ts":   "import x from \"zzmarker-outside\";\nimport y from \"./zzmarker-missing\";\nimport(\"zzmarker\" + n);\nimport z from \"./b.ts\";\n",
		"src/b.ts":   "export const b = 1;\n",
		"src/app.py": "from zzmarker_pkg import zzmarker_name\nimport zzmarker_missing\nfrom . import zzmarker_gone\nimportlib.import_module(\"zzmarker.\" + n)\n",
	}
	s := snapshot.Build(snapshottest.Files(tree, tree), conventions.Default())
	printed := fmt.Sprintf("%+v", s)
	if strings.Contains(printed, "zzmarker") {
		t.Errorf("the snapshot holds import text: %s", printed)
	}
}

func pythonTree() map[string]string {
	main := strings.Join([]string{
		"import os", "import json", "import app.store", "import app.models", "from app.models import User", "from app import (",
		"    store,  # kept", "    models,", ")", "from . import store", "from .models.user import User", "from .. import app",
		"from .... import x", "import app.both", "import app.gone", "import examples.demo.run", "from alembic import op", "",
		"def late():", "    import app.models.user",
	}, "\n") + "\n"
	return map[string]string{
		"src/app/__init__.py": "", "src/app/store.py": "x = 1\n", "src/app/models/user.py": "class User: ...\n",
		"src/app/models/__init__.py": "from .user import User\n", "src/app/both.py": "x = 1\n", "src/app/both/__init__.py": "",
		"src/app/json.py": "x = 1\n", "examples/demo/run.py": "x = 1\n", "alembic/env.py": "x = 1\n",
		"tools/helper.py": "def run(): ...\n", "tools/build.py": "from helper import run\nimport yaml\n", "src/app/main.py": main,
		"src/app/extra.py": strings.Join([]string{
			"from typing import TYPE_CHECKING", "import importlib", "", "if TYPE_CHECKING:", "    from app.models import user",
			"    import app.store", "", "m = importlib.import_module(\"app.store\")", "n = importlib.import_module(name)",
			"o = importlib.import_module(\".store\", __package__)", "p = __import__(\"json\")",
		}, "\n") + "\n",
	}
}

func TestBuildFollowsDartImportsToFiles(t *testing.T) {
	got := snapshot.Build(snapshottest.Load(t, "testdata/dart"), conventions.Default()).Now.Links
	// note.dart's part note.g.dart resolves, but a generated file is no code, so the list leaves that link out.
	want := []snapshot.LanguageImports{{Language: "dart", Resolved: 10, Outside: 5, PrefixFile: "pubspec.yaml", Prefix: snapshot.PrefixRead, Implicit: []string{"main.dart"}}}
	if !reflect.DeepEqual(got.Languages, want) {
		t.Errorf("Languages = %+v, want %+v", got.Languages, want)
	}
	wantList := []snapshot.Link{
		link("integration_test/app_test.dart", 2, "lib/main.dart"),
		link("lib/main.dart", 2, "lib/notes.dart"),
		link("lib/notes.dart", 3, "lib/src/app.dart"), link("lib/notes.dart", 4, "lib/src/store.dart"),
		link("lib/src/app.dart", 2, "lib/src/store.dart"),
		link("lib/src/store.dart", 1, "lib/config.dart"), link("lib/src/store.dart", 2, "lib/src/note.dart"),
		link("test/fakes/fake_store.dart", 1, "lib/src/store.dart"), link("test/store_test.dart", 2, "lib/src/store.dart"),
	}
	if !reflect.DeepEqual(got.List, wantList) {
		t.Errorf("List = %+v, want %+v", got.List, wantList)
	}
	if !reflect.DeepEqual(got.PassOn, []string{"lib/notes.dart"}) {
		t.Errorf("PassOn = %q, want the barrel lib/notes.dart", got.PassOn)
	}
}

func TestBuildFollowsImportsIntoDartWorkspaceMembers(t *testing.T) {
	got := linksOf(map[string]string{
		"pubspec.yaml":                  "name: _ws\nworkspace:\n  - app # the app\n  - 'packages/core'\n  - packages/gone\n",
		"app/pubspec.yaml":              "name: app\nresolution: workspace\n",
		"packages/core/pubspec.yaml":    "name: core\nresolution: workspace\n",
		"app/lib/main.dart":             "import 'package:core/core.dart';\nimport 'package:app/src/x.dart';\nimport 'package:http/http.dart';\n",
		"app/lib/src/x.dart":            "const x = 1;\n",
		"packages/core/lib/core.dart":   "const c = 1;\n",
		"packages/other/pubspec.yaml":   "name: other\n",
		"packages/other/lib/other.dart": "import 'package:core/core.dart';\n",
	})
	want := []snapshot.Link{
		link("app/lib/main.dart", 1, "packages/core/lib/core.dart"), link("app/lib/main.dart", 2, "app/lib/src/x.dart"),
		link("packages/other/lib/other.dart", 1, "packages/core/lib/core.dart"),
	}
	if !reflect.DeepEqual(got.List, want) || got.Languages[0].Outside != 1 || got.Languages[0].NotFound != 0 {
		t.Errorf("links = %+v, languages %+v; want the members' packages as own and http outside", got.List, got.Languages)
	}
}

func TestBuildFollowsPythonImportsToFiles(t *testing.T) {
	got := linksOf(pythonTree())
	want := []snapshot.LanguageImports{{
		Language: "python", Resolved: 14, Outside: 7, Ambiguous: 1, Unreadable: 2, NotFound: 2, Prefix: snapshot.PrefixRead, PrefixRequired: true,
		Roots: []string{".", "src"}, Implicit: []string{"__init__.py", "__main__.py"},
	}}
	if !reflect.DeepEqual(got.Languages, want) {
		t.Errorf("Languages = %+v, want %+v", got.Languages, want)
	}
	main, extra := "src/app/main.py", "src/app/extra.py"
	typeOnly := func(l snapshot.Link) snapshot.Link { l.TypeOnly = true; return l }
	wantList := []snapshot.Link{
		typeOnly(link(extra, 5, "src/app/models/user.py")), typeOnly(link(extra, 6, "src/app/store.py")), link(extra, 8, "src/app/store.py"),
		link(main, 3, "src/app/store.py"), link(main, 4, "src/app/models/__init__.py"), link(main, 5, "src/app/models/__init__.py"),
		link(main, 6, "src/app/models/__init__.py"), link(main, 6, "src/app/store.py"), link(main, 10, "src/app/store.py"),
		link(main, 11, "src/app/models/user.py"), link(main, 12, "src/app/__init__.py"), link(main, 16, "examples/demo/run.py"),
		link(main, 20, "src/app/models/user.py"),
		link("src/app/models/__init__.py", 1, "src/app/models/user.py"),
		link("tools/build.py", 1, "tools/helper.py"),
	}
	if !reflect.DeepEqual(got.List, wantList) {
		t.Errorf("List = %+v, want %+v", got.List, wantList)
	}
	if !reflect.DeepEqual(got.PassOn, []string{"src/app/models/__init__.py"}) {
		t.Errorf("PassOn = %q, want the models package", got.PassOn)
	}
	sample := snapshot.Build(snapshottest.Load(t, "testdata/python-declared"), conventions.Default()).Now.Links
	if l := sample.Languages[0]; l.Resolved != 3 || l.Outside != 0 || l.Prefix != snapshot.PrefixRead || !slices.Equal(l.Roots, []string{".", "src", "lib"}) ||
		!reflect.DeepEqual(sample.List, []snapshot.Link{link("checks/test_flow.py", 1, "checks/helpers.py"), link("src/app/service.py", 1, "lib/core/model.py"), link("tests/test_service.py", 1, "src/app/service.py")}) {
		t.Errorf("a declared root: Links = %+v", sample)
	}
	undeclared := snapshottest.Load(t, "testdata/python-declared")
	for i, entry := range undeclared.Now {
		if entry.Path == "pyproject.toml" {
			undeclared.Now[i].Content = []byte("[project]\nname = \"sample\"\ndependencies = []\n")
		}
	}
	if l := snapshot.Build(undeclared, conventions.Default()).Now.Links; l.Languages[0].Resolved != 2 || l.Languages[0].Outside != 1 || !slices.Equal(l.Languages[0].Roots, []string{".", "src"}) ||
		slices.ContainsFunc(l.List, func(k snapshot.Link) bool { return k.From == "src/app/service.py" }) {
		t.Errorf("no declared root: Links = %+v", l)
	}
	if l := linksOf(map[string]string{"pyproject.toml": "[tool.pytest.ini_options]\npythonpath = [\"lib\"]\n", "lib/app/__init__.py": "", "lib/app/b.py": "x = 1\n", "src/main.py": "import app.b\n"}); !reflect.DeepEqual(l.List, []snapshot.Link{link("src/main.py", 1, "lib/app/b.py")}) {
		t.Errorf("a declared root alone: List = %+v", l.List)
	}
	if crlf := linksOf(withCRLF(pythonTree())); !reflect.DeepEqual(crlf, got) {
		t.Errorf("CRLF line ends: Links = %+v, want %+v", crlf, got)
	}
	bom := pythonTree()
	bom[main] = "\uFEFF" + bom[main]
	if withBOM := linksOf(bom); !reflect.DeepEqual(withBOM, got) {
		t.Errorf("a byte order mark: Links = %+v, want %+v", withBOM, got)
	}
	lib := linksOf(map[string]string{"lib/app/a.py": "import app.b\n", "lib/app/b.py": "x = 1\n"})
	if l := lib.Languages[0]; l.Prefix != snapshot.PrefixAbsent || l.Outside != 1 || len(lib.List) != 0 {
		t.Errorf("packages outside the roots: %+v, want PrefixAbsent and the import outside", lib)
	}
	root := linksOf(map[string]string{"main.py": "import helper\nfrom . import helper\nfrom .. import x\nimport main\n", "helper.py": "x = 1\n"})
	if l := root.Languages[0]; l.Resolved != 3 || l.NotFound != 1 ||
		!reflect.DeepEqual(root.List, []snapshot.Link{link("main.py", 1, "helper.py"), link("main.py", 2, "helper.py")}) {
		t.Errorf("modules at the root: %+v", root)
	}
	if semicolon := linksOf(map[string]string{"src/a.py": "from app import b; x = 1\n", "src/app/__init__.py": "", "src/app/b.py": "x = 1\n"}); !reflect.DeepEqual(semicolon.List, []snapshot.Link{link("src/a.py", 1, "src/app/b.py")}) {
		t.Errorf("a name list ended by a semicolon: %+v, want the link to b", semicolon.List)
	}
	if stub := linksOf(map[string]string{"src/a.py": "import stub\n", "src/stub.pyi": "x: int\n"}); stub.Languages[0].Outside != 1 || len(stub.List) != 0 {
		t.Errorf("an import of a stub: %+v, want outside: a stub is no target", stub)
	}
	mixed := pythonTree()
	mixed["src/app/mixed.py"] = "from app import store, both\nfrom examples.demo import run, nope\n"
	m := linksOf(mixed)
	from := 0
	for _, l := range m.List {
		if l.From == "src/app/mixed.py" {
			from++
		}
	}
	if l := m.Languages[0]; l.Ambiguous != 2 || l.NotFound != 3 || from != 0 {
		t.Errorf("statements with several names: %+v, %d links from mixed.py; want each counted once and no link", l, from)
	}
}

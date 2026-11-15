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

// folderLink is a link to a folder and the code files it reaches there.
func folderLink(from string, line int, to string, files ...string) snapshot.Link {
	return snapshot.Link{From: from, Line: line, To: to, Folder: true, Files: files}
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
	wantList := []snapshot.Link{
		folderLink("internal/store/store.go", 3, ".", "main.go"),
		folderLink("internal/store/store_test.go", 5, "internal/store", "internal/store/store.go"),
		folderLink("main.go", 6, "internal/store", "internal/store/store.go"),
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
		// Outside, not found and resolved: the reader builds a folder path from the second and keeps none of them.
		"src/main/java/com/acme/A.java": "package com.acme;\nimport zzmarker.Outside;\nimport com.acme.zzmarker.Gone;\nimport com.acme.zzmarker;\n",
		// Not found through the project file's name, outside, and not readable: the reader keeps no name.
		"src/Acme/Acme.csproj": "<Project Sdk=\"Microsoft.NET.Sdk\" />\n",
		"src/Acme/A.cs":        "global using Acme.Core;\nusing Acme.zzmarker;\nusing zzmarker.Outside;\n\nnamespace Acme.Core;\n",
		"src/lib.rs":           "mod zzmarker_gone;\nuse zzmarker::Outside;\nuse crate::zzmarker_item::{A, zzmarker_b};\nmod zzmarker_inline {\n    use super::zzmarker_x;\n}\n",
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

// jvmTree holds a package in three folders (two source sets of one module and a test source set of another), a
// package under a test source set only, and imports that are outside or lead to no tracked package.
func jvmTree() map[string]string {
	return map[string]string{
		"app/src/main/java/com/acme/a/A.java":        "package com.acme.a;\n\nimport com.acme.b.B;\nimport com.acme.fakes.Fake;\nimport com.acme.a.gen.proto.Message;\nimport com.other.Lib;\n",
		"app/src/main/kotlin/com/acme/b/B.kt":        "package com.acme.b\n",
		"app/src/demo/kotlin/com/acme/b/Demo.kt":     "package com.acme.b\n",
		"app/src/test/java/com/acme/fakes/Fake.java": "package com.acme.fakes;\n",
		"app/src/test/java/com/acme/a/ATest.java":    "package com.acme.a;\n\nimport com.acme.b.*;\nimport com.acme.fakes.Fake;\n",
		"lib/src/test/kotlin/com/acme/b/BTest.kt":    "package com.acme.b\n\nimport `com`.acme.b.B\n",
	}
}

func TestBuildFollowsJavaAndKotlinImportsToPackages(t *testing.T) {
	entry := func(name string, resolved, outside, notFound int, implicit ...string) snapshot.LanguageImports {
		return snapshot.LanguageImports{
			Language: name, Resolved: resolved, Outside: outside, NotFound: notFound, Prefix: snapshot.PrefixRead, PrefixRequired: true,
			Folders: true, SourceSets: true, NoRings: "an import names a package, not a file",
			Roots: []string{"src/*/java", "src/*/kotlin"}, Implicit: implicit,
		}
	}
	java := func(resolved, outside, notFound int) snapshot.LanguageImports {
		return entry("java", resolved, outside, notFound, "package-info.java", "module-info.java")
	}
	notes, notesTest := "src/main/java/com/acme/notes", "src/test/java/com/acme/notes"
	store := []string{notes + "/store/NoteStore.java", notes + "/store/package-info.java"}
	tasks, legacy, tasksTest := "src/main/kotlin/com/acme/tasks", "src/main/java/com/acme/tasks/legacy", "src/test/kotlin/com/acme/tasks/data"
	device := "src/androidTest/kotlin/com/acme/tasks/MainScreenTest.kt"
	data := []string{tasks + "/data/Loaders.kt", tasks + "/data/TaskRepository.kt"}
	a, aTest, bTest := "app/src/main/java/com/acme/a/A.java", "app/src/test/java/com/acme/a/ATest.java", "lib/src/test/kotlin/com/acme/b/BTest.kt"
	demo, mainB := "app/src/demo/kotlin/com/acme/b", "app/src/main/kotlin/com/acme/b"
	cases := []struct {
		name      string
		got       snapshot.Links
		languages []snapshot.LanguageImports
		list      []snapshot.Link
	}{
		// App.java reaches the store folder under src/main only; AppTest.java also the one under src/test. The
		// import of the generated NoteProto resolves and gives no link.
		{"the Maven sample", snapshot.Build(snapshottest.Load(t, "testdata/java"), conventions.Default()).Now.Links,
			[]snapshot.LanguageImports{java(7, 5, 0)},
			[]snapshot.Link{
				folderLink(notes+"/App.java", 3, notes+"/model", notes+"/model/Note.java"),
				folderLink(notes+"/App.java", 6, notes+"/store", store...),
				folderLink(notes+"/store/NoteStore.java", 3, notes+"/model", notes+"/model/Note.java"),
				folderLink(notes+"/store/NoteStore.java", 4, notes+"/model", notes+"/model/Note.java"),
				folderLink(notesTest+"/AppTest.java", 5, notes+"/store", store...),
				folderLink(notesTest+"/AppTest.java", 5, notesTest+"/store", notesTest+"/store/FakeClock.java", notesTest+"/store/NoteStoreTest.java"),
				folderLink(notesTest+"/store/NoteStoreTest.java", 3, notes+"/model", notes+"/model/Note.java"),
			}},
		// Kotlin reaches the Java class and Java the Kotlin one; a top-level function is a member of its package.
		{"the Gradle sample", snapshot.Build(snapshottest.Load(t, "testdata/kotlin"), conventions.Default()).Now.Links,
			[]snapshot.LanguageImports{java(1, 0, 0), entry("kotlin", 10, 3, 0)},
			[]snapshot.Link{
				folderLink(device, 3, tasks+"/data", data...),
				folderLink(device, 3, tasksTest, tasksTest+"/TaskRepositoryTest.kt", tasksTest+"/TestTaskDao.kt"),
				folderLink(legacy+"/Clock.java", 3, tasks+"/model", tasks+"/model/Task.kt"),
				folderLink(tasks+"/Main.kt", 3, tasks+"/data", data...),
				folderLink(tasks+"/Main.kt", 4, tasks+"/data", data...),
				folderLink(tasks+"/Main.kt", 5, legacy, legacy+"/Clock.java"),
				folderLink(tasks+"/data/Loaders.kt", 3, tasks+"/model", tasks+"/model/Task.kt"),
				folderLink(tasks+"/data/Loaders.kt", 4, tasks+"/model", tasks+"/model/Task.kt"),
				folderLink(tasks+"/data/TaskRepository.kt", 3, legacy, legacy+"/Clock.java"),
				folderLink(tasks+"/data/TaskRepository.kt", 4, tasks+"/model", tasks+"/model/Task.kt"),
				folderLink(tasksTest+"/TaskRepositoryTest.kt", 3, legacy, legacy+"/Clock.java"),
				folderLink(tasksTest+"/TestTaskDao.kt", 3, tasks+"/model", tasks+"/model/Task.kt"),
			}},
		// A.java: fakes lies under a test source set only (outside), gen.proto is no tracked package (not found),
		// com.other shares no package (outside). BTest.kt imports its own package and gets no link to itself.
		{"a package in several folders", linksOf(jvmTree()),
			[]snapshot.LanguageImports{java(3, 2, 1), entry("kotlin", 1, 0, 0)},
			[]snapshot.Link{
				folderLink(a, 3, demo, demo+"/Demo.kt"), folderLink(a, 3, mainB, mainB+"/B.kt"),
				folderLink(aTest, 3, demo, demo+"/Demo.kt"), folderLink(aTest, 3, mainB, mainB+"/B.kt"),
				folderLink(aTest, 3, "lib/src/test/kotlin/com/acme/b", bTest),
				folderLink(aTest, 4, "app/src/test/java/com/acme/fakes", "app/src/test/java/com/acme/fakes/Fake.java"),
				folderLink(bTest, 3, demo, demo+"/Demo.kt"), folderLink(bTest, 3, mainB, mainB+"/B.kt"),
			}},
	}
	for _, tc := range cases {
		if !reflect.DeepEqual(tc.got.Languages, tc.languages) {
			t.Errorf("%s: Languages = %+v, want %+v", tc.name, tc.got.Languages, tc.languages)
		}
		if !reflect.DeepEqual(tc.got.List, tc.list) || len(tc.got.PassOn) != 0 {
			t.Errorf("%s: List = %+v, PassOn %q; want %+v and none", tc.name, tc.got.List, tc.got.PassOn, tc.list)
		}
	}
	want := linksOf(jvmTree())
	if crlf := linksOf(withCRLF(jvmTree())); !reflect.DeepEqual(crlf, want) {
		t.Errorf("CRLF line ends: Links = %+v, want %+v", crlf, want)
	}
	bom := jvmTree()
	bom[a] = "\uFEFF" + bom[a]
	if withBOM := linksOf(bom); !reflect.DeepEqual(withBOM, want) {
		t.Errorf("a byte order mark: Links = %+v, want %+v", withBOM, want)
	}
	noRoot := linksOf(map[string]string{"app/A.java": "import com.acme.b.B;\n", "app/com/acme/b/B.java": "package com.acme.b;\n"})
	if l := noRoot.Languages[0]; l.Prefix != snapshot.PrefixAbsent || l.Outside != 1 || len(noRoot.List) != 0 {
		t.Errorf("Java files without a source root: %+v, want PrefixAbsent and the import outside", noRoot)
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

// rustTree holds a library with an inline module, a module in both file forms, a module without a file, a second
// package whose library has another name, statements inside a string and a nested comment, and an integration test
// with a module of its own.
func rustTree() map[string]string {
	return map[string]string{
		"Cargo.toml":       "[package]\nname = \"app\"\n",
		"core/Cargo.toml":  "[package]\nname = \"app-core\"\n\n[lib]\nname = \"corelib\"\n",
		"core/src/lib.rs":  "pub mod util;\n",
		"core/src/util.rs": "pub fn f() {}\n",
		"src/lib.rs": strings.Join([]string{
			"mod a;", "mod both;", "mod gone;", "mod inline {", "    mod deep;", "    use super::a::A;", "    use self::deep::D;", "}",
			"use a::A;", "use crate::a::sub::S;", "use corelib::util::f;", "use app_core::util;", "use std::io;",
			"use {std::fmt, crate::a::{self, sub::{S, T}}};", "extern crate corelib;",
			"const TEXT: &str = \"", "use crate::gone_in_string;", "\";",
			"/* a comment", "use crate::gone_in_comment;", "/* nested */", "use crate::gone_after_nested;", "*/",
		}, "\n") + "\n",
		"src/a.rs": strings.Join([]string{
			"pub mod sub;", "use super::Root;", "use super::super::X;", "use self::sub::S;", "use crate::Root;",
			"#[cfg(test)]", "mod tests {", "    use super::*;", "    use super::super::Root;", "}",
		}, "\n") + "\n",
		"src/a/sub.rs": "pub struct S;\n", "src/both.rs": "", "src/both/mod.rs": "", "src/inline/deep.rs": "pub struct D;\n",
		"tests/it.rs":         "mod common;\nuse app::a::A;\nuse crate::common::helper;\n",
		"tests/common/mod.rs": "use super::thing;\nuse crate::common;\n",
	}
}

func TestBuildFollowsRustImportsToFiles(t *testing.T) {
	entry := func(resolved, outside, ambiguous, notFound int, state snapshot.PrefixState) []snapshot.LanguageImports {
		return []snapshot.LanguageImports{{
			Language: "rust", Resolved: resolved, Outside: outside, Ambiguous: ambiguous, NotFound: notFound,
			PrefixFile: "Cargo.toml", Prefix: state,
			NoRings:  "a crate's modules use each other, and Rust compiles them as one unit",
			Implicit: []string{"main.rs", "lib.rs", "build.rs"},
			NotSeen:  "a path written in code without a `use` line, and a module named by a `path` attribute",
		}}
	}
	check := func(name string, got snapshot.Links, languages []snapshot.LanguageImports, list []snapshot.Link, passOn []string) {
		t.Helper()
		if !reflect.DeepEqual(got.Languages, languages) {
			t.Errorf("%s: Languages = %+v, want %+v", name, got.Languages, languages)
		}
		if !reflect.DeepEqual(got.List, list) || !slices.Equal(got.PassOn, passOn) {
			t.Errorf("%s: List = %+v, PassOn %q; want %+v and %q", name, got.List, got.PassOn, list, passOn)
		}
	}

	// The sample: cli.rs is declared by main.rs, so its crate is main.rs; the uses inside `mod tests` of super
	// name their own file; the `use` line in the comment of lib.rs is no statement.
	lib, bin, memory, store, model, member := "src/lib.rs", "src/main.rs", "src/store/memory.rs", "src/store/mod.rs", "src/model.rs", "crates/notes-store/src/"
	check("the sample", snapshot.Build(snapshottest.Load(t, "testdata/rust"), conventions.Default()).Now.Links,
		entry(27, 3, 0, 0, snapshot.PrefixRead),
		[]snapshot.Link{
			link(member+"lib.rs", 1, member+"disk.rs"), link(member+"lib.rs", 3, member+"disk.rs"),
			link("src/cli.rs", 1, bin),
			link(lib, 5, "src/config.rs"), link(lib, 6, model), link(lib, 7, store), link(lib, 9, model), link(lib, 10, memory),
			link(bin, 1, "src/cli.rs"), link(bin, 3, "src/config.rs"), link(bin, 3, lib), link(bin, 3, store), link(bin, 4, member+"lib.rs"),
			link(memory, 1, store), link(memory, 2, model), link(memory, 2, store), link(memory, 19, model),
			link(store, 1, memory), link(store, 3, lib),
			link("tests/common/mod.rs", 1, lib),
			link("tests/notes.rs", 1, "tests/common/mod.rs"), link("tests/notes.rs", 3, memory), link("tests/notes.rs", 3, store),
		}, []string{member + "lib.rs"})

	// The tree: design 5.5, row by row. Line 14 of lib.rs names a.rs once and a/sub.rs twice: one link each.
	a, sub, deep, common := "src/a.rs", "src/a/sub.rs", "src/inline/deep.rs", "tests/common/mod.rs"
	wantTree := []snapshot.Link{
		link("core/src/lib.rs", 1, "core/src/util.rs"),
		link(a, 1, sub), link(a, 2, lib), link(a, 4, sub), link(a, 5, lib), link(a, 9, lib),
		link(lib, 1, a), link(lib, 5, deep), link(lib, 6, a), link(lib, 7, deep), link(lib, 9, a), link(lib, 10, sub),
		link(lib, 11, "core/src/util.rs"), link(lib, 14, a), link(lib, 14, sub), link(lib, 15, "core/src/lib.rs"),
		link(common, 1, "tests/it.rs"), link("tests/it.rs", 1, common), link("tests/it.rs", 2, a), link("tests/it.rs", 3, common),
	}
	got := linksOf(rustTree())
	check("the tree", got, entry(23, 3, 1, 2, snapshot.PrefixRead), wantTree, []string{"core/src/lib.rs"})
	if crlf := linksOf(withCRLF(rustTree())); !reflect.DeepEqual(crlf, got) {
		t.Errorf("CRLF line ends: Links = %+v, want %+v", crlf, got)
	}
	bom := rustTree()
	bom[lib], bom["Cargo.toml"] = "\uFEFF"+bom[lib], "\uFEFF"+bom["Cargo.toml"]
	if withBOM := linksOf(bom); !reflect.DeepEqual(withBOM, got) {
		t.Errorf("a byte order mark: Links = %+v, want %+v", withBOM, got)
	}

	// Without a manifest the crates' names are outside; every other link stays.
	bare := rustTree()
	delete(bare, "Cargo.toml")
	delete(bare, "core/Cargo.toml")
	byName := func(l snapshot.Link) bool {
		return l.From == lib && (l.Line == 11 || l.Line == 15) || l.From == "tests/it.rs" && l.Line == 2
	}
	check("no Cargo.toml", linksOf(bare), entry(20, 6, 1, 2, snapshot.PrefixAbsent), slices.DeleteFunc(slices.Clone(wantTree), byName), []string{"core/src/lib.rs"})

	// A package without a library: its name leads nowhere of its own.
	check("a binary alone", linksOf(map[string]string{
		"Cargo.toml": "[package]\nname = \"tool\"\n", "src/main.rs": "use tool::x;\nmod cli;\n", "src/cli.rs": "use crate::run;\n",
	}), entry(2, 1, 0, 0, snapshot.PrefixRead), []snapshot.Link{link("src/cli.rs", 1, "src/main.rs"), link("src/main.rs", 2, "src/cli.rs")}, nil)

	// A module that two crate roots declare: its crate paths agree, its parent does not. A file of one mod line
	// only passes imports on.
	check("a module of two tests", linksOf(map[string]string{
		"tests/a.rs": "mod common;\n", "tests/b.rs": "mod common;\n", "tests/common/mod.rs": "use crate::common::x;\nuse super::y;\n",
	}), entry(3, 0, 1, 0, snapshot.PrefixAbsent), []snapshot.Link{link("tests/a.rs", 1, common), link("tests/b.rs", 1, common)},
		[]string{"tests/a.rs", "tests/b.rs"})

	// A package table without a name names no crate; the links by crate, self and super need none.
	check("a package without a name", linksOf(map[string]string{
		"Cargo.toml": "[package]\nversion = \"1\"\n", "src/lib.rs": "mod a;\n", "src/a.rs": "use crate::X;\n",
	}), entry(2, 0, 0, 0, snapshot.PrefixAbsent), []snapshot.Link{link("src/a.rs", 1, "src/lib.rs"), link("src/lib.rs", 1, "src/a.rs")},
		[]string{"src/lib.rs"})

	// A brace that never closes: the inline module runs to the end of the file.
	check("a module that does not close", linksOf(map[string]string{"src/lib.rs": "mod a {\n    use super::b;\nmod c;\n"}),
		entry(1, 0, 0, 1, snapshot.PrefixAbsent), nil, nil)
}

// csharpTree holds a namespace in two folders, a folder with two namespaces, a nested declaration, an own global
// using, and usings that are outside or lead to no tracked namespace.
func csharpTree() map[string]string {
	return map[string]string{
		"src/Acme/Acme.csproj": "<Project Sdk=\"Microsoft.NET.Sdk\" />\n",
		"src/Acme/Billing/Invoice.cs": "using Acme.Core;\nusing Acme.Grpc;\nusing Acme;\nusing System.Text;\nusing static Acme.Core.Guard;\n" +
			"using Money = Acme.Core.Money;\nusing Path = System.IO.Path;\nusing Inner;\n\nnamespace Acme.Billing;\n",
		"src/Acme/Core/Guard.cs":           "namespace Acme.Core;\n",
		"src/Acme/Core/Money.cs":           "namespace Acme.Core\n{\n    namespace Inner\n    {\n    }\n}\n\nnamespace System.Text\n{\n}\n",
		"src/Acme.Extras/Core/More.cs":     "namespace Acme.Core;\n",
		"src/Acme/GlobalUsings.cs":         "global using Acme.Core;\nglobal using System.Linq;\n",
		"tests/Acme.Tests/InvoiceTests.cs": "using System;\nusing Acme.Billing;\nusing Acme.Core;\n\nnamespace Acme.Tests;\n",
	}
}

func TestBuildFollowsCSharpUsingsToNamespaces(t *testing.T) {
	var notSeen string
	for _, l := range conventions.Default().Languages {
		if l.Name == "csharp" {
			notSeen = l.Imports.NotSeen
		}
	}
	csharp := func(resolved, outside, unreadable, notFound int) snapshot.LanguageImports {
		return snapshot.LanguageImports{
			Language: "csharp", Resolved: resolved, Outside: outside, Unreadable: unreadable, NotFound: notFound, Prefix: snapshot.PrefixRead,
			PrefixRequired: true, Folders: true, NoRings: "a using names a namespace, not a file", Implicit: []string{"Program.cs"}, NotSeen: notSeen,
		}
	}
	notes, tests := "Acme.Notes", "Acme.Notes.Tests"
	model, store := []string{notes + "/Model/Note.cs"}, []string{notes + "/Store/Clock.cs", notes + "/Store/NoteStore.cs"}
	invoice, invoiceTests := "src/Acme/Billing/Invoice.cs", "tests/Acme.Tests/InvoiceTests.cs"
	core, extras := "src/Acme/Core", "src/Acme.Extras/Core"
	coreFiles := []string{core + "/Guard.cs", core + "/Money.cs"}
	cases := []struct {
		name      string
		got       snapshot.Links
		languages []snapshot.LanguageImports
		list      []snapshot.Link
	}{
		// Properties is declared by a designer file and Mapping by a file with the header: both resolve and give no
		// link. System and System.Collections.Generic stay outside beside the declared System.Runtime.CompilerServices.
		{"the sample", snapshot.Build(snapshottest.Load(t, "testdata/csharp"), conventions.Default()).Now.Links,
			[]snapshot.LanguageImports{csharp(11, 5, 0, 0)},
			[]snapshot.Link{
				folderLink(tests+"/NoteStoreTests.cs", 1, notes+"/Model", model...),
				folderLink(tests+"/NoteStoreTests.cs", 2, notes+"/Store", store...),
				folderLink(tests+"/NoteStoreTests.cs", 3, tests+"/Support", tests+"/Support/FixedClock.cs"),
				folderLink(tests+"/Support/FixedClock.cs", 2, notes+"/Store", store...),
				folderLink(notes+"/Program.cs", 1, notes+"/Model", model...),
				folderLink(notes+"/Program.cs", 3, notes+"/Store", store...),
				folderLink(notes+"/Program.cs", 4, notes+"/Store", store...),
				folderLink(notes+"/Store/NoteStore.cs", 3, notes+"/Model", model...),
				folderLink(notes+"/Store/NoteStore.cs", 4, notes+"/Model", model...),
			}},
		// Invoice.cs: Acme.Grpc is the project's own by its name and declared nowhere (not found); Acme and System
		// only hold namespaces, System.IO and the nested Inner are declared nowhere (outside); System.Text is
		// declared beside Acme.Core. The global using of Acme.Core is not readable and gives no link.
		{"a namespace in two folders", linksOf(csharpTree()),
			[]snapshot.LanguageImports{csharp(6, 5, 1, 1)},
			[]snapshot.Link{
				folderLink(invoice, 1, extras, extras+"/More.cs"), folderLink(invoice, 1, core, coreFiles...),
				folderLink(invoice, 4, core, coreFiles...),
				folderLink(invoice, 5, extras, extras+"/More.cs"), folderLink(invoice, 5, core, coreFiles...),
				folderLink(invoice, 6, extras, extras+"/More.cs"), folderLink(invoice, 6, core, coreFiles...),
				folderLink(invoiceTests, 2, "src/Acme/Billing", invoice),
				folderLink(invoiceTests, 3, extras, extras+"/More.cs"), folderLink(invoiceTests, 3, core, coreFiles...),
			}},
	}
	for _, tc := range cases {
		if !reflect.DeepEqual(tc.got.Languages, tc.languages) {
			t.Errorf("%s: Languages = %+v, want %+v", tc.name, tc.got.Languages, tc.languages)
		}
		if !reflect.DeepEqual(tc.got.List, tc.list) || len(tc.got.PassOn) != 0 {
			t.Errorf("%s: List = %+v, PassOn %q; want %+v and none", tc.name, tc.got.List, tc.got.PassOn, tc.list)
		}
	}
	want := linksOf(csharpTree())
	if crlf := linksOf(withCRLF(csharpTree())); !reflect.DeepEqual(crlf, want) {
		t.Errorf("CRLF line ends: Links = %+v, want %+v", crlf, want)
	}
	bom := csharpTree()
	bom[invoice] = "\uFEFF" + bom[invoice]
	if withBOM := linksOf(bom); !reflect.DeepEqual(withBOM, want) {
		t.Errorf("a byte order mark: Links = %+v, want %+v", withBOM, want)
	}
	noProject := csharpTree()
	delete(noProject, "src/Acme/Acme.csproj")
	if got := linksOf(noProject); !reflect.DeepEqual(got.Languages, []snapshot.LanguageImports{csharp(6, 6, 1, 0)}) || !reflect.DeepEqual(got.List, want.List) {
		t.Errorf("without a project file: %+v, want the import of Acme.Grpc outside and the same links", got)
	}
	bare := linksOf(map[string]string{"src/A.cs": "using Acme.B;\n", "src/B.cs": "class B {}\n"})
	if l := bare.Languages[0]; l.Prefix != snapshot.PrefixAbsent || l.Outside != 1 || len(bare.List) != 0 {
		t.Errorf("C# files that declare no namespace: %+v, want PrefixAbsent and the import outside", bare)
	}
}

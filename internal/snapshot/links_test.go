package snapshot_test

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

func linksOf(t testing.TB, files map[string]string) snapshot.Links {
	t.Helper()
	return snapshot.Build(snapshottest.Files(files, files), conventions.Default(), snapshottest.Readings(t)).Now.Links
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

// added is the link as a change that wrote its import gives it.
func added(l snapshot.Link) snapshot.Link {
	l.Added = true
	return l
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
	got := linksOf(t, files)
	wantLanguages := []snapshot.LanguageImports{{
		Language: "typescript", Resolved: 6, Outside: 2, Ambiguous: 1, Unreadable: 1, NotFound: 1,
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
	if crlf := linksOf(t, withCRLF(files)); !reflect.DeepEqual(crlf, got) {
		t.Errorf("CRLF line ends: Links = %+v, want %+v", crlf, got)
	}
	bom := map[string]string{}
	maps.Copy(bom, files)
	bom["src/a.ts"] = "\uFEFF" + importer
	if withBOM := linksOf(t, bom); !reflect.DeepEqual(withBOM, got) {
		t.Errorf("a byte order mark: Links = %+v, want %+v", withBOM, got)
	}

	index := linksOf(t, map[string]string{"src/feature/b.ts": `import { i } from ".";` + "\n", "src/feature/index.ts": "export const i = 1;\n"})
	if want := []snapshot.Link{link("src/feature/b.ts", 1, "src/feature/index.ts")}; !reflect.DeepEqual(index.List, want) {
		t.Errorf(`import "." = %+v, want the folder's index file`, index.List)
	}
	if out := linksOf(t, map[string]string{"src/a.ts": `import "../../out.ts";` + "\n"}); out.Languages[0].NotFound != 1 || len(out.List) != 0 {
		t.Errorf("a path that leaves the repository = %+v, want not found", out)
	}
	if self := linksOf(t, map[string]string{"src/self.ts": `import "./self.ts";` + "\n"}); self.Languages[0].Resolved != 1 || len(self.List) != 0 {
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
	got := linksOf(t, tree(config("", aliases)))
	wantList := []snapshot.Link{link("src/main.ts", 1, "src/app/util.ts"), link("src/main.ts", 2, "src/app/special/entry.ts"), link("src/main.ts", 5, "src/shared/local.ts")}
	if l := got.Languages[0]; !reflect.DeepEqual(got.List, wantList) || l.Resolved != 3 || l.NotFound != 1 || l.Outside != 2 || l.Prefix != snapshot.PrefixRead {
		t.Errorf("aliases: Links = %+v, want three links, 3 resolved, 1 not found, 2 outside, the prefix read", got)
	}
	extends := linksOf(t, tree(config("  \"extends\": \"./base.json\",\n", aliases)))
	if extends.Languages[0].Prefix != snapshot.PrefixExtends || !reflect.DeepEqual(extends.List, wantList) {
		t.Errorf("a config that extends another: %+v, want PrefixExtends and the same links", extends)
	}
	broken := linksOf(t, tree("{not json"))
	if l := broken.Languages[0]; l.Prefix != snapshot.PrefixUnreadable || l.Outside != 6 {
		t.Errorf("an unreadable config: %+v, want PrefixUnreadable and every alias outside", l)
	}
	if shape := linksOf(t, tree(config("", "      \"@app/*\": \"app/*\",\n"))); shape.Languages[0].Prefix != snapshot.PrefixUnreadable {
		t.Errorf("paths that are not lists: Prefix = %v, want PrefixUnreadable", shape.Languages[0].Prefix)
	}
	if bom := linksOf(t, tree("\uFEFF"+config("", aliases))); bom.Languages[0].Prefix != snapshot.PrefixRead {
		t.Errorf("a config with a byte order mark: Prefix = %v, want PrefixRead", bom.Languages[0].Prefix)
	}
}

func TestBuildListsFilesThatOnlyPassImportsOn(t *testing.T) {
	got := linksOf(t, map[string]string{
		"src/index.ts":  "export * from \"./a.ts\";\nexport { b } from \"./b.ts\";\n",
		"src/barrel.ts": "// barrel\n/* all\n of it */\nexport type { T } from './t'\n",
		"src/mixed.ts":  "export * from \"./a.ts\";\nexport const own = 1;\n",
		"src/reuse.ts":  "import { a } from './a'\nexport { a }\n",
		"src/plain.ts":  "export * from './a'\nexport const x = 1\n",
		"src/empty.ts":  "", "src/a.ts": "export const a = 1;\n", "src/b.ts": "export const b = 1;\n",
		"src/t.ts": "export type T = number;\n",
	})
	if !reflect.DeepEqual(got.PassOn, []string{"src/barrel.ts", "src/index.ts"}) {
		t.Errorf("PassOn = %q, want src/barrel.ts and src/index.ts", got.PassOn)
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

func TestBuildCountsTypeScriptImportsBuiltAtRunTime(t *testing.T) {
	cases := map[string]int{
		"await import(name)": 1, "import(`./x/${a}.ts`)": 1, "require(\"./a\" + b)": 1,
		"import(\"./a.ts\")": 0, "require('x', y)": 0, "obj.require(x)": 0,
	}
	for text, want := range cases {
		got := linksOf(t, map[string]string{"src/m.ts": text + "\n", "src/a.ts": "export const a = 1;\n"})
		if len(got.Languages) != 1 || got.Languages[0].Unreadable != want {
			t.Errorf("imports built at run time in %q = %+v, want %d", text, got.Languages, want)
		}
	}
}

func TestBuildLinksTheSameContentUnderTwoSuffixes(t *testing.T) {
	got := linksOf(t, map[string]string{"a.ts": "import \"./b\";\n", "a.js": "import \"./b\";\n", "b.ts": "export const b = 1;\n"})
	for _, from := range []string{"a.js", "a.ts"} {
		if !slices.ContainsFunc(got.List, func(l snapshot.Link) bool { return l.From == from && l.To == "b.ts" }) {
			t.Errorf("links = %+v, want %s -> b.ts", got.List, from)
		}
	}
}

func TestBuildMarksImportsUnderTypeChecking(t *testing.T) {
	cases := map[string]map[int]bool{ // the line of each import of a.py, b.py or c.py, and whether it is type-only
		"if TYPE_CHECKING:\n    from a import b\n\n    import c\nx = 1\n":       {2: true, 4: true},
		"if typing.TYPE_CHECKING:  # note\n    import a\nelse:\n    import b\n": {2: true, 4: false},
		"if TYPE_CHECKING:\r\n    import a":                                     {2: true},
		"if not TYPE_CHECKING:\n    import a\n":                                 {2: false},
		"def f():\n    if TYPE_CHECKING:\n        import a\n":                   {3: false},
	}
	for text, want := range cases {
		got := linksOf(t, map[string]string{"m.py": text, "a.py": "", "b.py": "", "c.py": ""})
		lines := map[int]bool{}
		for _, l := range got.List {
			lines[l.Line] = l.TypeOnly
		}
		if !reflect.DeepEqual(lines, want) {
			t.Errorf("type-only links of %q = %v, want %v", text, lines, want)
		}
	}
}

func TestBuildCountsPythonImportsBuiltAtRunTime(t *testing.T) {
	cases := map[string]int{
		"importlib.import_module(name)": 1, "import_module(f\"a.{x}\")": 1, "__import__(\"a.\" + x)": 1,
		"import_module(\"a\" if c else \"b\")": 1, "import_module(\".x\", __package__)": 1,
		"importlib.import_module(\"a.b\")": 0, "__import__('a', b)": 0,
	}
	for text, want := range cases {
		got := linksOf(t, map[string]string{"m.py": text + "\n"})
		if len(got.Languages) != 1 || got.Languages[0].Unreadable != want {
			t.Errorf("imports built at run time in %q = %+v, want %d", text, got.Languages, want)
		}
	}
}

func TestBuildListsPythonFilesThatOnlyPassImportsOn(t *testing.T) {
	got := linksOf(t, map[string]string{
		"pkg/__init__.py": "", "pkg/a.py": "A = 1\n", "pkg/b.py": "B = 1\n",
		"pkg/doc.py":    "\"\"\"Doc.\"\"\"\n\nfrom .a import A\nfrom .b import (\n    B,\n)\n\n__all__ = [\"A\", \"B\"]\n",
		"pkg/future.py": "# only this\nfrom __future__ import annotations\nfrom .a import A\n",
		"pkg/def.py":    "from .a import A\n\ndef f(): ...\n",
		"pkg/const.py":  "from .a import A\nVERSION = \"1\"\n",
		"pkg/mixed.py":  "from .a import A; import os\n",
	})
	if !reflect.DeepEqual(got.PassOn, []string{"pkg/doc.py", "pkg/future.py"}) {
		t.Errorf("PassOn = %q, want pkg/doc.py and pkg/future.py", got.PassOn)
	}
}

func TestBuildListsDartFilesThatOnlyPassImportsOn(t *testing.T) {
	got := linksOf(t, map[string]string{
		"lib/a.dart": "const a = 1;\n", "lib/b.dart": "const b = 1;\n",
		"lib/app.dart":    "library app;\n\nexport 'a.dart';\nexport \"b.dart\" show B;\n",
		"lib/barrel.dart": "/// Barrel.\n/* all */\nexport 'a.dart' hide x;\n",
		"lib/own.dart":    "export 'a.dart';\nconst x = 1;\n",
		"lib/reuse.dart":  "import 'a.dart';\nexport 'b.dart';\n",
	})
	if !reflect.DeepEqual(got.PassOn, []string{"lib/app.dart", "lib/barrel.dart"}) {
		t.Errorf("PassOn = %q, want lib/app.dart and lib/barrel.dart", got.PassOn)
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
	got := linksOf(t, goTree())
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
	if crlf := linksOf(t, withCRLF(goTree())); !reflect.DeepEqual(crlf, got) {
		t.Errorf("CRLF line ends: Links = %+v, want %+v", crlf, got)
	}
	without := goTree()
	delete(without, "go.mod")
	if l := linksOf(t, without).Languages[0]; l.Prefix != snapshot.PrefixAbsent || l.Outside != 9 || l.Resolved != 0 {
		t.Errorf("without go.mod: %+v, want PrefixAbsent and every import outside", l)
	}
	noModule := goTree()
	noModule["go.mod"] = "go 1.27\n"
	if l := linksOf(t, noModule).Languages[0]; l.Prefix != snapshot.PrefixUnreadable {
		t.Errorf("go.mod without a module line: Prefix = %v, want PrefixUnreadable", l.Prefix)
	}
	quoted := goTree()
	quoted["go.mod"] = "module \"example.com/m\" // note\n"
	if q := linksOf(t, quoted); q.Languages[0].Prefix != snapshot.PrefixRead || !reflect.DeepEqual(q.List, wantList) {
		t.Errorf("a quoted module line with a comment: %+v, want the same links", q)
	}
}

func TestBuildCountsEachLanguageApart(t *testing.T) {
	files := goTree()
	files["src/a.ts"] = `import { b } from "./b.ts";` + "\n"
	files["src/b.ts"] = "export const b = 1;\n"
	files["src/x.py"] = "x = 1\n"
	got := linksOf(t, files)
	if len(got.Languages) != 3 || got.Languages[0].Language != "typescript" || got.Languages[1].Language != "go" || got.Languages[2].Language != "python" {
		t.Errorf("Languages = %+v, want typescript, go, python: the entries' order", got.Languages)
	}
	if shell := linksOf(t, map[string]string{"bin/a.sh": "echo a\n"}); len(shell.Languages) != 0 || len(shell.List) != 0 {
		t.Errorf("a shell script only: Links = %+v, want none", shell)
	}
}

func TestBuildResolvesEachSideAgainstItsOwnFiles(t *testing.T) {
	build := func(base, now map[string]string) snapshot.Snapshot {
		return snapshot.Build(snapshottest.Files(base, now), conventions.Default(), snapshottest.Readings(t))
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
		!reflect.DeepEqual(moved.Now.Links.List, []snapshot.Link{added(link("src/a.ts", 1, "src/c.ts"))}) {
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
		"src/Acme/Acme.csproj":  "<Project Sdk=\"Microsoft.NET.Sdk\" />\n",
		"src/Acme/A.cs":         "global using Acme.Core;\nusing Acme.zzmarker;\nusing zzmarker.Outside;\n\nnamespace Acme.Core;\n",
		"Package.swift":         "let package = Package(name: \"x\", targets: [.target(name: \"Kit\")])\n",
		"Sources/Kit/Kit.swift": "import zzmarkerOutside\nimport Kit\n",
		// Outside in both forms, not readable, and resolved: the reader keeps no name.
		"src/zz.c":   "#include <zzmarker/outside.h>\n#include \"zzmarker/gone.h\"\n#include ZZMARKER_H\n#include \"zz.h\"\n",
		"src/zz.h":   "#pragma once\n",
		"src/lib.rs": "mod zzmarker_gone;\nuse zzmarker::Outside;\nuse crate::zzmarker_item::{A, zzmarker_b};\nmod zzmarker_inline {\n    use super::zzmarker_x;\n}\n",
	}
	s := snapshot.Build(snapshottest.Files(tree, tree), conventions.Default(), snapshottest.Readings(t))
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
	got := snapshot.Build(snapshottest.Load(t, "testdata/dart"), conventions.Default(), snapshottest.Readings(t)).Now.Links
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
	// A glob names members; a path dependency is own outside a workspace.
	globbed := linksOf(t, map[string]string{
		"pubspec.yaml": "name: _ws\nworkspace:\n  - packages/*\n", "packages/a/pubspec.yaml": "name: a\n",
		"packages/b/pubspec.yaml": "name: b\n", "packages/a/lib/a.dart": "import 'package:b/b.dart';\n", "packages/b/lib/b.dart": "",
		"tool/pubspec.yaml": "name: tool\ndependencies:\n  b:\n    path: ../packages/b\n", "tool/lib/t.dart": "import 'package:b/b.dart';\n",
	})
	if want := []snapshot.Link{link("packages/a/lib/a.dart", 1, "packages/b/lib/b.dart"), link("tool/lib/t.dart", 1, "packages/b/lib/b.dart")}; !reflect.DeepEqual(globbed.List, want) {
		t.Errorf("a glob member and a path dependency: links = %+v, want %+v", globbed.List, want)
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
	tasks, legacy, tasksTest := "src/main/kotlin/com/acme/tasks", "src/main/java/com/acme/tasks/legacy", "src/test/kotlin/com/acme/tasks/data"
	device := "src/androidTest/kotlin/com/acme/tasks/MainScreenTest.kt"
	a, aTest, bTest := "app/src/main/java/com/acme/a/A.java", "app/src/test/java/com/acme/a/ATest.java", "lib/src/test/kotlin/com/acme/b/BTest.kt"
	demo, mainB := "app/src/demo/kotlin/com/acme/b", "app/src/main/kotlin/com/acme/b"
	cases := []struct {
		name      string
		got       snapshot.Links
		languages []snapshot.LanguageImports
		list      []snapshot.Link
	}{
		// App.java and AppTest.java reach the file of the store package that declares NoteStore: the one under
		// src/test declares no such class. The import of the generated NoteProto resolves and gives no link.
		{"the Maven sample", snapshot.Build(snapshottest.Load(t, "testdata/java"), conventions.Default(), snapshottest.Readings(t)).Now.Links,
			[]snapshot.LanguageImports{java(7, 5, 0)},
			[]snapshot.Link{
				folderLink(notes+"/App.java", 3, notes+"/model", notes+"/model/Note.java"),
				folderLink(notes+"/App.java", 6, notes+"/store", notes+"/store/NoteStore.java"),
				folderLink(notes+"/store/NoteStore.java", 3, notes+"/model", notes+"/model/Note.java"),
				folderLink(notes+"/store/NoteStore.java", 4, notes+"/model", notes+"/model/Note.java"),
				folderLink(notesTest+"/AppTest.java", 5, notes+"/store", notes+"/store/NoteStore.java"),
				folderLink(notesTest+"/store/NoteStoreTest.java", 3, notes+"/model", notes+"/model/Note.java"),
			}},
		// Kotlin reaches the Java class and Java the Kotlin one; a top-level function is a member of its package. An
		// import reaches the file that declares its name.
		{"the Gradle sample", snapshot.Build(snapshottest.Load(t, "testdata/kotlin"), conventions.Default(), snapshottest.Readings(t)).Now.Links,
			[]snapshot.LanguageImports{java(1, 0, 0), entry("kotlin", 10, 3, 0)},
			[]snapshot.Link{
				folderLink(device, 3, tasks+"/data", tasks+"/data/TaskRepository.kt"),
				folderLink(legacy+"/Clock.java", 3, tasks+"/model", tasks+"/model/Task.kt"),
				folderLink(tasks+"/Main.kt", 3, tasks+"/data", tasks+"/data/TaskRepository.kt"),
				folderLink(tasks+"/Main.kt", 4, tasks+"/data", tasks+"/data/Loaders.kt"),
				folderLink(tasks+"/Main.kt", 5, legacy, legacy+"/Clock.java"),
				folderLink(tasks+"/data/Loaders.kt", 3, tasks+"/model", tasks+"/model/Task.kt"),
				folderLink(tasks+"/data/Loaders.kt", 4, tasks+"/model", tasks+"/model/Task.kt"),
				folderLink(tasks+"/data/TaskRepository.kt", 3, legacy, legacy+"/Clock.java"),
				folderLink(tasks+"/data/TaskRepository.kt", 4, tasks+"/model", tasks+"/model/Task.kt"),
				folderLink(tasksTest+"/TaskRepositoryTest.kt", 3, legacy, legacy+"/Clock.java"),
				folderLink(tasksTest+"/TestTaskDao.kt", 3, tasks+"/model", tasks+"/model/Task.kt"),
			}},
		// A.java: fakes lies under a test source set only (outside), gen.proto is no tracked package (not found),
		// com.other shares no package (outside). BTest.kt imports its own package and gets no link to itself; ATest.java
		// does not see the tests of the lib module.
		{"a package in several folders", linksOf(t, jvmTree()),
			[]snapshot.LanguageImports{java(3, 2, 1), entry("kotlin", 1, 0, 0)},
			[]snapshot.Link{
				folderLink(a, 3, demo, demo+"/Demo.kt"), folderLink(a, 3, mainB, mainB+"/B.kt"),
				folderLink(aTest, 3, demo, demo+"/Demo.kt"), folderLink(aTest, 3, mainB, mainB+"/B.kt"),
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
	want := linksOf(t, jvmTree())
	if crlf := linksOf(t, withCRLF(jvmTree())); !reflect.DeepEqual(crlf, want) {
		t.Errorf("CRLF line ends: Links = %+v, want %+v", crlf, want)
	}
	bom := jvmTree()
	bom[a] = "\uFEFF" + bom[a]
	if withBOM := linksOf(t, bom); !reflect.DeepEqual(withBOM, want) {
		t.Errorf("a byte order mark: Links = %+v, want %+v", withBOM, want)
	}
	noRoot := linksOf(t, map[string]string{"app/A.java": "import com.acme.b.B;\n", "app/com/acme/b/B.java": "package com.acme.b;\n"})
	if l := noRoot.Languages[0]; l.Prefix != snapshot.PrefixAbsent || l.Outside != 1 || len(noRoot.List) != 0 {
		t.Errorf("Java files without a source root: %+v, want PrefixAbsent and the import outside", noRoot)
	}
}

func TestBuildFollowsPythonImportsToFiles(t *testing.T) {
	got := linksOf(t, pythonTree())
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
	sample := snapshot.Build(snapshottest.Load(t, "testdata/python-declared"), conventions.Default(), snapshottest.Readings(t)).Now.Links
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
	if l := snapshot.Build(undeclared, conventions.Default(), snapshottest.Readings(t)).Now.Links; l.Languages[0].Resolved != 2 || l.Languages[0].Outside != 1 || !slices.Equal(l.Languages[0].Roots, []string{".", "src"}) ||
		slices.ContainsFunc(l.List, func(k snapshot.Link) bool { return k.From == "src/app/service.py" }) {
		t.Errorf("no declared root: Links = %+v", l)
	}
	if l := linksOf(t, map[string]string{"pyproject.toml": "[tool.pytest.ini_options]\npythonpath = [\"lib\"]\n", "lib/app/__init__.py": "", "lib/app/b.py": "x = 1\n", "src/main.py": "import app.b\n"}); !reflect.DeepEqual(l.List, []snapshot.Link{link("src/main.py", 1, "lib/app/b.py")}) {
		t.Errorf("a declared root alone: List = %+v", l.List)
	}
	if crlf := linksOf(t, withCRLF(pythonTree())); !reflect.DeepEqual(crlf, got) {
		t.Errorf("CRLF line ends: Links = %+v, want %+v", crlf, got)
	}
	bom := pythonTree()
	bom[main] = "\uFEFF" + bom[main]
	if withBOM := linksOf(t, bom); !reflect.DeepEqual(withBOM, got) {
		t.Errorf("a byte order mark: Links = %+v, want %+v", withBOM, got)
	}
	lib := linksOf(t, map[string]string{"lib/app/a.py": "import app.b\n", "lib/app/b.py": "x = 1\n"})
	if l := lib.Languages[0]; l.Prefix != snapshot.PrefixAbsent || l.Outside != 1 || len(lib.List) != 0 {
		t.Errorf("packages outside the roots: %+v, want PrefixAbsent and the import outside", lib)
	}
	root := linksOf(t, map[string]string{"main.py": "import helper\nfrom . import helper\nfrom .. import x\nimport main\n", "helper.py": "x = 1\n"})
	if l := root.Languages[0]; l.Resolved != 3 || l.NotFound != 1 ||
		!reflect.DeepEqual(root.List, []snapshot.Link{link("main.py", 1, "helper.py"), link("main.py", 2, "helper.py")}) {
		t.Errorf("modules at the root: %+v", root)
	}
	if semicolon := linksOf(t, map[string]string{"src/a.py": "from app import b; x = 1\n", "src/app/__init__.py": "", "src/app/b.py": "x = 1\n"}); !reflect.DeepEqual(semicolon.List, []snapshot.Link{link("src/a.py", 1, "src/app/b.py")}) {
		t.Errorf("a name list ended by a semicolon: %+v, want the link to b", semicolon.List)
	}
	if stub := linksOf(t, map[string]string{"src/a.py": "import stub\n", "src/stub.pyi": "x: int\n"}); stub.Languages[0].Outside != 1 || len(stub.List) != 0 {
		t.Errorf("an import of a stub: %+v, want outside: a stub is no target", stub)
	}
	mixed := pythonTree()
	mixed["src/app/mixed.py"] = "from app import store, both\nfrom examples.demo import run, nope\n"
	m := linksOf(t, mixed)
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
		{"the sample", snapshot.Build(snapshottest.Load(t, "testdata/csharp"), conventions.Default(), snapshottest.Readings(t)).Now.Links,
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
		{"a namespace in two folders", linksOf(t, csharpTree()),
			[]snapshot.LanguageImports{csharp(6, 5, 1, 1)},
			[]snapshot.Link{
				folderLink(invoice, 1, extras, extras+"/More.cs"), folderLink(invoice, 1, core, coreFiles...),
				folderLink(invoice, 4, core, core+"/Money.cs"),
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
	want := linksOf(t, csharpTree())
	if crlf := linksOf(t, withCRLF(csharpTree())); !reflect.DeepEqual(crlf, want) {
		t.Errorf("CRLF line ends: Links = %+v, want %+v", crlf, want)
	}
	bom := csharpTree()
	bom[invoice] = "\uFEFF" + bom[invoice]
	if withBOM := linksOf(t, bom); !reflect.DeepEqual(withBOM, want) {
		t.Errorf("a byte order mark: Links = %+v, want %+v", withBOM, want)
	}
	noProject := csharpTree()
	delete(noProject, "src/Acme/Acme.csproj")
	if got := linksOf(t, noProject); !reflect.DeepEqual(got.Languages, []snapshot.LanguageImports{csharp(6, 6, 1, 0)}) || !reflect.DeepEqual(got.List, want.List) {
		t.Errorf("without a project file: %+v, want the import of Acme.Grpc outside and the same links", got)
	}
	// A block inside another's declares the joined name; a using reaches the declaring files; an alias names the
	// namespace when one is declared, else the one before its last name.
	nested := linksOf(t, map[string]string{
		"src/Outer.cs": "namespace Acme\n{\n    namespace Store\n    {\n    }\n}\n", "src/Other.cs": "namespace Acme.Web;\n",
		"src/Scoped.cs": "namespace Acme.Jobs;\n",
		"app/App.cs":    "using Acme.Store;\nusing Acme.Jobs;\nusing S = Acme.Store;\nusing T = Acme.Web.Thing;\nusing L = Acme.Web.List<int>;\n",
	})
	if want := []snapshot.Link{folderLink("app/App.cs", 1, "src", "src/Outer.cs"), folderLink("app/App.cs", 2, "src", "src/Scoped.cs"),
		folderLink("app/App.cs", 3, "src", "src/Outer.cs"), folderLink("app/App.cs", 4, "src", "src/Other.cs"),
		folderLink("app/App.cs", 5, "src", "src/Other.cs")}; !reflect.DeepEqual(nested.List, want) {
		t.Errorf("nested namespaces and aliases: List = %+v, want %+v", nested.List, want)
	}
	bare := linksOf(t, map[string]string{"src/A.cs": "using Acme.B;\n", "src/B.cs": "class B {}\n"})
	if l := bare.Languages[0]; l.Prefix != snapshot.PrefixAbsent || l.Outside != 1 || len(bare.List) != 0 {
		t.Errorf("C# files that declare no namespace: %+v, want PrefixAbsent and the import outside", bare)
	}
}

// swiftTree holds a package with a target under its default folder, one under a path of its own, a test target, a
// target without a tracked file, and imports that are outside or inside a string.
func swiftTree() map[string]string {
	return map[string]string{
		"Package.swift": strings.Join([]string{
			"// swift-tools-version:5.9", "import PackageDescription", "", "let package = Package(", "    name: \"x\",", "    targets: [",
			"        .target(name: \"Core\"),", "        .target(", "            name: \"Legacy\",", "            path: \"Old/Legacy\"", "        ),",
			"        .executableTarget(", "            name: \"tool\",",
			"            dependencies: [\"Core\", .target(name: \"Legacy\"), .product(name: \"ArgumentParser\", package: \"swift-argument-parser\")],",
			"            plugins: [.plugin(name: \"Lint\", package: \"LintPlugins\")]", "        ),",
			"        // .target(name: \"Old\"),", "        .target(name: \"Gone\"),",
			"        .testTarget(name: \"CoreTests\", dependencies: [\"Core\"]),", "    ]", ")",
		}, "\n") + "\n",
		"Sources/Core/A.swift":            "import Foundation\n",
		"Sources/Core/Deep/B.swift":       "let text = \"\"\"\nimport Legacy\n\"\"\"\n",
		"Old/Legacy/L.swift":              "import Core\n",
		"Sources/tool/main.swift":         "import Core\n@testable import Legacy\nimport Gone\nimport ArgumentParser\nimport Old\nimport Lint\n",
		"Tests/CoreTests/CoreTests.swift": "import XCTest\n@testable import Core\n\nfinal class CoreTests: XCTestCase {\n    func testA() {}\n}\n",
	}
}

func TestBuildFollowsSwiftImportsToModules(t *testing.T) {
	swift := func(resolved, outside, notFound int, state snapshot.PrefixState) []snapshot.LanguageImports {
		return []snapshot.LanguageImports{{
			Language: "swift", Resolved: resolved, Outside: outside, NotFound: notFound, PrefixFile: "Package.swift", Prefix: state,
			PrefixRequired: true, Folders: true, NoRings: "an import names a module, and the compiler rejects modules that import each other",
			Implicit: []string{"main.swift", "Package.swift", "Package@swift-*.swift"}, NotSeen: "use between the files of one module, which needs no `import`",
		}}
	}
	core := []string{"Sources/NotesCore/Config.swift", "Sources/NotesCore/Model/Note.swift", "Sources/NotesCore/Store.swift"}
	sample := snapshot.Build(snapshottest.Load(t, "testdata/swift"), conventions.Default(), snapshottest.Readings(t)).Now.Links
	wantSample := []snapshot.Link{
		folderLink("Old/Legacy/Clock.swift", 2, "Sources/NotesCore", core...),
		folderLink("Sources/notes/main.swift", 2, "Sources/NotesCore", core...),
		folderLink("Tests/NotesCoreTests/Fixtures.swift", 1, "Sources/NotesCore", core...),
		folderLink("Tests/NotesCoreTests/StoreTests.swift", 2, "Sources/NotesCore", core...),
	}
	if !reflect.DeepEqual(sample.Languages, swift(4, 7, 0, snapshot.PrefixRead)) || !reflect.DeepEqual(sample.List, wantSample) {
		t.Errorf("the sample: Languages %+v, List %+v; want %+v and %+v", sample.Languages, sample.List, swift(4, 7, 0, snapshot.PrefixRead), wantSample)
	}

	// A module reaches every file below its folder; a target of its own path is found there; a target without a
	// tracked file is not found; an import inside a string is no import.
	coreFiles := []string{"Sources/Core/A.swift", "Sources/Core/Deep/B.swift"}
	wantTree := []snapshot.Link{
		folderLink("Old/Legacy/L.swift", 1, "Sources/Core", coreFiles...),
		folderLink("Sources/tool/main.swift", 1, "Sources/Core", coreFiles...),
		folderLink("Sources/tool/main.swift", 2, "Old/Legacy", "Old/Legacy/L.swift"),
		folderLink("Tests/CoreTests/CoreTests.swift", 2, "Sources/Core", coreFiles...),
	}
	got := linksOf(t, swiftTree())
	if !reflect.DeepEqual(got.Languages, swift(4, 6, 1, snapshot.PrefixRead)) || !reflect.DeepEqual(got.List, wantTree) {
		t.Errorf("the tree: Languages %+v, List %+v; want %+v and %+v", got.Languages, got.List, swift(4, 6, 1, snapshot.PrefixRead), wantTree)
	}
	if crlf := linksOf(t, withCRLF(swiftTree())); !reflect.DeepEqual(crlf, got) {
		t.Errorf("CRLF line ends: Links = %+v, want %+v", crlf, got)
	}

	// Without Package.swift an own module cannot be told from a framework.
	bare := swiftTree()
	delete(bare, "Package.swift")
	if l := linksOf(t, bare); !reflect.DeepEqual(l.Languages, swift(0, 10, 0, snapshot.PrefixAbsent)) || len(l.List) != 0 {
		t.Errorf("no Package.swift: %+v, want every import outside and no link", l)
	}
}

// includeTree holds includes found beside the file, in include folders by convention and by a CMake file's text,
// in two folders, in none, outside, and through a macro.
func includeTree() map[string]string {
	header := "#pragma once\n"
	return map[string]string{
		"CMakeLists.txt": "project(app)\n# include_directories(private)\ninclude_directories(src \"${CMAKE_CURRENT_SOURCE_DIR}/third\" ${UNKNOWN_DIR}/x)\nadd_subdirectory(tools)\n",
		"tools/CMakeLists.txt": "project(tools C)\ntarget_include_directories(tool SYSTEM PRIVATE ${CMAKE_SOURCE_DIR}/shared\n" +
			"  $<BUILD_INTERFACE:${PROJECT_SOURCE_DIR}/inc> $<INSTALL_INTERFACE:include>)\n",
		"include/app/api.h": header, "include/dup.h": header, "libs/kit/include/kit/kit.h": header, "libs/kit/include/dup.h": header,
		"src/core.h": "#include \"util/strings.h\"\n", "src/util/strings.h": header, "private/hidden/secret.h": header,
		"shared/common.h": header, "third/lib.h": header, "tools/inc/gen.h": header,
		"src/main.cc": "#include \"core.h\"\n#include <app/api.h>\n#include \"kit/kit.h\"\n#include \"util/strings.h\"\n#include <vector>\n" +
			"#include \"dup.h\"\n#include \"hidden/secret.h\"\n#include \"gtest/gtest.h\"\n#include CONFIG_HEADER\n/*\n#include \"commented.h\"\n*/\n" +
			"#include \"../shared/common.h\"\n#include <core.h>\n#include \"lib.h\"\n",
		"tools/tool.c": "#include \"common.h\"\n#include \"gen.h\"\n#include <stdio.h>\n",
	}
}

func TestBuildFollowsIncludesToFiles(t *testing.T) {
	var notSeen string
	for _, l := range conventions.Default().Languages {
		if l.Name == "c" {
			notSeen = l.Imports.NotSeen
		}
	}
	entry := func(name string, units []string, resolved, outside, ambiguous, unreadable, notFound int) snapshot.LanguageImports {
		return snapshot.LanguageImports{
			Language: name, Resolved: resolved, Outside: outside, Ambiguous: ambiguous, Unreadable: unreadable, NotFound: notFound,
			PrefixFile: "CMakeLists.txt", Prefix: snapshot.PrefixRead, Implicit: units, NotSeen: notSeen,
		}
	}
	c := func(counts ...int) snapshot.LanguageImports {
		return entry("c", []string{"*.c"}, counts[0], counts[1], counts[2], counts[3], counts[4])
	}
	cpp := func(counts ...int) snapshot.LanguageImports {
		return entry("cpp", []string{"*.cc", "*.cpp", "*.cxx"}, counts[0], counts[1], counts[2], counts[3], counts[4])
	}
	store := "include/notes/store.hpp"
	cases := []struct {
		name      string
		got       snapshot.Links
		languages []snapshot.LanguageImports
		list      []snapshot.Link
	}{
		{"the C sample", snapshot.Build(snapshottest.Load(t, "testdata/c"), conventions.Default(), snapshottest.Readings(t)).Now.Links,
			[]snapshot.LanguageImports{c(4, 2, 0, 0, 0)},
			[]snapshot.Link{
				link("src/buffer.c", 1, "src/buffer.h"), link("src/tally.c", 1, "include/tally.h"), link("src/tally.c", 5, "src/buffer.h"),
				link("tests/test_tally.c", 3, "include/tally.h"),
			}},
		// The include inside the comment of src/store.cc is not read; fake_clock.h reaches src/clock.h through the
		// folder that only the root CMakeLists.txt names, and store_test.cc its support folder through tests/CMakeLists.txt.
		{"the C++ sample", snapshot.Build(snapshottest.Load(t, "testdata/cpp"), conventions.Default(), snapshottest.Readings(t)).Now.Links,
			[]snapshot.LanguageImports{cpp(7, 7, 0, 0, 0)},
			[]snapshot.Link{
				link(store, 5, "include/notes/note.h"), link("src/main.cpp", 3, store), link("src/store.cc", 5, store),
				link("src/store.cc", 10, "src/clock.h"), link("tests/store_test.cc", 3, "tests/support/fake_clock.h"),
				link("tests/store_test.cc", 4, store), link("tests/support/fake_clock.h", 3, "src/clock.h"),
			}},
		// One .c and one .cc file: the headers stay with the first entry that lists .h.
		{"the tree", linksOf(t, includeTree()),
			[]snapshot.LanguageImports{c(3, 1, 0, 0, 0), cpp(7, 2, 1, 1, 1)},
			[]snapshot.Link{
				link("src/core.h", 1, "src/util/strings.h"),
				link("src/main.cc", 1, "src/core.h"), link("src/main.cc", 2, "include/app/api.h"), link("src/main.cc", 3, "libs/kit/include/kit/kit.h"),
				link("src/main.cc", 4, "src/util/strings.h"), link("src/main.cc", 13, "shared/common.h"), link("src/main.cc", 14, "src/core.h"),
				link("src/main.cc", 15, "third/lib.h"),
				link("tools/tool.c", 1, "shared/common.h"), link("tools/tool.c", 2, "tools/inc/gen.h"),
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
	want := linksOf(t, includeTree())
	if crlf := linksOf(t, withCRLF(includeTree())); !reflect.DeepEqual(crlf, want) {
		t.Errorf("CRLF line ends: Links = %+v, want %+v", crlf, want)
	}
	marked := includeTree()
	for _, p := range []string{"CMakeLists.txt", "tools/CMakeLists.txt", "src/main.cc"} {
		marked[p] = "\uFEFF" + marked[p]
	}
	if bom := linksOf(t, marked); !reflect.DeepEqual(bom, want) {
		t.Errorf("a byte order mark: Links = %+v, want %+v", bom, want)
	}
	// A command in capitals and over lines with a comment inside it names the same folders; one that never closes
	// is no failure.
	shouted := includeTree()
	shouted["CMakeLists.txt"] = "PROJECT(app)\nINCLUDE_DIRECTORIES(\n  src # the sources\n  \"${CMAKE_CURRENT_SOURCE_DIR}/third\"\n)\nadd_subdirectory(tools)\ninclude_directories(private"
	if got := linksOf(t, shouted); !reflect.DeepEqual(got.Languages, want.Languages) {
		t.Errorf("a command in capitals, over lines and unclosed: Languages = %+v, want %+v", got.Languages, want.Languages)
	}
	// Without the CMake files only the folders named include are known: what the declared folders led to is not
	// found where a tracked file ends in the include's path, and outside in angle brackets.
	bare := includeTree()
	delete(bare, "CMakeLists.txt")
	delete(bare, "tools/CMakeLists.txt")
	// A comment marker inside a string starts no comment, so the include after it is read.
	inString := linksOf(t, map[string]string{
		"src/a.c": "const char *s = \"/*\";\n#include \"b.h\"\nint x; /* end */\n", "src/b.h": "#pragma once\n",
	})
	if !reflect.DeepEqual(inString.List, []snapshot.Link{link("src/a.c", 2, "src/b.h")}) {
		t.Errorf("an include after a string holding /*: List = %+v, want src/a.c to src/b.h", inString.List)
	}
	// A file the grammar reads without an error: an include inside a raw string is text, not an include.
	raw := linksOf(t, map[string]string{
		"src/a.cpp": "const char *s = R\"(\n#include \"c.h\"\n)\";\n#include \"b.h\"\n", "src/b.h": "#pragma once\n",
		"src/c.h": "#pragma once\n",
	})
	if !reflect.DeepEqual(raw.List, []snapshot.Link{link("src/a.cpp", 4, "src/b.h")}) {
		t.Errorf("an include inside a raw string: List = %+v, want only src/a.cpp to src/b.h", raw.List)
	}
	// A file the grammar cannot read without an error is read line by line: every include counts, and the file is
	// not named as one whose imports may be missed.
	broken := snapshot.Build(snapshottest.Files(nil, map[string]string{
		"src/a.c": "#include \"b.h\"\nint f( {\nstruct S {\n#include \"c.h\"\n", "src/b.h": "#pragma once\n",
		"src/c.h": "#pragma once\n",
	}), conventions.Default(), snapshottest.Readings(t))
	if want := []snapshot.Link{added(link("src/a.c", 1, "src/b.h")), added(link("src/a.c", 4, "src/c.h"))}; !reflect.DeepEqual(broken.Now.Links.List, want) || len(broken.SyntaxErrors) != 0 {
		t.Errorf("a file with a syntax error: List = %+v, syntax errors %q; want %+v and none", broken.Now.Links.List, broken.SyntaxErrors, want)
	}
	// Read line by line, an include inside a raw string is text too; a quoted include stays an include.
	brokenRaw := snapshot.Build(snapshottest.Files(nil, map[string]string{
		"src/a.cpp": "auto q = R\"sql(\n#include \"c.h\"\n)sql\";\n#include \"b.h\"\nint f( {\n", "src/b.h": "#pragma once\n",
		"src/c.h": "#pragma once\n",
	}), conventions.Default(), snapshottest.Readings(t))
	if want := []snapshot.Link{added(link("src/a.cpp", 4, "src/b.h"))}; !reflect.DeepEqual(brokenRaw.Now.Links.List, want) {
		t.Errorf("a raw string in a file read line by line: List = %+v, want %+v", brokenRaw.Now.Links.List, want)
	}
	// A define with no value and a trailing space is misread by the grammar without an error: the next line becomes
	// its value. Such a file is read line by line too.
	define := linksOf(t, map[string]string{
		"src/a.h": "#ifndef A_H_\n#define A_H_ \n\n#include \"b.h\"\n#endif\n", "src/b.h": "#pragma once\n",
	})
	if want := []snapshot.Link{link("src/a.h", 4, "src/b.h")}; !reflect.DeepEqual(define.List, want) {
		t.Errorf("an include after a define with a trailing space: List = %+v, want %+v", define.List, want)
	}
	// A path written with Windows separators names the same file.
	windows := linksOf(t, map[string]string{"src/a.c": "#include \"sub\\x.h\"\n", "src/sub/x.h": "#pragma once\n"})
	if !reflect.DeepEqual(windows.List, []snapshot.Link{link("src/a.c", 1, "src/sub/x.h")}) {
		t.Errorf("an include with a backslash: List = %+v, want src/a.c to src/sub/x.h", windows.List)
	}
	if got := linksOf(t, bare).Languages; !reflect.DeepEqual(got, []snapshot.LanguageImports{c(1, 1, 0, 0, 2), cpp(5, 3, 1, 1, 2)}) {
		t.Errorf("no CMake file: Languages = %+v, want %+v", got, []snapshot.LanguageImports{c(1, 1, 0, 0, 2), cpp(5, 3, 1, 1, 2)})
	}
}

func TestBuildReadsImportsAroundASyntaxError(t *testing.T) {
	// An unfinished edit is read up to its error and named, since an import after it may be missed; a file the change
	// did not touch loses the same imports on both sides and is not named.
	same := "import pkg.b\nx = f(\n"
	base := map[string]string{"pkg/__init__.py": "", "pkg/b.py": "", "pkg/c.py": "", "pkg/same.py": same}
	now := map[string]string{
		"pkg/__init__.py": "", "pkg/b.py": "", "pkg/c.py": "", "pkg/same.py": same,
		"pkg/a.py": "import pkg.b\n\nCONFIG = dict(\n    port=1,\n\ndef main():\n    from pkg import c\n    return c\n",
		"go.mod":   "module example.com/m\n", "a/a.go": "package a\n", "b/b.go": "package b\n",
		"main.go": "package main\n\nimport \"example.com/m/a\"\n\nfunc f( {\n\nimport \"example.com/m/b\"\n",
	}
	s := snapshot.Build(snapshottest.Files(base, now), conventions.Default(), snapshottest.Readings(t))
	for _, want := range []snapshot.Link{link("main.go", 3, "a"), link("pkg/a.py", 1, "pkg/b.py")} {
		if !slices.ContainsFunc(s.Now.Links.List, func(l snapshot.Link) bool { return l.From == want.From && l.Line == want.Line && l.To == want.To }) {
			t.Errorf("Build(an unfinished edit): Links = %+v, want %s:%d -> %s", s.Now.Links.List, want.From, want.Line, want.To)
		}
	}
	if want := []string{"main.go", "pkg/a.py"}; !slices.Equal(s.SyntaxErrors, want) || len(s.Unparsed) != 0 {
		t.Errorf("Build(an unfinished edit) = syntax errors %q, unparsed %q; want %q and nothing unparsed", s.SyntaxErrors, s.Unparsed, want)
	}
}

// A test file sees its own module's tests, another module's test fixtures when its build file asks for them, and
// another module's tests when its pom depends on their test-jar; an import narrows to the declaring file only when
// every file of the package's folders was read whole.
func TestBuildNarrowsJavaImportsByModuleAndName(t *testing.T) {
	const fixture = "package com.acme.f;\n\npublic class Fixture {}\n"
	consumer := "package com.acme.c;\n\nimport com.acme.f.Fixture;\n"
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  []snapshot.Link
	}{
		{"test fixtures by project path", map[string]string{
			"a/build.gradle.kts":    "dependencies { testImplementation(testFixtures(project(\":lib:core\"))) }\n",
			"lib/core/build.gradle": "", "lib/core/src/testFixtures/java/com/acme/f/Fixture.java": fixture,
			"lib/core/src/test/java/com/acme/f/Other.java": fixture,
			"a/src/test/java/com/acme/c/C.java":            consumer,
		}, []snapshot.Link{folderLink("a/src/test/java/com/acme/c/C.java", 3, "lib/core/src/testFixtures/java/com/acme/f",
			"lib/core/src/testFixtures/java/com/acme/f/Fixture.java")}},
		{"test fixtures by accessor", map[string]string{
			"a/build.gradle.kts":            "dependencies { testImplementation(testFixtures(projects.lib.coreApi)) }\n",
			"lib/core-api/build.gradle.kts": "", "lib/core-api/src/testFixtures/java/com/acme/f/Fixture.java": fixture,
			"a/src/test/java/com/acme/c/C.java": consumer,
		}, []snapshot.Link{folderLink("a/src/test/java/com/acme/c/C.java", 3, "lib/core-api/src/testFixtures/java/com/acme/f",
			"lib/core-api/src/testFixtures/java/com/acme/f/Fixture.java")}},
		{"a test-jar", map[string]string{
			"a/pom.xml": "<project><groupId>com.acme</groupId><artifactId>a</artifactId><dependencies><dependency>" +
				"<groupId>com.acme</groupId><artifactId>lib</artifactId><type>test-jar</type></dependency></dependencies></project>\n",
			"lib/pom.xml": "<project><groupId>com.acme</groupId><artifactId>lib</artifactId></project>\n",
			"lib/src/test/java/com/acme/f/Fixture.java": fixture, "a/src/test/java/com/acme/c/C.java": consumer,
		}, []snapshot.Link{folderLink("a/src/test/java/com/acme/c/C.java", 3, "lib/src/test/java/com/acme/f", "lib/src/test/java/com/acme/f/Fixture.java")}},
		{"Groovy's project path", map[string]string{
			"a/build.gradle": "dependencies { testImplementation testFixtures(project(path: ':core')) }\n", "core/build.gradle": "",
			"core/src/testFixtures/java/com/acme/f/Fixture.java": fixture, "a/src/test/java/com/acme/c/C.java": consumer,
		}, []snapshot.Link{folderLink("a/src/test/java/com/acme/c/C.java", 3, "core/src/testFixtures/java/com/acme/f",
			"core/src/testFixtures/java/com/acme/f/Fixture.java")}},
		{"modules under a top-level src folder", map[string]string{
			"src/other/src/test/java/com/acme/f/Fixture.java": fixture, "src/app/src/test/java/com/acme/c/C.java": consumer,
		}, nil},
		{"a static import narrows by the class", map[string]string{
			"lib/src/main/java/com/acme/f/Fixture.java": "package com.acme.f;\n\npublic class Fixture { public static int X; }\n",
			"lib/src/main/java/com/acme/f/Other.java":   "package com.acme.f;\n\npublic class Other {}\n",
			"a/src/main/java/com/acme/c/C.java":         "package com.acme.c;\n\nimport static com.acme.f.Fixture.X;\n",
		}, []snapshot.Link{folderLink("a/src/main/java/com/acme/c/C.java", 3, "lib/src/main/java/com/acme/f", "lib/src/main/java/com/acme/f/Fixture.java")}},
		{"a folder of Java and Kotlin files narrows", map[string]string{
			"lib/src/main/java/com/acme/f/Fixture.kt": "package com.acme.f\n\nclass Fixture\n",
			"lib/src/main/java/com/acme/f/Other.java": "package com.acme.f;\n\npublic class Other {}\n",
			"a/src/main/java/com/acme/c/C.java":       consumer,
		}, []snapshot.Link{folderLink("a/src/main/java/com/acme/c/C.java", 3, "lib/src/main/java/com/acme/f", "lib/src/main/java/com/acme/f/Fixture.kt")}},
		{"no such dependency", map[string]string{
			"lib/src/test/java/com/acme/f/Fixture.java": fixture, "a/src/test/java/com/acme/c/C.java": consumer,
		}, nil},
		{"a file of the package that is not read whole keeps the folder", map[string]string{
			"lib/src/main/java/com/acme/f/Fixture.java": fixture, "lib/src/main/java/com/acme/f/Broken.java": "package com.acme.f;\n\nclass {\n",
			"a/src/main/java/com/acme/c/C.java": consumer,
		}, []snapshot.Link{folderLink("a/src/main/java/com/acme/c/C.java", 3, "lib/src/main/java/com/acme/f",
			"lib/src/main/java/com/acme/f/Broken.java", "lib/src/main/java/com/acme/f/Fixture.java")}},
	} {
		if got := linksOf(t, tc.files).List; !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: List = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

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

// TypeScript: a file's own config, extends chains, solution configs with references, and workspace packages with and
// without exports.
func TestBuildFollowsTypeScriptConfigsAndWorkspacePackages(t *testing.T) {
	imp := func(specs ...string) string {
		var lines []string
		for _, s := range specs {
			lines = append(lines, "import { x } from \""+s+"\";")
		}
		return strings.Join(lines, "\n") + "\n"
	}
	for _, tc := range []struct {
		name                          string
		files                         map[string]string
		want                          []snapshot.Link
		notFound, unreadable, outside int
	}{
		{"a nested config's paths", map[string]string{
			"web/tsconfig.json": `{"compilerOptions": {"paths": {"@/*": ["./src/*"]}}}`, "web/src/a.ts": imp("@/b"), "web/src/b.ts": "",
		}, []snapshot.Link{link("web/src/a.ts", 1, "web/src/b.ts")}, 0, 0, 0},
		{"paths inherited along extends, from the base's folder", map[string]string{
			"config/base.json": `{"compilerOptions": {"paths": {"~/*": ["../shared/*"]}}}`, "shared/s.ts": "",
			"app/tsconfig.json": `{"extends": "../config/base.json"}`, "app/a.ts": imp("~/s"),
		}, []snapshot.Link{link("app/a.ts", 1, "shared/s.ts")}, 0, 0, 0},
		{"a later base replaces an earlier one", map[string]string{
			"one.json": `{"compilerOptions": {"paths": {"x/*": ["./one/*"]}}}`, "two.json": `{"compilerOptions": {"paths": {"x/*": ["./two/*"]}}}`,
			"tsconfig.json": `{"extends": ["./one.json", "./two"]}`, "one/m.ts": "", "two/m.ts": "", "a.ts": imp("x/m"),
		}, []snapshot.Link{link("a.ts", 1, "two/m.ts")}, 0, 0, 0},
		{"a solution's referenced config that takes the file", map[string]string{
			"tsconfig.json":      `{"files": [], "references": [{"path": "./tsconfig.app.json"}, {"path": "./tsconfig.test.json"}]}`,
			"tsconfig.app.json":  `{"include": ["src"], "compilerOptions": {"paths": {"#/*": ["./src/*"]}}}`,
			"tsconfig.test.json": `{"include": ["test"], "compilerOptions": {"paths": {"#/*": ["./test/*"]}}}`,
			"src/a.ts":           imp("#/b"), "src/b.ts": "", "test/b.ts": "",
		}, []snapshot.Link{link("src/a.ts", 1, "src/b.ts")}, 0, 0, 0},
		{"a workspace package with exports", map[string]string{
			"packages/ui/package.json": `{"name": "@acme/ui", "exports": {".": {"types": "./src/index.ts", "default": "./dist/index.js"},
				"./button": "./src/button.ts", "./icons/*": "./src/icons/*.ts", "./internal/*": null}}`,
			"packages/ui/src/index.ts": "", "packages/ui/src/button.ts": "", "packages/ui/src/icons/star.ts": "",
			"app/a.ts": imp("@acme/ui", "@acme/ui/button", "@acme/ui/icons/star", "@acme/ui/internal/x", "@acme/ui/missing"),
		}, []snapshot.Link{link("app/a.ts", 1, "packages/ui/src/index.ts"), link("app/a.ts", 2, "packages/ui/src/button.ts"),
			link("app/a.ts", 3, "packages/ui/src/icons/star.ts")}, 2, 0, 0},
		{"a workspace package without exports, its build output mapped to its sources", map[string]string{
			"packages/core/package.json":  `{"name": "core", "main": "./dist/index.js", "types": "./dist/index.d.ts"}`,
			"packages/core/tsconfig.json": `{"compilerOptions": {"outDir": "dist", "rootDir": "lib"}}`,
			"packages/core/lib/index.ts":  "", "packages/core/util.ts": "",
			"app/a.ts": imp("core", "core/util", "left-pad"),
		}, []snapshot.Link{link("app/a.ts", 1, "packages/core/lib/index.ts"), link("app/a.ts", 2, "packages/core/util.ts")}, 0, 0, 1},
		{"a target with no tracked source is not readable", map[string]string{
			"packages/gen/package.json": `{"name": "gen", "main": "./dist/index.js"}`, "app/a.ts": imp("gen"),
		}, nil, 0, 1, 0},
	} {
		got := linksOf(t, tc.files)
		l := got.Languages[0]
		if !reflect.DeepEqual(got.List, tc.want) || l.NotFound != tc.notFound || l.Unreadable != tc.unreadable || l.Outside != tc.outside {
			t.Errorf("%s: List = %+v, %+v; want %+v, not found %d, not readable %d, outside %d", tc.name, got.List, l, tc.want,
				tc.notFound, tc.unreadable, tc.outside)
		}
	}
}

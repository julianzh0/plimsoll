package snapshot_test

import (
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

func TestBuildListsRustFilesThatOnlyPassImportsOn(t *testing.T) {
	got := linksOf(t, map[string]string{
		"Cargo.toml":    "[package]\nname = \"app\"\n",
		"src/lib.rs":    "mod p1;\nmod p2;\nmod p3;\nmod p4;\n",
		"src/p1/mod.rs": "#![deny(missing_docs)]\n\nmod a;\npub mod b;\n\npub use a::A;\npub use crate::p1::b::{B, C};\npub extern crate x as y;\n",
		"src/p1/a.rs":   "pub struct A;\n", "src/p1/b.rs": "pub struct B;\npub struct C;\n",
		"src/p2/mod.rs": "#[cfg(test)]\nmod tests;\nmod a;\n", "src/p2/tests.rs": "", "src/p2/a.rs": "",
		"src/p3/mod.rs": "mod a;\n\npub fn f() {}\n", "src/p3/a.rs": "",
		"src/p4/mod.rs": "mod a;\nuse a::A;\n", "src/p4/a.rs": "pub struct A;\n",
	})
	if want := []string{"src/lib.rs", "src/p1/mod.rs", "src/p2/mod.rs"}; !reflect.DeepEqual(got.PassOn, want) {
		t.Errorf("PassOn = %q, want %q", got.PassOn, want)
	}
}

// rustTree holds a library with an inline module, a module in both file forms, a module without a file, a second
// package whose library has another name, a path dependency on it, statements inside a string and a nested comment,
// and an integration test with a module of its own.
func rustTree() map[string]string {
	return map[string]string{
		"Cargo.toml":       "[package]\nname = \"app\"\nedition = \"2021\"\n\n[dependencies]\napp-core = { path = \"core\" }\n",
		"core/Cargo.toml":  "[package]\nname = \"app-core\"\nedition = \"2021\"\n\n[lib]\nname = \"corelib\"\n",
		"core/src/lib.rs":  "pub mod util;\n",
		"core/src/util.rs": "pub fn f() {}\n",
		"src/lib.rs": strings.Join([]string{
			"mod a;", "mod both;", "mod gone;", "mod inline {", "    mod deep;", "    use super::a::A;", "    use self::deep::D;", "}",
			"use a::A;", "use crate::a::sub::S;", "use corelib::util::f;", "use app_core::util;", "use std::io;",
			"use {std::fmt, crate::a::{self, sub::{S, T}}};", "extern crate corelib;",
			"const TEXT: &str = \"", "use crate::gone_in_string;", "\";",
			"/* a comment", "use crate::gone_in_comment;", "/* nested */", "use crate::gone_after_nested;", "*/",
			"pub struct Root;",
		}, "\n") + "\n",
		"src/a.rs": strings.Join([]string{
			"pub mod sub;", "use super::Root;", "use super::super::X;", "use self::sub::S;", "use crate::Root;",
			"#[cfg(test)]", "mod tests {", "    use super::*;", "    use super::super::Root;", "}", "pub struct A;",
		}, "\n") + "\n",
		"src/a/sub.rs": "pub struct S;\npub struct T;\n", "src/both.rs": "", "src/both/mod.rs": "", "src/inline/deep.rs": "pub struct D;\n",
		"tests/it.rs":         "mod common;\nuse app::a::A;\nuse crate::common::helper;\nfn thing() {}\n",
		"tests/common/mod.rs": "use super::thing;\nuse crate::common;\npub fn helper() {}\n",
	}
}

func TestBuildFollowsRustImportsToFiles(t *testing.T) {
	entry := func(resolved, outside, ambiguous, notFound int, state snapshot.PrefixState) []snapshot.LanguageImports {
		return []snapshot.LanguageImports{{
			Language: "rust", Resolved: resolved, Outside: outside, Ambiguous: ambiguous, NotFound: notFound,
			PrefixFile: "Cargo.toml", Prefix: state,
			NoRings:    "a crate's modules use each other, and Rust compiles them as one unit",
			Implicit:   []string{"main.rs", "lib.rs", "build.rs"},
			NotSeen:    "a path written in code without a `use` line, and a module or name a macro creates",
			NoImporter: "a parent file that only declares a module (`mod x;`)",
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
	check("the sample", snapshot.Build(snapshottest.Load(t, "testdata/rust"), conventions.Default(), snapshottest.Readings(t)).Now.Links,
		entry(27, 3, 0, 0, snapshot.PrefixRead),
		[]snapshot.Link{
			modLink(member+"lib.rs", 1, member+"disk.rs"), link(member+"lib.rs", 3, member+"disk.rs"),
			link("src/cli.rs", 1, bin),
			modLink(lib, 5, "src/config.rs"), modLink(lib, 6, model), modLink(lib, 7, store), link(lib, 9, model), link(lib, 10, memory),
			modLink(bin, 1, "src/cli.rs"), link(bin, 3, "src/config.rs"), link(bin, 3, lib), link(bin, 3, store), link(bin, 4, member+"lib.rs"),
			link(memory, 1, store), link(memory, 2, model), link(memory, 2, store), link(memory, 19, model),
			modLink(store, 1, memory), link(store, 3, lib),
			link("tests/common/mod.rs", 1, lib),
			modLink("tests/notes.rs", 1, "tests/common/mod.rs"), link("tests/notes.rs", 3, memory), link("tests/notes.rs", 3, store),
		}, []string{member + "lib.rs"})

	// The tree: design 5.5, row by row. Line 14 of lib.rs names a.rs once and a/sub.rs twice: one link each.
	a, sub, deep, common := "src/a.rs", "src/a/sub.rs", "src/inline/deep.rs", "tests/common/mod.rs"
	wantTree := []snapshot.Link{
		modLink("core/src/lib.rs", 1, "core/src/util.rs"),
		modLink(a, 1, sub), link(a, 2, lib), link(a, 4, sub), link(a, 5, lib), link(a, 9, lib),
		modLink(lib, 1, a), modLink(lib, 5, deep), link(lib, 6, a), link(lib, 7, deep), link(lib, 9, a), link(lib, 10, sub),
		link(lib, 11, "core/src/util.rs"), link(lib, 14, a), link(lib, 14, sub), link(lib, 15, "core/src/lib.rs"),
		link(common, 1, "tests/it.rs"), modLink("tests/it.rs", 1, common), link("tests/it.rs", 2, a), link("tests/it.rs", 3, common),
	}
	got := linksOf(t, rustTree())
	check("the tree", got, entry(23, 3, 1, 2, snapshot.PrefixRead), wantTree, []string{"core/src/lib.rs"})
	if crlf := linksOf(t, withCRLF(rustTree())); !reflect.DeepEqual(crlf, got) {
		t.Errorf("CRLF line ends: Links = %+v, want %+v", crlf, got)
	}
	bom := rustTree()
	bom[lib], bom["Cargo.toml"] = "\uFEFF"+bom[lib], "\uFEFF"+bom["Cargo.toml"]
	if withBOM := linksOf(t, bom); !reflect.DeepEqual(withBOM, got) {
		t.Errorf("a byte order mark: Links = %+v, want %+v", withBOM, got)
	}

	// Without a manifest the crates' names are outside; every other link stays.
	bare := rustTree()
	delete(bare, "Cargo.toml")
	delete(bare, "core/Cargo.toml")
	byName := func(l snapshot.Link) bool {
		return l.From == lib && (l.Line == 11 || l.Line == 15) || l.From == "tests/it.rs" && l.Line == 2
	}
	check("no Cargo.toml", linksOf(t, bare), entry(20, 6, 1, 2, snapshot.PrefixAbsent), slices.DeleteFunc(slices.Clone(wantTree), byName), []string{"core/src/lib.rs"})

	// A package without a library: its name leads nowhere of its own.
	check("a binary alone", linksOf(t, map[string]string{
		"Cargo.toml": "[package]\nname = \"tool\"\nedition = \"2021\"\n", "src/main.rs": "use tool::x;\nmod cli;\nfn run() {}\n", "src/cli.rs": "use crate::run;\n",
	}), entry(2, 1, 0, 0, snapshot.PrefixRead), []snapshot.Link{link("src/cli.rs", 1, "src/main.rs"), modLink("src/main.rs", 2, "src/cli.rs")}, nil)

	// A module that two crate roots declare: its crate paths agree, its parent does not.
	check("a module of two tests", linksOf(t, map[string]string{
		"tests/a.rs": "mod common;\nfn y() {}\n", "tests/b.rs": "mod common;\nfn y() {}\n", "tests/common/mod.rs": "use crate::common::x;\nuse super::y;\n",
	}), entry(3, 0, 1, 0, snapshot.PrefixAbsent), []snapshot.Link{modLink("tests/a.rs", 1, common), modLink("tests/b.rs", 1, common)}, nil)

	// A package table without a name names no crate; the links by crate, self and super need none.
	check("a package without a name", linksOf(t, map[string]string{
		"Cargo.toml": "[package]\nversion = \"1\"\n", "src/lib.rs": "mod a;\npub struct X;\n", "src/a.rs": "use crate::X;\n",
	}), entry(2, 0, 0, 0, snapshot.PrefixAbsent), []snapshot.Link{link("src/a.rs", 1, "src/lib.rs"), modLink("src/lib.rs", 1, "src/a.rs")}, nil)

	// A comment inside a group, holding a comma, is no part of a path; a character of several bytes before a macro
	// body moves no line.
	group := map[string]string{"src/c/mod.rs": "mod d;\nmod e;\n", "src/c/d.rs": "", "src/c/e.rs": "", "src/m.rs": ""}
	wantGroup := []snapshot.Link{modLink("src/c/mod.rs", 1, "src/c/d.rs"), modLink("src/c/mod.rs", 2, "src/c/e.rs"),
		modLink(lib, 1, "src/c/mod.rs"), link(lib, 2, "src/c/d.rs"), link(lib, 2, "src/c/e.rs"), modLink(lib, 6, "src/m.rs")}
	for _, text := range []string{
		"mod c;\nuse crate::c::{d /* , */, e};\n// " + strings.Repeat("é ", 14) + "\nx! {\n\n    mod m;\n}\n",
		"mod c;\nuse crate::c::{d, // y, z\n e};\n// —\nx! {\n    mod m;\n}\n",
	} {
		group[lib] = text
		if got := linksOf(t, group).List; !reflect.DeepEqual(got, wantGroup) {
			t.Errorf("Links of lib.rs %q = %+v, want %+v", text, got, wantGroup)
		}
	}

	// A brace that never closes: the inline module runs to the end of the file.
	check("a module that does not close", linksOf(t, map[string]string{"src/lib.rs": "mod a {\n    use super::b;\nmod c;\n"}),
		entry(1, 0, 0, 1, snapshot.PrefixAbsent), nil, nil)
}

// modLink is the link of a module declared in its own file (`mod x;`): containment, not use.
func modLink(from string, line int, to string) snapshot.Link {
	l := link(from, line, to)
	l.Declares = true
	return l
}

// Each row of the Rust reader: crates from their manifests, crate roots from their targets, the 2015 edition, path
// attributes, modules in macro bodies, and names no file binds.
func TestBuildFollowsRustCratesAndModules(t *testing.T) {
	const manifest = "[package]\nname = \"app\"\nedition = \"2021\"\n"
	store := map[string]string{"store/Cargo.toml": "[package]\nname = \"notes-store\"\nedition = \"2021\"\n", "store/src/lib.rs": "pub fn f() {}\n"}
	with := func(base map[string]string, files ...string) map[string]string {
		out := maps.Clone(base)
		for i := 0; i+1 < len(files); i += 2 {
			out[files[i]] = files[i+1]
		}
		return out
	}
	for _, tc := range []struct {
		name              string
		files             map[string]string
		want              []snapshot.Link
		outside, notFound int
	}{
		{"a library root named by [lib] path", map[string]string{
			"Cargo.toml": manifest + "\n[lib]\npath = \"lib/app.rs\"\n", "lib/app.rs": "pub mod util;\n", "lib/util.rs": "pub fn f() {}\n",
			"src/main.rs": "use app::util::f;\n",
		}, []snapshot.Link{modLink("lib/app.rs", 1, "lib/util.rs"), link("src/main.rs", 1, "lib/util.rs")}, 0, 0},
		{"a renamed path dependency, by its key", with(store,
			"Cargo.toml", manifest+"\n[dependencies]\nstore = { path = \"store\", package = \"notes-store\" }\n",
			"src/lib.rs", "use store::f;\nuse notes_store::f;\n"),
			[]snapshot.Link{link("src/lib.rs", 1, "store/src/lib.rs")}, 1, 0},
		{"a dependency without a path, beside a local crate of its name", with(store,
			"Cargo.toml", manifest+"\n[dependencies]\nnotes-store = \"1\"\n", "src/lib.rs", "use notes_store::f;\n"),
			nil, 1, 0},
		{"a path from the workspace's entry", with(store,
			"Cargo.toml", "[workspace]\nmembers = [\"app\", \"store\"]\n\n[workspace.dependencies]\nnotes-store = { path = \"store\" }\n",
			"app/Cargo.toml", manifest+"\n[dev-dependencies]\nnotes-store = { workspace = true }\n", "app/src/lib.rs", "use notes_store::f;\n"),
			[]snapshot.Link{link("app/src/lib.rs", 1, "store/src/lib.rs")}, 0, 0},
		{"the 2015 edition reads a first name from the crate root", map[string]string{
			"Cargo.toml": "[package]\nname = \"app\"\n", "src/lib.rs": "mod a;\nmod b;\n", "src/a.rs": "use b::B;\nuse std::io;\n",
			"src/b.rs": "pub struct B;\n",
		}, []snapshot.Link{link("src/a.rs", 1, "src/b.rs"), modLink("src/lib.rs", 1, "src/a.rs"), modLink("src/lib.rs", 2, "src/b.rs")}, 1, 0},
		{"the 2018 edition does not", map[string]string{
			"Cargo.toml": manifest, "src/lib.rs": "mod a;\nmod b;\n", "src/a.rs": "use b::B;\n", "src/b.rs": "pub struct B;\n",
		}, []snapshot.Link{modLink("src/lib.rs", 1, "src/a.rs"), modLink("src/lib.rs", 2, "src/b.rs")}, 1, 0},
		{"a crate root under another name holds its modules in its own folder", map[string]string{
			"Cargo.toml": manifest, "src/bin/tool.rs": "mod x;\n", "src/bin/x.rs": "", "src/bin/tool/x.rs": "",
		}, []snapshot.Link{modLink("src/bin/tool.rs", 1, "src/bin/x.rs")}, 0, 0},
		{"unless the manifest turns discovery off", map[string]string{
			"Cargo.toml": manifest + "autobins = false\n", "src/bin/tool.rs": "mod x;\n", "src/bin/x.rs": "", "src/bin/tool/x.rs": "",
		}, []snapshot.Link{modLink("src/bin/tool.rs", 1, "src/bin/tool/x.rs")}, 0, 0},
		{"a name no file binds is not found", map[string]string{
			"src/lib.rs": "mod a;\nuse a::Gone;\nuse a::Here;\n", "src/a.rs": "pub struct Here;\n",
		}, []snapshot.Link{modLink("src/lib.rs", 1, "src/a.rs"), link("src/lib.rs", 3, "src/a.rs")}, 0, 1},
		{"a glob use, a derive or a macro call may bind it", map[string]string{
			"src/lib.rs": "mod a;\nmod b;\nmod c;\nuse a::Gone;\nuse b::FooBuilder;\nuse c::Made;\n",
			"src/a.rs":   "pub use std::*;\n", "src/b.rs": "#[derive(Builder)]\npub struct Foo;\n", "src/c.rs": "make!(Made);\n",
		}, []snapshot.Link{modLink("src/lib.rs", 1, "src/a.rs"), modLink("src/lib.rs", 2, "src/b.rs"), modLink("src/lib.rs", 3, "src/c.rs"),
			link("src/lib.rs", 4, "src/a.rs"), link("src/lib.rs", 5, "src/b.rs"), link("src/lib.rs", 6, "src/c.rs")}, 1, 0},
		{"a path attribute names the module's file", map[string]string{
			"src/lib.rs": "#[path = \"other/x.rs\"]\nmod y;\nuse y::Z;\n", "src/other/x.rs": "pub struct Z;\n",
		}, []snapshot.Link{modLink("src/lib.rs", 2, "src/other/x.rs"), link("src/lib.rs", 3, "src/other/x.rs")}, 0, 0},
		{"a module that mounts itself ends the walk", map[string]string{
			"src/lib.rs": "mod a;\nuse a::again::again::again::X;\n", "src/a.rs": "#[path = \"a.rs\"]\nmod again;\n",
		}, []snapshot.Link{modLink("src/lib.rs", 1, "src/a.rs")}, 0, 1},
		{"a path attribute on a module in braces names its folder", map[string]string{
			"src/lib.rs": "#[path = \"thing\"]\nmod inner {\n    mod deep;\n}\n", "src/thing/deep.rs": "",
		}, []snapshot.Link{modLink("src/lib.rs", 3, "src/thing/deep.rs")}, 0, 0},
		{"an empty path attribute keeps the modules in braces in the file's folder", map[string]string{
			"src/lib.rs": "mod rules;\nuse rules::lints::dot::Dot;\n", "src/rules/mod.rs": "#[path = \"\"]\nmod lints {\n    pub mod dot;\n}\n",
			"src/rules/dot.rs": "pub struct Dot;\n",
		}, []snapshot.Link{modLink("src/lib.rs", 1, "src/rules/mod.rs"), link("src/lib.rs", 2, "src/rules/dot.rs"),
			modLink("src/rules/mod.rs", 3, "src/rules/dot.rs")}, 0, 0},
		{"an extern crate alias and a raw identifier", map[string]string{
			"src/lib.rs":  "pub extern crate alloc as heap;\nmod r#enum;\nuse heap::Vec;\nuse r#enum::E;\nuse crate::heap::Box;\n",
			"src/enum.rs": "pub enum E {}\n",
		}, []snapshot.Link{modLink("src/lib.rs", 2, "src/enum.rs"), link("src/lib.rs", 4, "src/enum.rs")}, 2, 0},
		{"the functions and statics of an extern block", map[string]string{
			"src/lib.rs": "mod ffi;\nuse ffi::foo;\nuse ffi::BAR;\n", "src/ffi.rs": "extern \"C\" {\n    pub fn foo();\n    pub static BAR: i32;\n}\n",
		}, []snapshot.Link{modLink("src/lib.rs", 1, "src/ffi.rs"), link("src/lib.rs", 2, "src/ffi.rs"), link("src/lib.rs", 3, "src/ffi.rs")}, 0, 0},
		{"items named by raw identifiers", map[string]string{
			"src/lib.rs": "mod a;\nuse a::r#match;\nuse a::r#type;\n", "src/a.rs": "pub fn r#match() {}\npub use std::r#type;\n",
		}, []snapshot.Link{modLink("src/lib.rs", 1, "src/a.rs"), link("src/lib.rs", 2, "src/a.rs"), link("src/lib.rs", 3, "src/a.rs")}, 1, 0},
		{"a module in braces inside cfg_if, whose items nobody read", map[string]string{
			"src/lib.rs":      "cfg_if! {\n    if #[cfg(unix)] {\n        mod imp {\n            mod util;\n            use self::util::f;\n            use self::Q;\n        }\n    }\n}\n",
			"src/imp/util.rs": "pub fn f() {}\n",
		}, []snapshot.Link{modLink("src/lib.rs", 4, "src/imp/util.rs"), link("src/lib.rs", 5, "src/imp/util.rs")}, 0, 0},
		{"a file only an include names is no crate root to judge", map[string]string{
			"src/lib.rs": "pub struct T;\ninclude!(\"gen.rs\");\n", "src/gen.rs": "use crate::T as U;\nuse crate::Missing;\n",
		}, nil, 0, 0},
		{"a 2015 extern crate alias is outside", map[string]string{
			"Cargo.toml": "[package]\nname = \"app\"\n", "src/lib.rs": "extern crate foo as bar;\nmod a;\n", "src/a.rs": "use bar::X;\n",
		}, []snapshot.Link{modLink("src/lib.rs", 2, "src/a.rs")}, 2, 0},
		{"a module in braces inside a macro body", map[string]string{
			"src/lib.rs": "m! {\n    mod inner {\n        mod deep;\n    }\n}\n", "src/inner/deep.rs": "",
		}, []snapshot.Link{modLink("src/lib.rs", 3, "src/inner/deep.rs")}, 0, 0},
	} {
		got := linksOf(t, tc.files)
		if !reflect.DeepEqual(got.List, tc.want) || got.Languages[0].Outside != tc.outside || got.Languages[0].NotFound != tc.notFound {
			t.Errorf("%s: List = %+v, outside %d, not found %d; want %+v, %d, %d", tc.name, got.List, got.Languages[0].Outside,
				got.Languages[0].NotFound, tc.want, tc.outside, tc.notFound)
		}
	}
}

// Two platform tables that give one key different crates: the import names both, the same way every time.
func TestBuildReadsATargetDependencyOfTwoPlatformsAsAmbiguous(t *testing.T) {
	files := map[string]string{
		"Cargo.toml": "[package]\nname = \"app\"\nedition = \"2021\"\n\n[target.'cfg(unix)'.dependencies]\nimp = { path = \"u\", package = \"u\" }\n\n" +
			"[target.'cfg(windows)'.dependencies]\nimp = { path = \"w\", package = \"w\" }\n",
		"u/Cargo.toml": "[package]\nname = \"u\"\n", "u/src/lib.rs": "pub fn f() {}\n",
		"w/Cargo.toml": "[package]\nname = \"w\"\n", "w/src/lib.rs": "pub fn f() {}\n",
		"src/lib.rs": "use imp::f;\n",
	}
	first := linksOf(t, files)
	if first.Languages[0].Ambiguous != 1 {
		t.Errorf("use imp::f with two platform crates: %+v, want it ambiguous", first.Languages[0])
	}
	for range 10 {
		if again := linksOf(t, files); !reflect.DeepEqual(again, first) {
			t.Fatalf("the same tree read twice: %+v, then %+v", first, again)
		}
	}
}

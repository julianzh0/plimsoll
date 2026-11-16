package snapshot_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/juzhxng/plimsoll/internal/conventions"
	"github.com/juzhxng/plimsoll/internal/snapshot"
	"github.com/juzhxng/plimsoll/internal/snapshot/snapshottest"
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
	check("the sample", snapshot.Build(snapshottest.Load(t, "testdata/rust"), conventions.Default(), snapshottest.Readings(t)).Now.Links,
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
		"Cargo.toml": "[package]\nname = \"tool\"\n", "src/main.rs": "use tool::x;\nmod cli;\n", "src/cli.rs": "use crate::run;\n",
	}), entry(2, 1, 0, 0, snapshot.PrefixRead), []snapshot.Link{link("src/cli.rs", 1, "src/main.rs"), link("src/main.rs", 2, "src/cli.rs")}, nil)

	// A module that two crate roots declare: its crate paths agree, its parent does not. A file of one mod line
	// only passes imports on.
	check("a module of two tests", linksOf(t, map[string]string{
		"tests/a.rs": "mod common;\n", "tests/b.rs": "mod common;\n", "tests/common/mod.rs": "use crate::common::x;\nuse super::y;\n",
	}), entry(3, 0, 1, 0, snapshot.PrefixAbsent), []snapshot.Link{link("tests/a.rs", 1, common), link("tests/b.rs", 1, common)},
		[]string{"tests/a.rs", "tests/b.rs"})

	// A package table without a name names no crate; the links by crate, self and super need none.
	check("a package without a name", linksOf(t, map[string]string{
		"Cargo.toml": "[package]\nversion = \"1\"\n", "src/lib.rs": "mod a;\n", "src/a.rs": "use crate::X;\n",
	}), entry(2, 0, 0, 0, snapshot.PrefixAbsent), []snapshot.Link{link("src/a.rs", 1, "src/lib.rs"), link("src/lib.rs", 1, "src/a.rs")},
		[]string{"src/lib.rs"})

	// A comment inside a group, holding a comma, is no part of a path; a character of several bytes before a macro
	// body moves no line.
	group := map[string]string{"src/c/mod.rs": "mod d;\nmod e;\n", "src/c/d.rs": "", "src/c/e.rs": "", "src/m.rs": ""}
	wantGroup := []snapshot.Link{link("src/c/mod.rs", 1, "src/c/d.rs"), link("src/c/mod.rs", 2, "src/c/e.rs"),
		link(lib, 1, "src/c/mod.rs"), link(lib, 2, "src/c/d.rs"), link(lib, 2, "src/c/e.rs"), link(lib, 6, "src/m.rs")}
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

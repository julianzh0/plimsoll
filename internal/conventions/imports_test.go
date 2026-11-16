package conventions

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

func language(t *testing.T, name string) Language {
	t.Helper()
	for _, l := range Default().Languages {
		if l.Name == name {
			return l
		}
	}
	t.Fatalf("no language entry %q", name)
	return Language{}
}

// found applies every form of an entry to a text and returns "spec" or "spec:type" per match, sorted.
func found(imports Imports, text string) []string {
	var all []string
	for _, form := range imports.Statements {
		spec, kind, everywhere := form.Pattern.SubexpIndex("spec"), form.Pattern.SubexpIndex("type"), form.Pattern.SubexpIndex("everywhere")
		for _, m := range form.Pattern.FindAllStringSubmatchIndex(text, -1) {
			item := text[m[2*spec]:m[2*spec+1]]
			if kind >= 0 && m[2*kind] >= 0 {
				item += ":type"
			}
			if everywhere >= 0 && m[2*everywhere] >= 0 {
				item += ":everywhere"
			}
			all = append(all, item)
		}
	}
	slices.Sort(all)
	return all
}

func TestEveryLanguageNamesItsImportForms(t *testing.T) {
	for _, l := range Default().Languages {
		imports := l.Imports
		if len(imports.Statements) == 0 || imports.Prefix.File == "" && len(imports.Prefix.Roots) == 0 && imports.PackageLine == nil {
			t.Errorf("%s: Imports = %+v, want import forms and a prefix file, roots or a package declaration", l.Name, imports)
		}
		for _, form := range imports.Statements {
			if form.Pattern.SubexpIndex("spec") < 0 {
				t.Errorf("%s: form %s has no group spec", l.Name, form.Pattern)
			}
		}
		switch imports.Target {
		case FileTarget:
			if imports.FolderExcept != nil || imports.FolderAlso != nil {
				t.Errorf("%s: a file target takes neither FolderExcept nor FolderAlso", l.Name)
			}
		case FolderTarget:
		default:
			t.Errorf("%s: Target = %v, want a file or a folder target", l.Name, imports.Target)
		}
	}
}

// importCases are, per language, texts and the imports their statement forms find in them.
func importCases() map[string]map[string][]string {
	return map[string]map[string][]string{
		"typescript": {
			"import {\n  a,\n  b,\n} from \"./x.ts\";\n":                {"./x.ts"},
			"import type { A } from './a.ts'\n":                         {"./a.ts:type"},
			"export enum E { A, B }\nimport z from \"./q\"\n":           {"./q"},
			"export { helper }\nimport { z } from \"./z\"\n":            {"./z"},
			"export default handler\n// was: import x from \"./old\"\n": nil,
			"export * from './a'\nexport * as ns from \"./b\";\nexport { x, y as z } from './c'\nexport type { T } from './d'\n": {
				"./a", "./b", "./c", "./d:type",
			},
			"import \"./setup.ts\";\n": {"./setup.ts"},
			"const a = await import(\"./lazy.ts\");\nconst b = require('./old.js')\n": {"./lazy.ts", "./old.js"},
			"x.import(\"./no\")\n":                                             nil,
			"export const mode = \"restrict\"\n":                               nil,
			"import {\n  a, // don't drop\n  /* b */ c,\n} from \"./m.ts\";\n": {"./m.ts"},
			"export {\n  a, // it's kept\n} from './n'\n":                      {"./n"},
			"import a from './a'\n// import b from './b'\n":                    {"./a"},
			"import from from \"./from\"\n":                                    {"./from"},
		},
		"go": {
			"package a\n\nimport \"fmt\"\nimport x \"os\"\n": {"fmt", "os"},
			"package a\n\nimport (\n\t\"fmt\"\n\t_ \"embed\"\n\t. \"math\"\n\t// \"commented\"\n\tk \"example.com/m/x\"\n)\n": {
				"embed", "example.com/m/x", "fmt", "math",
			},
			"package a\n\nimport `raw/path`\n": {"raw/path"},
		},
		"python": {
			"import a.b as c\n":                  {"a.b"},
			"import a, b\n":                      {"a"},
			"import a; import b\n":               {"a"},
			"import a.b  # why\n":                {"a.b"},
			"    import a\n\tfrom .x import y\n": {".x", "a"},
			"from a.b import (\n    c,  # (kept)\n    d as e,\n)\n":         {"a.b"},
			"from a import b, \\\n    c\n":                                  {"a"},
			"from . import x\nfrom .. import y\nfrom ...pkg.mod import z\n": {".", "..", "...pkg.mod"},
			"from a import *\n":                                           {"a"},
			"lazy import a\nlazy from b import c\n":                       {"a", "b"},
			"\"\"\"Docs.\n\nimport of the module is deferred.\n\"\"\"\n":  nil,
			"x = 1  # import a\n# from a import b\n":                      nil,
			"m = importlib.import_module(\"a.b\")\nn = __import__('c')\n": {"a.b", "c"},
		},
		"dart": {
			"import 'package:app/a.dart';\n":                        {"package:app/a.dart"},
			"import \"b.dart\" as b show c;\nexport '../c.dart';\n": {"../c.dart", "b.dart"},
			"part 'a.g.dart';\npart of 'lib.dart';\n":               {"a.g.dart", "lib.dart"},
			"part of my.library;\n":                                 nil,
			"import 'dart:io';\n":                                   {"dart:io"},
			"// import 'x.dart';\n":                                 nil,
			"import 'a.dart' if (dart.library.io) 'b.dart';\n":      {"a.dart"},
			"import 'package:a/a.dart'\n    deferred as a;\n":       {"package:a/a.dart"},
		},
		"java": {
			"package a;\n\nimport com.acme.billing.Invoice;\nimport static com.acme.billing.Invoice.MAX;\nimport com.acme.billing.*;\n": {
				"com.acme.billing.*", "com.acme.billing.Invoice", "com.acme.billing.Invoice.MAX",
			},
			"// import a.b.C;\n/**\n * import a.b.C;\n */\nString s = \"import a.b.C;\";\nimportant.call();\n": nil,
			"import a.b.C;\r\nimport a.b.D;\r\n": {"a.b.C", "a.b.D"},
		},
		"kotlin": {
			"import com.acme.a.B as C\nimport com.acme.a.load\nimport com.acme.a.*\n": {"com.acme.a.*", "com.acme.a.B", "com.acme.a.load"},
			"import `in`.acme.a.B\n":               {"`in`.acme.a.B"},
			"// import a.b.C\nval important = 1\n": nil,
		},
		"rust": {
			"mod a;\npub mod b;\npub(crate) mod c;\n    mod d;\n":                     {"mod a", "mod b", "mod c", "mod d"},
			"#[cfg(test)]\nmod tests;\n#[macro_use] mod macros;\n":                    {"mod macros", "mod tests"},
			"mod inline {\n    mod deep;\n}\n":                                        {"mod deep"},
			"use crate::a::B;\npub use self::c::D as E;\npub(crate) use super::f;\n":  {"crate::a::B", "self::c::D as E", "super::f"},
			"use crate::{\n    a::B,\n    c::{self, D},\n};\n":                        {"crate::{\n    a::B,\n    c::{self, D},\n}"},
			"    use std::fmt::Write;\n":                                              {"std::fmt::Write"},
			"extern crate alloc;\npub extern crate grep_cli as cli;\n":                {"alloc", "grep_cli"},
			"// use a::b;\n/// use a::b;\nlet used = 1;\nfn user() {}\nmodule!(x);\n": nil,
			"use a::b;\r\nmod c;\r\n":                                                 {"a::b", "mod c"},
		},
		"c":   includeCases(),
		"cpp": includeCases(),
		"swift": {
			"import Foundation\nimport struct Foo.Bar\n@testable import App\n@_exported import Kit\n@_implementationOnly import Hidden\n": {
				"App", "Foo", "Foundation", "Hidden", "Kit",
			},
			"import func Darwin.sqrt\nimport class UIKit.UIView\n@preconcurrency import Dispatch\n":     {"Darwin", "Dispatch", "UIKit"},
			"public import Kit\ninternal import Core\n#if canImport(UIKit)\n    import UIKit\n#endif\n": {"Core", "Kit", "UIKit"},
			"// import A\nlet important = 1\nfunc imported() {}\n":                                      nil,
			"import A\r\nimport B\r\n": {"A", "B"},
		},
		"csharp": {
			"using System;\nusing Acme.Billing;\nglobal using Acme.Core;\n": {"Acme.Billing", "Acme.Core:everywhere", "System"},
			"using static Acme.Billing.Invoice;\nusing Money = Acme.Billing.Money;\nglobal using static Acme.Core.Guard;\n": {
				"Acme.Billing", "Acme.Billing", "Acme.Core:everywhere",
			},
			"using global::Autofac;\nusing Map = System.Collections.Generic.Dictionary<string, int>;\n":                               {"Autofac", "System.Collections.Generic"},
			"namespace A\n{\n    using B.C;\n}\n":                                                                                     {"B.C"},
			"        using var scope = factory.CreateScope();\n        using (var x = Open())\n        await using var y = Make();\n": nil,
			"// using A.B;\nvar s = \"using A.B;\";\nusing static PipelineTests;\nusing P = (int X, int Y);\n":                        nil,
			"using A.B;\r\nusing C.D;\r\n": {"A.B", "C.D"},
		},
	}
}

func TestImportFormsFindEachStatement(t *testing.T) {
	cases := importCases()
	for _, l := range Default().Languages {
		texts, ok := cases[l.Name]
		if !ok {
			t.Errorf("no import cases for language %s", l.Name)
			continue
		}
		for text, want := range texts {
			if got := found(l.Imports, text); !slices.Equal(got, want) {
				t.Errorf("%s: imports of %q = %q, want %q", l.Name, text, got, want)
			}
		}
	}
}

func TestComputedAndWholeFilePatterns(t *testing.T) {
	for name, texts := range computedCases() {
		computed := language(t, name).Imports.Computed
		for text, want := range texts {
			if got := len(computed.Pattern.FindAllStringIndex(text, -1)); got != want {
				t.Errorf("%s Computed in %q: %d matches, want %d", name, text, got, want)
			}
		}
	}
	for name, texts := range passesOnCases() {
		passesOn := language(t, name).Imports.PassesOn
		for text, want := range texts {
			if got := passesOn.MatchString(text); got != want {
				t.Errorf("%s PassesOn(%q) = %v, want %v", name, text, got, want)
			}
		}
	}
	golang := language(t, "go").Imports
	text := "package a\n\nimport \"fmt\"\n\nfunc f() {}\n"
	if at := golang.Until.FindStringIndex(text); at == nil || !strings.HasPrefix(text[at[0]:], "func") {
		t.Errorf("go Until in %q = %v, want the start of func", text, at)
	}
	if golang.Until.MatchString("package a\n\nimport \"fmt\"\n") {
		t.Errorf("go Until matches a file without a declaration")
	}
	if golang.Computed.Pattern != nil || golang.PassesOn != nil || golang.NoRings != "the Go compiler rejects import cycles" {
		t.Errorf("go: Computed %v, PassesOn %v, NoRings %q", golang.Computed, golang.PassesOn, golang.NoRings)
	}
}

// passesOnCases are, per language with a PassesOn pattern, whole file contents and whether they only pass imports on.
func passesOnCases() map[string]map[string]bool {
	return map[string]map[string]bool{
		"typescript": {
			"export * from './a'\nexport { b } from \"./b\";\n":            true,
			"// barrel\n/* all\n of it */\nexport type { T } from './t'\n": true,
			"export * from './a'\nexport const x = 1\n":                    false,
			"import { a } from './a'\nexport { a }\n":                      false,
		},
		"python": {
			"\"\"\"Doc.\"\"\"\n\nfrom .a import A\nfrom .b import (\n    B,\n)\n\n__all__ = [\"A\", \"B\"]\n": true,
			"# only this\nfrom __future__ import annotations\nfrom .a import A\n":                             true,
			"from .a import A\n\ndef f(): ...\n":                                                              false,
			"from .a import A\nVERSION = \"1\"\n":                                                             false,
			"from .a import A; import os\n":                                                                   false,
		},
		"dart": {
			"library app;\n\nexport 'src/a.dart';\nexport \"src/b.dart\" show B;\n": true,
			"/// Barrel.\n/* all */\nexport 'a.dart' hide x;\n":                     true,
			"export 'a.dart';\nconst x = 1;\n":                                      false,
			"import 'a.dart';\nexport 'b.dart';\n":                                  false,
		},
		"rust": {
			"#![deny(missing_docs)]\n\nmod a;\npub mod b;\n\npub use a::A;\npub use crate::b::{B, C};\npub extern crate x as y;\n": true,
			"#[cfg(test)]\nmod tests;\nmod a;\n": true,
			"mod a;\n\npub fn f() {}\n":          false,
			"mod a;\nuse a::A;\n":                false,
			"mod a {\n}\n":                       false,
		},
	}
}

// computedCases are, per language with a Computed form, texts and how many imports with a target built at run time
// they hold.
func computedCases() map[string]map[string]int {
	return map[string]map[string]int{
		"typescript": {
			"await import(name)": 1, "import(`./x/${a}.ts`)": 1, "require(\"./a\" + b)": 1,
			"import(\"./a.ts\")": 0, "require('x', y)": 0, "obj.require(x)": 0,
		},
		"c":   includeComputedCases(),
		"cpp": includeComputedCases(),
		"python": {
			"importlib.import_module(name)": 1, "import_module(f\"a.{x}\")": 1, "__import__(\"a.\" + x)": 1,
			"import_module(\"a\" if c else \"b\")": 1, "import_module(\".x\", __package__)": 1,
			"importlib.import_module(\"a.b\")": 0, "__import__('a', b)": 0,
		},
	}
}

func TestPythonTypeOnlyBlock(t *testing.T) {
	block := language(t, "python").Imports.TypeOnlyBlock.Pattern
	for text, before := range map[string]string{
		"if TYPE_CHECKING:\n    from a import b\n\n    import c\nx = 1\n":       "x = 1",
		"if typing.TYPE_CHECKING:  # note\n    import a\nelse:\n    import b\n": "else:",
		"if TYPE_CHECKING:\r\n    import a":                                     "",
		"if not TYPE_CHECKING:\n    import a\n":                                 "-",
		"    if TYPE_CHECKING:\n        import a\n":                             "-",
	} {
		spans := block.FindAllStringIndex(text, -1)
		switch {
		case before == "-" && len(spans) != 0:
			t.Errorf("TypeOnlyBlock in %q = %v, want none", text, spans)
		case before != "-" && (len(spans) != 1 || spans[0][0] != 0 || text[spans[0][1]:] != text[strings.LastIndex(text, before):]):
			t.Errorf("TypeOnlyBlock in %q = %v, want one block ending before %q", text, spans, before)
		}
	}
}

func TestAFormMatchesOnlyTextThatHoldsOneOfItsWords(t *testing.T) {
	for _, l := range Default().Languages {
		forms := append(slices.Clone(l.Imports.Statements), l.Imports.Computed)
		texts := slices.Collect(maps.Keys(importCases()[l.Name]))
		texts = append(texts, slices.Collect(maps.Keys(computedCases()[l.Name]))...)
		worded := 0
		for _, form := range forms {
			if form.Pattern == nil || len(form.Words) == 0 {
				continue
			}
			worded++
			for _, text := range texts {
				holds := slices.ContainsFunc(form.Words, func(w string) bool { return strings.Contains(text, w) })
				if form.Pattern.MatchString(text) && !holds {
					t.Errorf("%s: %s matches %q, which holds none of its words %q", l.Name, form.Pattern, text, form.Words)
				}
			}
		}
		if want := map[string]int{"typescript": 2, "go": 0, "python": 2, "dart": 0, "java": 0, "kotlin": 0, "rust": 0, "csharp": 0, "swift": 0, "c": 0, "cpp": 0}[l.Name]; worded != want {
			t.Errorf("%s: %d forms with words, want %d", l.Name, worded, want)
		}
	}
}

func TestLiteralsBlankCommentsStringsAndCharacters(t *testing.T) {
	cases := map[string]map[string]string{ // per language with a Literals pattern: a text, and what stays of it
		"rust": {
			"a // b\nc":                    "a     \nc",
			"a /* b /* c */ d */ e":        "a                   e",
			"a /* src/**/x */ b":           "a                b",
			`s = "a \" } b"; t`:            "s =           ; t",
			"s = r#\"a \"} b\"#; t":        "s =            ; t",
			`c = '}'; d = '\''; e = b'"';`: "c =    ; d =     ; e = b   ;",
			"fn f<'a>(x: &'a str) {}":      "fn f<'a>(x: &'a str) {}",
			"s = \"line\n}\nend\"; t":      "s =      \n \n    ; t",
			"/**/ a /***/ b":               "     a       b",
		},
		"c":   commentCases(),
		"cpp": commentCases(),
		"swift": {
			"a // b\nc":                       "a     \nc",
			"a /* b */ c":                     "a         c",
			"s = \"\"\"\nimport X\n\"\"\"\nt": "s =    \n        \n   \nt",
			`s = "a \" b"; t`:                 "s =         ; t",
			`s = #"a "b" c"#; t`:              "s =            ; t",
		},
	}
	blank := func(match string) string {
		return strings.Map(func(r rune) rune {
			if r == '\n' {
				return r
			}
			return ' '
		}, match)
	}
	for _, l := range Default().Languages {
		texts, has := cases[l.Name]
		if (l.Literals != nil) != has {
			t.Errorf("%s: a Literals pattern %v, cases %v", l.Name, l.Literals != nil, has)
			continue
		}
		for text, want := range texts {
			if got := l.Literals.ReplaceAllStringFunc(text, blank); got != want {
				t.Errorf("%s Literals in %q leaves %q, want %q", l.Name, text, got, want)
			}
		}
	}
}

// includeCases are the import cases of the C and C++ entries, which read includes alike.
func includeCases() map[string][]string {
	return map[string][]string{
		"#include \"store.h\"\n#include <stdio.h>\n#include \"notes/note.h\"\n":                                                                  {"<stdio.h>", "notes/note.h", "store.h"},
		"  #  include \"a.h\"\n#include<b.h>\n#include\"c.h\"\n#\tinclude <sys/types.h> /* why */\n":                                             {"<b.h>", "<sys/types.h>", "a.h", "c.h"},
		"#if defined(_WIN32)\n#  include <windows.h>\n#else\n#  include \"posix.h\"\n#endif\n":                                                   {"<windows.h>", "posix.h"},
		"// #include \"a.h\"\nconst char *s = \"#include <b.h>\";\n#include_next <c.h>\n#import \"d.h\"\n#include CONFIG_H\n#define X \"e.h\"\n": nil,
		"#include \"a.h\"\r\n#include <b.h>\r\n":                                                                                                 {"<b.h>", "a.h"},
	}
}

// includeComputedCases are texts and the number of includes in them that a macro names.
func includeComputedCases() map[string]int {
	return map[string]int{
		"#include CONFIG_H": 1, "  #  include FT_FREETYPE_H\n#include BOOST_PP_ITERATE()": 2,
		"#include \"a.h\"": 0, "#include <a.h>": 0, "#include_next <a.h>": 0, "#includes": 0, "// #include X": 0,
	}
}

// commentCases are texts and what stays of them once their comments are blanked.
func commentCases() map[string]string {
	return map[string]string{
		"a // b\nc": "a     \nc",
		"/*\n#include \"x.h\"\n*/\n#include \"y.h\"": "  \n              \n  \n#include \"y.h\"",
		"#include <a.h> /* b */ // c":                "#include <a.h>             ",
		"x = 1; /* a // b */ y = 2; // z":            "x = 1;              y = 2;     ",
		"/**/ a /***/ b":                             "     a       b",
	}
}

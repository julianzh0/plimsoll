package conventions

import (
	"slices"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/parse"
	"github.com/julianzh0/plimsoll/internal/parse/parsetest"
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
		spec, everywhere := form.Pattern.SubexpIndex("spec"), form.Pattern.SubexpIndex("everywhere")
		for _, m := range form.Pattern.FindAllStringSubmatchIndex(text, -1) {
			item := text[m[2*spec]:m[2*spec+1]]
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
		if len(imports.Statements) == 0 && l.Grammar == "" || imports.Prefix.File == "" && len(imports.Prefix.Roots) == 0 && imports.PackageLine == nil {
			t.Errorf("%s: Imports = %+v, want import forms or a grammar, and a prefix file, roots or a package declaration", l.Name, imports)
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
			"const s = \"import x from './old'\"\n":                            nil,
			"import x from './a\\b'\n":                                         {"./a\\b"},
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
			"import a, b\n":                      {"a", "b"},
			"import a; import b\n":               {"a", "b"},
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
			"import module java.base;\n": nil,
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
			"let s = \"\"\"\nimport X\n\"\"\"\n": nil,
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

// targets renders the import statements an entry's query finds in a text: one item per match that holds a target,
// written as the decoder reads it, with ":type" and ":everywhere" when the match marks them.
func targets(t *testing.T, l Language, fileName, text string) []string {
	t.Helper()
	result, err := parsetest.Parser(t).Read(l.GrammarOf(fileName), l.Query, []byte(text))
	if err != nil {
		t.Fatalf("%s: Read(%q) error = %v", l.Name, text, err)
	}
	var all []string
	for _, match := range result.Matches {
		var specs []parse.Capture
		prefix, suffix := "", ""
		for _, c := range match {
			switch c.Name {
			case "spec", "spec.mod":
				specs = append(specs, c)
				if c.Name == "spec.mod" {
					prefix = "mod "
				}
			case "type":
				suffix += ":type"
			case "everywhere":
				suffix += ":everywhere"
			}
		}
		if len(specs) == 0 {
			continue
		}
		item := text[specs[0].Start:specs[len(specs)-1].End]
		if len(specs) > 1 {
			item = strings.Join(strings.Fields(item), "")
		}
		if len(item) >= 2 && strings.ContainsRune("\"'`", rune(item[0])) && item[len(item)-1] == item[0] {
			item = item[1 : len(item)-1]
		}
		all = append(all, prefix+item+suffix)
	}
	slices.Sort(all)
	return all
}

func TestEveryEntryQueryCompiles(t *testing.T) {
	for _, l := range Default().Languages {
		if l.Grammar == "" {
			continue
		}
		grammars := []string{l.Grammar}
		for _, g := range l.GrammarBySuffix {
			grammars = append(grammars, g)
		}
		for _, g := range grammars {
			if !slices.Contains(parse.Grammars(), g) {
				t.Errorf("%s names the grammar %q, which is not embedded", l.Name, g)
				continue
			}
			if _, err := parsetest.Parser(t).Read(g, l.Query, nil); err != nil {
				t.Errorf("%s: its query on the %s grammar: %v", l.Name, g, err)
			}
		}
	}
}

func TestNoEntryReadsAFactBothWays(t *testing.T) {
	for _, l := range Default().Languages {
		if l.Grammar == "" {
			continue
		}
		// Import patterns beside a query read only the stretches the query marks @text, which the grammar keeps as
		// tokens, or the files the grammar cannot read without an error.
		if len(l.Imports.Statements) > 0 && strings.Contains(l.Query, "@spec") && !strings.Contains(l.Query, "@text") && !l.LineReadWhenBroken {
			t.Errorf("%s reads its imports by patterns and by its query", l.Name)
		}
	}
}

func TestJavaScriptFilesAreReadByTheTSXGrammar(t *testing.T) {
	l := language(t, "typescript")
	cases := []struct{ file, text string }{
		{"a.js", "export const A = () => <div/>;\nimport b from './b'\n"},
		{"a.cjs", "const c = require('./b')\n"},
		{"a.mjs", "const c = require('./b')\n"},
	}
	for _, c := range cases {
		if got := targets(t, l, c.file, c.text); !slices.Equal(got, []string{"./b"}) {
			t.Errorf("imports of %s %q = %q, want [./b]", c.file, c.text, got)
		}
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
			got := found(l.Imports, text)
			if l.Grammar != "" {
				got = targets(t, l, "a"+l.Extensions[0], text)
			}
			if !slices.Equal(got, want) {
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
	golang := language(t, "go").Imports
	if golang.Computed.Pattern != nil || golang.NoRings != "the Go compiler rejects import cycles" {
		t.Errorf("go: Computed %v, NoRings %q", golang.Computed, golang.NoRings)
	}
}

// computedCases are, per language with a Computed form, texts and how many imports with a target built at run time
// they hold.
func computedCases() map[string]map[string]int {
	return map[string]map[string]int{
		"c":   includeComputedCases(),
		"cpp": includeComputedCases(),
	}
}

func TestLiteralsBlankCommentsAndKeepStrings(t *testing.T) {
	cases := map[string]map[string]string{ // per language with a Literals pattern: a text, and what stays of it
		"c":   commentCases(),
		"cpp": commentCases(),
	}
	blank := func(match string) string {
		if match[0] == '"' || match[0] == '\'' {
			return match
		}
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
		"s = \"/*\"; c = '\"'; x /* y */":            "s = \"/*\"; c = '\"'; x        ",
	}
}

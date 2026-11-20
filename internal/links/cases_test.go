package links

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

const goModule = "example.com/m"

// languageCase is how one language names its files and how one file imports another; a language without a case
// fails the test (N-10).
type languageCase struct {
	setup     map[string]string        // files every tree needs
	product   func(name string) string // the path of product file name
	test      func(name string) string // the path of the test file of name
	support   func(name string) string // the path of test support name
	testBody  string                   // a line that defines a test
	outside   []string                 // three imports of outside packages, as written
	nowhere   string                   // an own import that reaches no tracked file, as written
	importing func(from string, targets []string, specs ...string) string
	placed    func(folder, name string) string // the path of product file name in a folder below under
	under     string                           // the folder that placed files lie below
	// Set by eachLanguage from the entry:
	byFolder   bool   // its imports name folders
	sourceSets bool   // its packages lie under source roots
	noRings    string // why no rings are looked for among its files; "" looks for them
	notSeen    string // the entry's sentence on use without an import statement; "" for none
}

func languageCases() map[string]languageCase {
	return map[string]languageCase{
		"typescript": {
			setup:    map[string]string{},
			product:  func(name string) string { return "src/" + name + ".ts" },
			placed:   func(folder, name string) string { return "src/" + folder + "/" + name + ".ts" },
			under:    "src",
			test:     func(name string) string { return "tests/" + name + ".test.ts" },
			support:  func(name string) string { return "tests/support/" + name + ".ts" },
			testBody: `test("x", () => {});`,
			outside:  []string{"node:fs", "node:path", "lodash"},
			nowhere:  "./gone.ts",
			importing: func(from string, targets []string, specs ...string) string {
				var lines []string
				for _, target := range targets {
					relative, _ := filepath.Rel(path.Dir(from), target)
					relative = filepath.ToSlash(relative)
					if !strings.HasPrefix(relative, ".") {
						relative = "./" + relative
					}
					lines = append(lines, `import "`+relative+`";`)
				}
				for _, spec := range specs {
					lines = append(lines, `import "`+spec+`";`)
				}
				return strings.Join(lines, "\n") + "\n"
			},
		},
		"python": {
			// A package that stays on every side: without one, no import is the project's own.
			setup:    map[string]string{"src/app/__init__.py": ""},
			product:  func(name string) string { return "src/" + name + ".py" },
			placed:   func(folder, name string) string { return "src/" + folder + "/" + name + ".py" },
			under:    "src",
			test:     func(name string) string { return "tests/test_" + name + ".py" },
			support:  func(name string) string { return "tests/support/" + name + ".py" },
			testBody: "def test_x(): pass",
			outside:  []string{"os", "json", "httpx"},
			nowhere:  "a.gone",
			importing: func(_ string, targets []string, specs ...string) string {
				var lines []string
				for _, target := range targets {
					module := strings.TrimPrefix(strings.TrimSuffix(target, ".py"), "src/")
					lines = append(lines, "import "+strings.ReplaceAll(module, "/", "."))
				}
				for _, spec := range specs {
					lines = append(lines, "import "+spec)
				}
				return strings.Join(lines, "\n") + "\n"
			},
		},
		"dart": {
			setup:    map[string]string{"pubspec.yaml": "name: app\n"},
			product:  func(name string) string { return "lib/src/" + name + ".dart" },
			placed:   func(folder, name string) string { return "lib/" + folder + "/" + name + ".dart" },
			under:    "lib",
			test:     func(name string) string { return "test/" + name + "_test.dart" },
			support:  func(name string) string { return "test/support/" + name + ".dart" },
			testBody: "test('x', () {});",
			outside:  []string{"dart:io", "package:flutter/material.dart", "package:http/http.dart"},
			nowhere:  "package:app/gone.dart",
			importing: func(from string, targets []string, specs ...string) string {
				var lines []string
				for _, target := range targets {
					if rest, inLib := strings.CutPrefix(target, "lib/"); inLib {
						lines = append(lines, "import 'package:app/"+rest+"';")
						continue
					}
					relative, _ := filepath.Rel(path.Dir(from), target)
					lines = append(lines, "import '"+filepath.ToSlash(relative)+"';")
				}
				for _, spec := range specs {
					lines = append(lines, "import '"+spec+"';")
				}
				return strings.Join(lines, "\n") + "\n"
			},
		},
		"java":   jvmCase("java", ".java", ";", "@Test void x() {}"),
		"kotlin": jvmCase("kotlin", ".kt", "", "@Test fun x() {}"),
		"go": {
			setup:    map[string]string{"go.mod": "module " + goModule + "\n"},
			product:  func(name string) string { return "internal/" + name + "/" + name + ".go" },
			placed:   func(folder, name string) string { return "internal/" + folder + "/" + name + "/" + name + ".go" },
			under:    "internal",
			test:     func(name string) string { return "internal/" + name + "/" + name + "_test.go" },
			support:  func(name string) string { return "internal/test/" + name + "/" + name + ".go" },
			testBody: "func TestB(t *testing.T) {}",
			outside:  []string{"fmt", "os", "strings"},
			nowhere:  goModule + "/internal/gone",
			importing: func(from string, targets []string, specs ...string) string {
				lines := []string{"package " + path.Base(path.Dir(from)), "", "import ("}
				for _, target := range targets {
					lines = append(lines, "\t\""+goModule+"/"+path.Dir(target)+"\"")
				}
				for _, spec := range specs {
					lines = append(lines, "\t\""+spec+"\"")
				}
				return strings.Join(append(lines, ")"), "\n") + "\n"
			},
		},
		"c":      includeCase(".h", "tests/test_%s.c"),
		"cpp":    includeCase(".hpp", "tests/%s_test.cc"),
		"csharp": csharpCase(),
		"swift":  swiftCase(),
		"rust": {
			// A library root that stays on every side: its crate's name leads from any file to a module by its path.
			setup:    map[string]string{"Cargo.toml": "[package]\nname = \"app\"\nedition = \"2021\"\n", "src/lib.rs": ""},
			product:  func(name string) string { return "src/" + name + ".rs" },
			placed:   func(folder, name string) string { return "src/" + folder + "/" + name + ".rs" },
			under:    "src",
			test:     func(name string) string { return "tests/" + name + ".rs" },
			support:  func(name string) string { return "src/tests/" + name + ".rs" },
			testBody: "#[test]\nfn x() {}",
			outside:  []string{"std::io", "std::fmt", "serde::Serialize"},
			nowhere:  "super::gone::Gone", // no mod line declares the importing file, so it has no module above it
			importing: func(_ string, targets []string, specs ...string) string {
				var lines []string
				for _, target := range targets {
					module := strings.TrimSuffix(strings.TrimPrefix(target, "src/"), ".rs")
					lines = append(lines, "use app::"+strings.ReplaceAll(module, "/", "::")+";")
				}
				for _, spec := range specs {
					lines = append(lines, "use "+spec+";")
				}
				return strings.Join(lines, "\n") + "\n"
			},
		},
	}
}

// jvmCase is the case of a language whose imports name packages under source roots. Java and Kotlin differ in
// the source folder, the suffix, the end of a statement and the line that defines a test. Every product file is
// its own package, as in the Go case; test support is a package named test in the main source set, which
// product code may import.
func jvmCase(source, suffix, end, testBody string) languageCase {
	main, tests := "src/main/"+source+"/com/acme/", "src/test/"+source+"/com/acme/"
	class := func(name string) string { return strings.ToUpper(name[:1]) + name[1:] + suffix }
	return languageCase{
		setup:   map[string]string{},
		product: func(name string) string { return main + name + "/" + class(name) },
		placed:  func(folder, name string) string { return main + folder + "/" + name + "/" + class(name) },
		under:   strings.TrimSuffix(main, "/"),
		test: func(name string) string {
			return tests + name + "/" + strings.TrimSuffix(class(name), suffix) + "Test" + suffix
		},
		support:  func(name string) string { return main + "test/" + name + "/" + class(name) },
		testBody: testBody,
		outside:  []string{"java.util.List", "java.io.File", "org.junit.Assert"},
		nowhere:  "com.acme.a.gone.Gone",
		importing: func(from string, targets []string, specs ...string) string {
			lines := []string{"package " + packageOf(from) + end}
			for _, target := range targets {
				lines = append(lines, "import "+packageOf(target)+"."+strings.TrimSuffix(path.Base(target), suffix)+end)
			}
			for _, spec := range specs {
				lines = append(lines, "import "+spec+end)
			}
			return strings.Join(lines, "\n") + "\n"
		},
	}
}

// csharpCase is the case of C#. Every product file declares its own namespace, named after its folder below
// src/, and a project file makes a name that begins with Acme the project's own. Test support is a folder named
// test inside the product project, which product code may import.
func csharpCase() languageCase {
	class := func(name string) string { return strings.ToUpper(name[:1]) + name[1:] }
	return languageCase{
		setup:    map[string]string{"src/Acme/Acme.csproj": "<Project Sdk=\"Microsoft.NET.Sdk\" />\n"},
		product:  func(name string) string { return "src/Acme/" + name + "/" + class(name) + ".cs" },
		placed:   func(folder, name string) string { return "src/Acme/" + folder + "/" + name + "/" + class(name) + ".cs" },
		under:    "src/Acme",
		test:     func(name string) string { return "tests/Acme.Tests/" + name + "/" + class(name) + "Tests.cs" },
		support:  func(name string) string { return "src/Acme/test/" + name + "/" + class(name) + ".cs" },
		testBody: "[Fact] public void X() {}",
		outside:  []string{"System", "System.Linq", "Xunit"},
		nowhere:  "Acme.gone",
		importing: func(from string, targets []string, specs ...string) string {
			var lines []string
			for _, target := range targets {
				lines = append(lines, "using "+namespaceOf(target)+";")
			}
			for _, spec := range specs {
				lines = append(lines, "using "+spec+";")
			}
			return strings.Join(append(lines, "namespace "+namespaceOf(from)+";"), "\n") + "\n"
		},
	}
}

// swiftCase is the case of Swift. Every product file is a module of its own under Sources/, a test file one under
// Tests/, and test support a module under Tests/ that product code may import. Package.swift declares a target for
// every name the tests use, and one, gone, without a file.
func swiftCase() languageCase {
	var targets []string
	declare := func(kind, name, folder string) {
		targets = append(targets, fmt.Sprintf("    .%s(name: %q, path: %q),", kind, name, folder))
	}
	names := []string{"old", "moved", "n1", "n2", "n3", "t2", "db", "clock"}
	for c := 'a'; c <= 'z'; c++ {
		names = append(names, string(c), "t"+string(c), "p"+string(c))
	}
	for _, name := range names {
		declare("target", name, "Sources/"+name)
		declare("testTarget", name+"Tests", "Tests/"+name+"Tests")
		declare("target", name+"Support", "Tests/"+name)
	}
	for _, folder := range []string{"a", "b", "c", "d", "a/deep", "adapters", "domain", "domain/t", "shared", "other", "fresh"} {
		for _, name := range []string{"n", "o", "s", "u", "v", "w", "x", "y", "z", "db", "http", "order", "policy"} {
			declare("target", strings.ReplaceAll(folder, "/", "_")+"_"+name, "Sources/"+folder+"/"+name)
		}
	}
	declare("target", "gone", "Sources/gone")
	module := func(file string) string {
		folder := path.Dir(file)
		if rest, test := strings.CutPrefix(folder, "Tests/"); test && !strings.HasSuffix(rest, "Tests") {
			return rest + "Support"
		}
		return strings.ReplaceAll(strings.TrimPrefix(strings.TrimPrefix(folder, "Sources/"), "Tests/"), "/", "_")
	}
	return languageCase{
		setup:    map[string]string{"Package.swift": "let package = Package(name: \"app\", targets: [\n" + strings.Join(targets, "\n") + "\n])\n"},
		product:  func(name string) string { return "Sources/" + name + "/" + name + ".swift" },
		placed:   func(folder, name string) string { return "Sources/" + folder + "/" + name + "/" + name + ".swift" },
		under:    "Sources",
		test:     func(name string) string { return "Tests/" + name + "Tests/" + name + "Tests.swift" },
		support:  func(name string) string { return "Tests/" + name + "/" + name + ".swift" },
		testBody: "func testX() {}",
		outside:  []string{"Foundation", "XCTest", "Dispatch"},
		nowhere:  "gone",
		importing: func(_ string, targets []string, specs ...string) string {
			var lines []string
			for _, target := range targets {
				lines = append(lines, "import "+module(target))
			}
			for _, spec := range specs {
				lines = append(lines, "import "+spec)
			}
			return strings.Join(append(lines, "let x = 1"), "\n") + "\n"
		},
	}
}

// namespaceOf is the namespace a C# file of the cases declares: its folder below src/, with dots.
func namespaceOf(file string) string {
	return strings.ReplaceAll(strings.TrimPrefix(path.Dir(file), "src/"), "/", ".")
}

// packageOf is the dotted package of a file under a source root; a file elsewhere gets its folder's name.
func packageOf(file string) string {
	folder := path.Dir(file) + "/"
	for _, source := range []string{"/java/", "/kotlin/"} {
		if _, below, found := strings.Cut(folder, source); found {
			return strings.ReplaceAll(strings.TrimSuffix(below, "/"), "/", ".")
		}
	}
	return path.Base(path.Dir(file))
}

// includeCase is the case of a language whose files include each other by path. A product file is a header, since a
// source file is a unit of its own that no file includes; a test file is a source file. An include of a target is
// written relative to the including file. gone.h is tracked in a folder that no include folder leads to, so an
// include of its path is the project's own and not found.
func includeCase(header, testFile string) languageCase {
	return languageCase{
		setup:    map[string]string{"extern/missing/gone.h": "#pragma once\n"},
		product:  func(name string) string { return "src/" + name + header },
		placed:   func(folder, name string) string { return "src/" + folder + "/" + name + header },
		under:    "src",
		test:     func(name string) string { return fmt.Sprintf(testFile, name) },
		support:  func(name string) string { return "tests/support/" + name + header },
		testBody: "TEST(Suite, X) {}",
		outside:  []string{"<stdio.h>", "<string.h>", "<zlib.h>"},
		nowhere:  `"missing/gone.h"`,
		importing: func(from string, targets []string, specs ...string) string {
			lines := []string{"#pragma once"}
			for _, target := range targets {
				relative, _ := filepath.Rel(path.Dir(from), target)
				lines = append(lines, `#include "`+filepath.ToSlash(relative)+`"`)
			}
			for _, spec := range specs {
				lines = append(lines, "#include "+spec)
			}
			return strings.Join(lines, "\n") + "\n"
		},
	}
}

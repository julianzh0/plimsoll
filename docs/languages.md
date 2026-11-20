# How each language is read

Every detector works the same way for every language. A language only adds how its files are written, as one data
file in `internal/conventions/`. Each language below names what it adds, and anything it does not name follows the
defaults at the end of this section.

plimsoll reads code with [tree-sitter](https://tree-sitter.github.io/) grammars that run inside it, so a comment or a
string never counts as an import. Each language below names its grammar. The package and namespace lines of Java,
Kotlin and C# are read by text patterns, since generated files declare them too. Macros often confuse the C and C++
grammars, so the includes of a C/C++ file that its grammar reads only with an error are read line by line instead; its
functions are still read from the tree, and one that holds the error is counted as not measured. A file that would
need more than 128 MiB to parse is not read, and the answer names it under "not analysed". A changed file in which
the grammar meets a syntax error, such as an unfinished edit, is named there too, since an import after the error may
be missed. On a system that forbids executable memory the grammars
still run, interpreted and about 80 times slower.

<table>
  <tr><td><a href="#cc">C</a></td><td><a href="#cc">C++</a></td><td><a href="#c">C#</a></td></tr>
  <tr><td><a href="#dart">Dart</a></td><td><a href="#go">Go</a></td><td><a href="#javakotlin">Java</a></td></tr>
  <tr><td><a href="#typescriptjavascript">JavaScript</a></td><td><a href="#javakotlin">Kotlin</a></td><td><a href="#python">Python</a></td></tr>
  <tr><td><a href="#rust">Rust</a></td><td><a href="#swift">Swift</a></td><td><a href="#typescriptjavascript">TypeScript</a></td></tr>
</table>

### C/C++

- **Read by:** the grammars tree-sitter-c v0.24.2 and tree-sitter-cpp v0.23.4. A file that a grammar reads only with
  an error, or in which a define takes a directive as its value, is read line by line, comments blanked first.
- **Files:** `.c`, `.h` and `.cc`, `.cpp`, `.cxx`, `.hpp`, `.hh`, `.hxx`. A `.h` file counts with the one of the two
  that has more files of its own in the repository.
- **Imports:** `#include "x.h"` is the file beside the including one. Else, like `#include <x.h>`, it is the tracked
  file of that path under an include folder: a folder named `include`, or one that a `CMakeLists.txt` names by a path
  in `include_directories` or `target_include_directories`. Such a file under several folders is ambiguous, and an
  include that a macro names is not readable. Every `#if` branch is read.
- **Runtime dependencies:** `vcpkg.json`, the top-level `dependencies` without host tools and those of its default
  features.
- **Functions:** functions and, in C++, lambdas; a `goto` counts 1. When a file holds a function the grammar cannot
  read, a copy with three kinds of macro rewritten is read for those functions only, with every line kept: an
  attribute-like name where a declaration starts (`FMT_CONSTEXPR`) and an annotation (`_In_`, `_In_reads_(n)`,
  `__declspec(...)`) are blanked, a loop macro (`LIST_FOREACH(...)`) is read as `while`, and the macros of Qt
  (`emit`, `Q_D`, `Q_OBJECT`, `foreach`, signal and slot sections) and GLib (`g_autofree`, `g_autoptr`,
  `G_DEFINE_TYPE`, `G_GNUC_*`) are read as what they expand to. A function still unreadable, as behind `#ifdef`
  branches that split its braces or another framework's macros, is counted as unreadable and is not
  measured.
  Validated like the other languages on 61 blind-read marks of C and C++ agent pull requests (0.72 worth acting on,
  per-change lower bound 0.61), with two limits: they came from 13 repositories rather than 15, and 5 of 43 macro-heavy
  repositories kept more than 10% of their functions unreadable.

### C#

- **Read by:** the grammar tree-sitter-c-sharp v0.23.5.
- **Files:** `.cs`.
- **Imports:** `using A.B;` names the namespace `A.B`: the folders whose files declare it with `namespace A.B`, wherever
  they lie, nested `namespace` blocks joined. `using static` names a type of a namespace; an alias names the namespace
  of its target when one is declared, else the one before its last name. A `global using` reaches every file of its
  project and counts as not readable.
- **Import rings:** not looked for, because a using names a namespace, not a file.
- **Runtime dependencies:** every `*.csproj`, `Directory.Build.props` and `Directory.Build.targets` outside test code:
  its `PackageReference` items, without those whose `PrivateAssets` is `all`.
- **Changed together:** the folder is one unit.

### Dart

- **Read by:** the grammar tree-sitter-dart (UserNobody14, commit `0038280b`).
- **Files:** `.dart`.
- **Imports:** `import`, `export`, `part` and `part of`, relative when the target has no scheme, `package:<name>/` for the
  package `pubspec.yaml` names, the members of its `workspace:` (globs with `*` per segment) and its `path`
  dependencies.
- **Runtime dependencies:** every `pubspec.yaml`, `dependencies`, in block or flow form.

### Go

- **Read by:** the grammar tree-sitter-go v0.25.0.
- **Files:** `.go`.
- **Imports:** the import path of a folder under the module of the nearest `go.mod`, or under another tracked module
  when a `go.work` uses both or a local `replace` of a module the importer requires names its folder.
- **Import rings:** not looked for, because the compiler rejects them.
- **Runtime dependencies:** every `go.mod`, `require` without `// indirect`.
- **Changed together:** the package folder is one unit.

### Java/Kotlin

- **Read by:** the grammars tree-sitter-java v0.23.5 and tree-sitter-kotlin (fwcd, commit `d6af96d2`).
- **Files:** `.java`, `.kt`, `.kts`.
- **Imports:** `import a.b.C`, `import static` and `a.b.*` name the package `a.b`: the folders whose files under
  `src/<set>/java` and `src/<set>/kotlin` declare it, narrowed to the files that declare the imported name when every
  file of the package was read. A test file reaches its own module's test source sets, another module's test fixtures
  (`testFixtures(project(…))`, also by accessor) and a Maven `test-jar`.
- **Import rings:** not looked for, because an import names a package, not a file.
- **Runtime dependencies:** every `pom.xml`, `dependencies` in compile or runtime scope, with properties of the same
  pom filled in. Every `build.gradle` and `build.gradle.kts`, `implementation`, `api` and `runtimeOnly`, also of
  variants and through the version catalogue.
- **Changed together:** the package folder is one unit.

### Python

- **Read by:** the grammar tree-sitter-python v0.25.0.
- **Files:** `.py`, `.pyi`.
- **Imports:** `import x` and `from x import y`, relative with leading dots, `import_module()`, the project's packages at
  the root and under `src/`.
- **Type-only imports,** left out of rings: imports under `if TYPE_CHECKING:`.
- **Declared test folders and roots:** pytest's `testpaths` and `pythonpath`, from the one configuration pytest reads: the first of `pytest.toml`, `.pytest.toml`, `pytest.ini`, `.pytest.ini`, `pyproject.toml` with a pytest table, `tox.ini` with `[pytest]` and `setup.cfg` with `[tool:pytest]`. A `pyproject.toml` with both of pytest's tables is refused, as pytest refuses it. In such a folder only the files pytest collects are test code: its `python_files`, else `test_*.py` and `*_test.py`, with `conftest.py`.
- **Runtime dependencies:** every `pyproject.toml`, `[project] dependencies` and `[tool.poetry.dependencies]` without
  `python`; Poetry's groups and `[dependency-groups]` add none.

### Rust

- **Read by:** the grammar tree-sitter-rust v0.24.2. Macro bodies, which the grammar keeps as tokens, are read by
  text patterns, and so are setting reads, over the file with its comments and strings left out.
- **Files:** `.rs`.
- **Imports:** `mod x;` is `x.rs` or `x/mod.rs` below the file that declares it. A `use` or `extern crate` path that
  starts with `crate`, `self`, `super`, a module declared beside it or the name of a crate whose `Cargo.toml` is
  tracked names the file of its longest leading run of modules. Every path of a `use` group counts, and a module
  written in braces moves the paths inside it. Crates and their roots come from their `Cargo.toml` (`[lib] path`,
  targets, Cargo's discovery); a crate of the 2015 edition reads a first name from its root; `#[path]` names a
  module's file; a path dependency, renamed or from its workspace, is the crate it names; a name no module file or
  item binds is not found.
- **Import rings:** not looked for, because a crate's modules use each other.
- **Test code inside product files:** `#[cfg(test)]` items count as test lines.
- **Runtime dependencies:** every `Cargo.toml`, `[dependencies]` and the `dependencies` of `[target.…]` tables, by the
  package a renamed or inherited entry names.

### Swift

- **Read by:** the grammar tree-sitter-swift 0.7.4.
- **Files:** `.swift`.
- **Imports:** `import X`, `@testable import X` and `import struct X.Y` name the module `X`: a target of
  `Package.swift`, whose folder (its `path:`, else `Sources/X` or `Tests/X`) holds every file of it at any depth.
  Without `Package.swift` own modules cannot be told from frameworks.
- **Import rings:** not looked for, because the compiler rejects modules that import each other.
- **Runtime dependencies:** every `Package.swift` and `Package@swift-<version>.swift`, its `.package(url:)`, `.package(path:)` and `.package(id:)` lines, by the
  package's identity.
- **Changed together:** the folder is one unit.

### TypeScript/JavaScript

- **Read by:** the grammars of tree-sitter-typescript v0.23.2: TypeScript for `.ts`, TSX for the other suffixes.
- **Files:** `.ts`, `.tsx`, `.js`, `.mjs`, `.cjs`.
- **Imports:** `import … from`, `export … from`, `import "x"`, `import()` and `require()`, relative, through the paths
  of the nearest `tsconfig.json` that takes the file (with its `extends` chain and a solution's `references`), or to a
  workspace package by its `exports`, else its `types`, `typings`, `module` or `main`, mapped from `outDir` to its
  sources.
- **Type-only imports,** left out of rings: `import type`, `export type`.
- **Runtime dependencies:** every `package.json`, `dependencies`.

### Defaults

- **Test code** lives in folders named `tests`, `Tests`, `test`, `__tests__`, `spec` or `integration_test`, in folders
  directly in `src/` named `test[A-Z]*` or `*Test*` (`androidTest`, `testFixtures`), in folders whose name ends in
  `.Tests` or `.Test` (`Acme.Tests`), in files whose name ends in `.test`, `.spec`, `_test` or `-test` or starts with
  `test_`. A test file that defines a test (`func Test…`, `func test…`, `def test_…`, `test(…)`, `testWidgets(…)`,
  `it(…)`, `@Test`, `[Fact]`, `[Test]`, `#[test]`, `TEST(…)`, `TEST_CASE(…)`) counts as a test, any other test file as
  test infrastructure.
- **Type-only imports:** none.
- **Import rings** are looked for.
- **Changed together:** each file is one unit.
- **Added code:** top-level functions, types and exported values; a decorated or annotated declaration, a default
  export, generated code and example projects are left out.
- **Functions:** measured in every language below.

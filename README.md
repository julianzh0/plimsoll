<p align="center">
  <img src=".github/logo.svg" alt="plimsoll" width="240">
</p>

<h1 align="center">plimsoll</h1>

<p align="center"><strong>The load line for your repository: what a change did to its size, its tests, its wiring and its history, measured, never guessed.</strong></p>

<p align="center">
  <a href="https://github.com/julianzh0/plimsoll/releases/latest"><img src="https://img.shields.io/github/v/release/julianzh0/plimsoll" alt="Release"></a>
  <a href="https://github.com/julianzh0/plimsoll/actions/workflows/ci.yml"><img src="https://github.com/julianzh0/plimsoll/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/julianzh0/plimsoll" alt="Go"></a>
  <a href="https://modelcontextprotocol.io"><img src="https://img.shields.io/badge/MCP-server-blue" alt="MCP server"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License: MIT"></a>
</p>

A ship's Plimsoll line shows at a glance whether it is loaded past what is safe. plimsoll does that for a git
repository. You, or your coding agent, ask it what a branch or a change did, and it answers with a few findings
called marks. Each mark says what was measured, the limit it crossed, and how to check it yourself. For example:
a file grew past 1,000 lines, product code started importing test code, two files keep changing together, or a
new setting has no code that reads it. It is built for coding agents and speaks the Model Context Protocol (MCP).

It is not a score or a grade, not a dashboard, and not a linter: it writes nothing into the repositories it reads,
stores nothing, and never says what to do.

## Why plimsoll

A coding agent sees the files it works on, not the repository around them. plimsoll fills that gap: after each
change, it tells the agent what the change did to the repository as a whole. The answer is a few findings, and each
one says how to check it, so the agent does not have to take plimsoll's word for it. There is nothing to install
besides plimsoll itself and nothing that has to be configured.

What plimsoll brings together:

- **The change, not only the current state.** It compares now with a base you choose, such as `main`.
- **Several sides in one answer.** Size, tests, wiring, history, settings and working habits.
- **Findings, not a score.** Each finding names its measurement, its limit and a way to check it.
- **Nothing to set up.** It needs `git` and nothing else: no account, no server, no network. An optional
  `.plimsoll.toml` sets your own limits and declares your layout.

It works next to your linters and CI, not instead of them. They go deeper into one thing, and plimsoll keeps the
overview of a change.

## What it checks

Six detectors look at a change from different sides:

- **Size** (`shape`): how much product and test code there is, the largest files, new dependencies and new settings.
- **Working habits** (`process`): planning or review notes committed into the repository, and more churn in
  documentation than in code.
- **Settings** (`settings`): settings that no code reads, names defined twice, and variables only the container
  setup knows.
- **Hotspots** (`hotspots`): the files changed most often, weighed by their size.
- **Hidden coupling** (`co-change`): files that keep changing together in the same commits.
- **Wiring** (`links`): who imports whom, import rings, folders that depend on each other in a circle, product code
  that imports test code, and how far a change reaches.

Each detector compares now with a base you choose, such as `main`, so old debt stays quiet and new growth stands out.

## Highlights

- **Read-only** // runs only read-only `git` commands, never changes a byte, fetches nothing and keeps no state
- **No telemetry** // sends nothing anywhere and opens no network connection: it talks MCP over standard input and output
- **One binary** // no runtime and nothing that has to be configured, `git` is all it needs
- **Change-based** // compares now with a base revision, so standing debt stays quiet and new growth is named
- **Honest** // every number names its limit and source, and whatever it could not read is listed as "not analysed"
- **Language-agnostic** // the same detectors for Go, TypeScript, Python, Dart, Java, Kotlin, Rust, C#, Swift and C/C++, and a language is one data file

## Install

macOS and Linux, amd64 and arm64:

```sh
curl -fsSL https://raw.githubusercontent.com/julianzh0/plimsoll/main/install.sh | sh
```

The script downloads the binary of the latest [release](https://github.com/julianzh0/plimsoll/releases), checks its
SHA-256 checksum and puts it in `~/.local/bin` (`PLIMSOLL_INSTALL_DIR` changes the folder, `PLIMSOLL_VERSION` picks
a release). You can also download a binary and `checksums.txt` from the release page yourself, or build from source
with Go (see [Development](#development)).

## Connect your agent

plimsoll is a local MCP server on standard input and output. It takes no argument and reads no setting.

**Claude Code**

```sh
claude mcp add plimsoll -- plimsoll
```

**Codex** (the CLI, the IDE extension and the ChatGPT desktop app share this setting)

```sh
codex mcp add plimsoll -- plimsoll
```

or in `~/.codex/config.toml`:

```toml
[mcp_servers.plimsoll]
command = "plimsoll"
```

**OpenCode**

```sh
opencode mcp add plimsoll -- plimsoll
```

**Any other MCP client** (Claude Desktop, Cursor, Antigravity and others), in its server configuration:

```json
{
  "mcpServers": {
    "plimsoll": { "command": "plimsoll" }
  }
}
```

Then ask your agent, for example: *"Use plimsoll to check what this branch did to the repository since `main`."*

## Example

`status` on plimsoll's own repository, 30 commits back:

```text
plimsoll 0.3.0 status: base `HEAD~30` 422ce612e0, head 42a374430f
3 new, 0 grown, 0 resolved
Footprint of the change since the base: 30 commits, 72 files; product +1,645 -323, test +1,263 -75, documentation +16 -9; 0.8 test lines added per product line added.

1. [new, exact] `internal/conventions/rules.go` and `internal/snapshot/snapshot.go` changed together in 6 of the 25 commits that touched either (24%); 4 at the base; limit 5 shared commits (plimsoll's rule).
   verify: `explain` lists the shared commits by name. id: co-change.pair:internal/conventions/rules.go+internal/snapshot/snapshot.go
2. [new, exact] `internal/conventions/imports.go` and `internal/snapshot/links.go` changed together in 5 of the 12 commits that touched either (42%); 1 at the base; limit 5 shared commits (plimsoll's rule).
   verify: `explain` lists the shared commits by name. id: co-change.pair:internal/conventions/imports.go+internal/snapshot/links.go
3. [new, exact] `internal/conventions/rules.go` and `internal/snapshot/facts.go` changed together in 5 of the 32 commits that touched either (16%); 2 at the base; limit 5 shared commits (plimsoll's rule).
   verify: `explain` lists the shared commits by name. id: co-change.pair:internal/conventions/rules.go+internal/snapshot/facts.go

Standing (already at the base, not grown): 1 pair of files that changed together in at least 5 commits.
History: 105 commits read, 100 usable.
Detectors run: shape, process, settings, hotspots, co-change, links.
Not analysed: no `.env.example`, so no settings count; no `.env.example`, so no settings were compared; 2 changed files hold a syntax error, so an import after the error may be missed: `internal/links/links_test.go`, `internal/links/wording_test.go`.
```

## Tools

| Tool | Returns |
|---|---|
| `status(repository, base?)` | at most five new or grown marks, the standing line, the footprint and what was not analysed |
| `report(repository, base?, files?)` | everything measured, in a fixed layout. Standing items of one kind past five are counted by folder in the text, and `structuredContent` holds them all. With `files`, also the files that import each named file |
| `explain(repository, base?, mark)` | the evidence of one mark, computed again and capped |

Without `base`, a question compares with the default base the repository declares in `.plimsoll.toml`, and the
answer's first line says so. Every result carries the text in `content` and the same answer as `structuredContent`. An answer may shorten, never
hide: what it leaves out it counts ("3 more: `report` lists them"), and `report` holds every mark.

## What it measures in detail

Every number is measured at the base and now, from the tracked files only:

- product, test and other code lines, and the test ratio (test lines per product line). Test code inside a product file counts as test lines where a language marks it
- when the ratio is over its limit, test code split into files that define tests and test infrastructure that
  defines none, with the largest infrastructure files
- the three largest code files
- the settings in `.env.example` and the runtime dependencies of `package.json`, `go.mod`, `pyproject.toml` (`[project] dependencies`), `pubspec.yaml`, `pom.xml`, `build.gradle` or `build.gradle.kts`, `Cargo.toml`, `Package.swift`, `vcpkg.json`, and every `*.csproj` outside test code
- the footprint of the change since the base
- churn by area (product, tests, docs, other) over the last 150 non-merge commits
- tracked planning and review files, and documentation churn against product and test churn since the base
- the settings audit: settings no product file names, names defined twice in `.env.example`, compose variables that are not settings, and reads whose name is computed at run time
- hotspots: the three code files (or as many as `hotspot_files` sets) with the most changes in usable commits times their lines now, as report rows and a remark on their size mark, with no rule of its own
- changed together: pairs of code files that share usable commits, at least one product code, neither a file that defines tests, and not two files that the language treats as one unit
- test folders and package roots that a project declares in its own configuration or in `.plimsoll.toml`, added to the defaults and named with their source
- import links, read from the code: the files that import the changed files and that the change did not touch, the three files that import the most others, import rings, cycles between sibling folders, and each new product file that no file imports. With `files`, also the files that import each named file.

What it could not see is always named under "not analysed".

## How each language is read

Every detector works the same way for every language. A language only adds how its files are written, as one data
file in `internal/conventions/`. Each language below names what it adds, and anything it does not name follows the
defaults at the end of this section.

plimsoll reads code with [tree-sitter](https://tree-sitter.github.io/) grammars that run inside it, so a comment or a
string never counts as an import. Each language below names its grammar. The package and namespace lines of Java,
Kotlin and C# are read by text patterns, since generated files declare them too. Macros often confuse the C and C++
grammars, so a C/C++ file that its grammar reads only with an error is read line by line instead. A file that would
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
- **Runtime dependencies:** `vcpkg.json`, the top-level `dependencies` without host tools.
- **Setting reads:** a computed `getenv(…)` is counted.

### C#

- **Read by:** the grammar tree-sitter-c-sharp v0.23.5.
- **Files:** `.cs`.
- **Imports:** `using A.B;` names the namespace `A.B`: the folders whose files declare it with `namespace A.B`, wherever
  they lie. `using static` and an alias name a type of a namespace. A `global using` reaches every file of its project
  and counts as not readable.
- **Import rings:** not looked for, because a using names a namespace, not a file.
- **Runtime dependencies:** every `*.csproj` outside test code: its `PackageReference` items, without those whose
  `PrivateAssets` is `all`.
- **Setting reads:** a computed `Environment.GetEnvironmentVariable(…)` or index into a configuration object
  (`configuration[…]`) is counted.
- **Changed together:** the folder is one unit.

### Dart

- **Read by:** the grammar tree-sitter-dart (UserNobody14, commit `0038280b`).
- **Files:** `.dart`.
- **Imports:** `import`, `export`, `part` and `part of`, relative when the target has no scheme, `package:<name>/` for the
  package `pubspec.yaml` names and for the members of its `workspace:`.
- **Runtime dependencies:** `pubspec.yaml`, `dependencies`.
- **Setting reads:** a computed `Platform.environment[…]`, `String.fromEnvironment(…)` or `dotenv.env[…]` is counted.

### Go

- **Read by:** the grammar tree-sitter-go v0.25.0.
- **Files:** `.go`.
- **Imports:** the import path of a folder, under the module named in `go.mod`.
- **Import rings:** not looked for, because the compiler rejects them.
- **Runtime dependencies:** `go.mod`, `require` without `// indirect`.
- **Setting reads:** a computed `os.Getenv(…)` or `os.LookupEnv(…)` is counted.
- **Changed together:** the package folder is one unit.

### Java/Kotlin

- **Read by:** the grammars tree-sitter-java v0.23.5 and tree-sitter-kotlin (fwcd, commit `d6af96d2`).
- **Files:** `.java`, `.kt`, `.kts`.
- **Imports:** `import a.b.C`, `import static` and `a.b.*` name the package `a.b`: the folders whose files under
  `src/<set>/java` and `src/<set>/kotlin` declare it. A folder under a test source set is reached only from files under
  one.
- **Import rings:** not looked for, because an import names a package, not a file.
- **Runtime dependencies:** `pom.xml`, `dependencies` in compile or runtime scope. `build.gradle` and
  `build.gradle.kts`, `implementation`, `api` and `runtimeOnly`.
- **Setting reads:** a computed `System.getenv(…)` is counted.
- **Changed together:** the package folder is one unit.

### Python

- **Read by:** the grammar tree-sitter-python v0.25.0.
- **Files:** `.py`, `.pyi`.
- **Imports:** `import x` and `from x import y`, relative with leading dots, `import_module()`, the project's packages at
  the root and under `src/`.
- **Type-only imports,** left out of rings: imports under `if TYPE_CHECKING:`.
- **Declared test folders and roots:** pytest's `testpaths` and `pythonpath` in `pyproject.toml`.
- **Runtime dependencies:** `pyproject.toml`, `[project] dependencies`.
- **Setting reads:** a computed `os.environ[…]` or `os.getenv(…)` is counted, and a pydantic `env_prefix` plus a field
  of the same file counts as read.

### Rust

- **Read by:** the grammar tree-sitter-rust v0.24.2. Macro bodies, which the grammar keeps as tokens, are read by
  text patterns, and so are setting reads, over the file with its comments and strings left out.
- **Files:** `.rs`.
- **Imports:** `mod x;` is `x.rs` or `x/mod.rs` below the file that declares it. A `use` or `extern crate` path that
  starts with `crate`, `self`, `super`, a module declared beside it or the name of a crate whose `Cargo.toml` is
  tracked names the file of its longest leading run of modules. Every path of a `use` group counts, and a module
  written in braces moves the paths inside it.
- **Import rings:** not looked for, because a crate's modules use each other.
- **Test code inside product files:** `#[cfg(test)]` items count as test lines.
- **Runtime dependencies:** `Cargo.toml`, `[dependencies]` and the `dependencies` of `[target.…]` tables.
- **Setting reads:** a computed `env::var(…)` or `env::var_os(…)` is counted, also inside a macro such as `println!`.

### Swift

- **Read by:** the grammar tree-sitter-swift 0.7.4.
- **Files:** `.swift`.
- **Imports:** `import X`, `@testable import X` and `import struct X.Y` name the module `X`: a target of
  `Package.swift`, whose folder (its `path:`, else `Sources/X` or `Tests/X`) holds every file of it at any depth.
  Without `Package.swift` own modules cannot be told from frameworks.
- **Import rings:** not looked for, because the compiler rejects modules that import each other.
- **Runtime dependencies:** `Package.swift`, its `.package(url:)`, `.package(path:)` and `.package(id:)` lines, by the
  package's identity.
- **Setting reads:** a computed `ProcessInfo.processInfo.environment[…]` is counted.
- **Changed together:** the folder is one unit.

### TypeScript/JavaScript

- **Read by:** the grammars of tree-sitter-typescript v0.23.2: TypeScript for `.ts`, TSX for the other suffixes.
- **Files:** `.ts`, `.tsx`, `.js`, `.mjs`, `.cjs`.
- **Imports:** `import … from`, `export … from`, `import "x"`, `import()` and `require()`, relative or through the paths
  of `tsconfig.json`.
- **Type-only imports,** left out of rings: `import type`, `export type`.
- **Runtime dependencies:** `package.json`, `dependencies`.
- **Setting reads:** a computed `process.env[…]` is counted.

### Defaults

- **Test code** lives in folders named `tests`, `Tests`, `test`, `__tests__`, `spec` or `integration_test`, in folders
  directly in `src/` named `test[A-Z]*` or `*Test*` (`androidTest`, `testFixtures`), in folders whose name ends in
  `.Tests` or `.Test` (`Acme.Tests`), in files whose name ends in `.test`, `.spec`, `_test` or `-test` or starts with
  `test_`. A test file that defines a test (`func Test…`, `func test…`, `def test_…`, `test(…)`, `testWidgets(…)`,
  `it(…)`, `@Test`, `[Fact]`, `[Test]`, `#[test]`, `TEST(…)`, `TEST_CASE(…)`) counts as a test, any other test file as
  test infrastructure.
- **Type-only imports:** none.
- **Import rings** are looked for.
- **Setting reads:** every setting is also found by its name as a word.
- **Changed together:** each file is one unit.

## Rules

| Identifier | Fires when | Limit and source |
|---|---|---|
| `shape.test-ratio:repository` | the test ratio is over its limit now and was not at the base (new), or rose by 0.1 or more while over it (grown) | 2.0, plimsoll's rule: test code at most twice the product code |
| `shape.file-size:<path>` | a code file is over its line limit now and was not at the base (new), or grew while over it (grown) | 1,000 lines, the default of Pylint and SonarQube |
| `shape.dependencies:<manifest>` | a runtime dependency was added since the base. Its verify line names the new ones that no tracked lockfile holds (never installed, or not a real package) | any addition |
| `shape.settings:.env.example` | a setting was added since the base | any addition |
| `process.planning-file:<path>` | a file under a planning or review path is tracked now and was not at the base | repositories hold no planning records |
| `process.doc-churn:repository` | over the commits since the base, documentation churn is above product plus test churn | documentation above code |
| `process.limit-change:<key>` | `.plimsoll.toml` sets a limit to another value now than at the base, so a change that raised a limit to clear a mark says so | any change |
| `process.declaration-change:<key>` | `.plimsoll.toml` declares other values under a key of `[layout]` or `[defaults]` now than at the base | any change |
| `settings.no-reader:<NAME>` | a setting that no product file holds as a word or reads through a declared prefix (candidate) | plimsoll's rule: a setting has a reader |
| `settings.defined-twice:<NAME>` | a name on two or more lines of `.env.example` that are not commented out (candidate) | plimsoll's rule: one name, one setting |
| `settings.compose-only:<NAME>` | a compose `environment` key that takes its value from the host (no value, or `$`) and that `.env.example` does not name (candidate) | the three lists agree |
| `co-change.pair:<a>+<b>` | two code files changed together in at least 5 usable commits and did not at the base (new), or their share of the commits that touched either rose by at least 0.1 (grown). The mark shows that share and comes after every other mark | 5 shared commits, plimsoll's rule |
| `links.test-import:<path>` | a product file has an import of test code now and had none at the base (new), or has more (grown) (candidate) | product code imports no test code |
| `links.cycle:<path>` | two or more files import each other in a ring, by imports that are not type-only, and did not at the base (new), or the ring has more files (grown). The path is the ring's smallest (candidate) | files do not import each other in a ring |
| `links.folder-cycle:<folder>` | two or more folders directly under one folder depend on each other in a cycle, through imports between product files below them, and none of them did at the base (new), or the cycle has more folders (grown). The folder is the cycle's smallest (candidate) | the Acyclic Dependencies Principle (Robert C. Martin) |

Something over its limit at the base and no longer over it is reported as resolved. Something over it on both
sides that did not grow is standing.

### Your own limits and layout

A repository may set its own limits and declare its layout in `.plimsoll.toml` at its root, committed and reviewed
like any other file. Every key is optional:

```toml
[limits]
file_lines = 1500                           # a code file's line limit (default 1,000)
test_ratio = 3.0                            # test lines per product line (default 2.0)
shared_commits = 8                          # commits two files share before they are a pair (default 5)
hotspot_files = 5                           # files the report lists as hotspots (default 3)

[layout]
test_folders = ["checks", "tools/harness"]  # folders whose code is test code
test_files = ["*_spec.rb", "*IT.java"]      # names of test files

[defaults]
base = "main"                               # the base of a question that names none
```

| Key | What it declares |
|---|---|
| `file_lines`, `test_ratio`, `shared_commits` | The limits of the file-size, test-ratio and co-change rules |
| `hotspot_files` | How many files the report lists as hotspots, and so how many size marks say "Also a hotspot" |
| `test_folders` | Folders, as paths from the root, whose code files count as test code, beside the defaults |
| `test_files` | File names of code files that count as test code wherever they lie: `*` is any run of characters, `?` one |
| `base` | The branch, remote branch or tag a question without a base compares with. A remote branch is named with its remote (`origin/main`): `main` alone finds no remote branch. A branch of the name comes before a remote branch, and a remote branch before a tag |

A declaration adds to the defaults and takes none away, and now's file applies to both sides of a question. Every
mark names its limit's source, for example "1,500 lines, set by `.plimsoll.toml`", every report names what the file
declares, and a limit or a declaration that changed since the base is itself a mark. The file switches no rule off
and leaves no file out of the count: test code declared here counts as test code. A folder is a plain path, and a
bare name is a folder at the root. A test folder that is the root or a package root, and a file name that is only
`*`, are not applied.

A key plimsoll does not know, a table it does not read, a value that is no number above 0 (a whole number up to 2^53 for a limit
that counts), a list that is no list of text, a line outside a table or a key set twice is named under "not
analysed", and what it names keeps its default (a key set twice its first value). A key or table name that is no
plain name (letters, digits, `_`, `-`, parts joined by `.`) is counted there, not printed. A declared value that matches no
tracked file, or that is no plain path or file-name pattern, is named there too and not used. A question without a
base whose repository declares no usable default base is refused with one sentence that says why.

## Guarantees

- **trust**: plimsoll never changes a repository it reads. It runs only read-only git commands, takes no lock, starts no file monitor, runs no filter and fetches nothing from a remote. Tests: `internal/gitrepo/history_integration_test.go::TestIntegrationReadLeavesTheRepositoryUnchanged`, `internal/gitrepo/history_integration_test.go::TestIntegrationReadFetchesNothingFromARemote`, `cmd/plimsoll/tools_integration_test.go::TestIntegrationAnswersAreRepeatableAndLeaveNoTrace`
- **trust**: no answer shows the content of a file or the value of a setting: only names, paths, line numbers and counts, and, from the repository's own `.plimsoll.toml`, its limits, the declared values that are plain paths, file-name patterns or branch names, and key and table names that are plain names. Content plimsoll does not need is never read. Tests: `internal/snapshot/facts_test.go::TestBuildReadsSettingNamesNeverValues`, `internal/snapshot/links_test.go::TestBuildKeepsNoImportText`, `internal/links/links_test.go::TestNoLinksLineHoldsImportText`, `internal/snapshot/settings_test.go::TestBuildReadsComposeEnvironmentKeys`, `internal/shape/explain_test.go::TestExplainHoldsNoFileContentOrSettingValue`, `internal/gitrepo/reader_integration_test.go::TestIntegrationReadLeavesUnwantedAndBinaryContentUnread`, `internal/answer/layout_test.go::TestLayoutLinesHoldNoValueFromOutsideTheRepository`, `internal/answer/samples_test.go::TestNoAnswerPrintsADeclaredValueOutsideItsForm`
- **idempotency**: the same question about the same repository state gives the same answer, in the same process or a new one, whatever the user's git configuration. The history and the file readings kept between questions answer as a fresh read does, also for questions answered at the same time. Tests: `cmd/plimsoll/tools_integration_test.go::TestIntegrationAnswersAreRepeatableAndLeaveNoTrace`, `internal/snapshot/readings_test.go::TestKeptReadingsEqualFreshOnes`, `internal/gitrepo/history_integration_test.go::TestIntegrationKeptHistoryEqualsAFreshRead`, `internal/gitrepo/history_integration_test.go::TestIntegrationReadsAtTheSameTimeShareOneReader`, `internal/gitrepo/history_test.go::TestQuestionsAtTheSameTimeShareOneHistoryRead`, `internal/gitrepo/history_integration_test.go::TestIntegrationReadIsTheSameEverywhere`
- **ordering**: new and grown marks come in one order (a mark another detector backs with a remark first, then new before grown, exact before candidate, the farthest past its limit first, then by identifier, at most two of one detector before the others). Marks about the history around a change, the pairs of files that changed together, come after all others, and `status` names the ones it does not show. `status` shows at most five, and the report's sections follow the detector list. Tests: `internal/answer/mark_test.go::TestOrderPutsNewExactAndFarthestFirstWithTwoPerDetector`, `internal/answer/assemble_test.go::TestStatusShowsFiveMarksAndStaysInsideFortyLines`, `internal/answer/assemble_test.go::TestStatusShowsContextMarksLastAndNamesTheHiddenOnes`, `cmd/plimsoll/detectors_test.go::TestDetectorsAreListedInReportOrder`
- **cleanup**: a SIGTERM or the end of its input ends the server with exit status 0. Tests: `cmd/plimsoll/main_e2e_test.go::TestE2EProgramAnswersAndEndsOnASignal`, `cmd/plimsoll/main_e2e_test.go::TestE2EEndOfInputEndsTheProcess`
- **trust**: plimsoll sends no telemetry and opens no network connection. It talks MCP over standard input and output, runs only `git` and imports no network package. The MCP SDK it uses carries HTTP code for its other transports, which plimsoll never calls. Tests: `cmd/plimsoll/guard_test.go::TestModuleKeepsItsGuards`, `cmd/plimsoll/guard_test.go::TestGuardRulesNameEachViolation`, `internal/gitrepo/history_integration_test.go::TestIntegrationReadFetchesNothingFromARemote`
- **trust**: the grammars plimsoll runs are given no file, network, clock or environment, and the only functions they can call answer with a failure. Tests: `internal/parse/modules_test.go::TestModulesImportOnlyTheFailingHost`
- **ownership**: a parser belongs to whoever created it. Closing it frees its grammars and their memory, and any other parser keeps working. Tests: `internal/parse/parse_test.go::TestClosingOneParserLeavesAnotherWorking`
- **error mapping**: a file the parser cannot read, or a changed file in which it meets a syntax error, is named under "not analysed" and never fails the question. Tests: `internal/parse/parse_test.go::TestAFileOverTheCapIsAnErrorAndTheNextCallWorks`, `internal/snapshot/build_test.go::TestBuildListsAFileTheParserCannotRead`, `internal/snapshot/links_test.go::TestBuildReadsImportsAroundASyntaxError`
- **error mapping**: git's own refusal (exit status 1 or 128) keeps its kind, such as an unknown base or no repository. Any other git failure is "git failed". A question without a base whose repository declares no usable default base is refused, and its sentence names `.plimsoll.toml`. Every refusal is one plain sentence without structured content. A malformed request, an unknown method or an unknown tool is a JSON-RPC error with its standard code. Tests: `internal/gitrepo/refusal_test.go::TestRefusalKeepsItsKindOnlyForGitsOwnAnswer`, `cmd/plimsoll/tools_integration_test.go::TestIntegrationRefusals`, `internal/mcpserver/tools_test.go::TestBaseIsOptionalInEveryTool`, `cmd/plimsoll/protocol_test.go::TestProtocolErrors`

## Protocol

plimsoll speaks MCP through the official [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk): revision
2026-07-28, and the `initialize` handshake of earlier revisions for clients that still open that way. Answers may
arrive in any order, and each carries the id of its request. At most eight questions are answered at the same
time and the others wait their turn. Questions that need the same history share one read of it, and the grammars
parse one file at a time, so memory stays bounded however many questions a client sends at once.

## Development

### Setup

To build from source, clone the repository and run `make build`. The binary is `bin/plimsoll`.

You need Go 1.27.4 and golangci-lint 2.15.0, exactly. `make qa` fails on any other version. Install the verified golangci-lint release into your Go binary folder with:

```sh
make install-golangci-lint
```

After a fresh clone, enable the Git hook with:

```sh
make hooks
```

### Commands

| Command | What it does |
|---|---|
| `make qa` | Runs the tool-version, formatting, vet, lint, tidy, `go fix` and agent-file checks, `make cover` and the vulnerability check, stopping at the first failure |
| `make fix` | Formats the code, applies lint and `go fix` rewrites, tidies `go.mod` |
| `make unit` | Unit tests |
| `make integration` | Tests against real git in temporary repositories |
| `make e2e` | Tests that launch the real binary |
| `make cover` | Runs the three test levels once with coverage on and prints the merged coverage per package and in total (test helpers left out), as information, not a gate |
| `make flake TESTS=<pattern>` | Repeats the matching tests, 20 times by default (`COUNT=`) |
| `make lint` | golangci-lint |
| `make build` | Builds `bin/plimsoll` |
| `make hooks` | Enables the pre-commit hook in `.githooks/` |
| `make grammars` | Builds the grammar modules from the sources `internal/parse/grammars/pins.txt` names, with zig 0.17.1, and records their hashes |
| `make grammars-check` | Rebuilds the grammar modules and fails when one differs from its pin or from the committed file |

Tests run with `-race` and `-shuffle=on`, and record coverage in `bin/cover/`. CI shows the coverage table in each run's summary. `make qa` also checks formatting, `go vet`, `go mod tidy`, `go fix`, agent files and known vulnerabilities, so it needs network access. It fails when a tracked (or, when the project is not its own Git top level, present) `CLAUDE.md`, `AGENTS.md`, `.claude/`, `.cursor/` or `.vscode/tasks.json` file is not listed in `AGENT_FILES_ALLOWED` in the `Makefile`. The shipped `CLAUDE.md` is listed.

### Structure

```text
cmd/plimsoll/                    # the program
internal/conventions/            # the shared rules and the limits, as data, one file per language entry
internal/snapshot/               # the common model of one question, built from the reader's plain data
internal/gitrepo/                # the only code that runs git or reads the analysed repository
internal/answer/                 # the one result of every question, the detector contract, the text
internal/shape/                  # the shape detector
internal/process/                # the process detector: planning files, documentation churn
internal/settings/               # the settings audit
internal/hotspots/               # the files changed most often times their size
internal/cochange/               # pairs of files that change together
internal/links/                  # import links: reach, product code importing test code, rings
internal/mcpserver/              # the only code that knows MCP, through the official MCP Go SDK
internal/parse/                  # the only code that knows WebAssembly: runs the tree-sitter grammars through wazero
internal/parse/grammars/         # the grammar modules, their pins, the C shim and the build script
```

A language is one file in `internal/conventions/` (`typescript.go`, `golang.go`, `python.go`) and one line in the
list of `Default`. No detector names a language.

To add a grammar: add its row to `pins.txt` and run `make grammars`, add its licence to `THIRD_PARTY_NOTICES.md`,
name the grammar and write its query in the language's file, then extend the language's import cases and sample
repository. Building a grammar takes zig and up to several gigabytes of memory for a large grammar, so it is not part
of `make qa`.

Before a grammar replaces a reader, read the field repositories with both and compare them file by file: a grammar
replaces a reader only when the answers stay the same or the differences are understood. What the C and C++ grammars
taught:

- Inside a stretch the grammar cannot read, a statement may take another node's name (an include becomes a
  `preproc_call`) or be skipped token by token, so no query finds it. A reader for such a language keeps its text
  patterns for those files, as C and C++ do with `LineReadWhenBroken`.
- A grammar can also misread without an error, as a define with no value and a trailing space shows. Look for such
  cases in the comparison and mark them as errors in the query.
- Blanking forms the grammar cannot read, such as macro prefixes or conditional lines, with spaces of the same
  length helps where single forms disturb the grammar, not where macros make up a file's structure.
- Facts written one per line, such as includes, gain little from a grammar. Facts of a file's structure gain most
  and suffer most from macros.

`internal/gitrepo/gitrepotest`, `internal/parse/parsetest` and `internal/snapshot/snapshottest` are test support. Their files do not end in
`_test`, so plimsoll counts them as product code when it measures itself.

## License

[MIT](LICENSE)

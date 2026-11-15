<p align="center">
  <img src=".github/logo.svg" alt="plimsoll" width="240">
</p>

<h1 align="center">plimsoll</h1>

<p align="center"><strong>The load line for your repository: what a change did to its size, its tests, its wiring and its history, measured, never guessed.</strong></p>

<p align="center">
  <a href="https://github.com/juzhxng/plimsoll/releases/latest"><img src="https://img.shields.io/github/v/release/juzhxng/plimsoll" alt="Release"></a>
  <a href="https://github.com/juzhxng/plimsoll/actions/workflows/ci.yml"><img src="https://github.com/juzhxng/plimsoll/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/juzhxng/plimsoll" alt="Go"></a>
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

**plimsoll tells a coding agent, after every change, what that change did to the whole repository, as a few findings
it can check itself, with nothing to set up.**

Many tools cover one or two of these. plimsoll combines all four:

| | plimsoll | Typical alternatives |
|---|---|---|
| Looks at | the change, against a base you choose | the whole current state (sentrux, SonarQube, Qlty) |
| Covers | size, tests, wiring, history, settings and working habits in one answer | one aspect (linters, dependency-cruiser, diffguard) |
| Answers with | a few findings, each with its measurement, its limit and a way to check it | a score or a grade (CodeScene, sentrux, Qlty) |
| Needs | `git`, nothing else: no account, server, network or config file | an account or a server (CodeScene, SonarQube), or a rules file (dependency-cruiser) |

It works next to your linters and CI, not instead of them: they go deeper into one thing, plimsoll keeps the overview
of a change.

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
- **One binary** // no runtime and nothing to configure, `git` is all it needs
- **Change-based** // compares now with a base revision, so standing debt stays quiet and new growth is named
- **Honest** // every number names its limit and source, and whatever it could not read is listed as "not analysed"
- **Language-agnostic** // the same detectors for Go, TypeScript, Python and Dart, and a language is one data file

## Install

macOS and Linux, amd64 and arm64:

```sh
curl -fsSL https://raw.githubusercontent.com/juzhxng/plimsoll/main/install.sh | sh
```

The script downloads the binary of the latest [release](https://github.com/juzhxng/plimsoll/releases), checks its
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
plimsoll 0.1.0 status: base `HEAD~30` 422ce612e0, head 42a374430f
3 new, 1 grown, 0 resolved
Footprint of the change since the base: 30 commits, 75 files; product +1,649 -327, test +1,278 -90, documentation +18 -11; 0.8 test lines added per product line added.

1. [new, exact] `internal/conventions/rules.go` and `internal/snapshot/snapshot.go` changed together in 6 of the 25 commits that touched either; 4 at the base.
   verify: `explain` lists the shared commits by name. id: co-change.pair:internal/conventions/rules.go+internal/snapshot/snapshot.go
2. [new, exact] `internal/conventions/imports.go` and `internal/snapshot/links.go` changed together in 5 of the 12 commits that touched either; 1 at the base.
   verify: `explain` lists the shared commits by name. id: co-change.pair:internal/conventions/imports.go+internal/snapshot/links.go
...

Standing (already at the base, not grown): none.
History: 105 commits read, 100 usable.
Detectors run: shape, process, settings, hotspots, co-change, links.
Not analysed: no `.env.example`, so no settings count; no `.env.example`, so no settings were compared.
```

## Tools

| Tool | Returns |
|---|---|
| `status(repository, base)` | at most five new or grown marks, the standing line, the footprint and what was not analysed |
| `report(repository, base, files?)` | everything measured, in a fixed layout. Standing items of one kind past five are counted by folder in the text, and `structuredContent` holds them all. With `files`, also the files that import each named file |
| `explain(repository, base, mark)` | the evidence of one mark, computed again and capped |

Every result carries the text in `content` and the same answer as `structuredContent`. An answer may shorten, never
hide: what it leaves out it counts ("3 more: `report` lists them"), and `report` holds every mark.

## What it measures in detail

Every number is measured at the base and now, from the tracked files only:

- product, test and other code lines, and the test ratio (test lines per product line)
- when the ratio is over its limit, test code split into files that define tests and test infrastructure that
  defines none, with the largest infrastructure files
- the three largest code files
- the settings in `.env.example` and the runtime dependencies of `package.json`, `go.mod`, `pyproject.toml` (`[project] dependencies`) and `pubspec.yaml`
- the footprint of the change since the base
- churn by area (product, tests, docs, other) over the last 150 non-merge commits
- tracked planning and review files, and documentation churn against product and test churn since the base
- the settings audit: settings no product file names, names defined twice in `.env.example`, compose variables that are not settings, and reads whose name is computed at run time
- hotspots: the three code files with the most changes in usable commits times their lines now, as report rows and a remark on their size mark, with no rule of its own
- changed together: pairs of code files that share usable commits, at least one product code, neither a file that defines tests, and not two files that the language treats as one unit
- test folders and package roots that a project declares in its own configuration, added to the defaults and named with their source
- import links, read by text patterns: the files that import the changed files and that the change did not touch, the three files that import the most others, import rings, cycles between sibling folders, and each new product file that no file imports. With `files`, also the files that import each named file.

What it could not see is always named under "not analysed".

## How each language is read

Every detector works the same way for every language. A language only adds how its files are written, as one data
file in `internal/conventions/`.

| | TypeScript and JavaScript | Go | Python | Dart |
|---|---|---|---|---|
| Files | `.ts`, `.tsx`, `.js`, `.mjs`, `.cjs` | `.go` | `.py`, `.pyi` | `.dart` |
| Imports | `import … from`, `export … from`, `import "x"`, `import()` and `require()`, relative or through the paths of `tsconfig.json` | the import path of a folder, under the module named in `go.mod` | `import x` and `from x import y`, relative with leading dots, `import_module()`, the project's packages at the root and under `src/` | `import`, `export`, `part` and `part of`, relative when the target has no scheme, `package:<name>/` for the package `pubspec.yaml` names and for the members of its `workspace:` |
| Type-only imports, left out of rings | `import type`, `export type` | none, and no rings are looked for: the compiler rejects them | imports under `if TYPE_CHECKING:` | none |
| Declared test folders and roots | the defaults | the defaults | pytest's `testpaths` and `pythonpath` in `pyproject.toml` | the defaults |
| Runtime dependencies | `package.json`, `dependencies` | `go.mod`, `require` without `// indirect` | `pyproject.toml`, `[project] dependencies` | `pubspec.yaml`, `dependencies` |
| Setting reads, besides the name as a word | a computed `process.env[…]` is counted | a computed `os.Getenv(…)` or `os.LookupEnv(…)` is counted | a computed `os.environ[…]` or `os.getenv(…)` is counted, and a pydantic `env_prefix` plus a field of the same file counts as read | a computed `Platform.environment[…]`, `String.fromEnvironment(…)` or `dotenv.env[…]` is counted |
| One unit when files change together | the file | the package folder | the file | the file |

The defaults, shared by all: test code lives in folders named `tests`, `test`, `__tests__`, `spec` or `integration_test`, in files whose
name ends in `.test`, `.spec`, `_test` or `-test` or starts with `test_`, and in files that define a test
(`func Test…`, `def test_…`, `test(…)`, `testWidgets(…)`, `it(…)`).

Next languages: Java and Kotlin, then Rust, C#, C and C++, and Swift.

## Rules

| Identifier | Fires when | Limit and source |
|---|---|---|
| `shape.test-ratio:repository` | the test ratio is over its limit now and was not at the base (new), or rose by 0.1 or more while over it (grown) | 2.0, plimsoll's rule: test code at most twice the product code |
| `shape.file-size:<path>` | a code file is over its line limit now and was not at the base (new), or grew while over it (grown) | 1,000 lines, the default of Pylint and SonarQube |
| `shape.dependencies:<manifest>` | a runtime dependency was added since the base | any addition |
| `shape.settings:.env.example` | a setting was added since the base | any addition |
| `process.planning-file:<path>` | a file under a planning or review path is tracked now and was not at the base | repositories hold no planning records |
| `process.doc-churn:repository` | over the commits since the base, documentation churn is above product plus test churn | documentation above code |
| `settings.no-reader:<NAME>` | a setting that no product file holds as a word or reads through a declared prefix (candidate) | plimsoll's rule: a setting has a reader |
| `settings.defined-twice:<NAME>` | a name on two or more lines of `.env.example` that are not commented out (candidate) | plimsoll's rule: one name, one setting |
| `settings.compose-only:<NAME>` | a compose `environment` key that takes its value from the host (no value, or `$`) and that `.env.example` does not name (candidate) | the three lists agree |
| `co-change.pair:<a>+<b>` | two code files changed together in at least 5 usable commits | at least 5 shared commits |
| `links.test-import:<path>` | a product file has an import of test code now and had none at the base (new), or has more (grown) (candidate) | product code imports no test code |
| `links.cycle:<path>` | two or more files import each other in a ring, by imports that are not type-only, and did not at the base (new), or the ring has more files (grown). The path is the ring's smallest (candidate) | files do not import each other in a ring |
| `links.folder-cycle:<folder>` | two or more folders directly under one folder depend on each other in a cycle, through imports between product files below them, and none of them did at the base (new), or the cycle has more folders (grown). The folder is the cycle's smallest (candidate) | the Acyclic Dependencies Principle (Robert C. Martin) |

Something over its limit at the base and no longer over it is reported as resolved. Something over it on both
sides that did not grow is standing.

## Guarantees

- **trust**: plimsoll never changes a repository it reads. It runs only read-only git commands, takes no lock, starts no file monitor, runs no filter and fetches nothing from a remote. Tests: `internal/gitrepo/history_integration_test.go::TestIntegrationReadLeavesTheRepositoryUnchanged`, `internal/gitrepo/history_integration_test.go::TestIntegrationReadFetchesNothingFromARemote`, `cmd/plimsoll/tools_integration_test.go::TestIntegrationAnswersAreRepeatableAndLeaveNoTrace`
- **trust**: no answer shows the content of a file or the value of a setting: only names, paths, line numbers and counts. Content plimsoll does not need is never read. Tests: `internal/snapshot/facts_test.go::TestBuildReadsSettingNamesNeverValues`, `internal/snapshot/links_test.go::TestBuildKeepsNoImportText`, `internal/links/links_test.go::TestNoLinksLineHoldsImportText`, `internal/snapshot/settings_test.go::TestBuildReadsComposeEnvironmentKeys`, `internal/shape/explain_test.go::TestExplainHoldsNoFileContentOrSettingValue`, `internal/gitrepo/reader_integration_test.go::TestIntegrationReadLeavesUnwantedAndBinaryContentUnread`, `internal/answer/layout_test.go::TestLayoutLinesHoldNoValueFromOutsideTheRepository`
- **idempotency**: the same question about the same repository state gives the same answer, in the same process or a new one, whatever the user's git configuration. The history kept between questions answers as a fresh read does, also for questions answered at the same time. Tests: `cmd/plimsoll/tools_integration_test.go::TestIntegrationAnswersAreRepeatableAndLeaveNoTrace`, `internal/gitrepo/history_integration_test.go::TestIntegrationKeptHistoryEqualsAFreshRead`, `internal/gitrepo/history_integration_test.go::TestIntegrationReadsAtTheSameTimeShareOneReader`, `internal/gitrepo/history_integration_test.go::TestIntegrationReadIsTheSameEverywhere`
- **ordering**: new and grown marks come in one order (a mark another detector backs with a remark first, then new before grown, exact before candidate, the farthest past its limit first, then by identifier, at most two of one detector before the others). `status` shows at most five, and the report's sections follow the detector list. Tests: `internal/answer/mark_test.go::TestOrderPutsNewExactAndFarthestFirstWithTwoPerDetector`, `internal/answer/assemble_test.go::TestStatusShowsFiveMarksAndStaysInsideFortyLines`, `cmd/plimsoll/detectors_test.go::TestDetectorsAreListedInReportOrder`
- **cleanup**: a SIGTERM or the end of its input ends the server with exit status 0. Tests: `cmd/plimsoll/main_e2e_test.go::TestE2EProgramAnswersAndEndsOnASignal`, `cmd/plimsoll/main_e2e_test.go::TestE2EEndOfInputEndsTheProcess`
- **error mapping**: git's own refusal (exit status 1 or 128) keeps its kind, such as an unknown base or no repository. Any other git failure is "git failed". Every refusal is one plain sentence without structured content. A malformed request, an unknown method or an unknown tool is a JSON-RPC error with its standard code. Tests: `internal/gitrepo/refusal_test.go::TestRefusalKeepsItsKindOnlyForGitsOwnAnswer`, `cmd/plimsoll/tools_integration_test.go::TestIntegrationRefusals`, `cmd/plimsoll/protocol_test.go::TestProtocolErrors`

## Protocol

plimsoll speaks MCP through the official [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk): revision
2026-07-28, and the `initialize` handshake of earlier revisions for clients that still open that way. Answers may
arrive in any order, and each carries the id of its request.

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
```

A language is one file in `internal/conventions/` (`typescript.go`, `golang.go`, `python.go`) and one line in the
list of `Default`. No detector names a language.

`internal/gitrepo/gitrepotest` and `internal/snapshot/snapshottest` are test support. Their files do not end in
`_test`, so plimsoll counts them as product code when it measures itself.

## License

[MIT](LICENSE)

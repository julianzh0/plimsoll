# Development

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
internal/links/                  # import links: importers of the change, product code importing test code, rings, declared import rules
internal/mcpserver/              # the only code that knows MCP, through the official MCP Go SDK
internal/parse/                  # the only code that knows WebAssembly: runs the tree-sitter grammars through wazero
internal/parse/grammars/         # the grammar modules, their pins, the C shim and the build script
mcpb/                            # the MCP bundle: its manifest, the launcher, the packing and publisher scripts
server.json                      # the entry the release lists in the official MCP registry
glama.json                       # who maintains plimsoll's listing on Glama
```

A language is one file in `internal/conventions/` (`typescript.go`, `golang.go`, `python.go`) and one line in the
list of `Default`. No detector names a language. Its function tables (which node kinds are functions, branches, loops,
closures and boolean operators) and its token pattern live in that file too; a sample under
`internal/snapshot/testdata/functions` checks that each language counts as the measurement did.

To add a grammar: add its row to `pins.txt` and run `make grammars`, add its licence to `THIRD_PARTY_NOTICES.md`,
name the grammar and write its query in the language's file, add its name and literal kinds to
`internal/conventions/tokens.go` with a case in `internal/snapshot/leaftokens_internal_test.go` (the repeat rule's
tokens), then extend the language's import cases and sample repository. Building a grammar takes zig and up to several gigabytes of memory for a large grammar, so it is not part
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

### Releases

A release is a pushed tag `vX.Y.Z`. Before tagging, set the version in `cmd/plimsoll/detectors.go` and in
`server.json` (its `version` and the bundle URL); the release stops when either names another version. The release
workflow builds the binaries, packs `plimsoll.mcpb` with `mcpb/pack.sh`, publishes both with `checksums.txt`, and
lists the release in the official MCP registry: `mcp-publisher` logs in with the workflow's GitHub OIDC token, so no
token or browser login is needed. CI packs the bundle from every push, starts it on amd64 and arm64 Linux, and lets
the registry validate its `server.json`.

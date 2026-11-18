<p align="center">
  <img src=".github/logo.svg" alt="plimsoll" width="240">
</p>

<h1 align="center">plimsoll</h1>

<p align="center"><strong>Catch what your coding agent's changes do to your code: new dependencies, tangled imports, functions grown too complex.</strong></p>

<p align="center">
  <a href="https://github.com/julianzh0/plimsoll/releases/latest"><img src="https://img.shields.io/github/v/release/julianzh0/plimsoll" alt="Release"></a>
  <a href="https://github.com/julianzh0/plimsoll/actions/workflows/ci.yml"><img src="https://github.com/julianzh0/plimsoll/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/julianzh0/plimsoll" alt="Go"></a>
  <a href="https://modelcontextprotocol.io"><img src="https://img.shields.io/badge/MCP-server-blue" alt="MCP server"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License: MIT"></a>
</p>

<p align="center"><img src=".github/how-it-works.svg" alt="Your agent changes code, asks plimsoll, and gets a few measured findings back, each with a way to check it" width="900"></p>

After a change, your agent asks plimsoll and gets a few findings back, each measured, each with its limit and a way
to check it. Here is a real answer for the last 30 commits of [Obtainium](https://github.com/ImranR98/Obtainium):

```text
plimsoll 0.6.0 status: base `HEAD~30` e9087ce099, head 35c3e7fea4
3 new, 1 grown, 0 resolved; 14 more in `report` only

1. [new, exact] `pubspec.yaml` names 1 runtime dependency that was not there at the base: `cronet_http` (47 now, 46 at the base).
2. [new, candidate] `lib/pages/add_app.dart` `runSearch` crossed the limit of 25 in this change: cognitive complexity 22 → 46 (+24).
   verify: read lines 388–627 (172 of them added by this change): it nests 7 levels deep at line 576; …
3. [grown, candidate] The ring of `lib/app_sources/apk4free.dart` has 77 files, 74 at the base.
   verify: grown through `lib/components/app_list_tile.dart` line 9 importing `lib/app_sources/github.dart` and 20 more, added by this change; …
…
```

## What it catches

- **New dependencies** a change slipped in, and whether a lockfile holds them.
- **Functions the change made hard to read:** deep nesting or pasted blocks, in 11 languages.
- **Wiring:** an import ring the change closed or grew, product code importing test code.
- **Process leftovers:** a plan or review file committed, a setting defined twice or never read.

## Why you can trust it

Every finding is measured, names its limit and where that limit comes from, and says how to check it yourself. plimsoll
only reads: no network, no telemetry, nothing written into your repository, nothing stored. Each mark was read blind
on real agent pull requests before it was allowed into the short answer, and marks that failed that test show only
in the full report. MIT licensed. It works next to your linters and CI, not instead of them. [Guarantees and their tests](docs/guarantees.md).

## Install

macOS and Linux, amd64 and arm64:

```sh
curl -fsSL https://raw.githubusercontent.com/julianzh0/plimsoll/main/install.sh | sh
```

The script downloads the binary of the latest [release](https://github.com/julianzh0/plimsoll/releases), checks its
SHA-256 checksum and puts it in `~/.local/bin` (`PLIMSOLL_INSTALL_DIR` changes the folder, `PLIMSOLL_VERSION` picks
a release). You can also download a binary and `checksums.txt` from the release page yourself, or build from source
with Go (see [Development](docs/development.md)).

Windows, amd64 and arm64, in PowerShell:

```powershell
irm https://raw.githubusercontent.com/julianzh0/plimsoll/main/install.ps1 | iex
```

The script does the same and puts `plimsoll.exe` in `%LOCALAPPDATA%\plimsoll`, which it adds to your user PATH
(`$env:PLIMSOLL_INSTALL_DIR` and `$env:PLIMSOLL_VERSION` work as above). The Windows build is checked by compiling it,
not by a Windows test run.

### Connect your agent

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

## Tools

| Tool | Returns |
|---|---|
| `status(repository, base?)` | at most five new or grown marks, one line that counts the marks shown in `report` only, one that counts what the change touched but did not create, one that counts added code without a use, the standing line, one that counts the standing items the repository acknowledges, the footprint and what was not analysed |
| `report(repository, base?, files?)` | everything measured, in a fixed layout, the touched items included. Standing items of one kind past five are counted by folder in the text, and `structuredContent` holds them all. With `files`, also the files that import each named file |
| `explain(repository, base?, mark)` | the evidence of one mark, computed again and capped |

Without `base`, a question compares with the default base the repository declares in `.plimsoll.toml`, and the
answer's first line says so. Every result carries the text in `content` and the same answer as `structuredContent`. An answer may shorten, never
hide: what it leaves out it counts ("3 more: `report` lists them"), and `report` holds every mark.

## Learn more

- [What plimsoll checks](docs/rules.md): every rule, what it measures and its limit, and your own limits, layout,
  ignored files and acknowledged items in `.plimsoll.toml`.
- [How each language is read](docs/languages.md): the eleven languages, how their imports and test code are found.
- [Guarantees and protocol](docs/guarantees.md): what plimsoll promises, with the tests that pin each promise.
- [Development](docs/development.md): building, testing and the project's structure.

## License

[MIT](LICENSE)

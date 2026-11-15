# Agent instructions

Project behaviour and commands are in `README.md`. These are the rules that are not obvious from the code.

## Before finishing

Run `make fix`, then `make qa`. New or changed tests must also pass `make flake TESTS=<pattern>`.

## Where code goes

- Packages by feature under `internal/`. No `util`, `common` or `types` package, no empty layer.
- `internal/gitrepo` is the only package that runs a program or opens a file of an analysed repository, and it
  only reads. `internal/mcpserver` is the only package that knows MCP, and the only one that imports the MCP Go SDK.
- A detector is one package. It imports `internal/answer` and `internal/snapshot` and nothing else of this
  module. It holds no language name and no limit: both reach it through the snapshot.
- Rules, language entries and limits are data in `internal/conventions`, returned by functions.
- Standard library only, except the official MCP Go SDK in `internal/mcpserver`. `git` is the one outside program. Do not add assertion or mocking libraries.

## Design

- Add an interface only for a real second implementation or an external boundary. The consumer defines it. Never add one just to mock something; test the real implementation.
- Write synchronous functions and let the caller start goroutines. Every goroutine needs an obvious end.
- Handle an error once: return it with context, or log it, not both.

## Tests

- Unit tests have no build tag. `//go:build integration` with `TestIntegration*` crosses a real boundary in one process. `//go:build e2e` with `TestE2E*` runs the real binary.
- Test behaviour through public functions. Report failures as `Func(input) = <got>, want <want>`.
- Wait on a condition with a deadline, and end everything a test starts in its own cleanup.

## References

- The functional and non-functional requirements are in `projects/requirements/requirements.md`. The `projects/` folder is local and not in Git.
- The full project structure is explained in `README.md`
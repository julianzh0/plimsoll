package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/gitrepo"
	"github.com/julianzh0/plimsoll/internal/mcpserver"
	"github.com/julianzh0/plimsoll/internal/parse"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

// questions composes the rules, the reader, the kept readings of the parser and the detectors.
func questions(environ []string, parser *parse.Parser) answer.Questions {
	rules := conventions.Default()
	reader := gitrepo.NewReader(environ)
	readings := snapshot.NewReadings(parser)
	wants := snapshot.WantsOf(rules)
	return answer.Questions{
		Take: func(ctx context.Context, repository, base string) (snapshot.Snapshot, error) {
			source, err := reader.Read(ctx, repository, base, wants)
			if err != nil {
				return snapshot.Snapshot{}, err
			}
			return snapshot.Build(source, rules, readings), nil
		},
		Detectors: detectors(),
		Version:   version,
	}
}

// help is what --help prints: plimsoll has no setting, so the two flags are all it takes.
const help = `plimsoll %s: an MCP server that tells a coding agent what its change did to a git repository.
It serves MCP on standard input and output; connect your agent to it, for example:
  claude mcp add plimsoll -- plimsoll
It takes no setting. --version prints the version, --help this text. https://github.com/julianzh0/plimsoll
`

// run prints the version or the help when asked to, and otherwise serves MCP on the standard streams until the input
// ends. Any other argument is refused, so a mistyped one never starts a server that waits silently.
func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, environ []string) int {
	switch {
	case len(args) == 0:
	case len(args) == 1 && args[0] == "--version":
		_, _ = fmt.Fprintf(stdout, "plimsoll %s\n", version)
		return 0
	case len(args) == 1 && (args[0] == "--help" || args[0] == "-h"):
		_, _ = fmt.Fprintf(stdout, help, version)
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "plimsoll: unknown arguments %q; plimsoll takes none, or one of --version and --help\n", args)
		return 2
	}
	if file, ok := stdin.(*os.File); ok {
		if info, err := file.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
			_, _ = fmt.Fprintln(stderr, "plimsoll: waiting for an MCP client on standard input; see plimsoll --help")
		}
	}
	parser, err := parse.New()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "plimsoll:", err)
		return 1
	}
	defer parser.Close()
	server := mcpserver.Server{
		Questions: questions(environ, parser),
		Version:   version,
		Log:       slog.New(slog.NewJSONHandler(stderr, nil)),
	}
	if err := server.Serve(ctx, stdin, stdout); err != nil {
		_, _ = fmt.Fprintln(stderr, "plimsoll:", err)
		return 1
	}
	return 0
}

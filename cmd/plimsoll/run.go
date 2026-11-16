package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/juzhxng/plimsoll/internal/answer"
	"github.com/juzhxng/plimsoll/internal/conventions"
	"github.com/juzhxng/plimsoll/internal/gitrepo"
	"github.com/juzhxng/plimsoll/internal/mcpserver"
	"github.com/juzhxng/plimsoll/internal/parse"
	"github.com/juzhxng/plimsoll/internal/snapshot"
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

// run serves MCP on the standard streams until the input ends.
func run(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, environ []string) int {
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

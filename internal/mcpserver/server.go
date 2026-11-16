// Package mcpserver answers the three questions over the Model Context Protocol on standard input and output,
// through the official MCP Go SDK: revision 2026-07-28 and the initialize handshake of earlier revisions.
package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/julianzh0/plimsoll/internal/answer"
)

// concurrentAnswers is how many questions are answered at the same time; the others wait for a free slot. Measured on
// nlohmann/json (299,000 code lines, 4,686 commits), 12 cores, 2026-11-16: a burst of 64 questions after the history is
// read gives 5.0 answers per second with 8 slots, 5.4 with 16, 4.8 with 32 and 5.3 with 64, while the peak memory of
// plimsoll and its git processes grows from 0.8 GB to 1.0, 2.1 and 3.8 GB; with 2 slots it is 1.8 per second. Reading
// the history is the heavy part (about 780 MB of git) and is shared by questions that need it at the same time, so 8
// slots keep a burst near 1 GB. Without a limit, 634 questions sent at once exhausted 32 GB.
const concurrentAnswers = 8

// listTTL is how long, in milliseconds, a client may keep the tool list: it changes only with a new version.
const listTTL = 300000

// Server answers MCP requests with the questions.
type Server struct {
	Questions answer.Questions
	Version   string
	Log       *slog.Logger
}

// Serve answers requests until the input ends or ctx ends. It returns nil in both cases.
func (s Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	server := mcp.NewServer(&mcp.Implementation{Name: "plimsoll", Version: s.Version}, &mcp.ServerOptions{
		// Tools only, and the list never changes while the process runs.
		Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}},
		SetCacheable: func(_ context.Context, _ mcp.Request, c *mcp.Cacheable) { c.TTLMs = listTTL },
	})
	slots := make(chan struct{}, concurrentAnswers)
	for _, q := range questions() {
		server.AddTool(toolOf(q), s.handler(q, slots))
	}
	err := server.Run(ctx, &mcp.IOTransport{Reader: io.NopCloser(in), Writer: nopCloser{out}})
	if err == nil || ctx.Err() != nil || errors.Is(err, io.EOF) || errors.Is(err, mcp.ErrConnectionClosed) {
		return nil
	}
	return err
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

// handler answers one tool call when one of the slots is free; a refused question is a tool result, never a protocol
// error.
func (s Server) handler(q question, slots chan struct{}) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		case <-ctx.Done():
			return refusal(&answer.Refusal{Sentence: "The question ended before plimsoll could start it."}), nil
		}
		given := map[string]json.RawMessage{}
		if len(req.Params.Arguments) > 0 {
			if err := json.Unmarshal(req.Params.Arguments, &given); err != nil {
				return refusal(err), nil
			}
		}
		values, items, err := arguments(given, q.arguments, q.list)
		if err != nil {
			return refusal(err), nil
		}
		a, err := q.ask(ctx, s.Questions, values, items)
		if err != nil {
			return refusal(err), nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s.Questions.Text(a)}}, StructuredContent: a}, nil
	}
}

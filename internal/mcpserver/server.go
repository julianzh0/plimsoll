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

	"github.com/juzhxng/plimsoll/internal/answer"
)

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
	for _, q := range questions() {
		server.AddTool(toolOf(q), s.handler(q))
	}
	err := server.Run(ctx, &mcp.IOTransport{Reader: io.NopCloser(in), Writer: nopCloser{out}})
	if err == nil || ctx.Err() != nil || errors.Is(err, io.EOF) || errors.Is(err, mcp.ErrConnectionClosed) {
		return nil
	}
	return err
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

// handler answers one tool call; a refused question is a tool result, never a protocol error.
func (s Server) handler(q question) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

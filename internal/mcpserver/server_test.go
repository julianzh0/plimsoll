package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/juzhxng/plimsoll/internal/answer"
	"github.com/juzhxng/plimsoll/internal/conventions"
	"github.com/juzhxng/plimsoll/internal/snapshot"
	"github.com/juzhxng/plimsoll/internal/snapshot/snapshottest"
)

func TestServeEndsWhenItsContextEnds(t *testing.T) {
	in, writer := io.Pipe()
	t.Cleanup(func() { _ = writer.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Server{Log: slog.New(slog.DiscardHandler)}.Serve(ctx, in, io.Discard) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve after cancel = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return within 5 s of its context ending, with input still open")
	}
}

func TestAQuestionWaitsForAFreeAnswerSlot(t *testing.T) {
	taken := 0
	s := Server{Questions: answer.Questions{
		Take: func(context.Context, string, string) (snapshot.Snapshot, error) {
			taken++
			return snapshot.Build(snapshottest.Files(nil, map[string]string{"src/a.ts": "a\n"}), conventions.Default(), snapshottest.Readings(t)), nil
		},
		Version: "0.1.0",
	}}
	request := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "status", Arguments: json.RawMessage(`{"repository":"/repository","base":"main"}`)}}
	slots := make(chan struct{}, concurrentAnswers)
	for range concurrentAnswers {
		slots <- struct{}{}
	}
	// Every slot is taken: a question whose client gives up while it waits is refused, and nothing is read.
	gone, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := s.handler(questions()[0], slots)(gone, request)
	if err != nil || !result.IsError || taken != 0 {
		t.Errorf("a question with every slot taken = %+v, %v, %d snapshots taken; want a refusal and none taken", result, err, taken)
	}
	// One slot frees up: the question is answered, and it gives its slot back.
	<-slots
	result, err = s.handler(questions()[0], slots)(context.Background(), request)
	if err != nil || result.IsError || taken != 1 || len(slots) != concurrentAnswers-1 {
		t.Errorf("a question with a free slot = %+v, %v, %d taken, %d slots in use; want an answer, 1 taken, %d in use", result, err, taken, len(slots), concurrentAnswers-1)
	}
}

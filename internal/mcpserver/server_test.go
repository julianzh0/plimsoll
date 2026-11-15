package mcpserver

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
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

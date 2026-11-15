//go:build e2e

package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/juzhxng/plimsoll/internal/gitrepo/gitrepotest"
)

const (
	binaryKey     = "E2E_BINARY"
	buildErrorKey = "E2E_BUILD_ERROR"
	runBound      = 15 * time.Second
)

// TestMain builds the binary once and publishes its path, or the build failure, to the tests.
func TestMain(m *testing.M) {
	directory, err := os.MkdirTemp("", "plimsoll-e2e-")
	if err != nil {
		_ = os.Setenv(buildErrorKey, err.Error())
		os.Exit(m.Run())
	}
	binary := filepath.Join(directory, "plimsoll")
	build := []string{"build", "-race", "-buildvcs=false", "-o", binary}
	// make e2e collects coverage. go test sets GOCOVERDIR to a folder of its own, so the folder comes under another
	// name, and the program, which inherits this environment, writes its coverage there.
	if coverage := os.Getenv("PLIMSOLL_COVERDIR"); coverage != "" {
		build = append(build, "-cover", "-coverpkg=github.com/juzhxng/plimsoll/...")
		_ = os.Setenv("GOCOVERDIR", coverage)
	}
	if output, err := exec.Command("go", append(build, ".")...).CombinedOutput(); err != nil {
		_ = os.Setenv(buildErrorKey, err.Error()+"\n"+string(output))
	}
	_ = os.Setenv(binaryKey, binary)
	code := m.Run()
	_ = os.RemoveAll(directory)
	os.Exit(code)
}

func builtBinary(t *testing.T) string {
	t.Helper()
	if message := os.Getenv(buildErrorKey); message != "" {
		t.Fatalf("go build: %s", message)
	}
	return os.Getenv(binaryKey)
}

// runBinary runs the program with the given input and arguments, bounded to 15 seconds.
func runBinary(t *testing.T, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), runBound)
	t.Cleanup(cancel)
	command := exec.CommandContext(ctx, builtBinary(t), args...)
	command.Stdin = strings.NewReader(stdin)
	var out, errOut bytes.Buffer
	command.Stdout, command.Stderr = &out, &errOut
	err := command.Run()
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		return out.String(), errOut.String(), exit.ExitCode()
	}
	if err != nil {
		t.Fatalf("running plimsoll: %v", err)
	}
	return out.String(), errOut.String(), 0
}

func TestE2EProgramAnswersAndEndsOnASignal(t *testing.T) {
	repo := gitrepotest.New(t, gitrepotest.Commit{Message: "base", Files: map[string]string{"src/a.ts": "a\n"}})
	ctx, cancel := context.WithTimeout(context.Background(), runBound)
	t.Cleanup(cancel)
	command := exec.CommandContext(ctx, builtBinary(t))
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdoutPipe, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close() })
	request := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"status","arguments":{"repository":"` + repo.Dir +
		`","base":"HEAD"},"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}` + "\n"
	if _, err := io.WriteString(stdin, request); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(stdoutPipe).ReadString('\n')
	if err != nil || !strings.Contains(line, `"question":"status"`) || !strings.HasPrefix(line, `{"jsonrpc":"2.0","id":1,"result":`) {
		t.Fatalf("the program's first line = %q, %v; want the status result", line, err)
	}
	if err := command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Errorf("the program ended with %v after SIGTERM, want exit 0", err)
	}
}

func TestE2EEndOfInputEndsTheProcess(t *testing.T) {
	stdout, stderr, code := runBinary(t, "")
	if code != 0 || stdout != "" {
		t.Errorf("plimsoll with no input = %d, %q, %q; want 0 and no output", code, stdout, stderr)
	}
}

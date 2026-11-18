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

	"github.com/julianzh0/plimsoll/internal/gitrepo/gitrepotest"
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
		build = append(build, "-cover", "-coverpkg=github.com/julianzh0/plimsoll/...")
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

func TestE2EArgumentsOnlyPrint(t *testing.T) {
	cases := []struct {
		args       []string
		code       int
		stdout     string // a part the output must hold
		stderrPart string
	}{
		{args: []string{"--version"}, code: 0, stdout: "plimsoll " + version + "\n"},
		{args: []string{"--help"}, code: 0, stdout: "claude mcp add plimsoll -- plimsoll"},
		{args: []string{"-h"}, code: 0, stdout: "--version"},
		{args: []string{"serve"}, code: 2, stderrPart: "--help"},
		{args: []string{"--version", "--help"}, code: 2, stderrPart: "--help"},
	}
	for _, c := range cases {
		stdout, stderr, code := runBinary(t, "", c.args...)
		if code != c.code || !strings.Contains(stdout, c.stdout) || !strings.Contains(stderr, c.stderrPart) || (c.code == 2 && stdout != "") {
			t.Errorf("plimsoll %v = %d, %q, %q; want %d, output with %q, errors with %q",
				c.args, code, stdout, stderr, c.code, c.stdout, c.stderrPart)
		}
	}
	if stdout, _, _ := runBinary(t, "", "--version"); stdout != "plimsoll "+version+"\n" {
		t.Errorf("plimsoll --version = %q, want exactly %q", stdout, "plimsoll "+version+"\n")
	}
}

func TestE2EATerminalAsInputGetsAHintAndStillServes(t *testing.T) {
	devNull, err := os.Open(os.DevNull) // a character device, as a terminal is
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = devNull.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), runBound)
	t.Cleanup(cancel)
	command := exec.CommandContext(ctx, builtBinary(t))
	command.Stdin = devNull
	var out, errOut bytes.Buffer
	command.Stdout, command.Stderr = &out, &errOut
	if err := command.Run(); err != nil || out.String() != "" || !strings.Contains(errOut.String(), "waiting for an MCP client") {
		t.Errorf("plimsoll with a terminal as input = %v, %q, %q; want 0, no output and the hint on errors", err, out.String(), errOut.String())
	}
	_, stderr, _ := runBinary(t, "")
	if strings.Contains(stderr, "waiting for an MCP client") {
		t.Errorf("plimsoll with a pipe as input wrote the hint: %q", stderr)
	}
}

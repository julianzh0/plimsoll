// Package gitrepo is the only code that runs git or reads a file of an analysed repository. It only reads.
package gitrepo

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
)

// gitEnvironment is the environment of every git command: the reader's own, without any GIT_ variable that could
// point git at another repository, plus the settings that keep git from writing, fetching or asking.
func gitEnvironment(environ []string) []string {
	env := make([]string, 0, len(environ)+4)
	for _, entry := range environ {
		if !strings.HasPrefix(entry, "GIT_") {
			env = append(env, entry)
		}
	}
	return append(env, "GIT_OPTIONAL_LOCKS=0", "GIT_NO_LAZY_FETCH=1", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
}

// gitError is a git command that exited with an error: its exit code and the first line it wrote to stderr.
type gitError struct {
	code int
	line string
}

func (e *gitError) Error() string {
	return "git: " + e.line
}

// git runs one git command in folder and returns its standard output. The file monitor is switched off, because it
// would start a daemon that writes into .git.
func git(ctx context.Context, env []string, folder string, input []byte, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", folder, "-c", "core.fsmonitor=false"}, args...)...)
	command.Env = env
	if input != nil {
		command.Stdin = bytes.NewReader(input)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		line, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n")
		return output, &gitError{code: exit.ExitCode(), line: line}
	}
	return output, err
}

// emptyTree is the name of the empty tree in a repository's object format.
func emptyTree(format string) string {
	if format == "sha256" {
		return "6ef19b41225c5369f1c104d45d8d85efa9b057b53b14b4b9b939dd74decc5321"
	}
	return "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
}

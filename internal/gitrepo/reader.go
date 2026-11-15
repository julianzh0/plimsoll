package gitrepo

import (
	"context"
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/juzhxng/plimsoll/internal/snapshot"
)

// Reader reads a repository through git plumbing without changing it. It answers one question at a time: it
// starts no goroutine and holds no lock. It keeps the parsed history of each repository for its head commit, which
// is a pure function of that commit, so a kept history equals a fresh read.
// NOTE: one full read per new head commit (2.6 s on a repository of 981 commits). Upgrade: read only
// <old head>..<new head> when the old head is an ancestor.
type Reader struct {
	env       []string
	mu        sync.Mutex             // guards histories: questions may be answered at the same time
	histories map[string]keptHistory // by top folder
}

type keptHistory struct {
	head    string
	commits []snapshot.Commit
}

// NewReader returns a reader that starts git with the given process environment.
func NewReader(environ []string) *Reader {
	return &Reader{env: gitEnvironment(environ), histories: map[string]keptHistory{}}
}

// Read reads the tracked files at the base and in the working tree, the changes between them and the state of the
// working tree. A question that cannot be answered returns a *snapshot.Refusal and no source.
func (r *Reader) Read(ctx context.Context, repository, base string, wants snapshot.Wants) (snapshot.Source, error) {
	topOutput, err := git(ctx, r.env, repository, nil, "rev-parse", "--show-toplevel", "--show-object-format")
	if err != nil {
		return snapshot.Source{}, refusal(err, snapshot.NotARepository, "")
	}
	top, format, _ := strings.Cut(strings.TrimSpace(string(topOutput)), "\n")
	// Attributes are read from the empty tree, so no content filter, such as git-lfs's clean filter, runs and writes
	// into the repository while the working tree is compared.
	noAttributes := []string{"--attr-source=" + emptyTree(format), "-c", "core.attributesFile=/dev/null"}
	run := func(input []byte, args ...string) ([]byte, error) {
		return git(ctx, r.env, top, input, append(slices.Clone(noAttributes), args...)...)
	}

	head, err := run(nil, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil {
		return snapshot.Source{}, refusal(err, snapshot.NoCommit, "")
	}
	asked, err := run(nil, "rev-parse", "--verify", "--quiet", "--end-of-options", base+"^{commit}")
	if err != nil {
		return snapshot.Source{}, refusal(err, snapshot.UnknownBase, base)
	}
	source := snapshot.Source{AskedBase: base, Head: strings.TrimSpace(string(head))}
	askedCommit := strings.TrimSpace(string(asked))
	mergeBase, err := run(nil, "merge-base", askedCommit, "HEAD")
	if err != nil {
		return snapshot.Source{}, refusal(err, snapshot.NoCommonHistory, base)
	}
	source.Base = strings.TrimSpace(string(mergeBase))
	source.MergeBase = source.Base != askedCommit

	status, err := run(nil, "status", "--porcelain=v2", "-z", "--untracked-files=all", "--ignore-submodules=all")
	if err != nil {
		return snapshot.Source{}, gitFailed(err)
	}
	source.Modified, source.Untracked = parseStatus(status)

	if source.Changes, err = changes(run, source.Base); err != nil {
		return snapshot.Source{}, err
	}
	if source.Now, err = now(run, top, wants, &source); err != nil {
		return snapshot.Source{}, err
	}
	if source.Before, err = before(run, source.Base, source.Changes, wants); err != nil {
		return snapshot.Source{}, err
	}
	since, err := run(nil, "rev-list", source.Base+"..HEAD")
	if err != nil {
		return snapshot.Source{}, gitFailed(err)
	}
	source.Since = strings.Fields(string(since))
	if source.Commits, err = r.history(run, top, source.Head); err != nil {
		return snapshot.Source{}, err
	}
	return source, nil
}

// history returns the whole non-merge history, newest first, read once per head commit.
func (r *Reader) history(run runner, top, head string) ([]snapshot.Commit, error) {
	r.mu.Lock()
	kept, found := r.histories[top]
	r.mu.Unlock()
	if found && kept.head == head {
		return kept.commits, nil
	}
	output, err := run(nil, "log", "--no-merges", "--no-show-signature", "--numstat", "-z", "-M50%", "-l1000", "--no-textconv", "--no-ext-diff", "--format=%x01%H")
	if err != nil {
		return nil, gitFailed(err)
	}
	commits, err := parseLog(output)
	if err != nil {
		return nil, gitFailed(err)
	}
	r.mu.Lock()
	r.histories[top] = keptHistory{head: head, commits: commits}
	r.mu.Unlock()
	return commits, nil
}

type runner func(input []byte, args ...string) ([]byte, error)

func changes(run runner, base string) ([]snapshot.Change, error) {
	output, err := run(nil, "diff-index", "--numstat", "-z", "-M50%", "-l1000", "--no-textconv", "--no-ext-diff", base)
	if err != nil {
		return nil, gitFailed(err)
	}
	records := fields(output)
	var changes []snapshot.Change
	for i := 0; i < len(records); {
		var change snapshot.Change
		if change, i, err = parseNumstat(records, i); err != nil {
			return nil, gitFailed(err)
		}
		changes = append(changes, change)
	}
	return changes, nil
}

// now lists the tracked files and reads the wanted ones from the working tree, never outside the top folder.
func now(run runner, top string, wants snapshot.Wants, source *snapshot.Source) ([]snapshot.Entry, error) {
	output, err := run(nil, "ls-files", "-s", "-z")
	if err != nil {
		return nil, gitFailed(err)
	}
	root, err := os.OpenRoot(top)
	if err != nil {
		return nil, gitFailed(err)
	}
	defer func() { _ = root.Close() }()
	var entries []snapshot.Entry
	for _, tracked := range parseTracked(output, 1) {
		if tracked.mode == modeSubmodule {
			source.Submodules = append(source.Submodules, tracked.path)
			continue
		}
		if !regular(tracked.mode) {
			continue
		}
		entry := snapshot.Entry{Path: tracked.path}
		if wants.Path(tracked.path) {
			content, readErr := readFile(root, tracked.path, wants)
			switch {
			case errors.Is(readErr, os.ErrNotExist):
				continue // tracked, but deleted in the working tree
			case readErr != nil:
				source.Unreadable = append(source.Unreadable, tracked.path)
			case content == nil:
				entry.Binary = true
			default:
				entry.Content = content
			}
		} else if _, statErr := root.Lstat(tracked.path); errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// readFile reads a file through the root. It returns nil content for a binary file and an empty slice for an
// empty one.
func readFile(root *os.Root, path string, wants snapshot.Wants) ([]byte, error) {
	file, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	head := make([]byte, wants.SniffBytes)
	n, err := io.ReadFull(file, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	if wants.Binary(head[:n]) {
		return nil, nil
	}
	rest, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	return append(head[:n:n], rest...), nil
}

// before lists the tracked files at the base and reads, in one batch, only the wanted files that changed.
func before(run runner, base string, changes []snapshot.Change, wants snapshot.Wants) ([]snapshot.Entry, error) {
	output, err := run(nil, "ls-tree", "-r", "-z", "--full-tree", base)
	if err != nil {
		return nil, gitFailed(err)
	}
	changed := map[string]bool{}
	for _, change := range changes {
		changed[change.Path] = true
		if change.From != "" {
			changed[change.From] = true
		}
	}
	var entries []snapshot.Entry
	var wanted []string
	objects := map[string]string{}
	for _, tracked := range parseTracked(output, 2) {
		if !regular(tracked.mode) {
			continue
		}
		entries = append(entries, snapshot.Entry{Path: tracked.path})
		if changed[tracked.path] && wants.Path(tracked.path) {
			wanted = append(wanted, tracked.object)
			objects[tracked.path] = tracked.object
		}
	}
	if len(wanted) == 0 {
		return entries, nil
	}
	slices.Sort(wanted)
	batch, err := run([]byte(strings.Join(slices.Compact(wanted), "\n")+"\n"), "cat-file", "--batch")
	if err != nil {
		return nil, gitFailed(err)
	}
	contents, err := parseBatch(batch)
	if err != nil {
		return nil, gitFailed(err)
	}
	for i := range entries {
		object, found := objects[entries[i].Path]
		if !found {
			continue
		}
		content := contents[object]
		if wants.Binary(content[:min(len(content), wants.SniffBytes)]) {
			entries[i].Binary = true
			continue
		}
		entries[i].Content = content
	}
	return entries, nil
}

// refusal turns git's answer to the command that decides the kind into a refusal of that kind. Only exit codes 1
// ("no such thing") and 128 ("not a repository") are git's answer; any other failure, such as 129 for an option an
// old git does not know, is a git failure with git's own message.
func refusal(err error, kind snapshot.RefusalKind, detail string) error {
	if failure, ok := errors.AsType[*gitError](err); !ok || failure.code != 1 && failure.code != 128 {
		return gitFailed(err)
	}
	return &snapshot.Refusal{Kind: kind, Detail: detail}
}

// gitFailed is a refusal for git that could not run or failed: its first error line is the detail.
func gitFailed(err error) error {
	if failure, ok := errors.AsType[*gitError](err); ok {
		return &snapshot.Refusal{Kind: snapshot.GitFailed, Detail: failure.line}
	}
	return &snapshot.Refusal{Kind: snapshot.GitFailed, Detail: err.Error()}
}

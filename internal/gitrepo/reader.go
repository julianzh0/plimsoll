package gitrepo

import (
	"context"
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/julianzh0/plimsoll/internal/snapshot"
)

// Reader reads a repository through git plumbing without changing it. It starts no goroutine. It keeps the parsed
// history of each repository for its head commit, which is a pure function of that commit, so a kept history equals a
// fresh read; questions that need the same history at the same time share one read of it, the heaviest work of a
// question (about 780 MB of git on a repository of 4,686 commits).
// NOTE: one full read per new head commit (2.6 s on a repository of 981 commits). Upgrade: read only
// <old head>..<new head> when the old head is an ancestor.
type Reader struct {
	env       []string
	mu        sync.Mutex              // guards histories: questions may be answered at the same time
	histories map[string]*keptHistory // by top folder
}

// keptHistory is the history of one head commit: read once, and ready when read is closed. cancelled is a read that
// failed because the question that ran it was cancelled.
type keptHistory struct {
	head      string
	read      chan struct{}
	commits   []snapshot.Commit
	err       error
	cancelled bool
}

// NewReader returns a reader that starts git with the given process environment.
func NewReader(environ []string) *Reader {
	return &Reader{env: gitEnvironment(environ), histories: map[string]*keptHistory{}}
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

	missing, err := missingContent(run)
	if err != nil {
		return snapshot.Source{}, err
	}
	exclude := excluding(missing)
	if source.Changes, err = changes(run, source.Base, exclude); err != nil {
		return snapshot.Source{}, err
	}
	if source.Now, err = now(run, top, wants, &source); err != nil {
		return snapshot.Source{}, err
	}
	var absent []string
	if source.Before, absent, err = before(run, source.Base, source.Changes, wants); err != nil {
		return snapshot.Source{}, err
	}
	source.Missing = slices.Compact(slices.Sorted(slices.Values(append(missing, absent...))))
	since, err := run(nil, "rev-list", source.Base+"..HEAD")
	if err != nil {
		return snapshot.Source{}, gitFailed(err)
	}
	source.Since = strings.Fields(string(since))
	if source.Commits, err = r.history(ctx, run, top, source.Head, exclude); err != nil {
		return snapshot.Source{}, err
	}
	return source, nil
}

// history returns the whole non-merge history, newest first, read once per head commit, without the excluded paths. A
// question that comes while that read runs waits for it, or until it is cancelled itself. A failed read is not kept;
// when it failed only because the question that ran it was cancelled, a waiting question reads it again.
func (r *Reader) history(ctx context.Context, run runner, top, head string, exclude []string) ([]snapshot.Commit, error) {
	for {
		r.mu.Lock()
		kept, found := r.histories[top]
		if found && kept.head == head {
			r.mu.Unlock()
			select {
			case <-kept.read:
			case <-ctx.Done():
				return nil, gitFailed(ctx.Err())
			}
			if kept.cancelled && ctx.Err() == nil {
				continue
			}
			return kept.commits, kept.err
		}
		kept = &keptHistory{head: head, read: make(chan struct{})}
		r.histories[top] = kept
		r.mu.Unlock()
		commits, err := readHistory(run, exclude)
		r.mu.Lock()
		kept.commits, kept.err, kept.cancelled = commits, err, err != nil && ctx.Err() != nil
		if err != nil && r.histories[top] == kept {
			delete(r.histories, top) // before the waiters wake, so one that reads again starts a new read
		}
		r.mu.Unlock()
		close(kept.read)
		return commits, err
	}
}

// readHistory runs git for the whole non-merge history, without the excluded paths.
func readHistory(run runner, exclude []string) ([]snapshot.Commit, error) {
	output, err := run(nil, append([]string{"log", "--no-merges", "--no-show-signature", "--numstat", "-z", "-M50%", "-l1000", "--no-textconv", "--no-ext-diff", "--format=%x01%H"}, exclude...)...)
	if err != nil {
		return nil, gitFailed(err)
	}
	commits, err := parseLog(output)
	if err != nil {
		return nil, gitFailed(err)
	}
	if len(exclude) > 0 {
		// A path left out also leaves out the commits that changed nothing else; they stay, with no files, so the
		// history keeps its length and every window over it stays the same.
		ids, err := run(nil, "rev-list", "--no-merges", "HEAD")
		if err != nil {
			return nil, gitFailed(err)
		}
		commits = withEveryCommit(strings.Fields(string(ids)), commits)
	}
	return commits, nil
}

type runner func(input []byte, args ...string) ([]byte, error)

func changes(run runner, base string, exclude []string) ([]snapshot.Change, error) {
	output, err := run(nil, append([]string{"diff-index", "--numstat", "-z", "-M50%", "-l1000", "--no-textconv", "--no-ext-diff", base}, exclude...)...)
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

// before lists the tracked files at the base and reads, in one batch, only the wanted files that changed. absent are
// the wanted files whose content a partial clone lacks.
func before(run runner, base string, changes []snapshot.Change, wants snapshot.Wants) (entries []snapshot.Entry, absent []string, err error) {
	output, err := run(nil, "ls-tree", "-r", "-z", "--full-tree", base)
	if err != nil {
		return nil, nil, gitFailed(err)
	}
	changed := map[string]bool{}
	for _, change := range changes {
		changed[change.Path] = true
		if change.From != "" {
			changed[change.From] = true
		}
	}
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
		return entries, nil, nil
	}
	slices.Sort(wanted)
	batch, err := run([]byte(strings.Join(slices.Compact(wanted), "\n")+"\n"), "cat-file", "--batch")
	if err != nil {
		return nil, nil, gitFailed(err)
	}
	contents, err := parseBatch(batch)
	if err != nil {
		return nil, nil, gitFailed(err)
	}
	for i := range entries {
		object, found := objects[entries[i].Path]
		if !found {
			continue
		}
		content, present := contents[object]
		if !present {
			absent = append(absent, entries[i].Path)
			continue
		}
		if wants.Binary(content[:min(len(content), wants.SniffBytes)]) {
			entries[i].Binary = true
			continue
		}
		entries[i].Content = content
	}
	return entries, absent, nil
}

// missingContent finds, in a partial clone, the paths whose content some commit of the head's history lacks: the
// objects git reports missing, named by the raw log, which needs no content. A full clone has none. Nothing is
// fetched: lazy fetching is off in every git command.
// NOTE: a content that only a merge brings in is not looked for, so a base that is such a merge can still be
// refused. Upgrade: read the raw log with -m.
func missingContent(run runner) ([]string, error) {
	promisors, err := run(nil, "config", "--get-regexp", `^remote\..*\.promisor$`)
	if failure, ok := errors.AsType[*gitError](err); ok && failure.code == 1 {
		return nil, nil // no promisor remote: a full clone
	}
	if err != nil {
		return nil, gitFailed(err)
	}
	if !strings.Contains(string(promisors), " true") {
		return nil, nil
	}
	objects, err := run(nil, "rev-list", "--objects", "--missing=print", "HEAD")
	if err != nil {
		return nil, gitFailed(err)
	}
	lost := map[string]bool{}
	for line := range strings.Lines(string(objects)) {
		if oid, isMissing := strings.CutPrefix(strings.TrimSpace(line), "?"); isMissing {
			lost[oid] = true
		}
	}
	if len(lost) == 0 {
		return nil, nil
	}
	raw, err := run(nil, "log", "--raw", "--no-abbrev", "--no-renames", "-z", "--format=%x01%H", "HEAD")
	if err != nil {
		return nil, gitFailed(err)
	}
	return pathsOfObjects(raw, lost), nil
}

// withEveryCommit is the commits in the order of ids, each one read where it was, else with no files.
func withEveryCommit(ids []string, read []snapshot.Commit) []snapshot.Commit {
	byID := make(map[string]snapshot.Commit, len(read))
	for _, c := range read {
		byID[c.ID] = c
	}
	all := make([]snapshot.Commit, 0, len(ids))
	for _, id := range ids {
		c, found := byID[id]
		if !found {
			c = snapshot.Commit{ID: id}
		}
		all = append(all, c)
	}
	return all
}

// excluding is the pathspec that leaves the paths out, or nil when there are none. A path's glob characters are
// escaped: the exclude magic cannot be combined with the literal one.
func excluding(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	spec := []string{"--", "."}
	escape := strings.NewReplacer(`\`, `\\`, "*", `\*`, "?", `\?`, "[", `\[`)
	for _, p := range paths {
		spec = append(spec, ":(exclude)"+escape.Replace(p))
	}
	return spec
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

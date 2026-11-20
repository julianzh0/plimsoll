package gitrepo

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/snapshot"
)

// logOptions are the options of every history command: fixed on the command line, so that a commit's record depends
// on nothing but the commit, the parents git reports for it, the excluded paths and the attributes git reads (see
// view). -M50% is git's rename similarity; -l1000 is git's rename-detection limit, the most candidates it compares in
// one commit, not a count of commits.
func logOptions() []string {
	return []string{"--no-merges", "--no-show-signature", "--root", "--numstat", "-z", "-M50%", "-l1000",
		"--diff-algorithm=myers", "--no-textconv", "--no-ext-diff", "--format=%x01%H"}
}

// repoHistory is what a reader keeps of one repository: the commit records of its most recent heads and the reads in
// flight. records is never changed once published; a read that adds records publishes a new map.
type repoHistory struct {
	view    string                     // the excluded paths, the shallow boundary and info/attributes the records were read under
	records map[string]snapshot.Commit // by record key: the commit id and the parents git listed for it
	recent  []string                   // the heads of the most recent published reads, newest first
	byHead  map[string]keptHistory     // the published reads of the recent heads
	reading map[string]*readInFlight
}

// recordKey identifies a commit's record: its id and the parents git lists for it, space-separated (a shallow
// boundary has none), so a commit that gains a parent is read again.
func recordKey(id, parents string) string { return id + " " + parents }

// keptHistory is the history of one head: its non-merge commits, newest first, and the parents of every commit it
// reaches, merges included.
type keptHistory struct {
	commits []snapshot.Commit
	parents map[string][]string
	keys    []string // the record key of each commit
}

// readInFlight is one read of a head: ready when read is closed. cancelled is a read that failed because the question
// that ran it was cancelled.
type readInFlight struct {
	read      chan struct{}
	history   keptHistory
	err       error
	cancelled bool
}

// keptHeads is how many heads' records a reader keeps per repository: two, so that questions alternating between two
// heads read neither again.
const keptHeads = 2

// history returns the non-merge history of a head, newest first, and the parents of every commit it reaches. Commits
// already read under the same parents and view are taken from the kept records; only the others are read. One read
// of a head runs at a time: a question that comes while it runs waits for it, or until it is cancelled itself. A read
// publishes its head's complete record set only when it succeeds, under the reader's lock, into the records as they
// are then; a failed or cancelled read publishes nothing, and a waiting question then reads itself.
func (r *Reader) history(ctx context.Context, run runner, top, head string, exclude []string) (keptHistory, error) {
	for {
		view, err := historyView(run, top, exclude)
		if err != nil {
			return keptHistory{}, err
		}
		r.mu.Lock()
		repo := r.histories[top]
		if repo == nil || repo.view != view {
			repo = &repoHistory{view: view, records: map[string]snapshot.Commit{}, byHead: map[string]keptHistory{}, reading: map[string]*readInFlight{}}
			r.histories[top] = repo
		}
		if kept, found := repo.byHead[head]; found {
			r.mu.Unlock()
			return kept, nil
		}
		if inFlight, found := repo.reading[head]; found {
			r.mu.Unlock()
			select {
			case <-inFlight.read:
			case <-ctx.Done():
				return keptHistory{}, gitFailed(ctx.Err())
			}
			if inFlight.cancelled && ctx.Err() == nil {
				continue
			}
			return inFlight.history, inFlight.err
		}
		inFlight := &readInFlight{read: make(chan struct{})}
		repo.reading[head] = inFlight
		records := repo.records
		r.mu.Unlock()

		kept, read, err := readHistory(run, head, exclude, records)
		if err == nil {
			if after, viewErr := historyView(run, top, exclude); viewErr != nil || after != view {
				read = nil // the boundary or the attributes changed during the read: keep nothing
			}
		}
		r.mu.Lock()
		inFlight.history, inFlight.err, inFlight.cancelled = kept, err, err != nil && ctx.Err() != nil
		delete(repo.reading, head)
		if err == nil && read != nil && r.histories[top] == repo {
			repo.publish(head, kept, read)
		}
		r.mu.Unlock()
		close(inFlight.read)
		return kept, err
	}
}

// publish adds a head's complete record set to the kept records as they are now, makes it the most recent head and
// keeps the records of the most recent heads only. It runs under the reader's lock.
func (repo *repoHistory) publish(head string, kept keptHistory, read map[string]snapshot.Commit) {
	repo.recent = slices.Insert(slices.DeleteFunc(repo.recent, func(h string) bool { return h == head }), 0, head)
	repo.byHead[head] = kept
	if len(repo.recent) > keptHeads {
		for _, gone := range repo.recent[keptHeads:] {
			delete(repo.byHead, gone)
		}
		repo.recent = repo.recent[:keptHeads]
	}
	records := make(map[string]snapshot.Commit, len(read))
	for _, h := range repo.recent {
		for _, key := range repo.byHead[h].keys {
			if c, found := read[key]; found {
				records[key] = c
			} else if c, found := repo.records[key]; found {
				records[key] = c
			}
		}
	}
	repo.records = records
}

// historyView is what a commit's record depends on besides the commit and its parents: the excluded paths, the
// shallow boundary (a deepened clone gives its old boundary commits parents) and info/attributes, the one attributes
// file the fixed attribute source does not replace.
func historyView(run runner, top string, exclude []string) (string, error) {
	paths, err := run(nil, "rev-parse", "--git-path", "shallow", "--git-path", "info/attributes")
	if err != nil {
		return "", gitFailed(err)
	}
	var view strings.Builder
	view.WriteString(strings.Join(exclude, "\x00"))
	for p := range strings.FieldsSeq(string(paths)) {
		if !filepath.IsAbs(p) {
			p = filepath.Join(top, p)
		}
		content := readGitFile(p)
		view.WriteString("\x01")
		view.Write(content)
	}
	return view.String(), nil
}

// readGitFile reads a file git keeps for the repository, or nothing when it is missing: through a root opened on its
// folder, as every file of a repository is read.
func readGitFile(p string) []byte {
	root, err := os.OpenRoot(filepath.Dir(p))
	if err != nil {
		return nil
	}
	defer func() { _ = root.Close() }()
	content, _ := root.ReadFile(filepath.Base(p))
	return content
}

// readHistory reads a head's history: the non-merge commits it reaches in git's order, with their parents, and the
// parents of every commit, merges included. A commit whose record the kept records hold under the same parents is
// taken from there; the others are read in one git log, all of them when nothing is kept. read holds every record of
// the head, new and reused.
func readHistory(run runner, head string, exclude []string, kept map[string]snapshot.Commit) (keptHistory, map[string]snapshot.Commit, error) {
	listing, err := run(nil, "rev-list", "--no-merges", "--parents", head)
	if err != nil {
		return keptHistory{}, nil, gitFailed(err)
	}
	graph, err := run(nil, "rev-list", "--parents", head)
	if err != nil {
		return keptHistory{}, nil, gitFailed(err)
	}
	type listed struct{ id, parents string }
	var ids []listed
	var unknown []string
	for line := range strings.Lines(string(listing)) {
		id, parents, _ := strings.Cut(strings.TrimSpace(line), " ")
		if id == "" {
			continue
		}
		ids = append(ids, listed{id, parents})
		if _, found := kept[recordKey(id, parents)]; !found {
			unknown = append(unknown, id)
		}
	}
	readNow := map[string]snapshot.Commit{}
	if len(unknown) > 0 {
		args := append([]string{"log"}, logOptions()...)
		var input []byte
		if len(unknown) == len(ids) {
			args = append(args, head) // nothing kept: one walk, as a fresh read
		} else {
			args, input = append(args, "--no-walk=unsorted", "--stdin"), []byte(strings.Join(unknown, "\n")+"\n")
		}
		output, err := run(input, append(args, exclude...)...)
		if err != nil {
			return keptHistory{}, nil, gitFailed(err)
		}
		commits, err := parseLog(output)
		if err != nil {
			return keptHistory{}, nil, gitFailed(err)
		}
		for _, c := range commits {
			readNow[c.ID] = c
		}
	}
	history := keptHistory{commits: make([]snapshot.Commit, 0, len(ids)), parents: map[string][]string{}, keys: make([]string, 0, len(ids))}
	read := make(map[string]snapshot.Commit, len(ids))
	for _, l := range ids {
		key := recordKey(l.id, l.parents)
		c, found := kept[key]
		if !found {
			// A commit the excluded paths leave without changes is not in the log's output; it stays, with no files,
			// so the history keeps its length and every window over it stays the same.
			if c, found = readNow[l.id]; !found {
				c = snapshot.Commit{ID: l.id}
			}
		}
		read[key] = c
		history.commits, history.keys = append(history.commits, c), append(history.keys, key)
	}
	for line := range strings.Lines(string(graph)) {
		if fields := strings.Fields(line); len(fields) > 0 {
			history.parents[fields[0]] = fields[1:]
		}
	}
	return history, read, nil
}

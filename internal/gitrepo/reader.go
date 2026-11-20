package gitrepo

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/julianzh0/plimsoll/internal/snapshot"
)

// Reader reads a repository through git plumbing without changing it. It starts no goroutine. It keeps the parsed
// records of each repository's commits for its most recent heads: a commit's record is a pure function of the
// commit, the parents git reports for it and the view it was read under, so a kept history equals a fresh read, and a
// new head reads only the commits it adds. Questions that need the same head's history at the same time share one
// read of it, the heaviest work of a question (about 780 MB of git on a repository of 4,686 commits).
type Reader struct {
	env       []string
	mu        sync.Mutex              // guards histories: questions may be answered at the same time
	histories map[string]*repoHistory // by top folder
}

// NewReader returns a reader that starts git with the given process environment.
func NewReader(environ []string) *Reader {
	return &Reader{env: gitEnvironment(environ), histories: map[string]*repoHistory{}}
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
	declared := base == ""
	var askedCommit string
	if declared {
		if base, askedCommit, err = declaredBase(run, top, wants); err != nil {
			return snapshot.Source{}, err
		}
	} else {
		asked, err := run(nil, "rev-parse", "--verify", "--quiet", "--end-of-options", base+"^{commit}")
		if err != nil {
			return snapshot.Source{}, refusal(err, snapshot.UnknownBase, base)
		}
		askedCommit = strings.TrimSpace(string(asked))
	}
	source := snapshot.Source{AskedBase: base, BaseDeclared: declared, Head: strings.TrimSpace(string(head))}
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
	var tracked []string
	if len(missing) > 0 {
		listed, err := run(nil, "ls-files", "-z")
		if err != nil {
			return snapshot.Source{}, gitFailed(err)
		}
		tracked = fields(listed)
	}
	exclude := excluding(missing, tracked)
	if source.Changes, err = changes(run, source.Base, exclude); err != nil {
		return snapshot.Source{}, err
	}
	if source.Changes, err = withMissingChanges(run, source.Base, missing, source.Changes); err != nil {
		return snapshot.Source{}, err
	}
	if source.Now, err = now(run, top, wants, &source); err != nil {
		return snapshot.Source{}, err
	}
	var absent []string
	if source.Before, source.BaseOther, absent, err = before(run, source.Base, source.Changes, wants); err != nil {
		return snapshot.Source{}, err
	}
	source.Missing = slices.Compact(slices.Sorted(slices.Values(append(missing, absent...))))
	since, err := run(nil, "rev-list", source.Base+"..HEAD")
	if err != nil {
		return snapshot.Source{}, gitFailed(err)
	}
	source.Since = strings.Fields(string(since))
	kept, err := r.history(ctx, run, top, source.Head, exclude)
	if err != nil {
		return snapshot.Source{}, err
	}
	source.Commits, source.Parents = kept.commits, kept.parents
	return source, nil
}

type runner func(input []byte, args ...string) ([]byte, error)

// declaredBase is the base a repository declares in its tracked file, read from the working tree, and its commit: a
// branch first, then a remote branch, then a tag of that name, never a revision expression.
func declaredBase(run runner, top string, wants snapshot.Wants) (name, commit string, err error) {
	noBase := func(problem string) error {
		return &snapshot.Refusal{Kind: snapshot.NoBase, Detail: problem, File: wants.BaseFile}
	}
	listed, err := run(nil, "ls-files", "-s", "-z", "--", wants.BaseFile)
	if err != nil {
		return "", "", gitFailed(err)
	}
	tracked := parseTracked(listed, 1)
	if len(tracked) == 0 || !regular(tracked[0].mode) {
		return "", "", noBase("")
	}
	root, err := os.OpenRoot(top)
	if err != nil {
		return "", "", gitFailed(err)
	}
	defer func() { _ = root.Close() }()
	content, readErr := readFile(root, wants.BaseFile, wants)
	switch {
	case errors.Is(readErr, os.ErrNotExist):
		return "", "", noBase("")
	case readErr != nil:
		content = nil
	}
	name, problem := wants.Base(content)
	if name == "" {
		return "", "", noBase(problem)
	}
	for _, prefix := range []string{"refs/heads/", "refs/remotes/", "refs/tags/"} {
		found, err := run(nil, "rev-parse", "--verify", "--quiet", "--end-of-options", prefix+name+"^{commit}")
		if err == nil {
			return name, strings.TrimSpace(string(found)), nil
		}
		if failure, ok := errors.AsType[*gitError](err); !ok || failure.code != 1 && failure.code != 128 {
			return "", "", gitFailed(err) // git failed, rather than answered that no such name exists
		}
	}
	return "", "", &snapshot.Refusal{Kind: snapshot.UnknownDeclaredBase, Detail: name, File: wants.BaseFile}
}

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

// withMissingChanges adds the changed paths whose content the partial clone lacks, with no lines counted: their
// lines cannot be counted, but that they changed is known from the trees alone, so no answer reads them as unchanged.
func withMissingChanges(run runner, base string, missing []string, changes []snapshot.Change) ([]snapshot.Change, error) {
	if len(missing) == 0 {
		return changes, nil
	}
	output, err := run(nil, "diff-index", "--name-only", "-z", "--no-renames", base)
	if err != nil {
		return nil, gitFailed(err)
	}
	listed := map[string]bool{}
	for _, c := range changes {
		listed[c.Path], listed[c.From] = true, true
	}
	isMissing := map[string]bool{}
	for _, p := range missing {
		isMissing[p] = true
	}
	for _, p := range fields(output) {
		if isMissing[p] && !listed[p] {
			changes = append(changes, snapshot.Change{Path: p})
		}
	}
	slices.SortFunc(changes, func(a, b snapshot.Change) int { return strings.Compare(a.Path, b.Path) })
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
	files := parseTracked(output, 1)
	read := readAll(root, files, wants)
	var entries []snapshot.Entry
	for i, tracked := range files {
		if tracked.mode == modeSubmodule {
			source.Submodules = append(source.Submodules, tracked.path)
			continue
		}
		if !regular(tracked.mode) {
			continue
		}
		entry := snapshot.Entry{Path: tracked.path}
		switch r := read[i]; {
		case errors.Is(r.err, os.ErrNotExist), errors.Is(r.err, errNotRegular):
			continue // tracked, but deleted in the working tree or replaced there by a link
		case r.err != nil:
			source.Unreadable = append(source.Unreadable, tracked.path)
		case !r.wanted:
		case r.content == nil:
			entry.Binary = true
		default:
			entry.Content = r.content
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// fileRead is what reading one tracked file found: its content when the snapshot wants it, else only whether it
// exists (err holds os.ErrNotExist when it does not).
type fileRead struct {
	wanted  bool
	content []byte
	err     error
}

// readAll reads the regular tracked files of the working tree, one goroutine per processor, each taking every n-th
// folder: a folder is opened once and its files by name, since opening a path through the root opens each of its
// folders again (swc, 83,143 files: 5.9 s by one reader opening paths). The results keep the order of the files.
func readAll(root *os.Root, files []trackedEntry, wants snapshot.Wants) []fileRead {
	read := make([]fileRead, len(files))
	var folders []string
	inFolder := map[string][]int{}
	for i, f := range files {
		if !regular(f.mode) {
			continue
		}
		folder := path.Dir(f.path)
		if _, seen := inFolder[folder]; !seen {
			folders = append(folders, folder)
		}
		inFolder[folder] = append(inFolder[folder], i)
	}
	workers := runtime.GOMAXPROCS(0)
	var group sync.WaitGroup
	for worker := range workers {
		group.Go(func() {
			for n := worker; n < len(folders); n += workers {
				readFolder(root, folders[n], inFolder[folders[n]], files, wants, read)
			}
		})
	}
	group.Wait()
	return read
}

// readFolder reads the files at the given indexes, which all lie in folder, through one root opened on it.
func readFolder(root *os.Root, folder string, indexes []int, files []trackedEntry, wants snapshot.Wants, read []fileRead) {
	dir := root
	if folder != "." {
		sub, err := root.OpenRoot(folder)
		if err != nil {
			if info, statErr := root.Lstat(folder); statErr == nil && !info.IsDir() {
				// Something else stands where the folder was: its tracked files are gone, as git status says, on every
				// system (Windows reports them as absent, Unix as no directory).
				err = os.ErrNotExist
			}
			for _, i := range indexes {
				read[i] = missing(files[i].path, wants, err)
			}
			return
		}
		defer func() { _ = sub.Close() }()
		dir = sub
	}
	for _, i := range indexes {
		read[i] = readTracked(dir, files[i].path, wants)
	}
}

// errNotRegular is a tracked path whose working-tree entry is no regular file (a link put in its place).
var errNotRegular = errors.New("not a regular file in the working tree")

// readTracked reads one tracked file through the root of its folder. A path whose working-tree entry is no regular
// file is not read: it is not a file of the working tree, as the index alone would claim.
func readTracked(dir *os.Root, filePath string, wants snapshot.Wants) fileRead {
	name := path.Base(filePath)
	if info, err := dir.Lstat(name); err == nil && !info.Mode().IsRegular() {
		return fileRead{err: errNotRegular}
	}
	if wanted(filePath, wants) {
		content, err := readFile(dir, name, wants)
		return fileRead{wanted: true, content: content, err: err}
	}
	_, err := dir.Lstat(name)
	return missing(filePath, wants, err)
}

func wanted(filePath string, wants snapshot.Wants) bool {
	return wants.Path(filePath) || wants.Words != nil && wants.Words(filePath)
}

// missing is what an error reaching a tracked file means: a wanted file that cannot be read is unreadable; a file the
// snapshot does not read counts as gone only when it does not exist, and is kept otherwise.
func missing(filePath string, wants snapshot.Wants, err error) fileRead {
	if wanted(filePath, wants) {
		return fileRead{wanted: true, err: err}
	}
	if !errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	return fileRead{err: err}
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

// before lists the tracked files at the base and reads, in one batch, only the wanted files that changed. other are
// the tracked paths that are no regular file; absent the wanted files whose content a partial clone lacks.
func before(run runner, base string, changes []snapshot.Change, wants snapshot.Wants) (entries []snapshot.Entry, other, absent []string, err error) {
	output, err := run(nil, "ls-tree", "-r", "-z", "--full-tree", base)
	if err != nil {
		return nil, nil, nil, gitFailed(err)
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
			other = append(other, tracked.path)
			continue
		}
		entries = append(entries, snapshot.Entry{Path: tracked.path})
		if changed[tracked.path] && wants.Path(tracked.path) {
			wanted = append(wanted, tracked.object)
			objects[tracked.path] = tracked.object
		}
	}
	if len(wanted) == 0 {
		return entries, other, nil, nil
	}
	slices.Sort(wanted)
	batch, err := run([]byte(strings.Join(slices.Compact(wanted), "\n")+"\n"), "cat-file", "--batch")
	if err != nil {
		return nil, nil, nil, gitFailed(err)
	}
	contents, err := parseBatch(batch)
	if err != nil {
		return nil, nil, nil, gitFailed(err)
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
	return entries, other, absent, nil
}

// missingContent finds, in a partial clone, the paths whose content some commit of the head's history lacks: the
// objects git reports missing, named by the raw log, which needs no content; with -m it names what a merge brings in
// against each parent too. A full clone has none. Nothing is fetched: lazy fetching is off in every git command.
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
	raw, err := run(nil, "log", "--raw", "-m", "--no-abbrev", "--no-renames", "-z", "--format=%x01%H", "HEAD")
	if err != nil {
		return nil, gitFailed(err)
	}
	return pathsOfObjects(raw, lost), nil
}

// excluding is the pathspec that leaves the missing paths out, or nil when there are none: a folder in which no
// tracked file has content is one entry, so a partial clone that lacks thousands of files still fits a command line;
// every other missing path is its own. A path's glob characters are escaped: the exclude magic cannot be combined with
// the literal one.
func excluding(missing, tracked []string) []string {
	if len(missing) == 0 {
		return nil
	}
	isMissing := map[string]bool{}
	for _, p := range missing {
		isMissing[p] = true
	}
	holdsContent := map[string]bool{} // folders with a tracked file that has content, at any depth
	for _, p := range tracked {
		if isMissing[p] {
			continue
		}
		for dir := path.Dir(p); dir != "."; dir = path.Dir(dir) {
			holdsContent[dir] = true
		}
	}
	var entries []string
	for _, p := range missing {
		entry := p
		for dir := path.Dir(p); dir != "."; dir = path.Dir(dir) {
			if !holdsContent[dir] {
				entry = dir // the highest such folder wins, as the walk goes up
			}
		}
		entries = append(entries, entry)
	}
	slices.Sort(entries)
	spec := []string{"--", "."}
	escape := strings.NewReplacer(`\`, `\\`, "*", `\*`, "?", `\?`, "[", `\[`)
	for _, e := range slices.Compact(entries) {
		spec = append(spec, ":(exclude)"+escape.Replace(e))
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

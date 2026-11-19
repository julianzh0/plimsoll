package snapshot

import "fmt"

// Source is what the reader gives: plain data from git and the working tree, no git type.
type Source struct {
	BaseDeclared bool   // the question named no base, and the base is the one the repository declares
	AskedBase    string // the base as the caller named it
	Base, Head   string // full commit names; Base is the commit compared with
	MergeBase    bool   // Base is the merge base of the asked base and the head
	Modified     bool   // the working tree differs from the head commit
	Untracked    int
	Submodules   []string
	Unreadable   []string
	// Missing are the paths whose content a partial clone lacks in some commit; their lines and history are not read.
	Missing []string
	// Unavailable are the paths that differ between the base and now and whose base content the partial clone lacks:
	// a rule that compares the two states cannot read them.
	Unavailable []string
	Before      []Entry // tracked regular files at the base; Content only for wanted, changed paths
	// BaseOther are the tracked paths at the base that are no regular file (links, submodules): a file at such a path
	// now was not added by the change.
	BaseOther []string
	Now       []Entry  // tracked regular files in the working tree; Content only for wanted paths
	Changes   []Change // every tracked file that differs between the base and the working tree
	Since     []string // the commits in base..head, newest first
	Commits   []Commit // the whole non-merge history, newest first
}

// Entry is one tracked file on one side.
type Entry struct {
	Path    string
	Content []byte
	Binary  bool
}

// Change is one file that differs between the base and now, with lines added and removed.
type Change struct {
	Path    string
	From    string // the old name of a renamed file, else empty
	Added   int
	Removed int
}

// Commit is one non-merge commit of the history. Since and Usable are set by Build.
type Commit struct {
	ID     string
	Files  []FileChange
	Since  bool // the commit is in base..head
	Usable bool // the commit touches between one and the most paths a history detector reads
}

// FileChange is one path a commit changed. Area is set by Build, from the path alone.
type FileChange struct {
	Path    string
	Added   int
	Removed int
	Area    Area
}

// Wants tells the reader which contents to read, so the reader needs no rules of its own.
type Wants struct {
	Path       func(path string) bool // whether the reader reads this path's content
	Words      func(path string) bool // whether the reader reads this path of the working tree for its words alone
	Binary     func(head []byte) bool // whether these first bytes make a file binary
	SniffBytes int                    // how many first bytes Binary needs
	// BaseFile is the file that may declare the base of a question that names none, and Base reads that base from
	// its content (nil when it could not be read): the name, or the problem that makes it unusable.
	BaseFile string
	Base     func(content []byte) (name, problem string)
}

// RefusalKind says why a question cannot be answered.
type RefusalKind int

const (
	// NotARepository: the folder is not inside a git working tree.
	NotARepository RefusalKind = iota + 1
	// NoCommit: the repository has no commit yet.
	NoCommit
	// UnknownBase: git finds no commit by the base's name.
	UnknownBase
	// NoCommonHistory: the base shares no history with the head.
	NoCommonHistory
	// GitFailed: git could not run or failed otherwise.
	GitFailed
	// NoBase: the question names no base, and the repository declares none it can use.
	NoBase
	// UnknownDeclaredBase: git finds no branch, remote branch or tag by the declared base's name.
	UnknownDeclaredBase
)

// Refusal is the error of a question that cannot be answered. Detail is the base, or git's first error line.
type Refusal struct {
	Kind   RefusalKind
	Detail string
	File   string // the file that declares the base, for the two refusals about a declared base
}

func (r *Refusal) Error() string {
	return fmt.Sprintf("refused (%d): %s", r.Kind, r.Detail)
}

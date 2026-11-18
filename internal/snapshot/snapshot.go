// Package snapshot holds the common model of one question: both sides of a repository, the changes between
// them and the history, classified by the shared rules. It is pure: it knows nothing of git, MCP or any detector.
package snapshot

import (
	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/parse"
)

// ParserCapMiB is the memory the parser may use to read one file; a file that needs more is listed in Unparsed.
const ParserCapMiB = parse.MemoryCapMiB

// Snapshot is the common model every detector reads.
type Snapshot struct {
	AskedBase  string
	Base, Head string
	MergeBase  bool
	Modified   bool
	Limits     conventions.Limits // the limits over the repository: plimsoll's, or those its limits file sets now
	// ReportOnly are the kinds of mark shown in report only, as "<detector>.<rule>".
	ReportOnly []string
	// OwnFile is the file in which a repository sets its own limits and declares its layout. DeclaredLimits are the
	// limits it sets on either side, Declarations the text keys it declares on either side, and OwnFileProblems what
	// the file says now that is not read, each a phrase. OwnFileUnreadAtBase says the file could not be read at the
	// base, so a change to its declarations is not compared.
	BaseDeclared        bool // the question named no base; Base is the one the repository declares
	OwnFile             string
	DeclaredLimits      []DeclaredLimit
	Declarations        []DeclaredValues
	OwnFileProblems     []string
	OwnFileUnreadAtBase bool
	Before              Tree
	Now                 Tree
	Changes             []Change
	Footprint           Footprint
	History             History
	Layout              Layout      // what the project declares in its own standard files, for both sides
	TestRule            string      // how test code was recognized, as an answer prints it
	PlanningRule        string      // the planning and review paths, as an answer prints them
	Unverified          []Extension // code extensions without a proven language
	Untracked           int
	Submodules          []string
	Unreadable          []string
	Missing             []string   // paths whose content a partial clone lacks; their lines and history are not counted
	Unparsed            []string   // code files, on either side, that the parser could not read; sorted, each once
	Additions           []Addition // the top-level names the change declared in product files, with their uses; sorted
	// SkippedChanges are the skipped folders that hold changed files, each with how many: "`dist/` (3 files)", sorted.
	SkippedChanges []string
	SyntaxErrors   []string // changed code files whose reading on either side met a syntax error; sorted, each once
	// Functions are the functions of the changed product files that the change made complex or nested deeper.
	Functions Functions
	Named     []string // the files the caller named, cleaned, sorted, each once; nil when none
}

// Tree is one side of the repository.
type Tree struct {
	Paths     []string      // every tracked regular file, sorted
	Files     []File        // the code files, sorted by path
	Settings  *Settings     // nil when there is no .env.example at the root
	Manifests []Manifest    // the known manifests found: at the root, and a nested format's wherever it is tracked
	Broken    []string      // known manifests found that could not be read as their format
	Planning  []string      // tracked files under a planning or review path, sorted
	Compose   []ComposeFile // sorted by path
	Links     Links         // the imports of its code files, as far as they could be followed
}

// File is one code file.
type File struct {
	Path      string
	Language  string // an entry's name, or "" when no entry speaks for the suffix
	Role      Role
	Lines     int
	TestLines int               // lines of test code inside a product file, by the blocks its language names; part of Lines
	Limit     conventions.Limit // the line limit that applies: plimsoll's, or the one the repository's limits file sets
	Record    bool              // lies under a planning, review or measurement path; the history detectors leave it out
}

// Role is what a code file is.
type Role int

const (
	// Product is code the product runs.
	Product Role = iota + 1
	// Test is test code that defines at least one test.
	Test
	// TestInfrastructure is test code that defines none: fakes, helpers, fixtures, harness.
	TestInfrastructure
	// Other is code outside the product: scripts, examples, CI, documentation.
	Other
)

// IsTest says whether the role is test code: a file that defines tests or test infrastructure.
func (r Role) IsTest() bool { return r == Test || r == TestInfrastructure }

// String names the role as an answer prints it.
func (r Role) String() string {
	switch r {
	case Test:
		return "test code that defines a test"
	case TestInfrastructure:
		return "test infrastructure"
	case Other:
		return "other code"
	case Product:
		return "product code"
	}
	return ""
}

// Area is where a path of the history belongs, decided from the path alone.
type Area int

const (
	// AreaProduct is product code.
	AreaProduct Area = iota + 1
	// AreaTests is test code.
	AreaTests
	// AreaDocs is documentation.
	AreaDocs
	// AreaOther is everything else.
	AreaOther
	// AreaRecords are planning, review and measurement records: no churn row and no footprint line counts them, so
	// a story that adds or deletes records reads as neither documentation nor code.
	AreaRecords
)

// Extension counts the code files of one suffix that no language entry speaks for.
type Extension struct {
	Suffix string
	Files  int
	Lines  int
}

// Settings are the setting names of the settings template, in file order, a repeated name kept. Never a value.
type Settings struct {
	Path   string
	Names  []string // every name, a commented-out line included
	Active []string // the names of lines that are not commented out
}

// ComposeFile is a compose file's environment keys. Never a value.
type ComposeFile struct {
	Path   string
	Keys   []string // sorted, each once
	Unread int      // environment lines the reader could not read
}

// Manifest is a known manifest found with its runtime dependency names, sorted.
type Manifest struct {
	Path         string
	Dependencies []string
	Lockfiles    []string // the manifest's lockfiles that are tracked; nil when none is
	Unlocked     []string // dependencies whose name no tracked lockfile holds; nil when no lockfile is tracked
}

// Lines counts lines added and removed.
type Lines struct {
	Added   int
	Removed int
}

// Footprint is the change since the base, by area.
type Footprint struct {
	Commits        int
	Files          int
	Product        Lines
	Test           Lines
	Docs           Lines
	HoldsTestCode  int      // changed product files that hold test code now or at the base; their split is by growth
	TestPerProduct *float64 // test lines added per product line added; nil when no product line was added
}

// History is the whole non-merge history with how much of it a history detector can use.
type History struct {
	Commits []Commit
	Read    int
	Usable  int
}

// Churn is the lines added and removed per area over some commits.
type Churn struct {
	Commits int
	Added   map[Area]int
	Removed map[Area]int
}

// Total is the lines added and removed in an area.
func (c Churn) Total(area Area) int { return c.Added[area] + c.Removed[area] }

// ChurnOf sums the lines added and removed per area over the commits.
func ChurnOf(commits []Commit) Churn {
	churn := Churn{Commits: len(commits), Added: map[Area]int{}, Removed: map[Area]int{}}
	for _, commit := range commits {
		for _, file := range commit.Files {
			churn.Added[file.Area] += file.Added
			churn.Removed[file.Area] += file.Removed
		}
	}
	return churn
}

// SinceBase are the commits of the history that came after the base.
func (h History) SinceBase() []Commit {
	var since []Commit
	for _, commit := range h.Commits {
		if commit.Since {
			since = append(since, commit)
		}
	}
	return since
}

// Touches counts the usable commits that list one path: all of them, and those that are not since the base.
type Touches struct{ All, AtBase int }

// Touches counts, per path, the usable commits that list it.
// NOTE: a path is counted by its name; the changes of a renamed file before its rename are not carried over.
// Upgrade: follow the renames of the history's records.
func (h History) Touches() map[string]Touches {
	found := map[string]Touches{}
	for _, commit := range h.Commits {
		if !commit.Usable {
			continue
		}
		for _, file := range commit.Files {
			t := found[file.Path]
			t.All++
			if !commit.Since {
				t.AtBase++
			}
			found[file.Path] = t
		}
	}
	return found
}

// BasePath is the path a file had at the base: its old name when a change renamed it, else the path itself.
func (s Snapshot) BasePath(path string) string {
	for _, change := range s.Changes {
		if change.Path == path && change.From != "" {
			return change.From
		}
	}
	return path
}

// LineRange is a range of lines, from 1, both ends included.
type LineRange struct{ Start, End int }

// FunctionChange is a function of a changed product file that crossed a function rule: names and numbers, no text.
type FunctionChange struct {
	Path, Name         string
	Occurrence         int // 1 for the first function of the name in its file
	Start, End         int // its lines now
	New                bool
	BasePath           string // "" when New
	BaseStart, BaseEnd int
	ByBody             bool // matched to its base function by a like body, not by its name
	Complexity         int
	ComplexityBefore   int
	Nesting            int
	NestingBefore      int
	NestingLine        int
	Repeated           []LineRange // two ranges of a repeated block the change introduced; nil without one
	AddedLines         int         // lines of the function the change added
	MovedShare         float64     // the share of its tokens found in lines the change removed
	CopyFiles          int         // base files compared for a copy
	ClosestFile        string      // the compared file that held most of its tokens
	ClosestShare       float64
	AddedWhole         bool // its file is new in the change
}

// LeftOutReason says why a function that crossed a limit is not listed.
type LeftOutReason int

// The reasons a function is left out, in the order they are tried.
const (
	FromAnotherProject LeftOutReason = iota + 1 // its file came from another project
	Extracted                                   // split out of code that existed: fewer than half its lines are new
	Moved                                       // its code is code the change removed elsewhere
	Copied                                      // its code is in a file at the base
)

func (r LeftOutReason) String() string {
	switch r {
	case FromAnotherProject:
		return "from another project"
	case Extracted:
		return "extracted"
	case Moved:
		return "moved"
	case Copied:
		return "copied"
	}
	return "unknown"
}

// LeftOutFunction is a function that crossed a limit and is left out as code that existed before.
type LeftOutFunction struct {
	Path, Name string
	Occurrence int // its place among the functions of that name in the file, from 1
	Reason     LeftOutReason
	Source     string // for Copied, the file at the base that holds it
}

// Functions is what the function rules found in the changed product files.
type Functions struct {
	Files, Measured               int // changed product files measured, and their functions now
	Complex, Deeper               []FunctionChange
	ComplexCrossed, DeeperCrossed int // functions that crossed each rule and were not left out, the listed ones included
	LeftOut                       []LeftOutFunction
	Uncompared, Broken            int
	BrokenIn                      []FileCount // the files of the Broken functions, the most first
	Unread, Minified              []string
	PerMeasure, Checks, Run       int // the rules' numbers an answer prints; Run is the tokens of one compared run
	MinifiedLineBytes             int
	HeaderLines                   int // the lines of a measured file that are read for another project's header
	MovedShare, CopyShare         float64
}

// FileCount is how many of something one file holds.
type FileCount struct {
	Path  string
	Count int
}

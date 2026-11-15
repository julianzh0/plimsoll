// Package snapshot holds the common model of one question: both sides of a repository, the changes between
// them and the history, classified by the shared rules. It is pure: it knows nothing of git, MCP or any detector.
package snapshot

import "github.com/juzhxng/plimsoll/internal/conventions"

// Snapshot is the common model every detector reads.
type Snapshot struct {
	AskedBase    string
	Base, Head   string
	MergeBase    bool
	Modified     bool
	Limits       conventions.Limits // the limits over the repository
	Before       Tree
	Now          Tree
	Changes      []Change
	Footprint    Footprint
	History      History
	Layout       Layout      // what the project declares in its own standard files, for both sides
	TestRule     string      // how test code was recognized, as an answer prints it
	PlanningRule string      // the planning and review paths, as an answer prints them
	Unverified   []Extension // code extensions without a proven language
	Untracked    int
	Submodules   []string
	Unreadable   []string
	Named        []string // the files the caller named, cleaned, sorted, each once; nil when none
}

// Tree is one side of the repository.
type Tree struct {
	Paths     []string      // every tracked regular file, sorted
	Files     []File        // the code files, sorted by path
	Settings  *Settings     // nil when there is no .env.example at the root
	Manifests []Manifest    // the known manifests found at the root
	Broken    []string      // known manifests at the root that could not be read as their format
	Planning  []string      // tracked files under a planning or review path, sorted
	Uses      []SettingUse  // one per distinct setting name, sorted by name
	Dynamic   []DynamicRead // sorted by path; read on the working tree only
	Compose   []ComposeFile // sorted by path
	Links     Links         // the imports of its code files, as far as they could be followed
}

// File is one code file.
type File struct {
	Path     string
	Language string // an entry's name, or "" when no entry speaks for the suffix
	Role     Role
	Lines    int
	Limit    conventions.Limit // the line limit that applies: its entry's, else the shared one
	Record   bool              // lies under a planning, review or measurement path; the history detectors leave it out
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

// SettingUse says which code files hold a form of a setting's name as a word.
type SettingUse struct {
	Name    string
	Product []string // product files, sorted
	Test    []string // test files, tests and test infrastructure, sorted
	Rest    string   // the name without its first part, when no product file reads the name and one holds that
	// Prefixed are the product files that declare a prefix of the name and hold the rest of it in lower case, sorted.
	Prefixed []string
}

// DynamicRead is a product file and its settings reads whose name is computed at run time.
type DynamicRead struct {
	Path  string
	Count int
}

// ComposeFile is a compose file's environment keys. Never a value.
type ComposeFile struct {
	Path   string
	Keys   []string // sorted, each once
	Unread int      // environment lines the reader could not read
}

// Manifest is a known manifest at the root with its runtime dependency names, sorted.
type Manifest struct {
	Path         string
	Dependencies []string
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

// Package answer holds the one result of every question, how marks are derived and ordered, the contract every
// detector follows, and the text rendered from the result.
package answer

// Question is one of the three questions.
type Question string

// The three questions.
const (
	Status  Question = "status"
	Report  Question = "report"
	Explain Question = "explain"
)

// Change says how an observation changed since the base.
type Change string

// The five changes. Standing and touched items live in their own lists.
const (
	New      Change = "new"
	Grown    Change = "grown"
	Resolved Change = "resolved"
	Standing Change = "standing"
	// Touched is an item the change touched but did not create: a new or grown item no change of its own made.
	Touched Change = "touched"
)

// Confidence says whether a mark is a measurement or an inference from a search.
type Confidence string

// The two confidences.
const (
	Exact     Confidence = "exact"
	Candidate Confidence = "candidate"
)

// Answer is the result of every question. structuredContent is its JSON, and the text is rendered from it alone.
type Answer struct {
	Question   Question   `json:"question"`
	Version    string     `json:"version"`
	Base       Base       `json:"base"`
	Head       Head       `json:"head"`
	Footprint  *Footprint `json:"footprint,omitempty"` // status and report, the same in both
	Counts     Counts     `json:"counts"`              // all new, grown and resolved, not only those shown
	Marks      []Mark     `json:"marks"`               // new and grown, ordered
	More       int        `json:"more"`                // marks not shown
	Resolved   []Mark     `json:"resolved"`
	Touched    []Mark     `json:"touched"`    // in no count and no numbered mark
	Summaries  []string   `json:"summaries"`  // one sentence per list of report rows that status only counts
	ReportOnly []Mark     `json:"reportOnly"` // new and grown marks of a kind status only counts
	Standing   []Mark     `json:"standing"`
	// Acknowledged are the standing items whose identifiers the repository acknowledges in AcknowledgedIn; both are
	// left out when there is none, so an answer without such items is unchanged.
	Acknowledged   []Mark        `json:"acknowledged,omitempty"`
	AcknowledgedIn string        `json:"acknowledgedIn,omitempty"`
	History        History       `json:"history"`
	Detectors      []string      `json:"detectors"`
	NotAnalysed    []NotAnalysed `json:"notAnalysed"`
	Headline       *Headline     `json:"headline,omitempty"` // status and report: the change per area
	Radius         *Radius       `json:"radius,omitempty"`   // status and report: the blast radius of the change
	Sections       []Section     `json:"sections,omitempty"` // report: one per detector
	Evidence       *Evidence     `json:"evidence,omitempty"` // explain
}

// Base is the revision compared with.
type Base struct {
	Asked     string `json:"asked"`     // as the caller named it
	Commit    string `json:"commit"`    // the full name of the commit compared with
	MergeBase bool   `json:"mergeBase"` // Commit is the merge base of Asked and the head
	// DeclaredIn is the file that declares the base, when the question named none.
	DeclaredIn string `json:"declaredIn,omitempty"`
}

// Head is the head commit and whether the working tree differs from it.
type Head struct {
	Commit   string `json:"commit"`
	Modified bool   `json:"modified"`
}

// Footprint is the change since the base, by area.
type Footprint struct {
	Commits        int      `json:"commits"`
	Files          int      `json:"files"`
	Product        Lines    `json:"product"`
	Test           Lines    `json:"test"`
	Docs           Lines    `json:"docs"`
	TestPerProduct *float64 `json:"testPerProduct,omitempty"` // absent when no product line was added
}

// Lines counts lines added and removed.
type Lines struct {
	Added   int `json:"added"`
	Removed int `json:"removed"`
}

// Counts are the new, grown and resolved marks, each separately.
type Counts struct {
	New      int `json:"new"`
	Grown    int `json:"grown"`
	Resolved int `json:"resolved"`
}

// History says how many commits were read and how many a history detector could use.
type History struct {
	Read   int `json:"read"`
	Usable int `json:"usable"`
}

// Mark is one observation that something is new, has grown, was resolved, or stands since the base.
type Mark struct {
	ID         string     `json:"id"`
	Change     Change     `json:"change"`
	Subject    Subject    `json:"subject"`
	Measured   Measured   `json:"measured"`
	Confidence Confidence `json:"confidence"`
	Hint       string     `json:"hint"` // one line on how to verify it
}

// Subject is what a mark is about.
type Subject struct {
	Kind  string   `json:"kind"`            // repository, file, files, folders, setting, rule
	Paths []string `json:"paths,omitempty"` // one path for a file; two or more, sorted, for files
	Names []string `json:"names,omitempty"` // settings or dependencies the mark names, sorted; for a rule, the imports it names, in order
}

// Measured is the measurement behind a mark.
type Measured struct {
	Value       float64  `json:"value"`
	Before      *float64 `json:"before,omitempty"` // absent when the subject did not exist at the base
	Limit       *float64 `json:"limit,omitempty"`  // absent for a rule without a number
	LimitSource string   `json:"limitSource"`      // where the limit or the rule comes from
	Total       *float64 `json:"total,omitempty"`  // the whole that Value is a share of
	Unit        string   `json:"unit"`
}

// NotAnalysed is one thing a detector, or the repository's reading, could not see.
type NotAnalysed struct {
	Detector string `json:"detector"` // a detector's name, or "repository"
	What     string `json:"what"`
}

// Section is one detector's part of the report.
type Section struct {
	Detector   string      `json:"detector"`
	Statistics []Statistic `json:"statistics"`
	Notes      []string    `json:"notes"`
}

// Statistic is one number of the report, with its value at the base.
type Statistic struct {
	Name    string   `json:"name"`
	Subject string   `json:"subject,omitempty"`
	Value   *float64 `json:"value,omitempty"`
	Before  *float64 `json:"before,omitempty"`
	Unit    string   `json:"unit,omitempty"` // "ratio" prints with one decimal and no unit
	Note    string   `json:"note,omitempty"`
}

// Evidence is the evidence behind one mark, capped.
type Evidence struct {
	Mark    string   `json:"mark"`
	Lines   []string `json:"lines"`
	Omitted int      `json:"omitted"`
}

// Radius is the blast radius of a change: the files the change did not touch that use a name it altered or removed,
// or that use a changed file counted as a whole. State says whether the number is counted, a lower bound, or not
// analysed; Paths are the first of those files, product files first and each part in path order, and Omitted counts
// the rest. Changed are the changed code files by state; Direct and Total the untouched files that import a changed
// code file directly and at any distance, of the languages whose imports are read, with Folders of the total, the
// count in OtherFolders, and ProductCodeFiles of those languages, the share's denominator.
type Radius struct {
	State             RadiusState   `json:"state"`
	Files             int           `json:"files"`
	ByName            int           `json:"byName"`  // the files that use an altered or removed name
	AsWhole           int           `json:"asWhole"` // the other files: users of a changed file counted as a whole
	Product           int           `json:"product"`
	Tests             int           `json:"tests"`
	Other             int           `json:"other"`
	InChange          int           `json:"inChange"` // users the change itself touched, apart
	Paths             []string      `json:"paths"`
	Omitted           int           `json:"omitted"`
	Changed           ChangedFiles  `json:"changed"`
	Direct            Importers     `json:"direct"`
	Total             Importers     `json:"total"`
	Folders           []FolderCount `json:"folders"`
	OtherFolders      int           `json:"otherFolders"`
	ProductCodeFiles  int           `json:"productCodeFiles"`
	ImportersInChange int           `json:"importersInChange"` // importers the change itself touched, apart
}

// ChangedFiles counts the changed code files of a change by state.
type ChangedFiles struct {
	All            int `json:"all"`
	ByName         int `json:"byName"`         // altered or removed declarations, read by name
	AsWhole        int `json:"asWhole"`        // changed code that could not be told by name
	Deleted        int `json:"deleted"`        // exist only at the base
	NothingAltered int `json:"nothingAltered"` // comments, imports or added declarations only
	NotCounted     int `json:"notCounted"`     // skipped, ignored, generated, missing, or of a language whose links are not used
}

// Importers counts files by role.
type Importers struct {
	Files   int `json:"files"`
	Product int `json:"product"`
	Tests   int `json:"tests"`
	Other   int `json:"other"`
}

// FolderCount is how many files lie under a folder ("" for the root).
type FolderCount struct {
	Folder string `json:"folder"`
	Files  int    `json:"files"`
}

// Headline sums up the change per area: how many areas got worse and how many better, and each area's state
// with the identifiers it rests on. Only kinds of mark that are not report-only move an area.
type Headline struct {
	Worse  int            `json:"worse"`  // the failing areas
	Better int            `json:"better"` // the passing areas with a resolved item
	Areas  []HeadlineArea `json:"areas"`  // in the fixed order
}

// HeadlineArea is one area: failing when it has a new or grown mark of a kind that decides it, else not analysed
// when none of its deciding rules ran, else passing. Marks are the failing marks' identifiers, in mark order;
// Resolved the identifiers of its resolved items, sorted.
type HeadlineArea struct {
	Name     string    `json:"name"`
	State    AreaState `json:"state"`
	Marks    []string  `json:"marks"`
	Resolved []string  `json:"resolved"`
}

// Better says whether an area passes with a resolved item of a kind that decides it.
func (a HeadlineArea) Better() bool {
	return a.State == AreaPass && len(a.Resolved) > 0
}

// AreaState is the state of an area of the headline.
type AreaState string

// The states of an area.
const (
	AreaPass        AreaState = "pass"
	AreaFail        AreaState = "fail"
	AreaNotAnalysed AreaState = "not analysed"
)

// RadiusState says what a radius number is.
type RadiusState string

// The states of a radius.
const (
	RadiusCounted     RadiusState = "counted"
	RadiusAtLeast     RadiusState = "at least"
	RadiusNotAnalysed RadiusState = "not analysed"
)

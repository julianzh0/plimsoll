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

// The four changes. Standing items live in their own list.
const (
	New      Change = "new"
	Grown    Change = "grown"
	Resolved Change = "resolved"
	Standing Change = "standing"
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
	Question    Question      `json:"question"`
	Version     string        `json:"version"`
	Base        Base          `json:"base"`
	Head        Head          `json:"head"`
	Footprint   *Footprint    `json:"footprint,omitempty"` // status and report, the same in both
	Counts      Counts        `json:"counts"`              // all new, grown and resolved, not only those shown
	Marks       []Mark        `json:"marks"`               // new and grown, ordered
	More        int           `json:"more"`                // marks not shown
	hidden      []Mark        // the marks not shown, for the text to name the context marks among them
	Resolved    []Mark        `json:"resolved"`
	Standing    []Mark        `json:"standing"`
	History     History       `json:"history"`
	Detectors   []string      `json:"detectors"`
	NotAnalysed []NotAnalysed `json:"notAnalysed"`
	Sections    []Section     `json:"sections,omitempty"` // report: one per detector
	Evidence    *Evidence     `json:"evidence,omitempty"` // explain
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
	// Context marks describe the history around a change, not a fact of the change itself (pairs of files that
	// changed together): they rank after every other mark, so they fill only the places those leave.
	Context bool `json:"context,omitempty"`
}

// Subject is what a mark is about.
type Subject struct {
	Kind  string   `json:"kind"`            // repository, file, files, folders, setting
	Paths []string `json:"paths,omitempty"` // one path for a file; two or more, sorted, for files
	Names []string `json:"names,omitempty"` // settings or dependencies the mark names, sorted
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

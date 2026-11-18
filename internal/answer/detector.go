package answer

import "github.com/julianzh0/plimsoll/internal/snapshot"

// Detector is one family of rules: what it measures, and how it words its marks.
type Detector interface {
	// Name is the prefix of its identifiers and the title of its report section.
	Name() string
	// Detect measures both sides of the snapshot. It does no input or output.
	Detect(s snapshot.Snapshot) Findings
	// Wording is the sentence for one of its marks, built from the mark's fields alone.
	Wording(m Mark) string
	// Standing is its part of the one standing line, for its standing items.
	Standing(items []Mark) string
	// Explain gives every evidence line for one of its marks, computed again. The answer caps them.
	Explain(s snapshot.Snapshot, m Mark) []string
}

// Findings are what one detector found: every observation, its report numbers and notes, what it could not see.
type Findings struct {
	Marks       []Mark // new, grown, resolved or standing
	Statistics  []Statistic
	Notes       []string
	NotAnalysed []string
	// NotAnalysedFirst are said before every other "not analysed" entry but the folders skipped: what a repository
	// declared and plimsoll cannot check at all.
	NotAnalysedFirst []string
	Summaries        []string // one sentence per list of report rows that status only counts
}

// Result is one detector's findings, under its name.
type Result struct {
	Detector string
	Findings Findings
}

// Phrases are one detector's two wording functions.
type Phrases struct {
	Mark     func(Mark) string
	Standing func([]Mark) string
}

// Wording gives each detector's phrases by its name.
type Wording map[string]Phrases

// WordingOf collects the phrases of the detectors.
func WordingOf(detectors []Detector) Wording {
	wording := Wording{}
	for _, detector := range detectors {
		wording[detector.Name()] = Phrases{Mark: detector.Wording, Standing: detector.Standing}
	}
	return wording
}

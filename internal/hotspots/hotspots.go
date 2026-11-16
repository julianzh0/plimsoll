// Package hotspots is the hotspots detector: the code files changed most often times their size now.
package hotspots

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

const name = "hotspots"

// The shape detector's name and its size rule: they build the identifier of the size mark a hotspot remarks on.
const (
	shapeName     = "shape"
	shapeFileSize = "file-size"
)

const shown = 3

// Detector is the hotspots detector. It gives no mark: its rows are statistics and its remarks join the size marks.
type Detector struct{}

// Name is the prefix of the detector's identifiers.
func (Detector) Name() string { return name }

type spot struct {
	file            snapshot.File
	changes, before int
}

// Detect ranks the code files tracked now by their changes in usable commits times their lines now.
func (Detector) Detect(s snapshot.Snapshot) answer.Findings {
	if s.History.Usable == 0 {
		return answer.Findings{NotAnalysed: []string{"no usable commit in the history, so no hotspots"}}
	}
	touches := s.History.Touches()
	var spots []spot
	for _, file := range s.Now.Files {
		if t := touches[file.Path]; !file.Record && t.All > 0 {
			spots = append(spots, spot{file, t.All, t.AtBase})
		}
	}
	if len(spots) == 0 {
		return answer.Findings{NotAnalysed: []string{"no code file tracked now changed in a usable commit, so no hotspots"}}
	}
	slices.SortStableFunc(spots, func(a, b spot) int {
		return cmp.Compare(b.changes*b.file.Lines, a.changes*a.file.Lines)
	})
	f := answer.Findings{Notes: []string{"hotspots: changes in usable commits x lines now, per code file; planning, review and measurement paths left out"}}
	for _, spot := range spots[:min(shown, len(spots))] {
		value, atBase := float64(spot.changes), float64(spot.before)
		size := "x " + answer.Plural(spot.file.Lines, "line")
		f.Statistics = append(f.Statistics, answer.Statistic{
			Name: "hotspot", Subject: spot.file.Path, Value: &value, Before: &atBase, Unit: "changes", Note: size,
		})
		f.Remarks = append(f.Remarks, answer.Remark{
			Mark: answer.ID(shapeName, shapeFileSize, spot.file.Path),
			Text: fmt.Sprintf("Also a hotspot: %s %s.", answer.Plural(spot.changes, "change"), size),
		})
	}
	return f
}

// Wording is empty: the detector gives no mark.
func (Detector) Wording(answer.Mark) string { return "" }

// Standing is empty: the detector gives no mark.
func (Detector) Standing([]answer.Mark) string { return "" }

// Explain has nothing to explain: the detector gives no mark.
func (Detector) Explain(snapshot.Snapshot, answer.Mark) []string { return nil }

package process

import (
	"fmt"
	"slices"

	"github.com/juzhxng/plimsoll/internal/answer"
	"github.com/juzhxng/plimsoll/internal/snapshot"
)

// Explain gives the evidence of one mark, computed again: paths and counts, never a file's content.
func (Detector) Explain(s snapshot.Snapshot, m answer.Mark) []string {
	if answer.RuleOf(m.ID) == ruleDocChurn {
		return churnEvidence(s)
	}
	path := m.Subject.Paths[0]
	lines := []string{"tracked now: no"}
	if slices.Contains(s.Now.Planning, path) {
		lines[0] = "tracked now: yes"
	}
	atBase := "tracked at the base: no"
	switch basePath := s.BasePath(path); {
	case !slices.Contains(s.Before.Planning, basePath):
	case basePath != path:
		atBase = fmt.Sprintf("tracked at the base: yes, as `%s`", basePath)
	default:
		atBase = "tracked at the base: yes"
	}
	return append(lines, atBase, "planning and review paths: "+s.PlanningRule)
}

func churnEvidence(s snapshot.Snapshot) []string {
	churn := snapshot.ChurnOf(s.History.SinceBase())
	lines := []string{"commits since the base: " + answer.Count(float64(churn.Commits))}
	for _, area := range []struct {
		label string
		area  snapshot.Area
	}{{"documentation", snapshot.AreaDocs}, {"product", snapshot.AreaProduct}, {"tests", snapshot.AreaTests}, {"other", snapshot.AreaOther}} {
		lines = append(lines, fmt.Sprintf("%s: +%s -%s", area.label, answer.Count(float64(churn.Added[area.area])), answer.Count(float64(churn.Removed[area.area]))))
	}
	return lines
}

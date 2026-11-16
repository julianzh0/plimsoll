package process

import (
	"fmt"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

// Explain gives the evidence of one mark, computed again: paths and counts, never a file's content.
func (Detector) Explain(s snapshot.Snapshot, m answer.Mark) []string {
	switch answer.RuleOf(m.ID) {
	case ruleDocChurn:
		return churnEvidence(s)
	case ruleDeclarationChange:
		return declarationEvidence(s, m)
	case ruleLimitChange:
		unit := m.Measured.Unit
		return []string{
			fmt.Sprintf("`%s` at the base: %s", s.OwnFile, setOrNot(*m.Measured.Before, *m.Measured.Limit, unit)),
			fmt.Sprintf("`%s` now: %s", s.OwnFile, setOrNot(m.Measured.Value, *m.Measured.Limit, unit)),
			fmt.Sprintf("plimsoll's default: %s", withUnit(*m.Measured.Limit, unit)),
		}
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

// setOrNot is a side's value of a limit, as the evidence names it.
func setOrNot(value, defaultValue float64, unit string) string {
	if value == defaultValue {
		return withUnit(value, unit) + ", the default"
	}
	return withUnit(value, unit)
}

// declarationEvidence gives both sides' values of a changed declaration and what was added and removed.
func declarationEvidence(s snapshot.Snapshot, m answer.Mark) []string {
	at := slices.IndexFunc(s.Declarations, func(d snapshot.DeclaredValues) bool { return answer.ID(name, ruleDeclarationChange, d.Key) == m.ID })
	if at < 0 {
		return nil
	}
	d := s.Declarations[at]
	side := func(values []string) string {
		if len(values) == 0 {
			return "not set"
		}
		return strings.Join(answer.Quoted(values), ", ")
	}
	lines := []string{
		fmt.Sprintf("`%s` `[%s]` `%s` at the base: %s", s.OwnFile, d.Table, d.Key, side(d.Before)),
		fmt.Sprintf("`%s` `[%s]` `%s` now: %s", s.OwnFile, d.Table, d.Key, side(d.Now)),
	}
	if added := difference(d.Now, d.Before); len(added) > 0 {
		lines = append(lines, "added: "+strings.Join(answer.Quoted(added), ", "))
	}
	if removed := difference(d.Before, d.Now); len(removed) > 0 {
		lines = append(lines, "removed: "+strings.Join(answer.Quoted(removed), ", "))
	}
	return lines
}

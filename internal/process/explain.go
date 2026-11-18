package process

import (
	"fmt"
	"slices"

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
	match := s.PlanningMatches[path]
	if match.Key == "" {
		return append(lines, atBase, fmt.Sprintf("matches `%s`, a default planning and review path", match.Entry))
	}
	return append(lines, atBase, fmt.Sprintf("matches `%s`, declared in `%s` (`[%s]` `%s`)", match.Entry, s.OwnFile, match.Table, match.Key))
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
	if d.Whole {
		return tableEvidence(s.OwnFile, d)
	}
	side := func(values []string) string {
		if len(values) == 0 {
			return "not set"
		}
		return answer.QuotedList(values)
	}
	lines := []string{
		fmt.Sprintf("`%s` `[%s]` `%s` at the base: %s", s.OwnFile, d.Table, d.Key, side(d.Before)),
		fmt.Sprintf("`%s` `[%s]` `%s` now: %s", s.OwnFile, d.Table, d.Key, side(d.Now)),
	}
	if added := answer.Added(d.Before, d.Now); len(added) > 0 {
		lines = append(lines, "added: "+answer.QuotedList(added))
	}
	if removed := answer.Added(d.Now, d.Before); len(removed) > 0 {
		lines = append(lines, "removed: "+answer.QuotedList(removed))
	}
	return lines
}

// tableEvidence gives a whole table's entries on both sides by count, then each identifier added or removed on a
// line of its own, so the evidence cap applies to a long table.
func tableEvidence(ownFile string, d snapshot.DeclaredValues) []string {
	entries := func(values []string) string {
		if len(values) == 0 {
			return "no entry"
		}
		return answer.Agree(len(values), "entry", "entries")
	}
	lines := []string{
		fmt.Sprintf("`%s` `[%s]` at the base: %s", ownFile, d.Table, entries(d.Before)),
		fmt.Sprintf("`%s` `[%s]` now: %s", ownFile, d.Table, entries(d.Now)),
	}
	for _, id := range answer.Added(d.Before, d.Now) {
		lines = append(lines, fmt.Sprintf("added: `%s`", id))
	}
	for _, id := range answer.Added(d.Now, d.Before) {
		lines = append(lines, fmt.Sprintf("removed: `%s`", id))
	}
	return lines
}

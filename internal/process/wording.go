package process

import (
	"fmt"
	"strings"

	"github.com/julianzh0/plimsoll/internal/answer"
)

// Wording is the sentence for one of the detector's marks, from the mark's fields alone.
func (Detector) Wording(m answer.Mark) string {
	switch answer.RuleOf(m.ID) {
	case rulePlanning:
		path := m.Subject.Paths[0]
		switch m.Change {
		case answer.New:
			return fmt.Sprintf("`%s` is a planning or review record this change added to the repository (plimsoll's rule: repositories hold no planning records); keep it outside the repository.", path)
		case answer.Resolved:
			return fmt.Sprintf("`%s` is no longer tracked; it lay under a planning or review path at the base.", path)
		case answer.Grown, answer.Standing, answer.Touched:
			return fmt.Sprintf("`%s` is tracked and lies under a planning or review path; it was tracked at the base too.", path)
		}
	case ruleLimitChange:
		direction, unit := "raised", m.Measured.Unit
		if m.Measured.Value < *m.Measured.Before {
			direction = "lowered"
		}
		return fmt.Sprintf("This change %s the %s from %s to %s in `%s` (plimsoll's default: %s).", direction, m.Subject.Names[0],
			bare(*m.Measured.Before, unit), withUnit(m.Measured.Value, unit), m.Subject.Paths[0], bare(*m.Measured.Limit, unit))
	case ruleDeclarationChange:
		now, before := int(m.Measured.Value), int(*m.Measured.Before)
		var parts []string
		if added := m.Subject.Names; len(added) > acknowledgedNamed && strings.HasSuffix(m.ID, ":"+acknowledgedTable) {
			parts = append(parts, fmt.Sprintf("added %s and %d more", answer.QuotedList(added[:acknowledgedNamed]), len(added)-acknowledgedNamed))
		} else if len(added) > 0 {
			parts = append(parts, "added "+answer.QuotedList(added))
		}
		if removed := before + len(m.Subject.Names) - now; removed > 0 {
			parts = append(parts, fmt.Sprintf("removed %d", removed))
		}
		return fmt.Sprintf("This change altered the %s declared in `%s`: %s; %d declared now, %d at the base.", m.Measured.Unit, m.Subject.Paths[0],
			strings.Join(parts, ", "), now, before)
	case ruleDocChurn:
		return fmt.Sprintf("Documentation churn since the base, summed over its commits, is %s, above product plus test churn of %s; the footprint counts the net change instead.",
			answer.Plural(int(m.Measured.Value), "line"), answer.Plural(int(*m.Measured.Limit), "line"))
	}
	return m.ID
}

// Standing is the detector's part of the standing line.
func (Detector) Standing(items []answer.Mark) string {
	counts := map[string]int{}
	for _, m := range items {
		counts[answer.RuleOf(m.ID)]++
	}
	var fragments []string
	if n := counts[rulePlanning]; n > 0 {
		fragments = append(fragments, answer.Plural(n, "tracked planning or review file"))
	}
	if counts[ruleDocChurn] > 0 {
		fragments = append(fragments, "documentation churn above code churn")
	}
	if n := counts[ruleLimitChange]; n > 0 {
		fragments = append(fragments, answer.Plural(n, "limit")+" changed in the limits file")
	}
	if n := counts[ruleDeclarationChange]; n > 0 {
		fragments = append(fragments, answer.Plural(n, "declaration")+" changed in the limits file")
	}
	return strings.Join(fragments, "; ")
}

// withUnit is a limit's value as an answer prints it, with what it counts.
func withUnit(value float64, unit string) string {
	if unit == "" {
		return bare(value, unit)
	}
	return bare(value, unit) + " " + unit
}

// bare is a limit's value alone: a ratio (no unit) with one decimal, a count with its thousands separated.
func bare(value float64, unit string) string {
	if unit == "" {
		return answer.Decimal(value)
	}
	return answer.Count(value)
}

package process

import (
	"fmt"

	"github.com/juzhxng/plimsoll/internal/answer"
)

// Wording is the sentence for one of the detector's marks, from the mark's fields alone.
func (Detector) Wording(m answer.Mark) string {
	switch answer.RuleOf(m.ID) {
	case rulePlanning:
		path := m.Subject.Paths[0]
		switch m.Change {
		case answer.New:
			return fmt.Sprintf("`%s` is tracked and lies under a planning or review path; it was not tracked at the base.", path)
		case answer.Resolved:
			return fmt.Sprintf("`%s` is no longer tracked; it lay under a planning or review path at the base.", path)
		case answer.Grown, answer.Standing:
			return fmt.Sprintf("`%s` is tracked and lies under a planning or review path; it was tracked at the base too.", path)
		}
	case ruleDocChurn:
		return fmt.Sprintf("Documentation churn since the base is %s, above product plus test churn of %s.",
			answer.Plural(int(m.Measured.Value), "line"), answer.Plural(int(*m.Measured.Limit), "line"))
	}
	return m.ID
}

// Standing is the detector's part of the standing line.
func (Detector) Standing(items []answer.Mark) string {
	return answer.Plural(len(items), "tracked planning or review file")
}

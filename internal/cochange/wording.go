package cochange

import (
	"fmt"

	"github.com/julianzh0/plimsoll/internal/answer"
)

// Wording is the sentence for a pair mark.
func (Detector) Wording(m answer.Mark) string {
	shared, either := int(m.Measured.Value), int(*m.Measured.Total)
	return fmt.Sprintf("`%s` and `%s` changed together in %s (%d%%); %s at the base; limit %s shared commits (%s).",
		m.Subject.Paths[0], m.Subject.Paths[1], together(shared, either), percent(shared, either),
		answer.Count(*m.Measured.Before), answer.Count(*m.Measured.Limit), m.Measured.LimitSource)
}

// percent is shared as a rounded share of either.
func percent(shared, either int) int {
	if either == 0 {
		return 0
	}
	return (shared*100 + either/2) / either
}

// Standing counts the standing pairs, with the limit they reach.
func (Detector) Standing(items []answer.Mark) string {
	if len(items) == 0 {
		return ""
	}
	return answer.Plural(len(items), "pair") + " of files that changed together in at least " + answer.Count(*items[0].Measured.Limit) + " commits"
}

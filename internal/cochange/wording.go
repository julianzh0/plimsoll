package cochange

import (
	"fmt"

	"github.com/julianzh0/plimsoll/internal/answer"
)

// Wording is the sentence for a pair mark.
func (Detector) Wording(m answer.Mark) string {
	return fmt.Sprintf("`%s` and `%s` changed together in %s; %s at the base.", m.Subject.Paths[0], m.Subject.Paths[1],
		together(int(m.Measured.Value), int(*m.Measured.Total)), answer.Count(*m.Measured.Before))
}

// Standing counts the standing pairs, with the limit they reach.
func (Detector) Standing(items []answer.Mark) string {
	if len(items) == 0 {
		return ""
	}
	return answer.Plural(len(items), "pair") + " of files that changed together in at least " + answer.Count(*items[0].Measured.Limit) + " commits"
}

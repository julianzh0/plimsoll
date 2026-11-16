package shape

import (
	"fmt"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/answer"
)

// Wording is the sentence for one of the detector's marks, from the mark's fields alone.
func (Detector) Wording(m answer.Mark) string {
	switch answer.RuleOf(m.ID) {
	case ruleRatio:
		return ratioSentence(m.Change, m.Measured)
	case ruleSize:
		return sizeSentence(m.Change, m.Subject.Paths[0], m.Measured)
	case ruleDependencies:
		return additionSentence(m, "runtime dependency", "runtime dependencies")
	case ruleSettings:
		return additionSentence(m, "setting", "settings")
	}
	return m.ID
}

func ratioSentence(change answer.Change, measured answer.Measured) string {
	limit := fmt.Sprintf("the limit of %s (%s)", answer.Decimal(*measured.Limit), measured.LimitSource)
	before := "there was no product code at the base"
	if measured.Before != nil {
		before = fmt.Sprintf("it was %s at the base", answer.Decimal(*measured.Before))
	}
	switch change {
	case answer.Grown:
		return fmt.Sprintf("The test ratio rose from %s to %s; the limit is %s (%s).",
			answer.Decimal(*measured.Before), answer.Decimal(measured.Value), answer.Decimal(*measured.Limit), measured.LimitSource)
	case answer.Resolved:
		return fmt.Sprintf("The test ratio is %s, no longer over %s; %s.", answer.Decimal(measured.Value), limit, before)
	case answer.New, answer.Standing:
		return fmt.Sprintf("The test ratio is %s, over %s; %s.", answer.Decimal(measured.Value), limit, before)
	}
	return ""
}

func sizeSentence(change answer.Change, path string, measured answer.Measured) string {
	limit := fmt.Sprintf("the limit of %s (%s)", answer.Count(*measured.Limit), measured.LimitSource)
	before := "it did not exist at the base"
	if measured.Before != nil {
		before = fmt.Sprintf("it had %s at the base", answer.Count(*measured.Before))
	}
	switch change {
	case answer.Grown:
		return fmt.Sprintf("`%s` grew from %s to %s lines; it was already over %s.",
			path, answer.Count(*measured.Before), answer.Count(measured.Value), limit)
	case answer.Resolved:
		if measured.Value == 0 {
			return fmt.Sprintf("`%s` is no longer tracked; it had %s lines at the base, over %s.", path, answer.Count(*measured.Before), limit)
		}
		return fmt.Sprintf("`%s` has %s lines, no longer over %s; %s.", path, answer.Count(measured.Value), limit, before)
	case answer.New, answer.Standing:
		return fmt.Sprintf("`%s` has %s lines, over %s; %s.", path, answer.Count(measured.Value), limit, before)
	}
	return ""
}

func additionSentence(m answer.Mark, singular, plural string) string {
	names := answer.Quoted(m.Subject.Names)
	what := fmt.Sprintf("%d %s that were not there", len(names), plural)
	if len(names) == 1 {
		what = fmt.Sprintf("1 %s that was not there", singular)
	}
	counts := fmt.Sprintf("%s now; the file did not exist at the base", answer.Count(m.Measured.Value))
	if m.Measured.Before != nil {
		counts = fmt.Sprintf("%s now, %s at the base", answer.Count(m.Measured.Value), answer.Count(*m.Measured.Before))
	}
	return fmt.Sprintf("`%s` names %s at the base: %s (%s).", m.Subject.Paths[0], what, strings.Join(names, ", "), counts)
}

// Standing is the detector's part of the standing line: the ratio when it stands, then files over each limit.
func (Detector) Standing(items []answer.Mark) string {
	var fragments []string
	files := map[float64]int{}
	for _, m := range items {
		switch answer.RuleOf(m.ID) {
		case ruleRatio:
			fragments = append(fragments, fmt.Sprintf("test ratio %s (limit %s)", answer.Decimal(m.Measured.Value), answer.Decimal(*m.Measured.Limit)))
		case ruleSize:
			files[*m.Measured.Limit]++
		}
	}
	limits := make([]float64, 0, len(files))
	for limit := range files {
		limits = append(limits, limit)
	}
	slices.Sort(limits)
	for _, limit := range limits {
		noun := "files"
		if files[limit] == 1 {
			noun = "file"
		}
		fragments = append(fragments, fmt.Sprintf("%d %s over %s lines", files[limit], noun, answer.Count(limit)))
	}
	return strings.Join(fragments, "; ")
}

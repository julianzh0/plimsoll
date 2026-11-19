package shape

import (
	"fmt"
	"maps"
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
	case ruleComplexity:
		return complexitySentence(m)
	case ruleNesting:
		return nestingSentence(m)
	case ruleDuplicate:
		return duplicateSentence(m)
	}
	return m.ID
}

// complexitySentence says a function is new over the complexity limit, or crossed it.
func complexitySentence(m answer.Mark) string {
	named := fmt.Sprintf("`%s` `%s`", m.Subject.Paths[0], m.Subject.Names[0])
	limit, source := answer.Count(*m.Measured.Limit), "Limit: "+m.Measured.LimitSource+"."
	if m.Measured.Before == nil {
		return fmt.Sprintf("%s is new in this change with a cognitive complexity of %s, over the limit of %s. %s",
			named, answer.Count(m.Measured.Value), limit, source)
	}
	return fmt.Sprintf("%s crossed the limit of %s in this change: cognitive complexity %s. %s",
		named, limit, growth(*m.Measured.Before, m.Measured.Value), source)
}

// nestingSentence says a function is new and nests deep, or got deeper.
func nestingSentence(m answer.Mark) string {
	named := fmt.Sprintf("`%s` `%s`", m.Subject.Paths[0], m.Subject.Names[0])
	limit := fmt.Sprintf("the limit of %s (%s)", answer.Count(*m.Measured.Limit), m.Measured.LimitSource)
	switch {
	case m.Measured.Before == nil:
		return fmt.Sprintf("%s is new in this change and nests %s levels deep, at or over %s.", named, answer.Count(m.Measured.Value), limit)
	case m.Change == answer.Grown:
		return fmt.Sprintf("%s got deeper in this change: nesting %s → %s levels, already at the limit of %s at the base (%s).",
			named, answer.Count(*m.Measured.Before), answer.Count(m.Measured.Value), answer.Count(*m.Measured.Limit), m.Measured.LimitSource)
	}
	return fmt.Sprintf("%s got deeper in this change: nesting %s → %s levels, reaching %s.",
		named, answer.Count(*m.Measured.Before), answer.Count(m.Measured.Value), limit)
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
	case answer.New, answer.Standing, answer.Touched:
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
	source := "Limit: " + measured.LimitSource + "."
	switch {
	case change == answer.New && measured.Before == nil:
		return fmt.Sprintf("`%s` is new in this change with %s lines, over the limit of %s. %s",
			path, answer.Count(measured.Value), answer.Count(*measured.Limit), source)
	case change == answer.New:
		return fmt.Sprintf("`%s` crossed the limit of %s lines in this change: %s. %s",
			path, answer.Count(*measured.Limit), growth(*measured.Before, measured.Value), source)
	case change == answer.Grown:
		return fmt.Sprintf("`%s` grew in this change: %s lines, already over the limit of %s at the base. %s",
			path, growth(*measured.Before, measured.Value), answer.Count(*measured.Limit), source)
	case change == answer.Resolved:
		if measured.Value == 0 {
			return fmt.Sprintf("`%s` is no longer tracked; it had %s lines at the base, over %s.", path, answer.Count(*measured.Before), limit)
		}
		return fmt.Sprintf("`%s` has %s lines, no longer over %s; %s.", path, answer.Count(measured.Value), limit, before)
	}
	return fmt.Sprintf("`%s` has %s lines, over %s; %s.", path, answer.Count(measured.Value), limit, before)
}

// growth is a change of a count: "1,480 → 1,620 (+140)".
func growth(before, now float64) string {
	return fmt.Sprintf("%s → %s (%+d)", answer.Count(before), answer.Count(now), int(now-before))
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
	manifests, settings := 0, 0
	var dependencies []string
	grownMost, grownPath := 0.0, ""
	crossed, crossedCount, crossedMost, crossedPath := map[float64]int{}, 0, 0.0, ""
	for _, m := range items {
		switch answer.RuleOf(m.ID) {
		case ruleRatio:
			fragments = append(fragments, fmt.Sprintf("test ratio %s (limit %s)", answer.Decimal(m.Measured.Value), answer.Decimal(*m.Measured.Limit)))
		case ruleSize:
			if m.Change == answer.New {
				crossed[*m.Measured.Limit]++
				crossedCount++
				if m.Measured.Value > crossedMost {
					crossedMost, crossedPath = m.Measured.Value, m.Subject.Paths[0]
				}
				continue
			}
			files[*m.Measured.Limit]++
			if m.Change == answer.Grown && m.Measured.Before != nil && m.Measured.Value-*m.Measured.Before > grownMost {
				grownMost, grownPath = m.Measured.Value-*m.Measured.Before, m.Subject.Paths[0]
			}
		case ruleDependencies:
			manifests++
			dependencies = append(dependencies, m.Subject.Names...)
		case ruleSettings:
			settings += len(m.Subject.Names)
		}
	}
	for _, limit := range slices.Sorted(maps.Keys(crossed)) {
		fragments = append(fragments, fmt.Sprintf("%s crossed %s lines", answer.Plural(crossed[limit], "file"), answer.Count(limit)))
	}
	if crossedPath != "" {
		largest := ""
		if crossedCount > 1 {
			largest = "the largest "
		}
		fragments[len(fragments)-1] += fmt.Sprintf(", %s`%s` at %s", largest, crossedPath, answer.Count(crossedMost))
	}
	for _, limit := range slices.Sorted(maps.Keys(files)) {
		fragments = append(fragments, fmt.Sprintf("%s over %s lines", answer.Plural(files[limit], "file"), answer.Count(limit)))
	}
	if grownPath != "" && len(fragments) > 0 {
		fragments[len(fragments)-1] += fmt.Sprintf(" grew, `%s` the most (+%s)", grownPath, answer.Count(grownMost))
	}
	if manifests > 0 {
		named := answer.QuotedList(dependencies[:min(len(dependencies), namesShown)])
		if len(dependencies) > namesShown {
			named += fmt.Sprintf(" and %d more", len(dependencies)-namesShown)
		}
		fragments = append(fragments, fmt.Sprintf("runtime dependencies added in %s (%s)", answer.Plural(manifests, "manifest"), named))
	}
	if settings > 0 {
		fragments = append(fragments, answer.Plural(settings, "setting")+" added")
	}
	clauses := []string{
		functionsInStanding(items, ruleNesting, "1 function nested deeper is listed", "functions nested deeper are listed", "the deepest",
			func(v float64) string { return fmt.Sprintf("%d levels", int(v)) }),
		functionsInStanding(items, ruleDuplicate, "1 new function that repeats another is listed", "new functions that repeat another are listed",
			"the fullest repeat", func(v float64) string { return answer.Count(v) + "%" }),
	}
	for _, clause := range clauses {
		if clause != "" {
			fragments = append(fragments, clause)
		}
	}
	return strings.Join(fragments, "; ")
}

// namesShown is how many added dependencies the summary names before it counts the rest.
const namesShown = 5

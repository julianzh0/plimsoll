package shape

import (
	"fmt"
	"strings"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

// The function rules: a function the change made complex, and one it nested deeper (shown in `report` only).
const (
	ruleComplexity = "function-complexity"
	ruleNesting    = "function-nesting"
)

// complexityCounting is how the complexity counts, said in the verify line of every complexity mark and in `explain`.
const complexityCounting = "counted: 1 for each if, loop, switch, catch and conditional expression plus 1 per level it is nested in; 1 for each else, else-if and run of one boolean operator; functions written inside it and markup are not counted"

// functionID names a function: its path, its name, and #n from the second function of that name in its file on.
func functionID(f snapshot.FunctionChange) string {
	if f.Occurrence > 1 {
		return fmt.Sprintf("%s:%s#%d", f.Path, f.Name, f.Occurrence)
	}
	return f.Path + ":" + f.Name
}

// functionMarks are the marks of the functions the change made complex, and the items of those it nested deeper.
func functionMarks(s snapshot.Snapshot) []answer.Mark {
	var marks []answer.Mark
	complexity, nesting := s.Limits.FunctionComplexity, s.Limits.FunctionNesting
	for _, f := range s.Functions.Complex {
		m := answer.Mark{
			ID:         answer.ID(name, ruleComplexity, functionID(f)),
			Change:     answer.New,
			Subject:    answer.Subject{Kind: "function", Paths: []string{f.Path}, Names: []string{f.Name}},
			Confidence: answer.Candidate,
			Measured:   answer.Measured{Value: float64(f.Complexity), Limit: &complexity.Value, LimitSource: complexity.Source, Unit: "cognitive complexity"},
			Hint:       complexityHint(f, nesting.Value),
		}
		if !f.New {
			before := float64(f.ComplexityBefore)
			m.Measured.Before = &before
		}
		marks = append(marks, m)
	}
	for _, f := range s.Functions.Deeper {
		before := answer.Reading{Holds: !f.New && float64(f.NestingBefore) >= nesting.Value, Value: float64(f.NestingBefore)}
		change, found := answer.Compare(before, answer.Reading{Holds: float64(f.Nesting) >= nesting.Value, Value: float64(f.Nesting)}, nesting.Step)
		if !found || change != answer.New && change != answer.Grown {
			continue
		}
		m := answer.Mark{
			ID:         answer.ID(name, ruleNesting, functionID(f)),
			Change:     change,
			Subject:    answer.Subject{Kind: "function", Paths: []string{f.Path}, Names: []string{f.Name}},
			Confidence: answer.Candidate,
			Measured:   answer.Measured{Value: float64(f.Nesting), Limit: &nesting.Value, LimitSource: nesting.Source, Unit: "levels"},
			Hint:       fmt.Sprintf("read lines %d–%d and count the control structures around line %d.", f.Start, f.End, f.NestingLine),
		}
		if !f.New {
			value := float64(f.NestingBefore)
			m.Measured.Before = &value
		}
		marks = append(marks, m)
	}
	return marks
}

// complexityHint is the verify line of a complexity mark: where to read, and its reasons.
func complexityHint(f snapshot.FunctionChange, nestingLimit float64) string {
	var reasons []string
	if float64(f.Nesting) >= nestingLimit {
		reasons = append(reasons, fmt.Sprintf("it nests %d levels deep at line %d", f.Nesting, f.NestingLine))
	}
	if len(f.Repeated) == 2 {
		reasons = append(reasons, fmt.Sprintf("lines %d–%d are repeated at lines %d–%d", f.Repeated[0].Start, f.Repeated[0].End, f.Repeated[1].Start, f.Repeated[1].End))
	}
	return fmt.Sprintf("read lines %d–%d (%d of them added by this change): %s; %s.", f.Start, f.End, f.AddedLines, strings.Join(reasons, ", and "), complexityCounting)
}

// functionRows are the report's rows of the function rules, when a changed product file was measured.
func functionRows(fs snapshot.Functions) []answer.Statistic {
	if fs.Files == 0 {
		return nil
	}
	count := func(n int) *float64 { v := float64(n); return &v }
	rows := []answer.Statistic{
		{Name: "functions measured", Value: count(fs.Measured), Note: "in " + answer.Plural(fs.Files, "changed product file")},
		{Name: "functions over the complexity limit", Value: count(fs.ComplexCrossed), Note: listedNote(len(fs.Complex), fs.ComplexCrossed, fs.PerMeasure)},
		{Name: "functions nested deeper", Value: count(fs.DeeperCrossed), Note: listedNote(len(fs.Deeper), fs.DeeperCrossed, fs.PerMeasure)},
	}
	if len(fs.LeftOut) > 0 {
		rows = append(rows, answer.Statistic{Name: "functions left out as code that existed before", Value: count(len(fs.LeftOut)), Note: leftOutNote(fs.LeftOut)})
	}
	return rows
}

// perMeasure says how the lists are cut.
func perMeasure(n int) string {
	return fmt.Sprintf("at most %s per measure, the largest rise first", words(n))
}

// listedNote is "2 listed", or "3 listed (at most three per measure, the largest rise first)" when more crossed.
func listedNote(listed, crossed, cap int) string {
	if crossed <= listed {
		return fmt.Sprintf("%d listed", listed)
	}
	return fmt.Sprintf("%d listed (%s)", listed, perMeasure(cap))
}

// leftOutShown is how many copies the left-out row names.
const leftOutShown = 5

// leftOutNote counts the functions left out by reason, and names the copies with the file each was found in.
func leftOutNote(list []snapshot.LeftOutFunction) string {
	byReason := map[snapshot.LeftOutReason]int{}
	var copies []string
	for _, l := range list {
		byReason[l.Reason]++
		if l.Reason == snapshot.Copied && len(copies) < leftOutShown {
			copies = append(copies, fmt.Sprintf("`%s` `%s` in `%s`", l.Path, l.Name, l.Source))
		}
	}
	var parts []string
	for _, reason := range []snapshot.LeftOutReason{snapshot.FromAnotherProject, snapshot.Extracted, snapshot.Moved, snapshot.Copied} {
		n := byReason[reason]
		switch {
		case n == 0:
		case reason == snapshot.FromAnotherProject:
			parts = append(parts, fmt.Sprintf("%d in files from another project", n))
		case reason == snapshot.Extracted:
			parts = append(parts, fmt.Sprintf("%d split out of code that existed", n))
		case reason == snapshot.Moved:
			parts = append(parts, fmt.Sprintf("%d moved within the change", n))
		case reason == snapshot.Copied:
			parts = append(parts, fmt.Sprintf("%d found at the base (%s)", n, strings.Join(copies, ", ")))
		}
	}
	return fmt.Sprintf("%d: %s", len(list), strings.Join(parts, "; "))
}

// functionSummary is the status sentence when a list was cut: what is listed of what crossed, and that report counts
// the rest.
func functionSummary(fs snapshot.Functions) []string {
	var parts []string
	if fs.ComplexCrossed > len(fs.Complex) {
		parts = append(parts, fmt.Sprintf("%d of %d functions over the complexity limit are listed", len(fs.Complex), fs.ComplexCrossed))
	}
	if fs.DeeperCrossed > len(fs.Deeper) {
		parts = append(parts, fmt.Sprintf("%d of %d functions nested deeper are listed", len(fs.Deeper), fs.DeeperCrossed))
	}
	if len(parts) == 0 {
		return nil
	}
	return []string{fmt.Sprintf("Function limits: %s (%s); `report` counts the rest.", strings.Join(parts, ", and "), perMeasure(fs.PerMeasure))}
}

// functionsNotAnalysed are what the function rules could not see.
func functionsNotAnalysed(fs snapshot.Functions) []string {
	var found []string
	if len(fs.Unmeasured) > 0 {
		found = append(found, "functions are not measured for "+answer.QuotedList(fs.Unmeasured))
	}
	if fs.Broken > 0 {
		found = append(found, answer.Agree(fs.Broken, "function holds a syntax error, now or at the base, and is not measured",
			"functions hold a syntax error, now or at the base, and are not measured"))
	}
	if len(fs.Minified) > 0 {
		found = append(found, fmt.Sprintf("%s more than %d bytes a line on average, so its functions were not measured: %s",
			answer.Agree(len(fs.Minified), "changed file has", "changed files have"), fs.MinifiedLineBytes, answer.QuotedList(fs.Minified)))
	}
	if len(fs.Unread) > 0 {
		found = append(found, fmt.Sprintf("the parser could not read the functions of %s: %s", answer.Plural(len(fs.Unread), "changed file"), answer.QuotedList(fs.Unread)))
	}
	if fs.Uncompared > 0 {
		pronoun := "they are"
		if fs.Uncompared == 1 {
			pronoun = "it is"
		}
		found = append(found, fmt.Sprintf("%s compared with existing code (at most %d per measure are), so %s not listed",
			answer.Agree(fs.Uncompared, "function over a function limit was not", "functions over a function limit were not"), fs.Checks,
			pronoun))
	}
	return found
}

// functionsInStanding is the status clause of the nesting items: how many, and the deepest by name.
func functionsInStanding(items []answer.Mark) string {
	var nested []answer.Mark
	for _, m := range items {
		if answer.RuleOf(m.ID) == ruleNesting {
			nested = append(nested, m)
		}
	}
	if len(nested) == 0 {
		return ""
	}
	deepest := nested[0]
	for _, m := range nested[1:] {
		if m.Measured.Value > deepest.Measured.Value {
			deepest = m
		}
	}
	named := fmt.Sprintf("`%s` `%s`", deepest.Subject.Paths[0], deepest.Subject.Names[0])
	if len(nested) == 1 {
		return fmt.Sprintf("1 function nested deeper is listed, %s (%d levels)", named, int(deepest.Measured.Value))
	}
	return fmt.Sprintf("%d functions nested deeper are listed, %s the deepest of them (%d levels)", len(nested), named, int(deepest.Measured.Value))
}

// words writes a small count as a word, as a sentence reads it: "three".
func words(n int) string {
	names := []string{"zero", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten"}
	if n >= 0 && n < len(names) {
		return names[n]
	}
	return fmt.Sprint(n)
}

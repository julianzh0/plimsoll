package settings

import (
	"fmt"
	"strings"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

// Wording is the sentence for one of the detector's marks, from the mark's fields alone.
func (Detector) Wording(m answer.Mark) string {
	settingName, file := m.Subject.Names[0], snapshot.SettingsFile
	switch answer.RuleOf(m.ID) {
	case ruleDefinedTwice:
		if m.Change == answer.Resolved {
			return fmt.Sprintf("`%s` is defined on %s of `%s`, no longer on more than one; it was on %s at the base.",
				settingName, answer.Plural(int(m.Measured.Value), "line"), file, answer.Count(*m.Measured.Before))
		}
		before := fmt.Sprintf("`%s` did not exist at the base", file)
		if m.Measured.Before != nil {
			before = fmt.Sprintf("it was on %s at the base", answer.Count(*m.Measured.Before))
		}
		return fmt.Sprintf("`%s` is defined on %s of `%s`; %s. Keep one of them.", settingName, answer.Plural(int(m.Measured.Value), "line"), file, before)
	case ruleComposeOnly:
		if m.Change == answer.Resolved {
			return fmt.Sprintf("`%s` no longer stands as a compose variable without a setting.", settingName)
		}
		return fmt.Sprintf("`%s` is set in the `environment` of %s; `%s` has no line for it.", settingName, answer.Plural(int(m.Measured.Value), "compose file"), file)
	}
	return m.ID
}

// Standing is the detector's part of the standing line: one count per rule that has standing items.
func (Detector) Standing(items []answer.Mark) string {
	counts := map[string]int{}
	for _, m := range items {
		counts[answer.RuleOf(m.ID)]++
	}
	var fragments []string
	for _, part := range []struct{ rule, one, many string }{
		{ruleDefinedTwice, "name defined on more than one line", "names defined on more than one line"},
		{ruleComposeOnly, "compose variable without a setting", "compose variables without a setting"},
	} {
		switch n := counts[part.rule]; n {
		case 0:
		case 1:
			fragments = append(fragments, "1 "+part.one)
		default:
			fragments = append(fragments, fmt.Sprintf("%d %s", n, part.many))
		}
	}
	return strings.Join(fragments, "; ")
}

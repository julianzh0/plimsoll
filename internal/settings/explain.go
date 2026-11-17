package settings

import (
	"fmt"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

// Explain gives the evidence of one mark, computed again: names, paths and counts, never a value.
func (Detector) Explain(s snapshot.Snapshot, m answer.Mark) []string {
	settingName := m.Subject.Names[0]
	switch answer.RuleOf(m.ID) {
	case ruleDefinedTwice:
		return []string{fmt.Sprintf("lines that define it and are not commented out: %d; at the base: %d",
			activeLines(s.Now.Settings, settingName), activeLines(s.Before.Settings, settingName))}
	case ruleComposeOnly:
		var lines []string
		for _, path := range composeOnly(s.Now)[settingName] {
			lines = append(lines, "`"+path+"` sets it")
		}
		names := 0
		if s.Now.Settings != nil {
			names = len(s.Now.Settings.Names)
		}
		return append(lines, fmt.Sprintf("`%s` names: %d; this one is not among them", snapshot.SettingsFile, names))
	}
	return nil
}

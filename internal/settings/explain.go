package settings

import (
	"fmt"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

// Explain gives the evidence of one mark, computed again: names, paths and counts, never a value.
func (Detector) Explain(s snapshot.Snapshot, m answer.Mark) []string {
	settingName := m.Subject.Names[0]
	switch answer.RuleOf(m.ID) {
	case ruleNoReader:
		index := slices.IndexFunc(s.Now.Uses, func(u snapshot.SettingUse) bool { return u.Name == settingName })
		var use snapshot.SettingUse
		if index >= 0 {
			use = s.Now.Uses[index]
		}
		lines := []string{
			"forms searched as whole words: " + quoted(snapshot.Forms(settingName)),
			fmt.Sprintf("product files searched: %s; holding a form: %d", answer.Count(productFiles(s.Now)), len(use.Product)),
			fmt.Sprintf("test files holding a form: %d", len(use.Test)),
		}
		lines = append(lines, answer.Quoted(use.Test)...)
		rest := "no"
		if use.Rest != "" {
			rest = "yes"
		}
		_, remainder, _ := strings.Cut(settingName, "_")
		computed := "reads with a computed name in product code: 0"
		if reads, files := computedReads(s.Now); reads > 0 {
			computed = fmt.Sprintf("reads with a computed name in product code: %d, in %s", reads, named(files))
		}
		return append(lines, fmt.Sprintf("product files that declare a prefix of the name and hold the rest: %d", len(use.Prefixed)),
			fmt.Sprintf("the name without its first part, `%s`, in product code: %s", remainder, rest), computed)
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

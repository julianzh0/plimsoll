package snapshot

import (
	"slices"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// ImportRule is one import rule that now's own file declares, applied to the links of both sides.
type ImportRule struct {
	Kind       ImportRuleKind
	Table, Key string // where the file declares it
	From       string // the folder whose product code imports
	// Targets are, for a Never rule, the one folder From imports nothing under; for an Only rule, every folder its
	// entries name, in file order, each once: From imports code only under them and under itself.
	Targets []string
}

// ImportRuleKind is what an import rule says about the imports of its folder.
type ImportRuleKind int

const (
	// Never is a rule whose folder imports no code under its target.
	Never ImportRuleKind = iota + 1
	// Only is a rule whose folder imports code only under its targets and itself.
	Only
)

// importRules are the import rules now's own file declares, in the order of its keys: a Never rule per entry, an
// Only rule per source folder, in the order of the source's first entry.
func importRules(rules conventions.Rules, now ownFile) []ImportRule {
	var found []ImportRule
	for _, k := range rules.OwnKeys {
		if !k.Holds.ImportRule() {
			continue
		}
		kind := Never
		if k.Holds == conventions.OnlyImportRules {
			kind = Only
		}
		for _, value := range now.values[k.Key] {
			from, to := conventions.ImportRuleEnds(value)
			at := slices.IndexFunc(found, func(r ImportRule) bool { return kind == Only && r.Kind == Only && r.From == from })
			if at < 0 {
				found = append(found, ImportRule{Kind: kind, Table: k.Table, Key: k.Key, From: from})
				at = len(found) - 1
			}
			found[at].Targets = appendNew(found[at].Targets, to)
		}
	}
	return found
}

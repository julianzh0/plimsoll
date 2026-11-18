package snapshot

import (
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

// Entry prints the rule's entry for one of its targets, as the file declares it in its form: "a -> b".
func (r ImportRule) Entry(target string) string { return conventions.ImportRule(r.From, target) }

// ImportRuleKind is what an import rule says about the imports of its folder.
type ImportRuleKind int

const (
	// Never is a rule whose folder imports no code under its target.
	Never ImportRuleKind = iota + 1
	// Only is a rule whose folder imports code only under its targets and itself.
	Only
)

// importRules are the import rules now's own file declares, in the order of its keys: a Never rule per entry, an
// Only rule per source folder, in the order of the source's first entry. records are one per key that holds a value
// now, its values in use being the rules as rebuilt.
func importRules(rules conventions.Rules, now ownFile) (found []ImportRule, records []Declared) {
	onlyAt := map[string]int{} // an Only rule's source -> its index in found
	for _, k := range rules.OwnKeys {
		values := now.values[k.Key]
		if !k.Holds.ImportRule() || len(values)+now.unusable[k.Key] == 0 {
			continue
		}
		record := Declared{File: rules.OwnFile, Table: k.Table, Key: k.Key, Name: k.Name, AsWritten: true, Now: KeyRead, Unusable: now.unusable[k.Key], Form: k.Holds.Shape()}
		for _, value := range values {
			record.InUse = append(record.InUse, DeclaredFolder{Path: value})
			from, to := conventions.ImportRuleEnds(value)
			if k.Holds == conventions.NeverImportRules {
				found = append(found, ImportRule{Kind: Never, Table: k.Table, Key: k.Key, From: from, Targets: []string{to}})
				continue
			}
			at, known := onlyAt[from]
			if !known {
				at, onlyAt[from] = len(found), len(found)
				found = append(found, ImportRule{Kind: Only, Table: k.Table, Key: k.Key, From: from})
			}
			found[at].Targets = appendNew(found[at].Targets, to)
		}
		records = append(records, record)
	}
	return found, records
}

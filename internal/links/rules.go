package links

import (
	"cmp"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

// The rules of the import rules a repository declares.
const (
	ruleNever = "never"
	ruleOnly  = "only"
)

const (
	importRulesRow  = "import rules with imports that break them"
	importRulesNote = "import rules: a product file under a rule's first folder is the importer; `never` counts its imports of code " +
		"under the second folder, `only` its imports of code outside the first folder and the folders its entries name; " +
		"type-only imports count; test code and other code are no importers; outside packages and imports through a third file are not seen"
	notCountedLine = "not counted: imports of outside packages, and imports through a third file"
)

// withRules adds what the declared import rules show: their marks, their row and notes, and what they cannot check.
// The line of a rule that checks nothing goes first in the answer's "not analysed"; the rule lines come first among
// the detector's own.
func withRules(f answer.Findings, s snapshot.Snapshot, now, before graph) answer.Findings {
	var notAnalysed []string
	broken, brokenAtBase := 0.0, 0.0
	for _, r := range s.ImportRules {
		_, partly, nothing := checkOf(s, r, now)
		if nothing != "" {
			f.NotAnalysedFirst = append(f.NotAnalysedFirst, ruleLead(r)+": "+nothing+", so the rule checks nothing")
			continue
		}
		if partly != "" {
			notAnalysed = append(notAnalysed, ruleLead(r)+": "+partly)
		}
		notAnalysed = append(notAnalysed, targetsWithoutCode(s, r)...)
		nowImports, baseImports := breaking(r, s.Now, now, s.Unparsed), breaking(r, s.Before, before, s.Unparsed)
		if len(nowImports) > 0 {
			broken++
		}
		if len(baseImports) > 0 {
			brokenAtBase++
		}
		if m, found := ruleMark(s, r, nowImports, len(baseImports)); found {
			f.Marks = append(f.Marks, m)
		}
	}
	f.NotAnalysed = slices.Concat(notAnalysed, answer.OwnNotUsed(s.Layout.Rules), f.NotAnalysed)
	if len(s.ImportRules) > 0 {
		f.Statistics = append(f.Statistics, answer.Statistic{Name: importRulesRow, Value: &broken, Before: &brokenAtBase, Unit: "rules",
			Note: fmt.Sprintf("of the %s made of the %s in `%s`", answer.Plural(len(s.ImportRules), "rule"), answer.Agree(entries(s.ImportRules), "entry", "entries"), s.OwnFile)})
		f.Notes = append(f.Notes, importRulesNote)
	}
	f.Notes = append(f.Notes, answer.OwnNotes(s.Layout.Rules)...)
	return f
}

// entries counts the entries the rules were made of: a never rule is one, an only rule one per target.
func entries(rules []snapshot.ImportRule) int {
	n := 0
	for _, r := range rules {
		n += len(r.Targets)
	}
	return n
}

// ruleLead names a rule at the start of a sentence: a never rule by its entry, an only rule by its folder.
func ruleLead(r snapshot.ImportRule) string {
	if r.Kind == snapshot.Only {
		return fmt.Sprintf("`[%s]` `%s` rule of %s", r.Table, r.Key, answer.Folder(r.From))
	}
	return entryLead(r, r.Targets[0])
}

// entryLead names one entry of a rule at the start of a sentence.
func entryLead(r snapshot.ImportRule, target string) string {
	return fmt.Sprintf("`[%s]` `%s` rule `%s`", r.Table, r.Key, r.Entry(target))
}

// checkOf decides whether a rule is checked, on now's side. It gives the product files under the rule's folder whose
// imports are read, what says that some others are not, and what says that the rule checks nothing, or "": when no
// product code lies under its folder, when a never rule's target holds no code, or when no product file under its
// folder has its imports read.
func checkOf(s snapshot.Snapshot, r snapshot.ImportRule, now graph) (importers int, partly, nothing string) {
	products, notRead := 0, 0
	var suffixes, languages []string // what the product files whose imports are not read are: a suffix, a language
	for _, file := range s.Now.Files {
		if file.Role != snapshot.Product || !lies(file.Path, r.From) {
			continue
		}
		products++
		switch {
		case now.analysed(file.Path) && !slices.Contains(s.Unparsed, file.Path):
			importers++
		case file.Language == "":
			suffixes = append(suffixes, path.Ext(file.Path))
		default:
			languages = append(languages, file.Language)
			notRead++
		}
	}
	switch {
	case products == 0:
		return 0, "", gone(r.From, holdsCode(s.Before, r.From, true), "matches no product code")
	case r.Kind == snapshot.Never && !holdsCode(s.Now, r.Targets[0], false):
		return 0, "", gone(r.Targets[0], holdsCode(s.Before, r.Targets[0], false), "matches no code")
	case importers == 0:
		return 0, "", fmt.Sprintf("the imports of %s under %s are not read or not followed (%s)", answer.Plural(products, "product file"),
			answer.Folder(r.From), named(slices.Concat(suffixes, languages)))
	case notRead > 0:
		verb := "are"
		if notRead == 1 {
			verb = "is"
		}
		partly = fmt.Sprintf("%d of the %s under %s %s not checked, because their imports are not read or not followed (%s)",
			notRead, answer.Plural(products, "product file"), answer.Folder(r.From), verb, named(languages))
	}
	return importers, partly, ""
}

// gone says that a folder holds no code now, and whether it held some at the base.
func gone(folder string, heldAtBase bool, none string) string {
	if heldAtBase {
		return answer.Folder(folder) + " matched code at the base and matches none now"
	}
	return answer.Folder(folder) + " " + none
}

// targetsWithoutCode are the lines of an only rule's entries whose folder holds no code now: they allow nothing.
func targetsWithoutCode(s snapshot.Snapshot, r snapshot.ImportRule) []string {
	var lines []string
	for _, target := range r.Targets {
		if !holdsCode(s.Now, target, false) {
			lines = append(lines, entryLead(r, target)+": "+gone(target, holdsCode(s.Before, target, false), "matches no code"))
		}
	}
	return lines
}

// holdsCode says whether a code file lies under a folder on one side; productOnly counts product code only.
func holdsCode(tree snapshot.Tree, folder string, productOnly bool) bool {
	return slices.ContainsFunc(tree.Files, func(f snapshot.File) bool {
		return lies(f.Path, folder) && (!productOnly || f.Role == snapshot.Product)
	})
}

// named lists what cannot be checked, sorted, each once and in backticks; past three, the rest are counted.
func named(what []string) string {
	slices.Sort(what)
	what = slices.Compact(what)
	shown := answer.QuotedList(what[:min(len(what), 3)])
	if len(what) > 3 {
		shown += fmt.Sprintf(" and %d more", len(what)-3)
	}
	return shown
}

// lies says whether a file lies under a folder: the folder's names are the path's first names.
func lies(file, folder string) bool { return strings.HasPrefix(file, folder+"/") }

// breaking are the imports of one side that break a rule, one per importing file and target, by file, line and target
// as the graph holds them.
// A pair is type-only when every statement of it is, and added when one of them counts as added; it carries the
// line of its first statement that counts as added, or else of its first statement. A file the parser could not read
// on either side counts on neither, as checkOf says.
// NOTE: every rule walks the files of both sides, so the work grows with rules times files: 1,000 rules added at most
// 0.07 s to a status of nowinandroid (715 files) and vapor (898 files), measured 2026-11-18. Upgrade: index the files
// by folder once per side when a repository with far more files declares that many rules.
func breaking(r snapshot.ImportRule, tree snapshot.Tree, g graph, unparsed []string) []snapshot.Link {
	allowed := r.Allowed()
	var found []snapshot.Link
	for _, file := range tree.Files {
		if file.Role != snapshot.Product || !lies(file.Path, r.From) || slices.Contains(unparsed, file.Path) {
			continue
		}
		at := map[string]int{} // a target -> its pair in found
		for _, link := range g.from[file.Path] {
			if !breaks(r, allowed, link) {
				continue
			}
			i, seen := at[link.To]
			if !seen {
				at[link.To] = len(found)
				found = append(found, link)
				continue
			}
			found[i].TypeOnly = found[i].TypeOnly && link.TypeOnly
			if link.Added && !found[i].Added { // the pair names its first line counted as added
				found[i].Line, found[i].Added = link.Line, true
			}
		}
	}
	slices.SortStableFunc(found, func(a, b snapshot.Link) int { // an added line can move a pair down its file
		return cmp.Or(strings.Compare(a.From, b.From), cmp.Compare(a.Line, b.Line), strings.Compare(a.To, b.To))
	})
	return found
}

// breaks says whether one import breaks a rule: a never rule when every code file it reaches lies under the target,
// an only rule when none lies under an allowed folder (the rule's own and its targets). The snapshot leaves the
// importer out of the files a link reaches and keeps no link that reaches none.
func breaks(r snapshot.ImportRule, allowed []string, link snapshot.Link) bool {
	for _, f := range link.Files {
		if r.Kind == snapshot.Never && !lies(f, r.Targets[0]) {
			return false
		}
		if r.Kind == snapshot.Only && slices.ContainsFunc(allowed, func(folder string) bool { return lies(f, folder) }) {
			return false
		}
	}
	return true
}

// ruleMark is a rule's mark: new or grown when the change wrote one of its breaking imports, however many were taken
// away; touched when it has more than at the base and wrote none; standing or resolved otherwise.
func ruleMark(s snapshot.Snapshot, r snapshot.ImportRule, nowImports []snapshot.Link, atBase int) (answer.Mark, bool) {
	var clauses []string
	for _, link := range nowImports {
		if link.Added && len(clauses) < closingShown {
			clauses = append(clauses, clause(link))
		}
	}
	count, before := len(nowImports), float64(atBase)
	var change answer.Change
	switch {
	case len(clauses) > 0 && atBase == 0:
		change = answer.New
	case len(clauses) > 0:
		change = answer.Grown
	case count > atBase:
		change = answer.Touched
	case count > 0:
		change = answer.Standing
	case atBase > 0:
		change = answer.Resolved
	default:
		return answer.Mark{}, false
	}
	paths := r.Allowed() // an only rule's folder and the folders it allows
	if r.Kind == snapshot.Never {
		paths = []string{r.From, r.Targets[0]}
	}
	source := ruleSource(s, r)
	return answer.Mark{
		ID:         ruleID(r),
		Change:     change,
		Subject:    answer.Subject{Kind: "rule", Paths: paths, Names: clauses},
		Confidence: answer.Candidate,
		Measured:   answer.Measured{Value: float64(count), Before: &before, LimitSource: source, Unit: "imports"},
		Hint:       "compare its imports with " + source + "; `explain` names every import that breaks the rule by file and line.",
	}, true
}

// ruleSource names where a rule is declared: "`[imports]` `never` of `.plimsoll.toml`".
func ruleSource(s snapshot.Snapshot, r snapshot.ImportRule) string {
	return fmt.Sprintf("`[%s]` `%s` of `%s`", r.Table, r.Key, s.OwnFile)
}

// clause names one breaking import by file, line and target.
func clause(link snapshot.Link) string {
	text := importedAs(link, "importing")
	if link.TypeOnly {
		text += typeOnlySuffix
	}
	return text
}

// ruleWording is the sentence of a rule mark, from its fields alone.
func ruleWording(m answer.Mark) string {
	folder := answer.Folder(m.Subject.Paths[0])
	var what string
	if answer.RuleOf(m.ID) == ruleNever {
		what = "of code under " + answer.Folder(m.Subject.Paths[1])
	} else {
		allowed := make([]string, 0, len(m.Subject.Paths))
		for _, p := range m.Subject.Paths {
			allowed = append(allowed, answer.Folder(p))
		}
		what = "of code outside " + joined(allowed, "and")
	}
	count, atBase := int(m.Measured.Value), int(*m.Measured.Before)
	if m.Change == answer.Resolved {
		return fmt.Sprintf("Code under %s has no import %s now; it had %d at the base.", folder, what, atBase)
	}
	had := "none at the base"
	if atBase > 0 {
		had = fmt.Sprintf("%d at the base", atBase)
	}
	sentence := fmt.Sprintf("Code under %s has %s %s, %s", folder, answer.Plural(count, "import"), what, had)
	switch m.Change {
	case answer.New, answer.Grown:
		lead := ", among them "
		if len(m.Subject.Names) == count {
			lead = ": "
		}
		sentence += lead + strings.Join(m.Subject.Names, " and ") + addedSuffix
	case answer.Touched:
		sentence += "; this change added none of them"
	case answer.Resolved, answer.Standing:
	}
	return sentence + "; " + m.Measured.LimitSource + " allows none."
}

// ruleEvidence names the rule, its numbers and every import that breaks it, by file and line; for a resolved rule,
// those of the base.
func ruleEvidence(s snapshot.Snapshot, m answer.Mark, now, before graph) []string {
	at := slices.IndexFunc(s.ImportRules, func(r snapshot.ImportRule) bool { return ruleID(r) == m.ID })
	if at < 0 {
		return nil
	}
	r := s.ImportRules[at]
	var lines []string
	if r.Kind == snapshot.Never {
		lines = append(lines, "rule: `"+r.Entry(r.Targets[0])+"` ("+ruleSource(s, r)+")")
	} else {
		var entries []string
		for _, target := range r.Targets {
			entries = append(entries, "`"+r.Entry(target)+"`")
		}
		lines = append(lines, fmt.Sprintf("rule: code under %s imports only code under %s (`[%s]` `%s`: %s)", answer.Folder(r.From),
			folderList(r.Allowed()), r.Table, r.Key, strings.Join(entries, ", ")))
	}
	nowImports, baseImports := breaking(r, s.Now, now, s.Unparsed), breaking(r, s.Before, before, s.Unparsed)
	lines = append(lines, fmt.Sprintf("imports that break it now: %d; at the base: %d", len(nowImports), len(baseImports)))
	shown, prefix, verb := nowImports, "", "imports"
	if m.Change == answer.Resolved {
		shown, prefix, verb = baseImports, "at the base ", "imported"
	}
	for _, link := range shown {
		lines = append(lines, prefix+linkLine(link, verb))
	}
	importers, partly, _ := checkOf(s, r, now)
	lines = append(lines, fmt.Sprintf("searched: the imports of the %s under %s; test code and other code are not importers",
		answer.Plural(importers, "product file"), answer.Folder(r.From)))
	if partly != "" {
		lines = append(lines, partly)
	}
	return append(lines, notCountedLine)
}

// ruleID is the identifier of a rule's mark.
func ruleID(r snapshot.ImportRule) string {
	if r.Kind == snapshot.Only {
		return answer.ID(name, ruleOnly, r.From)
	}
	return answer.ID(name, ruleNever, r.From+"->"+r.Targets[0])
}

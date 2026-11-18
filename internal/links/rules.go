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

// rulesShown is how many imports the change added a rule mark names.
const rulesShown = 2

// ruleCheck is one declared rule with the product files under its folder whose imports are read now.
type ruleCheck struct {
	rule      snapshot.ImportRule
	importers int // product files under the rule's folder whose imports are read now
	products  int // product files under the rule's folder now
	partly    string
}

// withRules adds what the declared import rules show: their marks, their row and notes, and what they cannot check.
// The line of a rule that checks nothing goes first in the answer's "not analysed"; the rule lines come first among
// the detector's own.
func withRules(f answer.Findings, s snapshot.Snapshot, languages []snapshot.LanguageImports, now, before graph) answer.Findings {
	var notAnalysed []string
	broken, brokenAtBase := 0.0, 0.0
	for _, r := range s.ImportRules {
		check, nothing := checkOf(s, r, languages)
		if nothing != "" {
			f.NotAnalysedFirst = append(f.NotAnalysedFirst, ruleLead(r)+": "+nothing+", so the rule checks nothing")
			continue
		}
		if check.partly != "" {
			notAnalysed = append(notAnalysed, ruleLead(r)+": "+check.partly)
		}
		notAnalysed = append(notAnalysed, targetsWithoutCode(s, r)...)
		nowImports, baseImports := breaking(r, s.Now, now), breaking(r, s.Before, before)
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
			Note: fmt.Sprintf("of %d declared in `%s`", len(s.ImportRules), s.OwnFile)})
		f.Notes = append(f.Notes, importRulesNote)
	}
	f.Notes = append(f.Notes, answer.OwnNotes(s.Layout.Rules)...)
	return f
}

// ruleLead names a rule at the start of a sentence: a never rule by its entry, an only rule by its folder.
func ruleLead(r snapshot.ImportRule) string {
	if r.Kind == snapshot.Only {
		return fmt.Sprintf("`[%s]` `%s` rule of %s", r.Table, r.Key, answer.Folder(r.From))
	}
	return fmt.Sprintf("`[%s]` `%s` rule `%s`", r.Table, r.Key, r.Entry(r.Targets[0]))
}

// checkOf decides whether a rule is checked, on now's side: what says it checks nothing, or "" and the rule's
// importers. A rule checks nothing when no product code lies under its folder, when a never rule's target holds no
// code, or when no product file under its folder has its imports read.
func checkOf(s snapshot.Snapshot, r snapshot.ImportRule, languages []snapshot.LanguageImports) (ruleCheck, string) {
	used := map[string]bool{}
	for _, l := range languages {
		used[l.Language] = true
	}
	check := ruleCheck{rule: r}
	var notRead []string // what the product files whose imports are not read are: a suffix, or a language over the limit
	overLimit := 0
	for _, file := range s.Now.Files {
		if file.Role != snapshot.Product || !lies(file.Path, r.From) {
			continue
		}
		check.products++
		switch {
		case used[file.Language]:
			check.importers++
		case file.Language == "":
			notRead = appendOnce(notRead, path.Ext(file.Path))
		default:
			notRead = appendOnce(notRead, file.Language)
			overLimit++
		}
	}
	switch {
	case check.products == 0:
		return check, gone(r.From, holdsCode(s.Before, r.From, true), "matches no product code")
	case r.Kind == snapshot.Never && !holdsCode(s.Now, r.Targets[0], false):
		return check, gone(r.Targets[0], holdsCode(s.Before, r.Targets[0], false), "matches no code")
	case check.importers == 0:
		return check, fmt.Sprintf("the imports of %s under %s are not read or not followed (%s)", answer.Plural(check.products, "product file"),
			answer.Folder(r.From), named(notRead))
	case overLimit > 0:
		var languagesOver []string
		for _, what := range notRead {
			if !strings.HasPrefix(what, ".") {
				languagesOver = append(languagesOver, what)
			}
		}
		check.partly = fmt.Sprintf("%d of the %s under %s are not checked, because their imports are not followed (%s)",
			overLimit, answer.Plural(check.products, "product file"), answer.Folder(r.From), named(languagesOver))
	}
	return check, ""
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
	if r.Kind != snapshot.Only {
		return nil
	}
	var lines []string
	for _, target := range r.Targets {
		if !holdsCode(s.Now, target, false) {
			lines = append(lines, fmt.Sprintf("`[%s]` `%s` rule `%s`: %s", r.Table, r.Key, r.Entry(target), gone(target, holdsCode(s.Before, target, false), "matches no code")))
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

// named lists what cannot be checked, sorted, each in backticks; past three, the rest are counted.
func named(what []string) string {
	slices.Sort(what)
	shown := answer.QuotedList(what[:min(len(what), 3)])
	if len(what) > 3 {
		shown += fmt.Sprintf(" and %d more", len(what)-3)
	}
	return shown
}

func appendOnce(list []string, value string) []string {
	if slices.Contains(list, value) {
		return list
	}
	return append(list, value)
}

// lies says whether a file lies under a folder: the folder's names are the path's first names.
func lies(file, folder string) bool { return strings.HasPrefix(file, folder+"/") }

// breaking are the imports of one side that break a rule, one per importing file and target, by file, line and target.
// A pair is type-only when every statement of it is, and added when the change wrote one of them; it carries its
// first line.
func breaking(r snapshot.ImportRule, tree snapshot.Tree, g graph) []snapshot.Link {
	var found []snapshot.Link
	for _, file := range tree.Files {
		if file.Role != snapshot.Product || !lies(file.Path, r.From) {
			continue
		}
		at := map[string]int{} // a target -> its pair in found
		for _, link := range g.from[file.Path] {
			if !breaks(r, link) {
				continue
			}
			i, seen := at[link.To]
			if !seen {
				at[link.To] = len(found)
				found = append(found, link)
				continue
			}
			found[i].TypeOnly = found[i].TypeOnly && link.TypeOnly
			found[i].Added = found[i].Added || link.Added
		}
	}
	slices.SortStableFunc(found, func(a, b snapshot.Link) int {
		return cmp.Or(strings.Compare(a.From, b.From), cmp.Compare(a.Line, b.Line), strings.Compare(a.To, b.To))
	})
	return found
}

// breaks says whether one import breaks a rule: a never rule when every code file it reaches lies under the target,
// an only rule when none lies under the rule's folder or one of its targets. An import that reaches only its own file
// breaks nothing.
func breaks(r snapshot.ImportRule, link snapshot.Link) bool {
	reached := slices.DeleteFunc(slices.Clone(link.Files), func(f string) bool { return f == link.From })
	if len(reached) == 0 {
		return false
	}
	if r.Kind == snapshot.Never {
		return !slices.ContainsFunc(reached, func(f string) bool { return !lies(f, r.Targets[0]) })
	}
	allowed := append([]string{r.From}, r.Targets...)
	return !slices.ContainsFunc(reached, func(f string) bool {
		return slices.ContainsFunc(allowed, func(folder string) bool { return lies(f, folder) })
	})
}

// ruleMark is a rule's mark: new or grown when the change wrote one of its breaking imports, however many were taken
// away; touched when it has more than at the base and wrote none; standing or resolved otherwise.
func ruleMark(s snapshot.Snapshot, r snapshot.ImportRule, nowImports []snapshot.Link, atBase int) (answer.Mark, bool) {
	var clauses []string
	for _, link := range nowImports {
		if link.Added && len(clauses) < rulesShown {
			clauses = append(clauses, clause(link))
		}
	}
	count, before := len(nowImports), float64(atBase)
	var change answer.Change
	switch {
	case count > 0 && len(clauses) > 0 && atBase == 0:
		change = answer.New
	case count > 0 && len(clauses) > 0:
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
	paths := []string{r.From, r.Targets[0]}
	if r.Kind == snapshot.Only {
		paths = []string{r.From}
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
	text := fmt.Sprintf("`%s` line %d importing %s", link.From, link.Line, targetShown(link))
	if link.TypeOnly {
		text += typeOnlySuffix
	}
	return text
}

// ruleWording is the sentence of a rule mark, from its fields alone.
func ruleWording(m answer.Mark) string {
	folder := answer.Folder(m.Subject.Paths[0])
	what := "of code outside " + folder + " and the folders its rule names"
	if answer.RuleOf(m.ID) == ruleNever {
		what = "of code under " + answer.Folder(m.Subject.Paths[1])
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
	return sentence + "; " + m.Measured.LimitSource + " declares that it has none."
}

// ruleEvidence names the rule, its numbers and every import that breaks it, by file and line; for a resolved rule,
// those of the base.
func ruleEvidence(s snapshot.Snapshot, m answer.Mark, languages []snapshot.LanguageImports, now, before graph) []string {
	at := slices.IndexFunc(s.ImportRules, func(r snapshot.ImportRule) bool { return ruleID(r) == m.ID })
	if at < 0 {
		return nil
	}
	r := s.ImportRules[at]
	lines := []string{"rule: `" + r.Entry(r.Targets[0]) + "` (" + ruleSource(s, r) + ")"}
	if r.Kind == snapshot.Only {
		allowed, entries := []string{answer.Folder(r.From)}, []string{}
		for _, target := range r.Targets {
			allowed = appendOnce(allowed, answer.Folder(target))
			entries = append(entries, "`"+r.Entry(target)+"`")
		}
		lines[0] = fmt.Sprintf("rule: code under %s imports only code under %s (`[%s]` `%s`: %s)", answer.Folder(r.From),
			strings.Join(allowed, ", "), r.Table, r.Key, strings.Join(entries, ", "))
	}
	nowImports, baseImports := breaking(r, s.Now, now), breaking(r, s.Before, before)
	lines = append(lines, fmt.Sprintf("imports that break it now: %d; at the base: %d", len(nowImports), len(baseImports)))
	shown, prefix, verb := nowImports, "", "imports"
	if m.Change == answer.Resolved {
		shown, prefix, verb = baseImports, "at the base ", "imported"
	}
	for _, link := range shown {
		lines = append(lines, prefix+linkLine(link, verb))
	}
	check, _ := checkOf(s, r, languages)
	searched := fmt.Sprintf("searched: the imports of the %s under %s; test code and other code are not importers",
		answer.Plural(check.importers, "product file"), answer.Folder(r.From))
	lines = append(lines, searched)
	if check.partly != "" {
		lines = append(lines, check.partly)
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

package links

import (
	"fmt"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

// The rules of the detector.
const (
	ruleTestImport  = "test-import"
	ruleCycle       = "cycle"
	ruleFolderCycle = "folder-cycle"
)

const (
	cycleSource       = "files do not import each other in a ring"
	testImportSource  = "product code imports no test code"
	folderCycleSource = "the Acyclic Dependencies Principle (Robert C. Martin)"
	hint              = "found by reading the import statements; `explain` names each import by file and line."
	folderHint        = "found by reading the import statements; `explain` names the imports between the folders by file and line."
)

// shapeOf is, per rule, the subject kind, the limit's source, the unit and the hint of its marks.
func shapeOf(rule string) (kind, source, unit, markHint string) {
	switch rule {
	case ruleTestImport:
		return "file", testImportSource, "imports of test code", hint
	case ruleFolderCycle:
		return "folders", folderCycleSource, "folders", folderHint
	}
	return "files", cycleSource, "files", hint
}

// testImportMarks compares, per product file, its imports of test code at the base and now. A renamed file takes its
// base side from its old name; a product file gone since the base can be resolved under its base path.
func testImportMarks(s snapshot.Snapshot, now, before graph) []answer.Mark {
	created := func(path string) bool { return added(now.testImports(path)) }
	var marks []answer.Mark
	seen := map[string]bool{}
	testTargets := func(g graph, path string) float64 { return float64(g.testTargets(path)) }
	for _, file := range s.Now.Files {
		if file.Role != snapshot.Product {
			continue
		}
		basePath := s.BasePath(file.Path)
		seen[basePath] = true
		role, existed := before.roles[basePath]
		atBase := 0.0
		if role == snapshot.Product {
			atBase = testTargets(before, basePath)
		}
		marks = appendMark(marks, ruleTestImport, []string{file.Path}, atBase, atBase > 0, existed, testTargets(now, file.Path), created(file.Path))
	}
	for _, file := range s.Before.Files {
		if file.Role == snapshot.Product && !seen[file.Path] {
			atBase := testTargets(before, file.Path)
			marks = appendMark(marks, ruleTestImport, []string{file.Path}, atBase, atBase > 0, true, 0, false)
		}
	}
	return marks
}

// cycleMarks compares the groups now with those at the base for one rule. A group now is matched to the base groups
// that share a member, through the names atBase gives a member at the base; its value at the base is the largest of
// them. A base group that shares no member with a group now is resolved under its own first member. created says
// whether the change added an import a group rests on.
func cycleMarks(rule string, now, before [][]string, atBase func(member string) []string, created func(group []string) bool) []answer.Mark {
	ringAt := map[string]int{}
	for i, ring := range before {
		for _, member := range ring {
			ringAt[member] = i
		}
	}
	matched := map[int]bool{}
	var marks []answer.Mark
	for _, ring := range now {
		value, holdsBase := 0.0, false
		for _, member := range ring {
			for _, name := range atBase(member) {
				if i, shared := ringAt[name]; shared {
					matched[i], holdsBase = true, true
					value = max(value, float64(len(before[i])))
				}
			}
		}
		marks = appendMark(marks, rule, ring, value, holdsBase, holdsBase, float64(len(ring)), created(ring))
	}
	for i, ring := range before {
		if !matched[i] {
			marks = appendMark(marks, rule, ring, float64(len(ring)), true, true, 0, false)
		}
	}
	return marks
}

// appendMark adds the mark of one subject when Compare finds one. The subject's first path names it; the value at
// the base is kept when the subject existed there. A new or grown subject the change added no import of is touched.
func appendMark(marks []answer.Mark, rule string, paths []string, atBase float64, holdsBase, existed bool, now float64, created bool) []answer.Mark {
	change, found := answer.Compare(answer.Reading{Holds: holdsBase, Value: atBase}, answer.Reading{Holds: now > 0, Value: now}, 1)
	if !found {
		return marks
	}
	if (change == answer.New || change == answer.Grown) && !created {
		change = answer.Touched
	}
	kind, source, unit, markHint := shapeOf(rule)
	m := answer.Mark{
		ID:         answer.ID(name, rule, paths[0]),
		Change:     change,
		Subject:    answer.Subject{Kind: kind, Paths: paths},
		Confidence: answer.Candidate,
		Measured:   answer.Measured{Value: now, LimitSource: source, Unit: unit},
		Hint:       markHint,
	}
	if existed {
		m.Measured.Before = &atBase
	}
	return append(marks, m)
}

// added says whether one of the links is an import the change wrote.
func added(links []snapshot.Link) bool {
	return slices.ContainsFunc(links, func(l snapshot.Link) bool { return l.Added })
}

// closingShown is how many imports the change added a ring mark names before it counts the rest.
const closingShown = 2

// withClosingImports starts the hint of each new or grown ring with the imports between its members the change added:
// those that closed a new ring, or those it grew through.
func withClosingImports(marks []answer.Mark, now graph) []answer.Mark {
	for i, m := range marks {
		if m.Change != answer.New && m.Change != answer.Grown {
			continue
		}
		var closing []string
		for _, l := range now.ringLinks(m.Subject.Paths) {
			if l.Added {
				closing = append(closing, fmt.Sprintf("`%s` line %d importing %s", l.From, l.Line, targetShown(l)))
			}
		}
		if len(closing) == 0 {
			continue
		}
		named := strings.Join(closing[:min(len(closing), closingShown)], " and ")
		if len(closing) > closingShown {
			named += fmt.Sprintf(" and %d more", len(closing)-closingShown)
		}
		verb := "closed by "
		if m.Change == answer.Grown {
			verb = "grown through "
		}
		marks[i].Hint = verb + named + ", added by this change; " + m.Hint
	}
	return marks
}

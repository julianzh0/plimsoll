package links

import (
	"fmt"
	"path"
	"strings"

	"github.com/julianzh0/plimsoll/internal/answer"
)

// Wording is the sentence for one mark, from its fields alone.
func (Detector) Wording(m answer.Mark) string {
	switch answer.RuleOf(m.ID) {
	case ruleTestImport:
		return testImportWording(m)
	case ruleCycle:
		return cycleWording(m)
	case ruleFolderCycle:
		return folderCycleWording(m)
	}
	return m.ID
}

func folderCycleWording(m answer.Mark) string {
	first := answer.Folder(m.Subject.Paths[0])
	switch {
	case m.Change == answer.Resolved:
		return fmt.Sprintf("The folder cycle of %s had %s at the base; no cycle found among them now.", first, answer.Plural(int(*m.Measured.Before), "folder"))
	case m.Measured.Before == nil && m.Change == answer.Touched:
		return fmt.Sprintf("%s %s depend on each other in a cycle, the first %s; no cycle was read among them at the base, and %s.",
			answer.Plural(int(m.Measured.Value), "folder"), parentShown(m.Subject.Paths[0]), first, noneOfTheImportsBetween)
	case m.Measured.Before == nil:
		return fmt.Sprintf("%s %s depend on each other in a cycle, the first %s; none of them was in a folder cycle at the base.",
			answer.Plural(int(m.Measured.Value), "folder"), parentShown(m.Subject.Paths[0]), first)
	}
	grew := fmt.Sprintf("The folder cycle %s of %s has %s, %s at the base", parentShown(m.Subject.Paths[0]), first,
		answer.Plural(int(m.Measured.Value), "folder"), answer.Count(*m.Measured.Before))
	return touchedEnding(m, grew, noneOfTheImportsBetween)
}

// The endings of a touched item: the change added none of the imports it rests on.
const (
	noneOfItsImports        = "this change added none of its imports"
	noneOfTheImportsBetween = "this change added none of the imports between them"
)

// touchedEnding ends a sentence: with what the change did not add when the item is touched, else with a full stop.
func touchedEnding(m answer.Mark, sentence, ending string) string {
	if m.Change == answer.Touched {
		return sentence + "; " + ending + "."
	}
	return sentence + "."
}

// parentShown names where a folder lies: "under `src/`", or "at the root".
func parentShown(folder string) string { return places([]string{path.Dir(folder)}, "") }

func testImportWording(m answer.Mark) string {
	path := m.Subject.Paths[0]
	if m.Change == answer.Resolved {
		return fmt.Sprintf("No import of test code found in `%s` now; it had %s at the base.", path, answer.Count(*m.Measured.Before))
	}
	targets := "1 file or package"
	if m.Measured.Value != 1 {
		targets = answer.Count(m.Measured.Value) + " files or packages"
	}
	has := fmt.Sprintf("`%s` is product code and imports %s of test code", path, targets)
	switch {
	case m.Change == answer.Touched && m.Measured.Before != nil:
		return has + ", " + answer.Count(*m.Measured.Before) + " at the base; this change added none of their imports."
	case m.Measured.Before == nil:
		return has + "; it did not exist at the base."
	case *m.Measured.Before == 0:
		return has + "; it imported none at the base."
	}
	return has + ", " + answer.Count(*m.Measured.Before) + " at the base."
}

func cycleWording(m answer.Mark) string {
	path := m.Subject.Paths[0]
	switch {
	case m.Change == answer.Resolved:
		return fmt.Sprintf("The ring of `%s` had %s at the base; no ring found among them now.", path, answer.Plural(int(*m.Measured.Before), "file"))
	case m.Measured.Before == nil && m.Change == answer.Touched:
		return fmt.Sprintf("%s import each other in a ring, the first `%s`; no ring was read among them at the base, and %s.",
			answer.Plural(int(m.Measured.Value), "file"), path, noneOfItsImports)
	case m.Measured.Before == nil:
		return fmt.Sprintf("%s import each other in a ring, the first `%s`; none of them was in a ring at the base.",
			answer.Plural(int(m.Measured.Value), "file"), path)
	}
	grew := fmt.Sprintf("The ring of `%s` has %s, %s at the base", path, answer.Plural(int(m.Measured.Value), "file"), answer.Count(*m.Measured.Before))
	return touchedEnding(m, grew, noneOfItsImports)
}

// Standing is the detector's part of the one standing line.
func (Detector) Standing(items []answer.Mark) string {
	var fragments []string
	testImports, rings, folders := 0, 0, 0
	for _, m := range items {
		switch answer.RuleOf(m.ID) {
		case ruleTestImport:
			testImports++
		case ruleCycle:
			rings++
		case ruleFolderCycle:
			folders++
		}
	}
	if testImports > 0 {
		fragments = append(fragments, answer.Plural(testImports, "product file")+" with imports of test code")
	}
	if rings > 0 {
		fragments = append(fragments, answer.Plural(rings, "import ring"))
	}
	if folders > 0 {
		fragments = append(fragments, answer.Plural(folders, "folder cycle"))
	}
	return strings.Join(fragments, "; ")
}

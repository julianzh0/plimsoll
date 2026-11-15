package links

import (
	"fmt"
	"path"
	"strings"

	"github.com/juzhxng/plimsoll/internal/answer"
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
	case m.Measured.Before == nil:
		return fmt.Sprintf("%s %s depend on each other in a cycle, the first %s; none of them was in a folder cycle at the base.",
			answer.Plural(int(m.Measured.Value), "folder"), parentShown(m.Subject.Paths[0]), first)
	}
	return fmt.Sprintf("The folder cycle of %s has %s, %s at the base.", first, answer.Plural(int(m.Measured.Value), "folder"), answer.Count(*m.Measured.Before))
}

// parentShown names where a folder lies: "under `src/`", or "at the root".
func parentShown(folder string) string { return places([]string{path.Dir(folder)}, "") }

func testImportWording(m answer.Mark) string {
	path := m.Subject.Paths[0]
	if m.Change == answer.Resolved {
		return fmt.Sprintf("No import of test code found in `%s` now; it had %s at the base.", path, answer.Count(*m.Measured.Before))
	}
	has := fmt.Sprintf("`%s` is product code and has %s of test code", path, answer.Plural(int(m.Measured.Value), "import"))
	switch {
	case m.Measured.Before == nil:
		return has + "; it did not exist at the base."
	case *m.Measured.Before == 0:
		return has + "; it had none at the base."
	}
	return has + ", " + answer.Count(*m.Measured.Before) + " at the base."
}

func cycleWording(m answer.Mark) string {
	path := m.Subject.Paths[0]
	switch {
	case m.Change == answer.Resolved:
		return fmt.Sprintf("The ring of `%s` had %s at the base; no ring found among them now.", path, answer.Plural(int(*m.Measured.Before), "file"))
	case m.Measured.Before == nil:
		return fmt.Sprintf("%s import each other in a ring, the first `%s`; none of them was in a ring at the base.",
			answer.Plural(int(m.Measured.Value), "file"), path)
	}
	return fmt.Sprintf("The ring of `%s` has %s, %s at the base.", path, answer.Plural(int(m.Measured.Value), "file"), answer.Count(*m.Measured.Before))
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

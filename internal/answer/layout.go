package answer

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/julianzh0/plimsoll/internal/snapshot"
)

// The layout a project declares is a repository fact two detectors print: shape the test folders, links the roots.
// Its sentences live here, once, with the answer's own wording helpers.

// TestFolderNotes are the note lines of the test-folder declarations with a folder in use.
func TestFolderNotes(l snapshot.Layout) []string {
	return layoutNotes(l.TestFolders, "test folders", false)
}

// RootNotes are the note lines of the root declarations with a folder in use; each starts with its language.
func RootNotes(l snapshot.Layout) []string { return layoutNotes(l.Roots, "roots", true) }

// TestFoldersNotUsed are the "not analysed" sentences about declared test folders and about shadowing files.
func TestFoldersNotUsed(l snapshot.Layout) []string {
	sentences := layoutGaps(l.TestFolders, "test folders", false)
	for _, file := range l.Shadowing {
		parts := []string{fmt.Sprintf("`%s` is not read: test folders and roots declared there are not seen", file)}
		named := map[string]bool{}
		for _, d := range l.TestFolders {
			if d.ShadowedBy == file && !named[d.File+" "+d.Key] {
				named[d.File+" "+d.Key] = true
				parts = append(parts, "and "+shadowedClause(d))
			}
		}
		sentences = append(sentences, strings.Join(parts, ", "))
	}
	return sentences
}

// OwnNotes are the note lines of the keys of a repository's own file with a value in use.
func OwnNotes(declared []snapshot.Declared) []string { return layoutNotes(declared, "", false) }

// OwnNotUsed are the "not analysed" sentences about the keys of a repository's own file: held values, values that
// match no tracked file, and values without their form.
func OwnNotUsed(declared []snapshot.Declared) []string { return layoutGaps(declared, "", false) }

// RootsNotUsed are the "not analysed" sentences about declared roots; each starts with its language.
func RootsNotUsed(l snapshot.Layout) []string { return layoutGaps(l.Roots, "roots", true) }

// layoutNotes are the note lines of declared keys with a value in use; a key of the own file names itself.
func layoutNotes(declared []snapshot.Declared, things string, lead bool) []string {
	var lines []string
	for _, d := range declared {
		if len(d.InUse) == 0 {
			continue
		}
		values := make([]string, 0, len(d.InUse))
		for _, value := range d.InUse {
			shown := declaredValue(d, value.Path)
			if value.BaseOnly {
				shown += " (at the base only)"
			}
			values = append(values, shown)
		}
		lines = append(lines, leadOf(d, lead)+fmt.Sprintf("%s declared in `%s` (`[%s]` `%s`): %s", cmp.Or(d.Name, things), d.File, d.Table, d.Key, strings.Join(values, ", ")))
	}
	return lines
}

// declaredValue is one declared value as an answer prints it: a file-name pattern as written, a folder as a folder.
func declaredValue(d snapshot.Declared, value string) string {
	if d.AsWritten {
		return "`" + value + "`"
	}
	return Folder(value)
}

func layoutGaps(declared []snapshot.Declared, things string, lead bool) []string {
	var sentences []string
	for _, d := range declared {
		if d.ShadowedBy != "" {
			if lead {
				sentences = append(sentences, leadOf(d, lead)+shadowedClause(d))
			}
			continue
		}
		if state := stateSentence(d, things); state != "" {
			sentences = append(sentences, leadOf(d, lead)+state)
		}
		for _, held := range d.Held {
			sentences = append(sentences, leadOf(d, lead)+heldSentence(d, held))
		}
		if len(d.Unmatched) > 0 {
			values := make([]string, 0, len(d.Unmatched))
			for _, value := range d.Unmatched {
				values = append(values, declaredValue(d, value))
			}
			verb := "match no tracked file; they are not used"
			if len(values) == 1 {
				verb = "matches no tracked file; it is not used"
			}
			sentences = append(sentences, fmt.Sprintf("`%s` `%s`: %s %s", d.File, d.Key, strings.Join(values, ", "), verb))
		}
		if d.Unusable > 0 {
			sentences = append(sentences, leadOf(d, lead)+fmt.Sprintf("`%s` `%s`: %s", d.File, d.Key, unusableClause(d)))
		}
	}
	return sentences
}

// unusableClause says how many values of a key are not used for want of their form.
func unusableClause(d snapshot.Declared) string {
	switch {
	case d.Name == "":
		return Agree(d.Unusable, "value is absolute, leaves the repository or holds a pattern; it is not used",
			"values are absolute, leave the repository or hold a pattern; they are not used")
	case d.Form != "":
		return Agree(d.Unusable, "value is no "+d.Form+"; it is not used", "values are no "+d.Form+"; they are not used")
	case d.AsWritten:
		return Agree(d.Unusable, "value is no plain file-name pattern; it is not used", "values are no plain file-name patterns; they are not used")
	}
	return Agree(d.Unusable, "value is no plain path from the root; it is not used", "values are no plain paths from the root; they are not used")
}

// shadowedClause says that a key is not used because its tool reads another file.
func shadowedClause(d snapshot.Declared) string {
	return fmt.Sprintf("`%s` in `%s` is not used, because %s reads `%s` instead", d.Key, d.File, d.Tool, d.ShadowedBy)
}

// stateSentence says what became of a key that one side could not read, or "" when both sides read it or lack it.
func stateSentence(d snapshot.Declared, things string) string {
	problem := func(state snapshot.KeyState) bool {
		return state == snapshot.KeyUnreadable || state == snapshot.KeyFormNotRead
	}
	state, when, other, otherWhen := d.Now, "", d.AtBase, "at the base"
	if !problem(state) {
		state, when, other, otherWhen = d.AtBase, " at the base", d.Now, "now"
	}
	if !problem(state) {
		return ""
	}
	phrase := fmt.Sprintf("`%s` `%s` could not be read%s", d.File, d.Key, when)
	if state == snapshot.KeyFormNotRead {
		phrase = fmt.Sprintf("`%s` holds `%s` as a dotted key or an inline table%s, which is not read", d.File, d.Key, when)
	}
	if other == snapshot.KeyRead {
		return phrase + fmt.Sprintf("; the %s declared %s are used for both sides", things, otherWhen)
	}
	return phrase + fmt.Sprintf(", so the %s declared there are not used", things)
}

func heldSentence(d snapshot.Declared, held snapshot.HeldFolder) string {
	switch held.Why {
	case snapshot.HeldRepositoryRoot:
		return fmt.Sprintf("`%s` `%s` names the repository root; only the shared rules apply", d.File, d.Key)
	case snapshot.HeldPackageRoot:
		return fmt.Sprintf("`%s` `%s` names %s, which is or holds a package root; only the shared rules apply inside it", d.File, d.Key, Folder(held.Path))
	case snapshot.HeldEveryName:
		return fmt.Sprintf("`%s` `%s` names every file; only the shared rules apply", d.File, d.Key)
	case snapshot.HeldTestFolders:
	}
	return fmt.Sprintf("`%s` `%s` names %s, which holds test folders of its own: it is where tests are collected, so only the shared rules apply inside it", d.File, d.Key, Folder(held.Path))
}

// leadOf starts a sentence about roots with their language: roots are a language's, test folders the repository's.
func leadOf(d snapshot.Declared, lead bool) string {
	if lead {
		return "`" + d.Language + "`: "
	}
	return ""
}

// folderName prints a repository folder: "the repository root" for ".", else the path with a trailing slash.
func Folder(p string) string {
	if p == "." {
		return "the repository root"
	}
	return "`" + p + "/`"
}

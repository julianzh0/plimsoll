package conventions

import (
	"path"
	"regexp"
	"slices"
	"strings"
)

// NoteNames are the words that make the name of an added Markdown file a working note: a summary, status or report
// an agent leaves behind. Words compare in upper case; a name is split at "_", "-", ".", spaces and at a change from a
// lower-case letter or digit to an upper-case one.
type NoteNames struct {
	Suffixes []string    // file suffixes, as written, of the files the rule looks at
	Alone    []string    // words that make a working-note name on their own
	Numbered []string    // words that do so when a number follows them in the same word (PHASE3)
	Pairs    [][2]string // two words that do so when they stand next to each other
	Combined []string    // words that do so only beside a second word with a letter: alone they also name lasting files
	Kept     []string    // folder names whose files tools and projects keep on purpose; never a working note there
}

// defaultNoteNames is the starting list, measured blind on agent pull requests before it was set: 72 of 80 marks worth
// acting on, none on 2,200 human commits (docs/rules.md, "Working-note names"). A word joins after the same check.
func defaultNoteNames() NoteNames {
	return NoteNames{
		Suffixes: []string{".md", ".mdx"},
		// HANDOFF, HANDOVER, WORKLOG, SCRATCH: agent rule files and tools' ignore lists; COMPLETE, COMPLETION, FINAL:
		// agent pull requests (PHASE3_COMPLETE, FINAL_STATUS) and the Cursor forum's reports. SESSION was taken out on
		// audit: alone it named mostly lasting pages (sqlite_session.md); SESSION_SUMMARY is caught by SUMMARY.
		Alone:    []string{"HANDOFF", "HANDOVER", "WORKLOG", "SCRATCH", "COMPLETE", "COMPLETION", "FINAL"},
		Numbered: []string{"PHASE", "TASK"},
		Pairs:    [][2]string{{"NEXT", "STEPS"}, {"WORK", "LOG"}},
		// The words of GitHub's most common agent names (IMPLEMENTATION_SUMMARY, TEST_REPORT, PROJECT_STATUS); alone
		// they are also book tables of contents (SUMMARY.md), generated reports and status pages.
		Combined: []string{"SUMMARY", "STATUS", "REPORT", "PLAN", "ANALYSIS", "FINDINGS", "INVESTIGATION", "VERIFICATION", "RESULTS", "PROGRESS", "CHECKLIST"},
		Kept:     []string{".changeset", ".github", ".claude", ".cursor", "templates", "template", "fixtures", "__fixtures__", "examples", "example", "snippets"},
	}
}

var (
	caseChange = regexp.MustCompile(`([a-z0-9])([A-Z])`)
	nameBreaks = regexp.MustCompile(`[_.\- ]+`)
)

// NoteWord says whether a path's file name makes it a working note, and names the first word, reading from the left,
// that does: in lower case, a numbered word with its number, a pair with a space. Only ASCII letters fold.
func (r Rules) NoteWord(filePath string) (word string, found bool) {
	n := r.NoteNames
	if !slices.Contains(n.Suffixes, path.Ext(filePath)) || slices.ContainsFunc(strings.Split(path.Dir(filePath), "/"), func(folder string) bool { return slices.Contains(n.Kept, folder) }) {
		return "", false
	}
	stem := strings.TrimSuffix(path.Base(filePath), path.Ext(filePath))
	var words []string
	for _, w := range nameBreaks.Split(caseChange.ReplaceAllString(stem, "${1}_${2}"), -1) {
		if w != "" {
			words = append(words, upperASCII(w))
		}
	}
	lettered := 0
	for _, w := range words {
		if strings.ContainsFunc(w, func(c rune) bool { return c >= 'A' && c <= 'Z' }) {
			lettered++
		}
	}
	for i, w := range words {
		switch {
		case slices.Contains(n.Alone, w), numbered(n.Numbered, w):
			return strings.ToLower(w), true
		case i+1 < len(words) && slices.Contains(n.Pairs, [2]string{w, words[i+1]}):
			return strings.ToLower(w + " " + words[i+1]), true
		case lettered >= 2 && slices.Contains(n.Combined, w):
			return strings.ToLower(w), true
		}
	}
	return "", false
}

// NoteRule lists the words of NoteNames, as an answer prints them.
func (r Rules) NoteRule() string {
	n := r.NoteNames
	quoted := func(words []string, suffix string) string {
		parts := make([]string, 0, len(words))
		for _, w := range words {
			parts = append(parts, "`"+strings.ToLower(w)+suffix+"`")
		}
		return strings.Join(parts, ", ")
	}
	pairs := make([]string, 0, len(n.Pairs))
	for _, p := range n.Pairs {
		pairs = append(pairs, strings.ToLower(p[0]+" "+p[1]))
	}
	return "on its own " + quoted(n.Alone, "") + "; with a number " + quoted(n.Numbered, "<n>") + "; as a pair " + quoted(pairs, "") +
		"; only beside another word " + quoted(n.Combined, "") + "; any case, the first match from the left"
}

// upperASCII upper-cases the ASCII letters of a word and leaves every other character as it is, so the same name gives
// the same words in every locale.
func upperASCII(word string) string {
	return strings.Map(func(c rune) rune {
		if c >= 'a' && c <= 'z' {
			return c - 'a' + 'A'
		}
		return c
	}, word)
}

// numbered says whether a word is one of the numbered words followed by digits only.
func numbered(words []string, w string) bool {
	return slices.ContainsFunc(words, func(prefix string) bool {
		digits, has := strings.CutPrefix(w, prefix)
		return has && digits != "" && strings.Trim(digits, "0123456789") == ""
	})
}

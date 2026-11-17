package snapshot

import (
	"cmp"
	"slices"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// lineChanges are the line changes of the changed files, each made once, on first need.
type lineChanges struct {
	changes     []Change
	now, before map[string][]byte
	made        map[string]lineChange // by path now, or by base path for a file gone since
}

// of is the line change of the file at the path now, or of a file gone since at its base path.
func (l *lineChanges) of(change Change) lineChange {
	key := cmp.Or(change.Path, change.From)
	if l.made == nil {
		l.made = map[string]lineChange{}
	}
	if made, found := l.made[key]; found {
		return made
	}
	made := compareLines(l.before[cmp.Or(change.From, change.Path)], l.now[change.Path])
	l.made[key] = made
	return made
}

// added are the lines the change added to the file at the path now.
func (l *lineChanges) added(path string) []LineRange {
	for _, change := range l.changes {
		if change.Path == path {
			return l.of(change).added
		}
	}
	return nil
}

// movedShare is the share of a candidate's counted tokens that lie in runs of Run tokens found in lines the change
// removed from its code files, each removed run used once. A new function counts all its tokens, a token on a line the
// change did not add as found; a function that existed counts the tokens on added lines, and 0 with none.
func movedShare(rules conventions.Rules, c candidate, added []LineRange, lines *lineChanges) float64 {
	language, known := rules.LanguageOf(c.change.Path)
	if !known || language.Tokens == nil {
		return 0
	}
	run := rules.FunctionRules.Run
	text := lines.now[c.change.Path][c.fn.start:c.fn.end]
	toks := tokensOf(language.Tokens, text, c.change.Start)
	if len(toks) < run {
		return 0
	}
	runs := make([]uint64, len(toks)-run+1)
	wanted := map[uint64]int{}
	for i := range runs {
		runs[i] = runHash(toks[i : i+run])
		wanted[runs[i]] = 0
	}
	for _, change := range lines.changes {
		other, readable := rules.LanguageOf(cmp.Or(change.From, change.Path))
		if !readable || other.Tokens == nil {
			continue
		}
		base := lines.before[cmp.Or(change.From, change.Path)]
		if base == nil {
			continue
		}
		baseLines := linesOf(base)
		for _, r := range lines.of(change).removed {
			removed := tokensOf(other.Tokens, base[lineOffset(baseLines, r.Start):lineOffset(baseLines, r.End+1)], r.Start)
			for i := 0; i+run <= len(removed); i++ {
				if h := runHash(removed[i : i+run]); hasKey(wanted, h) {
					wanted[h]++
				}
			}
		}
	}
	covered := make([]bool, len(toks))
	for i, h := range runs {
		if wanted[h] > 0 {
			wanted[h]--
			for j := i; j < i+run; j++ {
				covered[j] = true
			}
		}
	}
	onAdded := func(line int) bool {
		return slices.ContainsFunc(added, func(r LineRange) bool { return r.Start <= line && line <= r.End })
	}
	counted, hit := 0, 0
	for i, t := range toks {
		isAdded := onAdded(t.line)
		if !c.change.New && !isAdded {
			continue
		}
		counted++
		if covered[i] || c.change.New && !isAdded {
			hit++
		}
	}
	if counted == 0 {
		return 0
	}
	return float64(hit) / float64(counted)
}

func hasKey(m map[uint64]int, k uint64) bool {
	_, found := m[k]
	return found
}

// lineOffset is the byte offset where the line starts, from 1; past the last line, the end of the content.
func lineOffset(newlines lineIndex, line int) int {
	switch {
	case line <= 1:
		return 0
	case line-2 < len(newlines):
		return newlines[line-2] + 1
	}
	if len(newlines) == 0 {
		return 0
	}
	return newlines[len(newlines)-1] + 1
}

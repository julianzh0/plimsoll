package snapshot

import (
	"bytes"
	"cmp"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// functionChecks is how many functions of one measure are compared with the code at the base in one question.
// Each comparison reads up to CopyFiles base files: in localsend (396 Dart files), twelve new functions named
// by words 100 to 180 of its files hold took six searches of 20 files each, 0.06 s more than the same functions under
// names no file holds, so ten searches add about 0.1 s to an answer.
const functionChecks = 10

// copyCheck is the result of comparing one function with the code at the base.
type copyCheck struct {
	files        int     // base files compared
	source       string  // the first file that holds it as a copy, "" for none
	closest      string  // the compared file that held the largest share of it
	closestShare float64 // that share
}

// copyAtBase compares a candidate with the base code files of its entry that may hold it: those that hold its name as a
// whole word, or every one of its long names (at least two), the nearest first, at most CopyFiles; for a function that
// existed also its own base file, without its old body. A file holds it as a copy when CopyShare or more of its counted
// tokens (all for a new function, those on added lines otherwise) lie in runs of Run tokens found in that file. Each
// base file is read as a stream of tokens; only the function's own runs are kept.
func copyAtBase(rules conventions.Rules, c candidate, baseFiles []File, base map[string][]byte, nowContent []byte) copyCheck {
	fr := rules.FunctionRules
	language, known := rules.LanguageOf(c.change.Path)
	if !known || language.Tokens == nil || len(c.tokens) < fr.Run {
		return copyCheck{}
	}
	counted := make([]bool, len(c.tokens))
	n := 0
	for i, t := range c.tokens {
		counted[i] = c.change.New || c.added[t.line-c.change.Start]
		if counted[i] {
			n++
		}
	}
	if n == 0 {
		return copyCheck{}
	}
	name := []byte(c.change.Name)
	long := longNames(fr, c, nowContent)
	holds := func(content []byte) bool {
		if len(name) >= fr.NameLength && holdsWord(content, name) {
			return true
		}
		return len(long) >= 2 && !slices.ContainsFunc(long, func(w []byte) bool { return !bytes.Contains(content, w) })
	}
	var candidates []File
	for _, f := range baseFiles {
		other, readable := rules.LanguageOf(f.Path)
		if readable && other.Name == language.Name && base[f.Path] != nil && holds(base[f.Path]) {
			candidates = append(candidates, f)
		}
	}
	files := nearest(candidates, c.change.Path, fr.CopyFiles)
	if !c.change.New && !slices.Contains(files, c.change.BasePath) && base[c.change.BasePath] != nil {
		files = append(files, c.change.BasePath)
	}
	starts := map[uint64][]int{} // the function's runs: hash -> where they start
	for i, h := range windowHashes(c.tokens, fr.Run) {
		starts[h] = append(starts[h], i)
	}
	result := copyCheck{files: len(files)}
	for _, path := range files {
		covered := make([]bool, len(c.tokens))
		w := newWindows(fr.Run)
		for t := range eachToken(language.Tokens, base[path], 1) {
			if path == c.change.BasePath && c.change.BaseStart <= t.line && t.line <= c.change.BaseEnd {
				continue
			}
			w.push(t.hash, func(_ int, hash uint64, _ int) {
				for _, i := range starts[hash] {
					for j := i; j < i+fr.Run; j++ {
						covered[j] = true
					}
				}
			})
		}
		hit := 0
		for i := range c.tokens {
			if counted[i] && covered[i] {
				hit++
			}
		}
		share := float64(hit) / float64(n)
		if share > result.closestShare {
			result.closest, result.closestShare = path, share
		}
		if share >= fr.CopyShare {
			result.source = path
			return result
		}
	}
	return result
}

// longNames are the candidate's longest distinct names other than its own, at least LongNameLength long, at most
// LongNames of them, each cut to 80 bytes.
func longNames(fr conventions.FunctionRules, c candidate, nowContent []byte) [][]byte {
	var long []string
	for _, t := range c.tokens {
		word := strings.Trim(string(nowContent[c.fn.start+t.start:c.fn.start+t.end]), "\"'`")
		if t.word && word != c.change.Name && len(word) >= fr.LongNameLength && !slices.Contains(long, word) {
			long = append(long, word)
		}
	}
	slices.SortFunc(long, func(a, b string) int { return cmp.Or(len(b)-len(a), cmp.Compare(a, b)) })
	out := make([][]byte, 0, min(len(long), fr.LongNames))
	for _, w := range long[:min(len(long), fr.LongNames)] {
		out = append(out, []byte(w[:min(len(w), 80)]))
	}
	return out
}

// holdsWord says whether content holds the name with no ASCII letter, digit or _ right before or after it.
func holdsWord(content, name []byte) bool {
	wordByte := func(b byte) bool {
		return b == '_' || '0' <= b && b <= '9' || 'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z'
	}
	for at := 0; ; {
		i := bytes.Index(content[at:], name)
		if i < 0 {
			return false
		}
		start, end := at+i, at+i+len(name)
		if (start == 0 || !wordByte(content[start-1])) && (end == len(content) || !wordByte(content[end])) {
			return true
		}
		at = start + 1
	}
}

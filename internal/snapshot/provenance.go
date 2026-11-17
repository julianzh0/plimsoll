package snapshot

import (
	"bytes"
	"cmp"
	"slices"
	"strings"
	"unicode"

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

// origins tells, for a file the change added whole, whether it came from another project: by its header, an upstream
// address or a note that it was ported, or by a vendor folder with its own licence. What is the repository's own is read
// from its tracked files only: the holders of its root licence files, and what the headers of its nearest base product
// files share.
type origins struct {
	rules      conventions.FunctionRules
	nowPaths   []string
	nowContent map[string][]byte
	baseFiles  []File
	base       map[string][]byte
	own        map[string]bool            // holder words of the root licence files
	shared     map[string]map[string]bool // folder -> the holder sets and addresses at least NearAgree nearest files hold
}

// fromAnotherProject says whether the file at the path, added whole, came from another project.
func (o *origins) fromAnotherProject(path string, content []byte) bool {
	fr := o.rules
	for _, line := range commentLines(content, fr.HeaderLines) {
		if words, holds := o.holderOf(line); holds && !o.ownHolder(words, path) {
			return true
		}
		for _, m := range fr.CodeHost.FindAllStringSubmatch(line, -1) {
			if !o.sharedNear(path)[addressKey(m[1], m[2])] {
				return true
			}
		}
		if fr.Ported.MatchString(line) {
			return true
		}
	}
	parts := strings.Split(path, "/")
	for k := range len(parts) - 1 {
		if !slices.Contains(fr.VendorFolders, strings.ToLower(parts[k])) {
			continue
		}
		folder := strings.Join(parts[:k+1], "/") + "/"
		if slices.ContainsFunc(o.nowPaths, func(p string) bool {
			rest, under := strings.CutPrefix(p, folder)
			return under && !strings.Contains(rest, "/") && fr.VendorLicence.MatchString(rest)
		}) {
			return true
		}
	}
	return false
}

// holderOf is the holder of a header line, as a set of words: the group of a "Licensed to" or "part of" line, else the
// whole of a copyright line; false for a line that names no holder.
func (o *origins) holderOf(line string) (string, bool) {
	fr := o.rules
	if m := fr.LicensedTo.FindStringSubmatch(line); m != nil {
		return holderWords(fr, m[1]), true
	}
	if m := fr.PartOf.FindStringSubmatch(line); m != nil {
		return holderWords(fr, m[1]), true
	}
	if fr.Copyright.MatchString(line) {
		return holderWords(fr, line), true
	}
	return "", false
}

// ownHolder says whether a holder is the repository's own: no words, a word of a root licence file's holder, or a set
// the nearest base product files share.
func (o *origins) ownHolder(words, path string) bool {
	if words == "" {
		return true
	}
	if o.own == nil {
		o.own = map[string]bool{}
		for _, p := range o.nowPaths {
			if strings.Contains(p, "/") || !o.rules.RootLicence.MatchString(p) {
				continue
			}
			for line := range strings.SplitSeq(string(o.nowContent[p]), "\n") {
				if o.rules.Copyright.MatchString(line) {
					for w := range strings.SplitSeq(holderWords(o.rules, line), " ") {
						o.own[w] = true
					}
				}
			}
		}
	}
	for w := range strings.SplitSeq(words, " ") {
		if o.own[w] {
			return true
		}
	}
	return o.sharedNear(path)[words]
}

// sharedNear are the holder sets and code-host addresses that at least NearAgree of the NearFiles base product files
// nearest to the path hold in their headers: most shared leading folders first, then by path.
func (o *origins) sharedNear(path string) map[string]bool {
	folder := path[:strings.LastIndex(path, "/")+1]
	if found, made := o.shared[folder]; made {
		return found
	}
	if o.shared == nil {
		o.shared = map[string]map[string]bool{}
	}
	sharedFolders := func(p string) int {
		a, b := strings.Split(p, "/"), strings.Split(path, "/")
		n := 0
		for n < len(a)-1 && n < len(b)-1 && a[n] == b[n] {
			n++
		}
		return n
	}
	var near []string
	for _, f := range o.baseFiles {
		if f.Role == Product {
			near = append(near, f.Path)
		}
	}
	slices.SortFunc(near, func(a, b string) int { return cmp.Or(sharedFolders(b)-sharedFolders(a), cmp.Compare(a, b)) })
	count := map[string]int{}
	for _, p := range near[:min(len(near), o.rules.NearFiles)] {
		keys := map[string]bool{}
		for _, line := range commentLines(o.base[p], o.rules.HeaderLines) {
			if words, holds := o.holderOf(line); holds {
				keys[words] = true
			}
			for _, m := range o.rules.CodeHost.FindAllStringSubmatch(line, -1) {
				keys[addressKey(m[1], m[2])] = true
			}
		}
		for k := range keys {
			count[k]++
		}
	}
	found := map[string]bool{}
	for k, n := range count {
		if n >= o.rules.NearAgree {
			found[k] = true
		}
	}
	o.shared[folder] = found
	return found
}

// addressKey names a code-host address by its owner and repository, apart from any holder set.
func addressKey(owner, name string) string {
	return "\x00" + strings.ToLower(owner) + "/" + strings.TrimSuffix(strings.ToLower(name), ".git")
}

// holderWords are the words of a holder, sorted and joined: the text in lower case split at every character that is no
// letter or digit, without stop words, words of at most two characters and years (4 or 8 digits).
func holderWords(fr conventions.FunctionRules, text string) string {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	var kept []string
	for _, w := range words {
		year := (len(w) == 4 || len(w) == 8) && strings.IndexFunc(w, func(r rune) bool { return !unicode.IsDigit(r) }) < 0
		if len(w) > 2 && !year && !slices.Contains(fr.HolderStopWords, w) {
			kept = append(kept, w)
		}
	}
	slices.Sort(kept)
	return strings.Join(slices.Compact(kept), " ")
}

// commentLines are the comment lines among the first n lines of content: a line that starts with //, #, /*, *, -- or
// <!-- (not a preprocessor line), every line of a /* */ comment or of a triple-quoted string opened there.
func commentLines(content []byte, n int) []string {
	var out []string
	block, triple := false, ""
	lines := strings.SplitN(string(content), "\n", n+1)
	for _, line := range lines[:min(len(lines), n)] {
		s := strings.TrimSpace(line)
		switch {
		case block:
			out = append(out, line)
			block = !strings.Contains(s, "*/")
			continue
		case triple != "":
			out = append(out, line)
			if strings.Contains(s, triple) {
				triple = ""
			}
			continue
		}
		if hasAnyPrefix(s, "#include", "#import", "#define", "#if", "#!") {
			continue
		}
		if hasAnyPrefix(s, "//", "#", "/*", "*", "--", "<!--") {
			out = append(out, line)
			block = strings.HasPrefix(s, "/*") && !strings.Contains(s[2:], "*/")
			continue
		}
		for _, q := range []string{`"""`, "'''"} {
			if hasAnyPrefix(s, q, "r"+q, "b"+q) {
				out = append(out, line)
				if !strings.Contains(s[strings.Index(s, q)+3:], q) {
					triple = q
				}
				break
			}
		}
	}
	return out
}

func hasAnyPrefix(s string, prefixes ...string) bool {
	return slices.ContainsFunc(prefixes, func(p string) bool { return strings.HasPrefix(s, p) })
}

// removedTwice says whether the tokens of a block of the file at the path stand at least twice in the lines the change
// removed from its code files, each removed range read as one stream of tokens.
func removedTwice(rules conventions.Rules, path string, block blockSpan, lines *lineChanges) bool {
	language, known := rules.LanguageOf(path)
	if !known || language.Tokens == nil {
		return false
	}
	want := tokensOf(language.Tokens, lines.now[path][block.start:block.end], 0)
	if len(want) == 0 {
		return false
	}
	found := 0
	for _, change := range lines.changes {
		other, readable := rules.LanguageOf(cmp.Or(change.From, change.Path))
		base := lines.before[cmp.Or(change.From, change.Path)]
		if !readable || other.Tokens == nil || base == nil {
			continue
		}
		baseLines := linesOf(base)
		for _, r := range lines.of(change).removed {
			removed := tokensOf(other.Tokens, base[lineOffset(baseLines, r.Start):lineOffset(baseLines, r.End+1)], r.Start)
			for i := 0; i+len(want) <= len(removed); i++ {
				if slices.EqualFunc(removed[i:i+len(want)], want, func(a, b token) bool { return a.hash == b.hash }) {
					found++
					i += len(want) - 1
				}
			}
			if found >= 2 {
				return true
			}
		}
	}
	return false
}

// functionChecks is how many functions of one measure are compared with the code at the base in one question.
// NOTE: provisional, derived in Task 15 from the functions per change that cross a rule in the field repositories.
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
// tokens (all for a new function, those on added lines otherwise) lie in runs of Run tokens found in that file.
func copyAtBase(rules conventions.Rules, c candidate, added []LineRange, baseFiles []File, base map[string][]byte, nowContent []byte) copyCheck {
	fr := rules.FunctionRules
	language, known := rules.LanguageOf(c.change.Path)
	if !known || language.Tokens == nil {
		return copyCheck{}
	}
	toks := tokensOf(language.Tokens, nowContent[c.fn.start:c.fn.end], c.change.Start)
	onAdded := func(line int) bool {
		return slices.ContainsFunc(added, func(r LineRange) bool { return r.Start <= line && line <= r.End })
	}
	counted := make([]bool, len(toks))
	n := 0
	for i, t := range toks {
		counted[i] = c.change.New || onAdded(t.line)
		if counted[i] {
			n++
		}
	}
	if n == 0 || len(toks) < fr.Run {
		return copyCheck{}
	}
	text := func(t token) string { return string(nowContent[c.fn.start+t.start : c.fn.start+t.end]) }
	var long []string
	for _, t := range toks {
		word := strings.Trim(text(t), "\"'`")
		if t.word && word != c.change.Name && len(word) >= fr.LongNameLength && !slices.Contains(long, word) {
			long = append(long, word)
		}
	}
	slices.SortFunc(long, func(a, b string) int { return cmp.Or(len(b)-len(a), cmp.Compare(a, b)) })
	long = long[:min(len(long), fr.LongNames)]
	for i := range long {
		long[i] = long[i][:min(len(long[i]), 80)]
	}
	holds := func(content []byte) bool {
		if len(c.change.Name) >= fr.NameLength && holdsWord(content, c.change.Name) {
			return true
		}
		return len(long) >= 2 && !slices.ContainsFunc(long, func(w string) bool { return !bytes.Contains(content, []byte(w)) })
	}
	var files []string
	for _, f := range baseFiles {
		other, readable := rules.LanguageOf(f.Path)
		if readable && other.Name == language.Name && base[f.Path] != nil && holds(base[f.Path]) {
			files = append(files, f.Path)
		}
	}
	shared := func(p string) int {
		a, b := strings.Split(p, "/"), strings.Split(c.change.Path, "/")
		k := 0
		for k < len(a)-1 && k < len(b)-1 && a[k] == b[k] {
			k++
		}
		return k
	}
	slices.SortFunc(files, func(a, b string) int { return cmp.Or(shared(b)-shared(a), cmp.Compare(a, b)) })
	files = files[:min(len(files), fr.CopyFiles)]
	if !c.change.New && !slices.Contains(files, c.change.BasePath) && base[c.change.BasePath] != nil {
		files = append(files, c.change.BasePath)
	}
	runs := make([]uint64, len(toks)-fr.Run+1)
	for i := range runs {
		runs[i] = runHash(toks[i : i+fr.Run])
	}
	result := copyCheck{files: len(files)}
	for _, path := range files {
		fileToks := tokensOf(language.Tokens, base[path], 1)
		if path == c.change.BasePath {
			fileToks = slices.DeleteFunc(fileToks, func(t token) bool { return c.change.BaseStart <= t.line && t.line <= c.change.BaseEnd })
		}
		present := map[uint64]bool{}
		for i := 0; i+fr.Run <= len(fileToks); i++ {
			present[runHash(fileToks[i:i+fr.Run])] = true
		}
		covered := make([]bool, len(toks))
		for i, h := range runs {
			if present[h] {
				for j := i; j < i+fr.Run; j++ {
					covered[j] = true
				}
			}
		}
		hit := 0
		for i := range toks {
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

// holdsWord says whether content holds the name with no ASCII letter, digit or _ right before or after it.
func holdsWord(content []byte, name string) bool {
	wordByte := func(b byte) bool {
		return b == '_' || '0' <= b && b <= '9' || 'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z'
	}
	for at := 0; ; {
		i := bytes.Index(content[at:], []byte(name))
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

package snapshot

import (
	"bytes"
	"cmp"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// origins tells, for a measured file, whether it came from another project: by its header, an upstream
// address or a note that it was ported, or by a vendor folder whose project holds its own licence. What is the
// repository's own is read from its tracked files only: the copyright holders of its root licence files, and what the
// headers of its nearest base product files share.
type origins struct {
	rules      conventions.FunctionRules
	nowPaths   []string
	nowContent map[string][]byte
	baseFiles  []File
	base       map[string][]byte
	product    []File                     // the base product files, made on first need
	own        map[string]bool            // holder words of the root licence files
	shared     map[string]map[string]bool // folder -> the holder sets and addresses at least NearAgree nearest files hold
	decided    map[string]bool            // path -> from another project
}

// fromAnotherProject says whether the file at the path came from another project; decided once per file.
func (o *origins) fromAnotherProject(path string, content []byte) bool {
	if o.decided == nil {
		o.decided = map[string]bool{}
	}
	if foreign, done := o.decided[path]; done {
		return foreign
	}
	foreign := o.headerIsForeign(path, content) || o.vendored(path)
	o.decided[path] = foreign
	return foreign
}

func (o *origins) headerIsForeign(path string, content []byte) bool {
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
	return false
}

// vendored says whether a folder on the path has a vendor name and, at it or below it on the way to the file, a
// folder directly holds a tracked licence file.
func (o *origins) vendored(path string) bool {
	parts := strings.Split(path, "/")
	for k := range len(parts) - 1 {
		if !slices.Contains(o.rules.VendorFolders, strings.ToLower(parts[k])) {
			continue
		}
		for depth := k + 1; depth < len(parts); depth++ {
			folder := strings.Join(parts[:depth], "/")
			if slices.ContainsFunc(o.nowPaths, func(p string) bool {
				rest, in := under(p, folder)
				return in && !strings.Contains(rest, "/") && o.rules.VendorLicence.MatchString(rest)
			}) {
				return true
			}
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

// noticeLine is a licence file's line that names a holder: it starts with a copyright sign or word. A licence's own
// wording ("grants a ... copyright license") names none.
var noticeLine = regexp.MustCompile(`(?i)^\s*(?:copyright\b|©|\(c\))`)

// ownHolder says whether a holder is the repository's own: no words, a word of a root licence file's holders, or a set
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
				if noticeLine.MatchString(line) {
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
	if o.product == nil {
		o.product = slices.DeleteFunc(slices.Clone(o.baseFiles), func(f File) bool { return f.Role != Product })
	}
	count := map[string]int{}
	for _, p := range nearest(o.product, path, o.rules.NearFiles) {
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

// nearest are the paths of the files nearest to path, at most n: most shared leading folders first, then by path.
func nearest(files []File, path string, n int) []string {
	type near struct {
		path   string
		shared int
	}
	list := make([]near, len(files))
	for i, f := range files {
		list[i] = near{f.Path, sharedFolders(f.Path, path)}
	}
	slices.SortFunc(list, func(a, b near) int { return cmp.Or(b.shared-a.shared, cmp.Compare(a.path, b.path)) })
	paths := make([]string, 0, min(n, len(list)))
	for _, x := range list[:min(n, len(list))] {
		paths = append(paths, x.path)
	}
	return paths
}

// sharedFolders counts the leading folders two paths share.
func sharedFolders(a, b string) int {
	n := 0
	for {
		i, j := strings.IndexByte(a, '/'), strings.IndexByte(b, '/')
		if i < 0 || j < 0 || a[:i] != b[:j] {
			return n
		}
		a, b = a[i+1:], b[j+1:]
		n++
	}
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
	head := content
	for i, at := 0, 0; i < n; i++ {
		next := bytes.IndexByte(content[at:], '\n')
		if next < 0 {
			break
		}
		at += next + 1
		head = content[:at]
	}
	var out []string
	block, triple := false, ""
	lines := strings.Split(string(head), "\n")
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

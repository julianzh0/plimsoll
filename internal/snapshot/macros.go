package snapshot

import (
	"bytes"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// rewriteMacros applies the rules in order, each in one pass over the file: what a rule's group "macro" finds is
// blanked, or replaced by its As padded with spaces; every byte offset and line break stays where it was. nil when no
// rule found anything.
func rewriteMacros(rules []conventions.MacroRule, content []byte, learned *macroNames) []byte {
	rewritten, changed := slices.Clone(content), false
	if learned != nil {
		rules = append([]conventions.MacroRule{{Pattern: learned.attributes}}, rules...)
	}
	for _, rule := range rules {
		group := rule.Pattern.SubexpIndex("macro")
		for _, m := range rule.Pattern.FindAllSubmatchIndex(rewritten, -1) {
			start, end := m[2*group], m[2*group+1]
			original := slices.Clone(rewritten[start:end])
			copy(rewritten[start:end], rule.As)
			blankKeepingLines(rewritten[start+len(rule.As) : end])
			if learned != nil && rule.As == "" { // a name the code declares or defines as no attribute stays
				for _, w := range macroWord.FindAllIndex(original, -1) {
					if word := original[w[0]:w[1]]; learned.kept[string(word)] && learned.shape.Match(word) {
						copy(rewritten[start+w[0]:start+w[1]], original[w[0]:w[1]])
					}
				}
			}
			changed = changed || !bytes.Equal(original, rewritten[start:end])
		}
	}
	if !changed {
		return nil
	}
	return rewritten
}

// withRewritten takes, from the functions read on a copy with macros rewritten, those that overlap no function read
// cleanly as written; a function unreadable as written stays unless a function of the copy overlaps it, which then
// stands for it (or for the pieces the grammar split it into). So a function that read cleanly never changes.
func withRewritten(asWritten, rewritten []function) []function {
	var clean, unreadable, taken []function
	for _, f := range asWritten {
		if f.broken {
			unreadable = append(unreadable, f)
		} else {
			clean = append(clean, f)
		}
	}
	for _, b := range rewritten {
		if !overlapsAny(clean, b) {
			taken = append(taken, b)
		}
	}
	found := slices.Concat(clean, taken)
	for _, f := range unreadable {
		if !overlapsAny(taken, f) {
			found = append(found, f)
		}
	}
	slices.SortStableFunc(found, func(a, b function) int { return a.start - b.start })
	return found
}

// overlapsAny says whether f shares a byte with one of the functions.
func overlapsAny(functions []function, f function) bool {
	return slices.ContainsFunc(functions, func(g function) bool { return g.start < f.end && f.start < g.end })
}

// macroWord is a name inside a rewritten macro use.
var macroWord = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

// macroNames is what one side's own #define lines say: a pattern of the attribute macros it defines, where a
// declaration starts, the names it keeps as written, and the shape of the names that rule blanks by convention;
// digest names it for the reading cache.
type macroNames struct {
	attributes *regexp.Regexp
	kept       map[string]bool
	shape      *regexp.Regexp
	digest     string
}

// learnMacroNames reads the #define lines and type declarations of one side's files of the entries that learn macro
// names. A name exactly one file defines, outside any condition but its open include guard (an #ifndef first, its
// #define next), as nothing or an attribute text is an attribute; a name such a file defines as anything else, or
// the code declares as a type, is kept; names several files define decide nothing. Continued lines are joined and
// comments left out before a body is judged.
func learnMacroNames(rules *conventions.MacroNames, contents [][]byte) *macroNames {
	facts := make([]fileMacros, 0, len(contents))
	for _, content := range contents {
		facts = append(facts, macrosOf(rules, content))
	}
	return mergedMacroNames(rules, facts)
}

// fileMacros is what one file's #define lines and declarations say: the names it defines unconditionally as an
// attribute or as something else, and the types it declares.
type fileMacros struct {
	attribute, other, types map[string]bool
}

// macrosOf reads one file's definitions and type declarations (see learnMacroNames).
func macrosOf(rules *conventions.MacroNames, content []byte) fileMacros {
	f := fileMacros{attribute: map[string]bool{}, other: map[string]bool{}, types: map[string]bool{}}
	{
		depth, guard, guardOpen, directives := 0, "", false, 0
		for line := range directiveLines(content) {
			directives++
			directive := strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), "#"))
			if len(directive) == 0 {
				continue
			}
			switch word := directiveWord(directive[0]); {
			case strings.HasPrefix(word, "if"):
				if directives == 1 && word == "ifndef" && len(directive) > 1 {
					guard = directive[1]
				}
				depth++
			case word == "endif":
				if depth--; depth == 0 {
					guardOpen = false
				}
			case word == "define":
				m := rules.Define.FindStringSubmatch(strings.TrimSpace(line))
				if m == nil {
					break
				}
				name, body := m[1], strings.TrimSpace(withoutComments(cCommentsOnly(), m[3]))
				if directives == 2 && name == guard && depth == 1 {
					guardOpen = true
					break
				}
				if depth > 1 || depth == 1 && !guardOpen {
					break // a definition under a condition
				}
				if body == "" || slices.ContainsFunc(rules.Attributes, func(a string) bool { return strings.HasPrefix(body, a) }) {
					f.attribute[name] = true
				} else {
					f.other[name] = true
				}
			}
		}
		for _, m := range rules.Type.FindAllStringSubmatch(string(content), -1) {
			f.types[m[1]+m[2]+m[3]] = true
		}
	}
	return f
}

// mergedMacroNames decides, from the files' facts, which names are attributes and which stay (see learnMacroNames).
func mergedMacroNames(rules *conventions.MacroNames, facts []fileMacros) *macroNames {
	definedBy := map[string]int{}
	attribute, other, types := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, f := range facts {
		for name := range f.attribute {
			definedBy[name]++
			attribute[name] = true
		}
		for name := range f.other {
			if !f.attribute[name] {
				definedBy[name]++
			}
			other[name] = true
		}
		maps.Copy(types, f.types)
	}
	learned := &macroNames{kept: map[string]bool{}, shape: rules.Shape}
	var attributes []string
	for name, files := range definedBy {
		switch {
		case files != 1 || attribute[name] && other[name]:
		case attribute[name] && !types[name]:
			attributes = append(attributes, regexp.QuoteMeta(name))
		default:
			learned.kept[name] = true
		}
	}
	for name := range types {
		if definedBy[name] <= 1 && !attribute[name] {
			learned.kept[name] = true
		}
	}
	slices.Sort(attributes)
	kept := slices.Sorted(maps.Keys(learned.kept))
	learned.digest = strings.Join(attributes, ",") + ";" + strings.Join(kept, ",")
	if len(attributes) == 0 {
		attributes = []string{"\\b\\B"} // matches nothing
	}
	learned.attributes = regexp.MustCompile(rules.DeclarationStart + `(?P<macro>(?:\b(?:` + strings.Join(attributes, "|") +
		`)\b(?:[ \t]*\((?:[^()]|\([^()]*\))*\))?[ \t]*)+)[A-Za-z_~]`)
	return learned
}

// directiveLines are the preprocessor lines of a text, each with the lines it continues joined.
func directiveLines(content []byte) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		joined := ""
		for line := range strings.Lines(string(content)) {
			line = strings.TrimRight(line, "\r\n")
			if before, ok := strings.CutSuffix(line, `\`); ok {
				joined += before + " "
				continue
			}
			line, joined = joined+line, ""
			if strings.HasPrefix(strings.TrimSpace(line), "#") && !yield(line) {
				return
			}
		}
	}
}

// directiveWord is a directive's name: the letters it starts with (#endif//x, #else/* … */).
func directiveWord(word string) string {
	end := strings.IndexFunc(word, func(r rune) bool { return r < 'a' || r > 'z' })
	if end < 0 {
		return word
	}
	return word[:end]
}

// cCommentsOnly matches C's line and block comments, and its strings so a marker inside one starts none.
func cCommentsOnly() *regexp.Regexp {
	return regexp.MustCompile(`"(?:[^"\\\n]|\\.)*"|//[^\n]*|/\*(?:[^*]|\*+[^*/])*\*+/`)
}

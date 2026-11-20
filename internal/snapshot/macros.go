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
					if word := original[w[0]:w[1]]; learned.kept[string(word)] && capitalisedMacro.Match(word) {
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

var (
	// macroWord is a name inside a rewritten macro use.
	macroWord = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)
	// capitalisedMacro is the shape of a name the declaration rule blanks as an attribute by convention.
	capitalisedMacro = regexp.MustCompile(`^[A-Z][A-Z0-9]*_[A-Z0-9_]+$`)
)

// macroNames is what one side's own #define lines say: a pattern of the attribute macros it defines, where a
// declaration starts, and the names it keeps as written; digest names it for the reading cache.
type macroNames struct {
	attributes *regexp.Regexp
	kept       map[string]bool
	digest     string
}

var (
	cDefine   = regexp.MustCompile(`^[ \t]*#[ \t]*define[ \t]+([A-Za-z_][A-Za-z0-9_]*)(\([^)]*\))?[ \t]*(.*)$`)
	cTypeName = regexp.MustCompile(`\b(?:struct|class|enum(?:[ \t]+class)?|union)[ \t]+([A-Za-z_][A-Za-z0-9_]*)|\busing[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]*=|\btypedef\b[^;{]*?\b([A-Za-z_][A-Za-z0-9_]*)[ \t]*;`)
)

// learnMacroNames reads the #define lines and type declarations of one side's files of an entry. A name exactly one
// file defines, outside any condition but its include guard, as nothing or an attribute text is an attribute; a name
// such a file defines as anything else, or the code declares as a type, is kept; names several files define decide
// nothing.
func learnMacroNames(rules *conventions.MacroNames, contents [][]byte) *macroNames {
	definedBy := map[string]int{}
	attribute, other, types := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, content := range contents {
		mine := map[string]bool{}
		depth, guard, first := 0, "", true
		for line := range strings.Lines(string(content)) {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "#") {
				continue
			}
			directive := strings.Fields(strings.TrimPrefix(trimmed, "#"))
			if len(directive) == 0 {
				continue
			}
			switch word := directive[0]; {
			case strings.HasPrefix(word, "if"):
				if first && word == "ifndef" && len(directive) > 1 {
					guard = directive[1]
				}
				depth++
			case word == "endif":
				depth--
			case word == "define":
				m := cDefine.FindStringSubmatch(trimmed)
				if m == nil {
					break
				}
				name, body := m[1], strings.TrimSpace(m[3])
				if name == guard || depth > 1 || depth == 1 && guard == "" {
					break // the guard itself, or a definition under a condition
				}
				if !mine[name] {
					mine[name] = true
					definedBy[name]++
				}
				if body == "" || slices.ContainsFunc(rules.Attributes, func(a string) bool { return strings.HasPrefix(body, a) }) {
					attribute[name] = true
				} else {
					other[name] = true
				}
			}
			first = false
		}
		for _, m := range cTypeName.FindAllStringSubmatch(string(content), -1) {
			types[m[1]+m[2]+m[3]] = true
		}
	}
	learned := &macroNames{kept: map[string]bool{}}
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

package snapshot

import (
	"bytes"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// unreachable are the stretches of a file that no build reads: the branches of #if chains that their literal
// conditions rule out, nested chains included. A chain inside an unreachable branch is unreachable whole. Trailing
// comments and line continuations around a condition are allowed.
func unreachable(c *conventions.Conditions, content []byte) []span {
	type chain struct{ reachable, taken, current bool }
	var stack []chain
	readable := func() bool {
		return !slices.ContainsFunc(stack, func(f chain) bool { return !f.current })
	}
	literal := func(condition string) (isTrue, isFalse bool) {
		condition, _, _ = strings.Cut(condition, "//")
		condition, _, _ = strings.Cut(condition, "/*")
		condition = strings.TrimSpace(condition)
		return slices.Contains(c.True, condition), slices.Contains(c.False, condition)
	}
	var found []span
	start, at := -1, 0
	lines := strings.SplitAfter(string(content), "\n")
	for i := 0; i < len(lines); i++ {
		lineStart, line := at, lines[i]
		at += len(line)
		for strings.HasSuffix(strings.TrimRight(line, "\r\n"), `\`) && i+1 < len(lines) { // a continued line
			i++
			line = strings.TrimSuffix(strings.TrimRight(line, "\r\n"), `\`) + " " + lines[i]
			at += len(lines[i])
		}
		directive, condition, isDirective := strings.Cut(strings.TrimSpace(line), "#")
		if !isDirective || directive != "" {
			continue
		}
		condition = strings.TrimSpace(condition)
		word := directiveWord(condition)
		condition = condition[len(word):]
		before := readable()
		switch isTrue, isFalse := literal(condition); {
		case strings.HasPrefix(word, "if"):
			f := chain{reachable: before}
			if word == "if" {
				f.current = before && !isFalse
				f.taken = f.current && isTrue
			} else { // ifdef, ifndef: no literal condition
				f.current = before
			}
			stack = append(stack, f)
		case len(stack) == 0:
			continue
		case slices.Contains(c.ElseIf, word):
			f := &stack[len(stack)-1]
			f.current = f.reachable && !f.taken && !isFalse
			f.taken = f.taken || f.current && isTrue
		case word == "else":
			f := &stack[len(stack)-1]
			f.current = f.reachable && !f.taken
		case word == "endif":
			stack = stack[:len(stack)-1]
		default:
			continue
		}
		switch after := readable(); {
		case before && !after:
			start = at // the branch's lines after its directive
		case !before && after:
			found = append(found, span{start: start, end: lineStart})
		}
	}
	if start >= 0 && !readable() {
		found = append(found, span{start: start, end: len(content)})
	}
	return found
}

// reachableStatements drops the statements an entry's literal conditions rule out.
func reachableStatements(language conventions.Language, content []byte, statements []statement) []statement {
	c := language.Imports.Conditions
	if c == nil || !bytes.Contains(content, []byte("#")) {
		return statements
	}
	spans := unreachable(c, masked(language, content)) // a directive inside a comment, a string or a raw string is none
	if len(spans) == 0 {
		return statements
	}
	return slices.DeleteFunc(statements, func(st statement) bool {
		return slices.ContainsFunc(spans, func(s span) bool { return s.start <= st.at && st.at < s.end })
	})
}

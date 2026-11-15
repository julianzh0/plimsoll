package snapshot

import (
	"bytes"
	"regexp"

	"github.com/juzhxng/plimsoll/internal/conventions"
)

// masked returns the content with every match of a language's literal pattern as spaces, line ends kept, so offsets
// and line numbers stay those of the file. A nil pattern returns the content itself.
func masked(literals *regexp.Regexp, content []byte) []byte {
	if literals == nil {
		return content
	}
	return literals.ReplaceAllFunc(content, func(match []byte) []byte {
		blank := make([]byte, len(match))
		for i, c := range match {
			blank[i] = ' '
			if c == '\n' {
				blank[i] = '\n'
			}
		}
		return blank
	})
}

// span is a stretch of a file's text, both ends included, and the name of the block that opens it, if it has one.
type span struct {
	start, end int
	name       string
}

// spansOf are the stretches an opener's matches cover, in the text's order: a match that ends with "{" runs to the
// brace that closes it, any other match is the stretch itself. A brace that never closes runs to the end. The text
// is read with its literals blanked, so a brace in a string or a comment counts for nothing.
func spansOf(opener *regexp.Regexp, text []byte) []span {
	if opener == nil {
		return nil
	}
	name := opener.SubexpIndex("name")
	var found []span
	for _, m := range opener.FindAllSubmatchIndex(text, -1) {
		s := span{start: m[0], end: m[1] - 1}
		if name >= 0 && m[2*name] >= 0 {
			s.name = string(text[m[2*name]:m[2*name+1]])
		}
		if text[s.end] == '{' {
			s.end = closingBrace(text, s.end)
		}
		found = append(found, s)
	}
	return found
}

// closingBrace is the offset of the brace that closes the one at open, or the text's last offset when none does.
func closingBrace(text []byte, open int) int {
	depth := 0
	for i := open; i < len(text); i++ {
		switch text[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(text) - 1
}

// testLines counts the lines that a file's test code touches, each line once: the spans of the language's test
// blocks on the text with its literals blanked, a span that starts inside the one before it being part of that one.
func testLines(language conventions.Language, content []byte) int {
	if language.TestBlock == nil {
		return 0
	}
	text := masked(language.Literals, content)
	counted, lastCounted, until := 0, 0, -1
	for _, s := range spansOf(language.TestBlock, text) {
		if s.start <= until {
			continue
		}
		until = s.end
		first := max(1+bytes.Count(text[:s.start], []byte("\n")), lastCounted+1)
		last := 1 + bytes.Count(text[:s.end], []byte("\n"))
		if last >= first {
			counted, lastCounted = counted+last-first+1, last
		}
	}
	return counted
}

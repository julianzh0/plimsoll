package snapshot

import (
	"regexp"
	"strings"
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

// closing is the offset of the bracket that closes the one at open, a parenthesis or a brace, or the text's last
// offset when none does.
func closing(text []byte, open int) int {
	opener := text[open]
	closer := map[byte]byte{'(': ')', '{': '}'}[opener]
	depth := 0
	for i := open; i < len(text); i++ {
		switch text[i] {
		case opener:
			depth++
		case closer:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(text) - 1
}

// withoutComments is a text with every match of a pattern of strings and comments that is a comment blanked: a
// match that starts with a quote is a string and stays, so a comment marker inside one is no comment.
func withoutComments(stringsAndComments *regexp.Regexp, text string) string {
	return stringsAndComments.ReplaceAllStringFunc(text, func(match string) string {
		if match[0] == '"' {
			return match
		}
		return strings.Repeat(" ", len(match))
	})
}

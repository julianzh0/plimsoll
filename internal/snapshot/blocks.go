package snapshot

import (
	"regexp"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// masked returns the content with every comment its language's literal pattern matches as spaces, line ends kept, so
// offsets and line numbers stay those of the file; a match that starts with a quote is a string or a character
// literal and stays. A C++ raw string is blanked first, by the token scanner, so a line inside one is no directive.
// A language without a literal pattern returns the content itself.
func masked(language conventions.Language, content []byte) []byte {
	if language.Literals == nil {
		return content
	}
	if language.Tokens.Strings == conventions.CppRawStrings {
		content = withoutRawStrings(language.Tokens, content)
	}
	return language.Literals.ReplaceAllFunc(content, func(match []byte) []byte {
		if match[0] == '"' || match[0] == '\'' {
			return match
		}
		blank := slices.Clone(match)
		blankKeepingLines(blank)
		return blank
	})
}

// withoutRawStrings is the content with every C++ raw string blanked, line ends kept; the content itself when it holds
// none.
func withoutRawStrings(tokens conventions.Tokens, content []byte) []byte {
	out, cloned := content, false
	for t := range eachToken(tokens, content, 1) {
		if _, _, raw := cppRawAt(content, t.start); raw && t.word && !t.identifier {
			if !cloned {
				out, cloned = slices.Clone(content), true
			}
			blankKeepingLines(out[t.start:t.end])
		}
	}
	return out
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

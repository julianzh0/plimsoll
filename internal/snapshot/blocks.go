package snapshot

import (
	"bytes"
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
		content = withoutRawStrings(language.Literals, content)
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
// none. A raw string starts at an R" (with its prefix) that no name runs into and that no comment or ordinary literal
// before it holds (read once from the start, and on from the end of each raw string), and ends at its closing delimiter.
func withoutRawStrings(literals *regexp.Regexp, content []byte) []byte {
	out, cloned := content, false
	cursor := literalCursor{literals: literals, content: content}
	for at := 0; ; {
		i := bytes.Index(content[at:], []byte(`R"`))
		if i < 0 {
			return out
		}
		if held := cursor.holding(at + i); held > 0 {
			at = held
			continue
		}
		start := at + i
		for _, prefix := range []string{"u8", "u", "U", "L"} {
			if start >= len(prefix) && string(content[start-len(prefix):start]) == prefix {
				start -= len(prefix)
				break
			}
		}
		form, length, raw := cppRawAt(content, start)
		if !raw || start > 0 && identifierByte(content[start-1]) {
			at = at + i + 2
			continue
		}
		end := len(content)
		if close := bytes.Index(content[start+length:], []byte(form.closer)); close >= 0 {
			end = start + length + close + len(form.closer)
		}
		if !cloned {
			out, cloned = slices.Clone(content), true
		}
		blankKeepingLines(out[start:end])
		at = end
		cursor.read, cursor.pending = end, false
	}
}

// literalCursor reads a content's comments and ordinary literals once, left to right, for offsets asked in order: it
// keeps the next match it found, so no part of the content is searched twice.
type literalCursor struct {
	literals *regexp.Regexp
	content  []byte
	read     int    // searched up to here
	next     [2]int // the next match, when pending
	pending  bool
}

// holding is the end of the comment or ordinary literal that holds the offset, or 0 when none does.
func (c *literalCursor) holding(offset int) int {
	for {
		if !c.pending {
			if c.read > offset {
				return 0
			}
			m := c.literals.FindIndex(c.content[c.read:])
			switch {
			case m == nil:
				c.read = len(c.content) // nothing left to hold an offset
				return 0
			case m[1] == m[0]:
				c.read++
				continue
			}
			c.next, c.pending = [2]int{c.read + m[0], c.read + m[1]}, true
		}
		switch {
		case c.next[0] >= offset:
			return 0
		case c.next[1] > offset:
			c.read, c.pending = c.next[1], false
			return c.next[1]
		}
		c.read, c.pending = c.next[1], false
	}
}

// identifierByte says whether a byte can stand in a C name.
func identifierByte(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// span is a stretch of a file's text, both ends included, and the name of the block that opens it, if it has one.
type span struct {
	start, end int
	name       string
	place      string // a module's: the name it gives the place inside it, its path attribute (Rust) or its name
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

package snapshot

import (
	"regexp"
	"strings"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// The TOML that plimsoll reads by text: tables, their keys and arrays of strings, for the manifest and for the layout
// a project declares.
// NOTE: a reader by text, not a TOML parser; each caller names what it does not read. Upgrade: a TOML reader.

// tomlTable is the text of one TOML table: the lines after its header, a trailing comment allowed, up to the next line
// that opens a table.
func tomlTable(content, name string) (string, bool) {
	var table strings.Builder
	inside, found := false, false
	for line := range strings.Lines(content) {
		if header, isHeader := tomlHeader(line); isHeader {
			inside = header != "" && header == name
			found = found || inside
			continue
		}
		if inside {
			table.WriteString(line)
		}
	}
	return table.String(), found
}

// stringArray reads the strings of an array whose opening bracket is consumed, up to its closing bracket outside a
// string; closed is false when the array does not end, holds something that is not a string, or lacks the comma
// between two strings. rest is the text after the closing bracket.
func stringArray(text string) (items []string, rest string, closed bool) {
	separated := true
	for i := 0; i < len(text); i++ {
		switch c := text[i]; c {
		case ' ', '\t', '\r', '\n':
		case ',':
			separated = true
		case '#':
			end := strings.IndexByte(text[i:], '\n')
			if end < 0 {
				return nil, "", false
			}
			i += end
		case ']':
			return items, text[i+1:], true
		case '"', '\'':
			item, after, ok := quotedString(text[i:])
			if !ok || !separated {
				return nil, "", false
			}
			items, i, separated = append(items, item), len(text)-len(after)-1, false
		default:
			return nil, "", false
		}
	}
	return nil, "", false
}

// openedBrackets counts the brackets and braces a line opens minus those it closes, outside strings and before a
// comment, so a value that goes on over lines is followed to its end.
func openedBrackets(line string) int {
	depth := 0
	for i := 0; i < len(line); i++ {
		switch c := line[i]; c {
		case '#':
			return depth
		case '[', '{':
			depth++
		case ']', '}':
			depth--
		case '"', '\'':
			for i++; i < len(line) && line[i] != c; i++ {
				if c == '"' && line[i] == '\\' {
					i++
				}
			}
		}
	}
	return depth
}

// tomlHeader reads a line that opens a TOML table: the table's dotted name, and whether the line is a header at all. A
// header may be indented and carry a comment; an array-of-tables header is a header that names no table ("").
func tomlHeader(line string) (name string, isHeader bool) {
	text, _, _ := strings.Cut(line, "#")
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "[") || !strings.HasSuffix(text, "]") {
		return "", false
	}
	if strings.HasPrefix(text, "[[") {
		return "", true
	}
	return dottedName(strings.TrimSuffix(text[1:], "]")), true
}

// keyLine matches the start of a line that assigns a key, quoted or not, up to its value.
func keyLine(key string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^[ \t]*["']?` + regexp.QuoteMeta(key) + `["']?[ \t]*=[ \t]*`)
}

// valueOf reads a value that is an array of strings, or one string that holds values separated by white space.
func valueOf(value string) ([]string, KeyState) {
	switch {
	case strings.HasPrefix(value, "["):
		items, _, closed := stringArray(value[1:])
		if !closed {
			return nil, KeyUnreadable
		}
		return items, KeyRead
	case strings.HasPrefix(value, `"`) || strings.HasPrefix(value, "'"):
		end := strings.IndexByte(value[1:], value[0])
		if end < 0 || strings.Contains(value[1:1+end], "\n") {
			return nil, KeyUnreadable
		}
		return strings.Fields(value[1 : 1+end]), KeyRead
	}
	return nil, KeyUnreadable
}

// formNotRead says whether the file writes the key in a form the reader does not read: as a dotted key whose full
// name is the table's and the key's, or in an inline table assigned to the table.
func formNotRead(text, table, key string) bool {
	header := ""
	for line := range strings.Lines(text) {
		if name, isHeader := tomlHeader(line); isHeader {
			header = name
			continue
		}
		line = strings.TrimSpace(line)
		name, value, assigns := strings.Cut(line, "=")
		if !assigns {
			continue
		}
		full := dottedName(name)
		if header != "" {
			full = header + "." + full
		}
		if full == table+"."+key {
			return true
		}
		if inline := strings.TrimSpace(value); full == table && strings.HasPrefix(inline, "{") {
			if keyLine(key).MatchString(strings.ReplaceAll(strings.Trim(inline, "{}"), ",", "\n")) {
				return true
			}
		}
	}
	return false
}

// dottedName is a TOML key as one dotted name: quotes and the spaces around dots removed.
func dottedName(key string) string {
	parts := strings.Split(strings.NewReplacer(`"`, "", "'", "").Replace(key), ".")
	for i, part := range parts {
		parts[i] = strings.TrimSpace(part)
	}
	return strings.Join(parts, ".")
}

// tomlString is the one string a key of a table holds; "" when the table, the key or a single value is not there.
func tomlString(content []byte, table, key string) string {
	values, state := readDeclared(content, conventions.Declaration{Table: table, Key: key})
	if state != KeyRead || len(values) != 1 {
		return ""
	}
	return values[0]
}

// quotedString reads the basic or literal string that text starts with, on one line, and gives its content as
// written (no escape is decoded) and the text after its closing quote. In a basic string a backslash takes the next
// character with it, so an escaped quote does not end it.
func quotedString(text string) (content, rest string, ok bool) {
	if text == "" || text[0] != '"' && text[0] != '\'' {
		return "", text, false
	}
	quote := text[0]
	for end := 1; end < len(text) && text[end] != '\n'; end++ {
		switch {
		case quote == '"' && text[end] == '\\':
			end++
		case text[end] == quote:
			return text[1:end], text[end+1:], true
		}
	}
	return "", text, false
}

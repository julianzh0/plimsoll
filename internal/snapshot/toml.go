package snapshot

import (
	"regexp"
	"strings"
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
// string; closed is false when the array does not end or holds something that is not a string.
func stringArray(text string) (items []string, closed bool) {
	for i := 0; i < len(text); i++ {
		switch c := text[i]; c {
		case ' ', '\t', '\r', '\n', ',':
		case '#':
			end := strings.IndexByte(text[i:], '\n')
			if end < 0 {
				return items, false
			}
			i += end
		case ']':
			return items, true
		case '"', '\'':
			end := i + 1
			for end < len(text) && text[end] != c && text[end] != '\n' {
				if c == '"' && text[end] == '\\' {
					end++
				}
				end++
			}
			if end >= len(text) || text[end] != c {
				return items, false
			}
			items, i = append(items, text[i+1:end]), end
		default:
			return items, false
		}
	}
	return items, false
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
		items, closed := stringArray(value[1:])
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

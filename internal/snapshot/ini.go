package snapshot

import (
	"bytes"
	"strings"
)

// iniValue is the value of a key in a section of an INI file, as Python's configparser reads it: "key = value" or
// "key: value", continuation lines indented below it, and comment lines that start with "#" or ";".
func iniValue(content []byte, section, key string) (value string, found bool) {
	inSection := false
	var lines []string
	collecting := false
	for line := range strings.Lines(string(bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n")))) {
		line = strings.TrimRight(line, "\n")
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";"):
			continue
		case collecting && (line[0] == ' ' || line[0] == '\t'):
			lines = append(lines, trimmed)
			continue
		case collecting:
			return strings.Join(lines, "\n"), true
		case strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]"):
			inSection = strings.TrimSpace(trimmed[1:len(trimmed)-1]) == section
			continue
		case !inSection:
			continue
		}
		at := strings.IndexAny(trimmed, "=:")
		if at < 0 || strings.ToLower(strings.TrimSpace(trimmed[:at])) != key {
			continue
		}
		lines, collecting = []string{strings.TrimSpace(trimmed[at+1:])}, true
	}
	if collecting {
		return strings.Join(lines, "\n"), true
	}
	return "", false
}

// iniHolds says whether an INI file has the given section.
func iniHolds(content []byte, section string) bool {
	for line := range strings.Lines(string(content)) {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") && strings.TrimSpace(trimmed[1:len(trimmed)-1]) == section {
			return true
		}
	}
	return false
}

// shellWords splits a text as a POSIX shell splits arguments, as pytest splits an "args" option: at white space,
// with single and double quotes holding white space and a backslash escaping one character. ok is false for a quote
// that does not close.
func shellWords(text string) (words []string, ok bool) {
	var word strings.Builder
	inWord := false
	var quote rune
	escaped := false
	for _, r := range text {
		switch {
		case escaped:
			word.WriteRune(r)
			escaped = false
		case quote == '\'' && r == '\'', quote == '"' && r == '"':
			quote = 0
		case quote == '\'':
			word.WriteRune(r)
		case r == '\\':
			escaped, inWord = true, true
		case quote == '"':
			word.WriteRune(r)
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case r == ' ' || r == '\t' || r == '\n':
			if inWord {
				words = append(words, word.String())
				word.Reset()
				inWord = false
			}
		default:
			word.WriteRune(r)
			inWord = true
		}
	}
	if quote != 0 {
		return nil, false
	}
	if inWord {
		words = append(words, word.String())
	}
	return words, true
}

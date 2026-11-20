package snapshot

import (
	"strings"
)

// iniValue is the value of a key in a section of an INI file, as pytest's INI reader (iniconfig) reads it:
// "key = value" or "key: value" with the key as written, continuation lines indented below it, and comment lines that
// start with "#" or ";".
func iniValue(content []byte, section, key string) (value string, found bool) {
	inSection := false
	var lines []string
	for line := range strings.Lines(strings.ReplaceAll(string(content), "\r\n", "\n")) {
		line = strings.TrimRight(line, "\n")
		trimmed := strings.TrimSpace(line)
		switch name, isHeader := iniSection(trimmed); {
		case trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";"):
		case found && (line[0] == ' ' || line[0] == '\t'):
			lines = append(lines, trimmed)
		case found:
			return strings.Join(lines, "\n"), true
		case isHeader:
			inSection = name == section
		case inSection:
			at := strings.IndexAny(trimmed, "=:")
			if at >= 0 && strings.TrimSpace(trimmed[:at]) == key {
				lines, found = []string{strings.TrimSpace(trimmed[at+1:])}, true
			}
		}
	}
	return strings.Join(lines, "\n"), found
}

// iniHolds says whether an INI file has the given section.
func iniHolds(content []byte, section string) bool {
	for line := range strings.Lines(string(content)) {
		if name, isHeader := iniSection(strings.TrimSpace(line)); isHeader && name == section {
			return true
		}
	}
	return false
}

// iniSection is the name of a section header line, trimmed: "[name]".
func iniSection(trimmed string) (name string, isHeader bool) {
	if !strings.HasPrefix(trimmed, "[") || !strings.HasSuffix(trimmed, "]") {
		return "", false
	}
	return strings.TrimSpace(trimmed[1 : len(trimmed)-1]), true
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

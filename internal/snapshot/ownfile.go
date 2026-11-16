package snapshot

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// DeclaredValues are the values one text key of a repository's own file declares on each side: the usable values in
// file order, each once. Compared is false when the file, or the key, could not be read on either side; UnreadAtBase
// says the key could not be read at the base.
type DeclaredValues struct {
	Table, Key, Name string
	Now, Before      []string
	Compared         bool
	UnreadAtBase     bool
}

// ownFile is what one side's own file says.
type ownFile struct {
	unreadable bool                // tracked, and its content could not be read
	limits     map[string]float64  // the limits it sets, by key
	values     map[string][]string // per text key that was read: its usable values, in file order, each once
	unusable   map[string]int      // per text key: the values that have not their form
	unread     map[string]bool     // text keys whose value could not be read
	problems   []string            // what the file says that is not read: phrases, sorted, each once
}

// readOwnFile reads one side's own file in one pass: its limits and its text keys. A table, a key or a value it does
// not read is a problem phrase, and what it names keeps plimsoll's default. Values are brought to their form; one
// without it is counted, never kept.
func readOwnFile(rules conventions.Rules, content map[string][]byte) ownFile {
	o := ownFile{limits: map[string]float64{}, values: map[string][]string{}, unusable: map[string]int{}, unread: map[string]bool{}}
	file, tracked := content[rules.OwnFile]
	switch {
	case !tracked:
		return o
	case file == nil:
		o.unreadable, o.problems = true, []string{"it could not be read"}
		return o
	}
	known := map[string]bool{rules.LimitsTable: true}
	for _, k := range rules.OwnKeys {
		known[k.Table] = true
	}
	problem := func(format string, args ...any) { o.problems = append(o.problems, fmt.Sprintf(format, args...)) }
	seen := map[string]bool{}
	table, found := "", false
	lines := slices.Collect(strings.Lines(strings.TrimPrefix(string(file), byteOrderMark)))
	for i := 0; i < len(lines); i++ {
		if header, isHeader := tomlHeader(lines[i]); isHeader {
			table = header
			found = found || known[header]
			if header != "" && !known[header] {
				problem("`[%s]` is no table plimsoll reads", header)
			}
			continue
		}
		code := uncommented(lines[i])
		if strings.TrimSpace(code) == "" {
			continue
		}
		name, value, assigns := strings.Cut(code, "=")
		key := dottedName(name)
		switch {
		case table == "" && assigns:
			problem("`%s` lies outside a table", key)
			continue
		case !known[table]:
			continue
		case !assigns:
			problem("`[%s]` holds a line that is no `key = value`", table)
			continue
		}
		for depth := openedBrackets(value); depth > 0 && i+1 < len(lines); depth += openedBrackets(lines[i]) {
			i++
			value += lines[i]
		}
		if table == rules.LimitsTable {
			o.readLimit(rules, key, value, problem)
			continue
		}
		at := slices.IndexFunc(rules.OwnKeys, func(k conventions.OwnKey) bool { return k.Table == table && k.Key == key })
		switch {
		case at < 0:
			problem("`%s` is no key of `[%s]`", key, table)
		case seen[key]:
			problem("`%s` is set twice", key)
		default:
			seen[key] = true
			o.readList(rules.OwnKeys[at], value, problem)
		}
	}
	if !found {
		problem("it holds no table plimsoll reads")
	}
	slices.Sort(o.problems)
	o.problems = slices.Compact(o.problems)
	return o
}

// readLimit reads one key of the limits table: a number above 0, whole for a limit that counts.
func (o *ownFile) readLimit(rules conventions.Rules, key, value string, problem func(string, ...any)) {
	if !slices.ContainsFunc(rules.LimitKeys, func(k conventions.LimitKey) bool { return k.Key == key }) {
		problem("`%s` is no limit plimsoll knows", key)
		return
	}
	if _, twice := o.limits[key]; twice {
		problem("`%s` is set twice", key)
		return
	}
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	whole := rules.Limits.ByKey(key).Step >= 1
	switch {
	case err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number <= 0:
		problem("`%s` is not a number above 0", key)
	case whole && number != math.Trunc(number):
		problem("`%s` is not a whole number above 0", key)
	default:
		o.limits[key] = number
	}
}

// readList reads a list of strings, each brought to its key's form.
func (o *ownFile) readList(key conventions.OwnKey, value string, problem func(string, ...any)) {
	text := strings.TrimSpace(value)
	items, closed := []string(nil), false
	if strings.HasPrefix(text, "[") {
		items, closed = stringArray(text[1:])
	}
	if !closed {
		o.unread[key.Key] = true
		problem("`%s` is not a list of text", key.Key)
		return
	}
	var values []string
	for _, item := range items {
		formed, usable := key.Holds.Form(item)
		switch {
		case !usable:
			o.unusable[key.Key]++
		case !slices.Contains(values, formed):
			values = append(values, formed)
		}
	}
	o.values[key.Key] = values
}

// uncommented is a line up to a "#" that lies outside a string.
func uncommented(line string) string {
	var quote byte
	for i := 0; i < len(line); i++ {
		switch c := line[i]; {
		case quote != 0 && c == '\\' && quote == '"':
			i++
		case quote != 0 && c == quote:
			quote = 0
		case quote == 0 && (c == '"' || c == '\''):
			quote = c
		case quote == 0 && c == '#':
			return line[:i]
		}
	}
	return line
}

// declaredValues are the text keys declared on either side, in the order of the rules, each with the usable values
// of both sides.
func declaredValues(rules conventions.Rules, now, before ownFile) []DeclaredValues {
	var declared []DeclaredValues
	for _, k := range rules.OwnKeys {
		nowValues, beforeValues := now.values[k.Key], before.values[k.Key]
		if len(nowValues) == 0 && len(beforeValues) == 0 {
			continue
		}
		declared = append(declared, DeclaredValues{
			Table: k.Table, Key: k.Key, Name: k.Name, Now: nowValues, Before: beforeValues,
			Compared:     !now.unreadable && !before.unreadable && !now.unread[k.Key] && !before.unread[k.Key],
			UnreadAtBase: before.unread[k.Key],
		})
	}
	return declared
}

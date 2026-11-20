package snapshot

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// DeclaredValues are the values one text key of a repository's own file declares on each side: the usable values in
// file order, each once. Compared is false when the file, or the key, could not be read on either side; UnreadAtBase
// says the key could not be read at the base.
type DeclaredValues struct {
	Table, Key, Name string
	Now, Before      []string
	Whole            bool // a whole table whose keys are the values; Key is then the table's name
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
	single     map[string]string   // per key that holds one name and could not be used: its problem phrase
	problems   []string            // what the file says that is not read: phrases, sorted, each once
	ack        acknowledged        // its acknowledged table
}

// acknowledged is what one side's acknowledged table says: the identifiers in their form, each once, the first of
// them whose entry has a usable reason (applied), and its problem phrases.
type acknowledged struct {
	inForm, applied    []string
	seen               map[string]bool // the identifiers in inForm
	noEntry, notInForm int
	named              []string // phrases that name an identifier in its form, in file order
}

// readAcknowledged reads one line of the acknowledged table: a quoted identifier, `=`, a quoted reason, and nothing
// else. The identifier is kept byte for byte; the reason is only checked.
func (a *acknowledged) read(line string) {
	id, rest, ok := quotedString(strings.TrimSpace(line))
	rest, assigns := strings.CutPrefix(strings.TrimLeft(rest, " \t"), "=")
	switch {
	case !ok || !assigns:
		a.noEntry++
		return
	case !conventions.MarkID(id):
		a.notInForm++
		return
	case a.seen[id]:
		a.named = append(a.named, fmt.Sprintf("`%s` is set twice; its first entry decides", id))
		return
	}
	if a.seen == nil {
		a.seen = map[string]bool{}
	}
	a.seen[id] = true
	a.inForm = append(a.inForm, id)
	reason, after, ok := quotedString(strings.TrimSpace(rest))
	switch {
	case !ok || strings.TrimSpace(after) != "" || strings.TrimSpace(reason) == "":
		a.named = append(a.named, fmt.Sprintf("`%s` has no reason written as text on its line; it is not applied", id))
	case utf8.RuneCountInString(reason) > conventions.ReasonLength:
		a.named = append(a.named, fmt.Sprintf("the reason of `%s` is longer than %d characters; it is not applied", id, conventions.ReasonLength))
	default:
		a.applied = append(a.applied, id)
	}
}

// problems are the table's phrases: counted lines and identifiers first, then the named ones in file order.
func (a acknowledged) problems() []string {
	var phrases []string
	if a.noEntry > 0 {
		phrases = append(phrases, counted(a.noEntry, "line is no `\"identifier\" = \"reason\"` entry; it is", "lines are no `\"identifier\" = \"reason\"` entries; they are")+" not applied")
	}
	if a.notInForm > 0 {
		phrases = append(phrases, counted(a.notInForm, "identifier is no plain mark identifier; it is", "identifiers are no plain mark identifiers; they are")+" not applied")
	}
	return append(phrases, a.named...)
}

// counted puts a count before the singular or the plural phrase.
func counted(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// readOwnFile reads one side's own file in one pass: its limits and its text keys. A table, a key or a value it does
// not read is a problem phrase, and what it names keeps plimsoll's default. Values are brought to their form; one
// without it is counted, never kept.
func readOwnFile(rules conventions.Rules, content map[string][]byte) ownFile {
	o := ownFile{limits: map[string]float64{}, values: map[string][]string{}, unusable: map[string]int{}, unread: map[string]bool{}, single: map[string]string{}}
	file, tracked := content[rules.OwnFile]
	switch {
	case !tracked:
		return o
	case file == nil:
		o.unreadable, o.problems = true, []string{"it could not be read"}
		return o
	}
	known := map[string]bool{rules.LimitsTable: true, rules.AcknowledgedTable: true}
	for _, k := range rules.OwnKeys {
		known[k.Table] = true
	}
	problem := func(format string, args ...any) { o.problems = append(o.problems, fmt.Sprintf(format, args...)) }
	seen := map[string]bool{}
	table, found := "", false
	lines := tomlLines(string(file))
	for i := 0; i < len(lines); i++ {
		if header, isHeader := tomlHeader(lines[i]); isHeader {
			table = header
			found = found || known[header]
			if header != "" && !known[header] {
				problem("%s is no table plimsoll reads", tableName(header))
			}
			continue
		}
		code := uncommented(lines[i])
		if strings.TrimSpace(code) == "" {
			continue
		}
		if table == rules.AcknowledgedTable {
			o.ack.read(code)
			continue
		}
		name, value, assigns := strings.Cut(code, "=")
		key := dottedName(name)
		switch {
		case table == "" && assigns:
			problem("%s lies outside a table", keyName(key))
			continue
		case !known[table]:
			continue
		case !assigns:
			problem("`[%s]` holds a line that is no `key = value`", table)
			continue
		}
		for unclosed(value) && i+1 < len(lines) {
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
			problem("%s is no key of `[%s]`", keyName(key), table)
		case seen[key]:
			problem("`%s` is set twice", key)
		default:
			seen[key] = true
			if rules.OwnKeys[at].Holds.Single() {
				o.readString(rules.OwnKeys[at], value, problem)
			} else {
				o.readList(rules.OwnKeys[at], value, problem)
			}
		}
	}
	if !found {
		problem("it holds no table plimsoll reads")
	}
	slices.Sort(o.problems)
	o.problems = slices.Compact(o.problems)
	return o
}

// plainName is a TOML bare key, dotted or not: the only form in which a name the file gives and plimsoll does not
// know is printed. Any other name is only counted, so no text of the file reaches an answer.
var plainName = regexp.MustCompile(`^[A-Za-z0-9_-]+(\.[A-Za-z0-9_-]+)*$`)

// keyName and tableName print a name the file gives and plimsoll does not know.
func keyName(name string) string {
	if len(name) <= conventions.OwnValueLength && plainName.MatchString(name) {
		return "`" + name + "`"
	}
	return "a key with no plain name"
}

func tableName(name string) string {
	if len(name) <= conventions.OwnValueLength && plainName.MatchString(name) {
		return "`[" + name + "]`"
	}
	return "a table with no plain name"
}

// largestExactCount is the largest whole number a float64 holds exactly (2^53); a larger one would be rounded.
const largestExactCount = 1 << 53

// readLimit reads one key of the limits table: a number above 0, whole for a limit that counts.
func (o *ownFile) readLimit(rules conventions.Rules, key, value string, problem func(string, ...any)) {
	if !slices.ContainsFunc(rules.LimitKeys, func(k conventions.LimitKey) bool { return k.Key == key }) {
		problem("%s is no limit plimsoll knows", keyName(key))
		return
	}
	if _, twice := o.limits[key]; twice {
		problem("`%s` is set twice", key)
		return
	}
	text := strings.TrimSpace(value)
	if rules.Limits.ByKey(key).Step >= 1 {
		// A limit that counts is a TOML integer that a float holds exactly, so every conversion back to an int is
		// exact: 2.5 and 1e19 are not.
		count, err := strconv.ParseInt(text, 10, 64)
		if err != nil || count <= 0 || count > largestExactCount {
			problem("`%s` is not a whole number above 0", key)
			return
		}
		o.limits[key] = float64(count)
		return
	}
	number, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number <= 0 {
		problem("`%s` is not a number above 0", key)
		return
	}
	o.limits[key] = number
}

// readList reads a list of strings, each brought to its key's form.
func (o *ownFile) readList(key conventions.OwnKey, value string, problem func(string, ...any)) {
	text := strings.TrimSpace(value)
	items, rest, closed := []string(nil), "", false
	if strings.HasPrefix(text, "[") {
		items, rest, closed = stringArray(text[1:])
	}
	if !closed || strings.TrimSpace(uncommented(rest)) != "" {
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
		default:
			values = appendNew(values, formed)
		}
	}
	o.values[key.Key] = values
}

// readString reads one basic or literal string on one line, which must have its key's form.
func (o *ownFile) readString(key conventions.OwnKey, value string, problem func(string, ...any)) {
	text := strings.TrimSpace(value)
	if len(text) < 2 || (text[0] != '"' && text[0] != '\'') || text[len(text)-1] != text[0] || strings.ContainsAny(text[1:len(text)-1], "\"'\n") {
		o.unread[key.Key] = true
		o.single[key.Key] = fmt.Sprintf("`%s` is not text", key.Key)
		problem("%s", o.single[key.Key])
		return
	}
	formed, usable := key.Holds.Form(text[1 : len(text)-1])
	if !usable {
		o.unusable[key.Key]++
		o.single[key.Key] = fmt.Sprintf("`%s` is not the name of a branch, a remote branch or a tag", key.Key)
		problem("%s", o.single[key.Key])
		return
	}
	o.values[key.Key] = []string{formed}
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
	if len(now.ack.inForm) > 0 || len(before.ack.inForm) > 0 {
		declared = append(declared, DeclaredValues{
			Table: rules.AcknowledgedTable, Key: rules.AcknowledgedTable, Name: rules.AcknowledgedName,
			Now: now.ack.inForm, Before: before.ack.inForm, Whole: true, Compared: !now.unreadable && !before.unreadable,
		})
	}
	return declared
}

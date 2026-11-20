package snapshot

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"

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
	unreadable bool                  // tracked, and its content could not be read
	limits     map[string]float64    // the limits it sets, by key
	values     map[string][]string   // per text key that was read: its usable values, in file order, each once
	unusable   map[string]int        // per text key: the values that have not their form
	unread     map[string]bool       // text keys whose value could not be read
	single     map[string]string     // per key that holds one name and could not be used: its problem phrase
	problems   []string              // what the file says that is not read: phrases, sorted, each once
	ack        acknowledged          // its acknowledged table
	written    map[string][]ownEntry // per list key: its usable values as written, plain and widened, in file order
}

// ownEntry is one usable value of a list key: in its form, whether that form is the widened one, and its position in
// the list as written (from 1).
type ownEntry struct {
	value    string
	wide     bool
	position int
}

// acknowledged is what one side's acknowledged table says: the identifiers in their form, sorted, those whose entry
// has a usable reason (applied), and its problem phrases.
type acknowledged struct {
	inForm, applied []string
	widened         []string // identifiers in the widened form with a usable reason, sorted; never printed
	notInForm       int
	named           []string // phrases that name an identifier in its form, by identifier
}

// read reads one entry of the acknowledged table: an identifier and a reason written as text. The identifier is kept
// byte for byte; the reason is only checked.
func (a *acknowledged) read(id string, value any) {
	wide := !conventions.MarkID(id)
	if wide && !conventions.WideMarkID(id) {
		a.notInForm++
		return
	}
	reason, isText := value.(string)
	if wide { // applied only when an answer holds an item of this identifier; never named
		if isText && strings.TrimSpace(reason) != "" && utf8.RuneCountInString(reason) <= conventions.ReasonLength {
			a.widened = append(a.widened, id)
		}
		return
	}
	a.inForm = append(a.inForm, id)
	switch {
	case !isText || strings.TrimSpace(reason) == "":
		a.named = append(a.named, fmt.Sprintf("`%s` has no reason written as text; it is not applied", id))
	case utf8.RuneCountInString(reason) > conventions.ReasonLength:
		a.named = append(a.named, fmt.Sprintf("the reason of `%s` is longer than %d characters; it is not applied", id, conventions.ReasonLength))
	default:
		a.applied = append(a.applied, id)
	}
}

// problems are the table's phrases: the counted identifiers first, then the named ones by identifier.
func (a acknowledged) problems() []string {
	var phrases []string
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

// readOwnFile reads one side's own file: its limits and its text keys. A file that is not valid TOML applies nothing;
// its problem names the line, never its text. In a valid file, a table, a key or a value it does not read is a problem
// phrase, and what it names keeps plimsoll's default. Values are brought to their form; one without it is counted,
// never kept.
func readOwnFile(rules conventions.Rules, content map[string][]byte) ownFile {
	o := ownFile{
		limits: map[string]float64{}, values: map[string][]string{}, unusable: map[string]int{}, unread: map[string]bool{},
		single: map[string]string{}, written: map[string][]ownEntry{},
	}
	file, tracked := content[rules.OwnFile]
	switch {
	case !tracked:
		return o
	case file == nil:
		o.unreadable, o.problems = true, []string{"it could not be read"}
		return o
	}
	var document map[string]any
	if err := toml.Unmarshal(file, &document); err != nil {
		o.unreadable, o.problems = true, []string{notTOML(err) + ", so none of it is applied and its import rules were not checked"}
		return o
	}
	known := map[string]bool{rules.LimitsTable: true, rules.AcknowledgedTable: true}
	for _, k := range rules.OwnKeys {
		known[k.Table] = true
	}
	problem := func(format string, args ...any) { o.problems = append(o.problems, fmt.Sprintf(format, args...)) }
	found := false
	for _, name := range slices.Sorted(maps.Keys(document)) {
		table, isTable := document[name].(map[string]any)
		switch {
		case !isTable:
			problem("%s lies outside a table", keyName(name))
			continue
		case !known[name]:
			problem("%s is no table plimsoll reads", tableName(name))
			continue
		}
		found = true
		for _, key := range slices.Sorted(maps.Keys(table)) {
			value := table[key]
			switch name {
			case rules.AcknowledgedTable:
				o.ack.read(key, value)
				continue
			case rules.LimitsTable:
				o.readLimit(rules, key, value, problem)
				continue
			}
			at := slices.IndexFunc(rules.OwnKeys, func(k conventions.OwnKey) bool { return k.Table == name && k.Key == key })
			switch {
			case at < 0:
				problem("%s is no key of `[%s]`", keyName(key), name)
			case rules.OwnKeys[at].Holds.Single():
				o.readString(rules.OwnKeys[at], value, problem)
			default:
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

// notTOML says why a file is no valid TOML: the line where decoding stopped, never the decoder's message, which
// quotes the file.
func notTOML(err error) string {
	if decodeError, ok := errors.AsType[*toml.DecodeError](err); ok {
		if row, _ := decodeError.Position(); row > 0 {
			return fmt.Sprintf("it is not valid TOML at line %d", row)
		}
	}
	return "it is not valid TOML"
}

// plainName is a TOML bare key, dotted or not, or a table name of such keys: the only form in which a name the file gives and plimsoll does not
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
func (o *ownFile) readLimit(rules conventions.Rules, key string, value any, problem func(string, ...any)) {
	if !slices.ContainsFunc(rules.LimitKeys, func(k conventions.LimitKey) bool { return k.Key == key }) {
		problem("%s is no limit plimsoll knows", keyName(key))
		return
	}
	count, isInteger := value.(int64)
	if rules.Limits.ByKey(key).Step >= 1 {
		// A limit that counts is a TOML integer that a float holds exactly, so every conversion back to an int is
		// exact: 2.5 and 1e19 are not.
		if !isInteger || count <= 0 || count > largestExactCount {
			problem("`%s` is not a whole number above 0", key)
			return
		}
		o.limits[key] = float64(count)
		return
	}
	number, isFloat := value.(float64)
	if isInteger {
		number, isFloat = float64(count), true
	}
	if !isFloat || math.IsNaN(number) || math.IsInf(number, 0) || number <= 0 {
		problem("`%s` is not a number above 0", key)
		return
	}
	o.limits[key] = number
}

// readList reads a list of text, each value brought to its key's form.
func (o *ownFile) readList(key conventions.OwnKey, value any, problem func(string, ...any)) {
	items, isList := value.([]any)
	texts := make([]string, 0, len(items))
	for _, item := range items {
		if text, isText := item.(string); isText {
			texts = append(texts, text)
		}
	}
	if !isList || len(texts) != len(items) {
		o.unread[key.Key] = true
		problem("`%s` is not a list of text", key.Key)
		return
	}
	var values []string
	for i, text := range texts {
		formed, usable := key.Holds.Form(text)
		if usable {
			values = appendNew(values, formed)
			o.written[key.Key] = append(o.written[key.Key], ownEntry{value: formed, position: i + 1})
			continue
		}
		if formed, usable = key.Holds.WideForm(text); usable {
			o.written[key.Key] = append(o.written[key.Key], ownEntry{value: formed, wide: true, position: i + 1})
			continue
		}
		o.unusable[key.Key]++
	}
	o.values[key.Key] = values
}

// withWideValues adds each widened value that names a tracked path to its key's values, in file order: a folder
// under which a tracked path lies, an import rule whose two folders both do. Such a value is the leading part of a
// tracked path, which answers print anyway. A widened value that names none is not used, and only its key and
// position are named.
func (o ownFile) withWideValues(rules conventions.Rules, tracked []string) ownFile {
	names := func(folder string) bool {
		return slices.ContainsFunc(tracked, func(p string) bool { _, in := under(p, folder); return in })
	}
	for _, k := range rules.OwnKeys {
		entries := o.written[k.Key]
		if !slices.ContainsFunc(entries, func(e ownEntry) bool { return e.wide }) {
			continue
		}
		var values []string
		for _, e := range entries {
			from, to := conventions.ImportRuleEnds(e.value)
			switch {
			case !e.wide:
			case k.Holds.ImportRule() && names(from) && names(to), !k.Holds.ImportRule() && names(e.value):
			case k.Holds.ImportRule():
				o.problems = append(o.problems, fmt.Sprintf("value %d of `%s` matches no tracked folder; it is not used", e.position, k.Key))
				continue
			default:
				o.problems = append(o.problems, fmt.Sprintf("value %d of `%s` matches no tracked file; it is not used", e.position, k.Key))
				continue
			}
			values = appendNew(values, e.value)
		}
		o.values[k.Key] = values
	}
	slices.Sort(o.problems)
	return o
}

// readString reads one text, which must have its key's form.
func (o *ownFile) readString(key conventions.OwnKey, value any, problem func(string, ...any)) {
	text, isText := value.(string)
	if !isText {
		o.unread[key.Key] = true
		o.single[key.Key] = fmt.Sprintf("`%s` is not text", key.Key)
		problem("%s", o.single[key.Key])
		return
	}
	formed, usable := key.Holds.Form(text)
	if !usable {
		o.unusable[key.Key]++
		o.single[key.Key] = fmt.Sprintf("`%s` is not the name of a branch, a remote branch or a tag", key.Key)
		problem("%s", o.single[key.Key])
		return
	}
	o.values[key.Key] = []string{formed}
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

package snapshot

import (
	"fmt"
	"maps"
	"math"
	"regexp"
	"slices"
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
	Whole            bool     // a whole table whose keys are the values; Key is then the table's name
	Hidden           []string // values compared but never printed: identifiers that are no plain names
	// Ordered says the order of the table's entries decides what they do; OrderChanged that it changed: the entries
	// both sides name stand in another order.
	Ordered, OrderChanged bool
	Compared              bool
	UnreadAtBase          bool
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
	tracked    []string            // the tracked paths of either side, against which a widened value is checked
	tests      testsTable          // its tests table
}

// acknowledged is what one side's acknowledged table says: the identifiers in their form, sorted, those whose entry
// has a usable reason (applied), and its problem phrases.
type acknowledged struct {
	inForm, applied []string
	widened         []string // identifiers in the widened form with a usable reason, sorted; never printed
	wideAll         []string // every identifier in the widened form, sorted, for comparing the two sides; never printed
	wideUnusable    int      // identifiers in the widened form without a usable reason
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
	isText = isText && strings.TrimSpace(reason) != ""
	tooLong := utf8.RuneCountInString(reason) > conventions.ReasonLength
	if wide { // applied only when an answer holds an item of this identifier; never named
		a.wideAll = append(a.wideAll, id)
		if isText && !tooLong {
			a.widened = append(a.widened, id)
		} else {
			a.wideUnusable++
		}
		return
	}
	a.inForm = append(a.inForm, id)
	switch {
	case !isText:
		a.named = append(a.named, fmt.Sprintf("`%s` has no reason written as text; it is not applied", id))
	case tooLong:
		a.named = append(a.named, fmt.Sprintf("the reason of `%s` is longer than %d characters; it is not applied", id, conventions.ReasonLength))
	default:
		a.applied = append(a.applied, id)
	}
}

// problems are the table's phrases: the counted identifiers first, then the named ones by identifier.
func (a acknowledged) problems() []string {
	var phrases []string
	if a.wideUnusable > 0 {
		phrases = append(phrases, counted(a.wideUnusable, "entry that is no plain identifier has no reason written as text within its length; it is", "entries that are no plain identifiers have no reason written as text within its length; they are")+" not applied")
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

// readOwnFile reads one side's own file: its limits and its text keys. A file that is not valid TOML applies nothing;
// its problem names the line, never its text. In a valid file, a table, a key or a value it does not read is a problem
// phrase, and what it names keeps plimsoll's default. Values are brought to their form; one without it is counted,
// never kept.
func readOwnFile(rules conventions.Rules, content map[string][]byte, tracked []string) ownFile {
	o := ownFile{
		limits: map[string]float64{}, values: map[string][]string{}, unusable: map[string]int{}, unread: map[string]bool{},
		single: map[string]string{}, tracked: tracked,
	}
	file, isTracked := content[rules.OwnFile]
	switch {
	case !isTracked:
		return o
	case file == nil:
		o.unreadable, o.problems = true, []string{"it could not be read"}
		return o
	}
	document, err := decodeTOML(file)
	if err != nil {
		o.unreadable, o.problems = true, []string{notTOML(err) + ", so none of it is applied and its import rules were not checked"}
		return o
	}
	known := map[string]bool{rules.LimitsTable: true, rules.AcknowledgedTable: true, rules.TestsTable: true}
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
		if name == rules.TestsTable {
			for _, kind := range tomlKeyOrder(file, name, table) {
				o.tests.read(name, kind, table[kind])
			}
			continue
		}
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
	o.problems = append(o.problems, o.tests.problems(rules.TestsTable)...)
	slices.Sort(o.problems)
	o.problems = slices.Compact(o.problems)
	return o
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

// readList reads a list of text, each value brought to its key's form. A value in the widened form is kept only when
// it names a tracked path: a folder under which one lies, an import rule whose two folders both have one. It is then
// the leading part of a tracked path, which answers print anyway; one that names none is named by key and position.
func (o *ownFile) readList(key conventions.OwnKey, value any, problem func(string, ...any)) {
	texts, isList := tomlTexts(value)
	if !isList {
		o.unread[key.Key] = true
		problem("`%s` is not a list of text", key.Key)
		return
	}
	var values []string
	for i, text := range texts {
		formed, usable := key.Holds.Form(text)
		if !usable {
			if formed, usable = key.Holds.WideForm(text); !usable {
				o.unusable[key.Key]++
				continue
			}
			if !o.namesTracked(key, formed) {
				problem("value %d of `%s` matches no tracked folder; it is not used", i+1, key.Key)
				continue
			}
		}
		values = appendNew(values, formed)
	}
	o.values[key.Key] = values
}

// namesTracked says whether a value in its form names a tracked path: a folder under which one lies, or an import
// rule whose two folders both have one.
func (o *ownFile) namesTracked(key conventions.OwnKey, value string) bool {
	holds := func(folder string) bool {
		prefix := folder + "/"
		return slices.ContainsFunc(o.tracked, func(p string) bool { return strings.HasPrefix(p, prefix) })
	}
	if key.Holds.ImportRule() {
		from, to := conventions.ImportRuleEnds(value)
		return holds(from) && holds(to)
	}
	return holds(value)
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
	nowIDs, beforeIDs := slices.Concat(now.ack.inForm, now.ack.wideAll), slices.Concat(before.ack.inForm, before.ack.wideAll)
	if len(nowIDs) > 0 || len(beforeIDs) > 0 {
		declared = append(declared, DeclaredValues{
			Table: rules.AcknowledgedTable, Key: rules.AcknowledgedTable, Name: rules.AcknowledgedName,
			Now: nowIDs, Before: beforeIDs, Hidden: slices.Concat(now.ack.wideAll, before.ack.wideAll),
			Whole: true, Compared: !now.unreadable && !before.unreadable,
		})
	}
	if nowPairs, beforePairs := now.tests.pairs(), before.tests.pairs(); len(nowPairs) > 0 || len(beforePairs) > 0 {
		declared = append(declared, DeclaredValues{
			Table: rules.TestsTable, Key: rules.TestsTable, Name: rules.TestKindsName, Now: nowPairs, Before: beforePairs,
			Whole: true, Ordered: true, Compared: !now.unreadable && !before.unreadable, OrderChanged: orderChanged(now.tests.names(), before.tests.names()),
		})
	}
	return declared
}

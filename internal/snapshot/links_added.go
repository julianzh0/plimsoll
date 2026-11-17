package snapshot

import (
	"slices"
	"strconv"
	"strings"
)

// baseImports is what the base side says of the files a change names: enough to tell whether the change wrote an
// import of now, or the import was there before and perhaps only became readable.
type baseImports struct {
	paths      map[string]string          // a path a change names now -> its path at the base, "" when it is not there
	unread     map[string]bool            // a path at the base whose content was not read: its imports are unknown
	statements map[string]map[string]bool // a path at the base -> the keys of the import statements its content holds
	targets    map[string]map[string]bool // a path at the base -> the keys of the targets its links reach
}

// baseImportsOf collects, for every path a change names, its path at the base, the import statements its content held
// there and the targets its links reached there.
func baseImportsOf(changes []Change, before Tree, atBase map[string]bool, beforeReadings map[string]reading) *baseImports {
	base := &baseImports{paths: map[string]string{}, unread: map[string]bool{}, statements: map[string]map[string]bool{}, targets: map[string]map[string]bool{}}
	for _, change := range changes {
		switch {
		case change.From != "":
			base.paths[change.Path] = change.From
		case atBase[change.Path]:
			base.paths[change.Path] = change.Path
		default:
			base.paths[change.Path] = ""
		}
	}
	for _, basePath := range base.paths {
		if basePath == "" || base.statements[basePath] != nil {
			continue
		}
		r, read := beforeReadings[basePath]
		base.unread[basePath] = !read || r.unread
		keys := map[string]bool{}
		for _, st := range r.statements {
			keys[statementKey(st)] = true
		}
		base.statements[basePath] = keys
		base.targets[basePath] = map[string]bool{}
	}
	for _, link := range before.Links.List {
		if reached, named := base.targets[link.From]; named {
			reached[targetKey(link.To, link.TypeOnly)] = true
		}
	}
	return base
}

// added says whether the change wrote the import of statement st from the file from to the target to. A file no change
// names did not change its imports; a file new in the change wrote all of them; otherwise the import is the change's
// only when its base content holds neither the same statement nor a link to the same target.
func (base *baseImports) added(from, to string, st statement) bool {
	if base == nil {
		return false
	}
	basePath, named := base.paths[from]
	switch {
	case !named, base.unread[basePath]:
		return false
	case basePath == "":
		return true
	case base.statements[basePath][statementKey(st)]:
		return false
	}
	targetAtBase := to
	if renamed, moved := base.paths[to]; moved && renamed != "" {
		targetAtBase = renamed
	}
	return !base.targets[basePath][targetKey(targetAtBase, st.typeOnly)]
}

// statementKey names a statement by what it imports, not where it stands: its written target, its names in order,
// and whether it imports types only.
func statementKey(st statement) string {
	names := slices.Clone(st.names)
	slices.Sort(names)
	return st.spec + "\x00" + strings.Join(names, ",") + "\x00" + strconv.FormatBool(st.typeOnly)
}

// targetKey names a link's target and whether it imports types only.
func targetKey(to string, typeOnly bool) string {
	return to + "\x00" + strconv.FormatBool(typeOnly)
}

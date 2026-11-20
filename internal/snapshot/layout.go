package snapshot

import (
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// Layout is what the project's own standard files declare beside the shared rules. One question uses it for both
// sides.
type Layout struct {
	TestFolders []Declared // one per test-folder key found on either side, in the entries' order
	Roots       []Declared // one per root key found on either side, in the entries' order
	Shadowing   []string   // root files tracked now that are not read and that a tool reads before a declaring file
	// Own are the list keys of the repository's own file that hold a value now, in the order of the rules, and
	// OwnBase the default base it declares now, if any.
	Own     []Declared
	OwnBase []Declared
	// Planning are the planning keys of the repository's own file that hold a value now, in the order of the rules.
	Planning []Declared
	// Rules are the import-rule keys of the repository's own file that hold a value now, in the order of the rules;
	// their values in use are the rules as rebuilt.
	Rules []Declared
}

// Declared is one declared key and what became of its values.
type Declared struct {
	Language         string
	File, Table, Key string
	Tool             string
	Name             string           // what an answer calls the key of the own file; "" for a tool's key
	AsWritten        bool             // the values are file-name patterns, named as written, not folders
	Now, AtBase      KeyState         // the zero value: the side does not hold the key
	InUse            []DeclaredFolder // those of now first, in file order, each once
	Held             []HeldFolder     // repository folders that are declared and not used
	Unusable         int              // values that are absolute, leave the repository or hold a pattern
	ShadowedBy       string           // the file the tool reads instead, tracked now; then no value is looked at
	Unmatched        []string         // values that match no tracked path of either side, in file order
	Form             string           // what a usable value looks like, for a key whose values are no paths or patterns
}

// KeyState is what one side's file says about a declared key.
type KeyState int

const (
	// KeyRead is a key whose value was read.
	KeyRead KeyState = iota + 1
	// KeyUnreadable is a key found whose value is no array of strings and no string.
	KeyUnreadable
	// KeyFormNotRead is a key written as a dotted key or in an inline table, which the reader does not read.
	KeyFormNotRead
)

// DeclaredFolder is one declared folder in use.
type DeclaredFolder struct {
	Path     string // from the repository root; "." is the root itself
	BaseOnly bool   // declared at the base and not now
}

// HeldFolder is a declared folder that is not used, and why.
type HeldFolder struct {
	Path string
	Why  HeldReason
}

// HeldReason is why a declared test folder is not used.
type HeldReason int

const (
	// HeldRepositoryRoot is the repository root itself.
	HeldRepositoryRoot HeldReason = iota + 1
	// HeldPackageRoot is, or holds, a package root of a language entry.
	HeldPackageRoot
	// HeldTestFolders holds test folders of its own, so it is more than a test folder.
	HeldTestFolders
	// HeldEveryName is a file-name pattern made only of "*": it names every file.
	HeldEveryName
)

// trackedPaths are the paths tracked on either side, each once.
func trackedPaths(now, before map[string][]byte) []string {
	tracked := slices.Collect(maps.Keys(now))
	for p := range before {
		if _, inNow := now[p]; !inNow {
			tracked = append(tracked, p)
		}
	}
	return tracked
}

// declaredLayout reads what both sides declare and decides which folders are in use. now and before map each
// tracked path to its content, nil when it was not read.
func declaredLayout(rules conventions.Rules, now, before map[string][]byte, tracked []string) Layout {
	var layout Layout
	for _, language := range rules.Languages {
		for _, d := range language.Layout {
			for _, shadow := range d.Shadowed {
				if _, isTracked := now[shadow]; isTracked && !slices.Contains(layout.Shadowing, shadow) {
					layout.Shadowing = append(layout.Shadowing, shadow)
				}
			}
			nowValues, nowState := readDeclared(now[d.File], d)
			baseValues, baseState := readDeclared(before[d.File], d)
			if nowState == 0 && baseState == 0 {
				continue
			}
			record := Declared{
				Language: language.Name, File: d.File, Table: d.Table, Key: d.Key, Tool: d.Tool, Now: nowState, AtBase: baseState,
			}
			if i := slices.IndexFunc(d.Shadowed, func(p string) bool { _, isTracked := now[p]; return isTracked }); i >= 0 {
				record.ShadowedBy = d.Shadowed[i]
			} else {
				record.decide(rules, tracked, nowValues, baseValues, d.Means == conventions.TestFolders)
			}
			if d.Means == conventions.TestFolders {
				layout.TestFolders = append(layout.TestFolders, record)
			} else {
				layout.Roots = append(layout.Roots, record)
			}
		}
	}
	return layout
}

// decide sorts the values of both sides into folders in use, held folders and unusable values (design 2.3, 2.4).
func (d *Declared) decide(rules conventions.Rules, tracked, nowValues, baseValues []string, testFolders bool) {
	seen, unusable := map[string]bool{}, map[string]bool{}
	for i, value := range append(slices.Clone(nowValues), baseValues...) {
		if strings.TrimPrefix(strings.TrimSuffix(value, "/"), "./") == "" {
			continue
		}
		cleaned := path.Clean(value)
		if path.IsAbs(cleaned) || strings.ContainsAny(cleaned, "*?[~$") || leaves(cleaned) {
			unusable[cleaned] = true
			continue
		}
		if seen[cleaned] || !slices.ContainsFunc(tracked, func(p string) bool { _, in := under(p, cleaned); return in }) {
			continue
		}
		seen[cleaned] = true
		baseOnly := i >= len(nowValues)
		if testFolders {
			if why := heldBecause(rules, tracked, cleaned); why != 0 {
				d.Held = append(d.Held, HeldFolder{Path: cleaned, Why: why})
				continue
			}
		}
		d.InUse = append(d.InUse, DeclaredFolder{Path: cleaned, BaseOnly: baseOnly})
	}
	d.Unusable = len(unusable)
}

// heldBecause is why a declared test folder is not used, or 0 when it is used: a shared test folder name at the root
// is used as it is; the root, a package root of an entry or a folder above one, and a folder that holds test
// folders of its own are held.
// NOTE: a package whose tests are colocated only as test-named files, with no test folder, is taken whole as test
// code. Upgrade: hold a value that product code outside it imports.
func heldBecause(rules conventions.Rules, tracked []string, folder string) HeldReason {
	if why := heldAsRoot(rules, folder); why != 0 || sharedTestFolder(rules, folder) {
		return why
	}
	for _, p := range tracked {
		if rest, in := under(p, folder); in && inFolders(rest, rules.TestFolders) {
			return HeldTestFolders
		}
	}
	return 0
}

// heldAsRoot is why a declared test folder is held as a root: the repository root, or a folder that is or holds a
// package root of a language entry. A shared test-folder name is never held.
func heldAsRoot(rules conventions.Rules, folder string) HeldReason {
	if sharedTestFolder(rules, folder) {
		return 0
	}
	if folder == "." {
		return HeldRepositoryRoot
	}
	for _, language := range rules.Languages {
		for _, root := range language.Imports.Prefix.Roots {
			if _, holds := under(root, folder); root != "." && (root == folder || holds) {
				return HeldPackageRoot
			}
		}
	}
	return 0
}

// sharedTestFolder says whether a declared folder is a shared test-folder name at the root, used as it is.
func sharedTestFolder(rules conventions.Rules, folder string) bool {
	return !strings.Contains(folder, "/") && slices.Contains(rules.TestFolders, folder)
}

// ownLayout adds the keys of the repository's own file: one record per key that holds a value now. A value is held
// when it names the root or every file, unmatched when no tracked path of either side lies under it or bears a
// matching name, held when a test or ignored folder is or holds a package root, and in use otherwise. A planning
// folder is not held for holding code: its files become planning items, each of them shown.
// Unlike a tool's declaration (decide), the own file is plimsoll's: a value that matches nothing is named, a folder
// holding test folders of its own is not held, and only now's values apply. So the two resolve apart.
func ownLayout(layout Layout, rules conventions.Rules, now ownFile, tracked []string) Layout {
	for _, k := range rules.OwnKeys {
		values, read := now.values[k.Key]
		if !read || len(values)+now.unusable[k.Key] == 0 {
			continue
		}
		if k.Holds.ImportRule() { // import rules name no layout; importRules reads them
			continue
		}
		if k.Holds.Single() {
			layout.OwnBase = append(layout.OwnBase, Declared{
				File: rules.OwnFile, Table: k.Table, Key: k.Key, Name: k.Name, AsWritten: true, Now: KeyRead, InUse: []DeclaredFolder{{Path: values[0]}},
			})
			continue
		}
		pattern, planning := k.Holds.Pattern(), k.Holds.Planning()
		d := Declared{File: rules.OwnFile, Table: k.Table, Key: k.Key, Name: k.Name, AsWritten: pattern, Now: KeyRead, Unusable: now.unusable[k.Key]}
		for _, value := range values {
			var why HeldReason
			switch {
			case pattern:
				if strings.Trim(value, "*") == "" {
					why = HeldEveryName
				}
			case value == ".":
				why = HeldRepositoryRoot
			case !planning:
				why = heldAsRoot(rules, value)
			}
			switch {
			case why == HeldEveryName || why == HeldRepositoryRoot:
				d.Held = append(d.Held, HeldFolder{Path: value, Why: why})
			case !slices.ContainsFunc(tracked, func(p string) bool { return matches(pattern, planning, value, p) }):
				d.Unmatched = append(d.Unmatched, value)
			case why != 0:
				d.Held = append(d.Held, HeldFolder{Path: value, Why: why})
			default:
				d.InUse = append(d.InUse, DeclaredFolder{Path: value})
			}
		}
		if planning {
			layout.Planning = append(layout.Planning, d)
		} else {
			layout.Own = append(layout.Own, d)
		}
	}
	return layout
}

// matches says whether a tracked path lies under a declared folder or bears a declared file name; a planning file name
// matches without regard to case.
func matches(pattern, planning bool, value, filePath string) bool {
	switch {
	case pattern && planning:
		return nameMatchesFolded(value, filePath)
	case pattern:
		return nameMatches(value, filePath)
	}
	_, in := under(filePath, value)
	return in
}

// readDeclared reads one declared key from a file's content: its values, and what the file says about it.
// NOTE: a multi-line string, and a line inside one that starts like the key, are not read. Upgrade: a TOML reader.
func readDeclared(content []byte, d conventions.Declaration) ([]string, KeyState) {
	if content == nil {
		return nil, 0
	}
	text := string(content)
	if table, found := tomlTable(text, d.Table); found {
		if at := keyLine(d.Key).FindStringIndex(table); at != nil {
			return valueOf(table[at[1]:])
		}
	}
	if formNotRead(text, d.Table, d.Key) {
		return nil, KeyFormNotRead
	}
	return nil, 0
}

// withLayout is the rules of one question: a copy of the given value with the folders in use added. The given value
// is not altered.
func withLayout(rules conventions.Rules, layout Layout) conventions.Rules {
	rules.TestPaths, rules.TestNames = slices.Clone(rules.TestPaths), slices.Clone(rules.TestNames)
	rules.IgnoredPaths, rules.IgnoredNames = slices.Clone(rules.IgnoredPaths), slices.Clone(rules.IgnoredNames)
	rules.PlanningFolders, rules.PlanningNames = slices.Clone(rules.PlanningFolders), slices.Clone(rules.PlanningNames)
	for _, d := range layout.TestFolders {
		for _, value := range d.InUse {
			rules.TestPaths = appendNew(rules.TestPaths, value.Path)
		}
	}
	for _, d := range slices.Concat(layout.Own, layout.Planning) {
		var list *[]string
		switch ownValueOf(rules, d) {
		case conventions.TestFolderPaths:
			list = &rules.TestPaths
		case conventions.TestFileNames:
			list = &rules.TestNames
		case conventions.IgnoredFolderPaths:
			list = &rules.IgnoredPaths
		case conventions.IgnoredFileNames:
			list = &rules.IgnoredNames
		case conventions.PlanningFolderPaths:
			list = &rules.PlanningFolders
		case conventions.PlanningFileNames:
			list = &rules.PlanningNames
		case conventions.BaseName, conventions.NeverImportRules, conventions.OnlyImportRules: // no layout
			continue
		}
		for _, value := range d.InUse {
			*list = appendNew(*list, value.Path)
		}
	}
	rules.Languages = slices.Clone(rules.Languages)
	for i := range rules.Languages {
		prefix := &rules.Languages[i].Imports.Prefix
		prefix.Roots = slices.Clone(prefix.Roots)
		for _, d := range layout.Roots {
			for _, folder := range d.InUse {
				if d.Language == rules.Languages[i].Name && !slices.Contains(prefix.Roots, folder.Path) {
					prefix.Roots = append(prefix.Roots, folder.Path)
				}
			}
		}
	}
	return rules
}

// ownValueOf is what a key of the repository's own file holds, or 0 for a declaration of another file.
func ownValueOf(rules conventions.Rules, d Declared) conventions.OwnValue {
	if d.File != rules.OwnFile {
		return 0
	}
	for _, k := range rules.OwnKeys {
		if k.Table == d.Table && k.Key == d.Key {
			return k.Holds
		}
	}
	return 0
}

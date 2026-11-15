package snapshot

import (
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/juzhxng/plimsoll/internal/conventions"
)

// Layout is what the project's own standard files declare beside the shared rules. One question uses it for both
// sides.
type Layout struct {
	TestFolders []Declared // one per test-folder key found on either side, in the entries' order
	Roots       []Declared // one per root key found on either side, in the entries' order
	Shadowing   []string   // root files tracked now that are not read and that a tool reads before a declaring file
}

// Declared is one declared key and what became of its values.
type Declared struct {
	Language         string
	File, Table, Key string
	Tool             string
	Now, AtBase      KeyState         // the zero value: the side does not hold the key
	InUse            []DeclaredFolder // those of now first, in file order, each once
	Held             []HeldFolder     // repository folders that are declared and not used
	Unusable         int              // values that are absolute, leave the repository or hold a pattern
	ShadowedBy       string           // the file the tool reads instead, tracked now; then no value is looked at
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
)

// declaredLayout reads what both sides declare and decides which folders are in use. now and before map each
// tracked path to its content, nil when it was not read.
func declaredLayout(rules conventions.Rules, now, before map[string][]byte) Layout {
	var layout Layout
	tracked := slices.Collect(maps.Keys(now))
	for p := range before {
		if _, inNow := now[p]; !inNow {
			tracked = append(tracked, p)
		}
	}
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
	if !strings.Contains(folder, "/") && slices.Contains(rules.TestFolders, folder) {
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
	for _, p := range tracked {
		if rest, in := under(p, folder); in && inFolders(rest, rules.TestFolders) {
			return HeldTestFolders
		}
	}
	return 0
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
	rules.TestPaths = slices.Clone(rules.TestPaths)
	for _, d := range layout.TestFolders {
		for _, folder := range d.InUse {
			if !slices.Contains(rules.TestPaths, folder.Path) {
				rules.TestPaths = append(rules.TestPaths, folder.Path)
			}
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

package snapshot

import (
	"path"
	"slices"
	"strings"
)

// cargoDependencies reads the runtime dependencies of a Cargo.toml by text: the keys of [dependencies] and of the
// target tables ([target.<...>.dependencies]), and the name of a sub-table ([dependencies.<name>],
// [target.<...>.dependencies.<name>]). The dev, build and workspace tables add nothing. manifest is false when the
// file has neither a [package] table nor a dependency table: a workspace root that only lists members. readable is
// false when a line of a table of keys is no key.
// NOTE: the dependencies of workspace members are not counted, and a dependency renamed with package = "..." is
// listed by its key. Upgrade: read every tracked Cargo.toml.
func cargoDependencies(content []byte) (names []string, manifest, readable bool) {
	text := strings.TrimPrefix(string(content), byteOrderMark)
	names = []string{}
	inKeys, depth := false, 0
	for line := range strings.Lines(text) {
		if depth > 0 {
			depth += openedBrackets(line)
			continue
		}
		if header, isHeader := tomlHeader(line); isHeader {
			table, name := cargoTable(header)
			inKeys = table && name == ""
			manifest = manifest || table || header == "package"
			if name != "" {
				names = appendNew(names, name)
			}
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		depth += openedBrackets(line)
		if !inKeys {
			continue
		}
		key, ok := cargoKey(trimmed)
		if !ok {
			return nil, true, false
		}
		names = appendNew(names, key)
	}
	if !manifest {
		return nil, false, true
	}
	return names, true, true
}

// cargoTable says whether a header opens a runtime dependency table, and the dependency it names when it is a
// sub-table of one.
func cargoTable(header string) (table bool, name string) {
	parts := strings.Split(header, ".")
	last := len(parts) - 1
	isTarget := len(parts) >= 3 && parts[0] == "target"
	switch {
	case header == "dependencies" || isTarget && parts[last] == "dependencies":
		return true, ""
	case len(parts) == 2 && parts[0] == "dependencies", isTarget && len(parts) >= 4 && parts[last-1] == "dependencies":
		return true, parts[last]
	}
	return false, ""
}

// cargoKey reads the key a line assigns, bare or quoted, up to "=" or to its first "." (log.workspace = true is log).
func cargoKey(line string) (string, bool) {
	name, _, assigns := strings.Cut(line, "=")
	key, _, _ := strings.Cut(dottedName(name), ".")
	if !assigns || key == "" || strings.ContainsAny(key, " \t") {
		return "", false
	}
	return key, true
}

// openedBrackets counts the brackets and braces a line opens minus those it closes, outside strings and before a
// comment, so a value that goes on over lines is followed to its end.
func openedBrackets(line string) int {
	depth := 0
	for i := 0; i < len(line); i++ {
		switch c := line[i]; c {
		case '#':
			return depth
		case '[', '{':
			depth++
		case ']', '}':
			depth--
		case '"', '\'':
			for i++; i < len(line) && line[i] != c; i++ {
				if c == '"' && line[i] == '\\' {
					i++
				}
			}
		}
	}
	return depth
}

// cargoCrates reads what Rust's own imports name. Every tracked Cargo.toml with a package name names a crate: the
// name of its [lib] table, else the package name with "-" as "_"; the crate is own when src/lib.rs beside it is
// tracked, its library root. The mod lines of the side's Rust files make the module tree. mod lines and crate, self
// and super paths are followed without any manifest; a path that starts with an own crate's name needs one.
// NOTE: a file that no mod line declares is read as a crate root: one declared by a macro's output or under a path
// attribute, whose mod line is looked for under the module's name. Upgrade: read the attribute's file name.
// NOTE: a path whose first module is no file names the file it starts from, also when the module does not exist, so
// a deleted module that is still imported is not "not found". Upgrade: none without a parser.
// NOTE: a library root named by [lib] path, and a dependency under another name (package = "..."), are not read:
// such paths count as outside. Upgrade: read both keys.
// NOTE: in the 2015 edition "use a::B;" means crate::a::B; it counts as outside. Upgrade: read edition.
// NOTE: for a crate root under another name, "mod x;" is looked for in the folder of that name first: with
// src/bin/tool/x.rs beside src/bin/x.rs the first is taken, where the compiler takes the second. Upgrade: read the
// targets of the manifest.
func cargoCrates(manifest string, content map[string][]byte, tracked map[string]bool, readings map[string]reading) prefix {
	m := rustModules{tracked: tracked, crates: map[string][]string{}, parents: map[string][]string{}, declared: map[declaration]bool{}}
	state := PrefixAbsent
	for file := range tracked {
		if path.Base(file) != manifest || content[file] == nil {
			continue
		}
		name := tomlString(content[file], "package", "name")
		if name == "" {
			continue
		}
		state = PrefixRead
		if lib := tomlString(content[file], "lib", "name"); lib != "" {
			name = lib
		} else {
			name = strings.ReplaceAll(name, "-", "_")
		}
		if root := path.Join(path.Dir(file), "src/lib.rs"); tracked[root] {
			m.crates[name] = append(m.crates[name], root)
		}
	}
	for file, r := range readings {
		if !strings.HasSuffix(file, ".rs") {
			continue
		}
		for _, st := range r.statements {
			name, isMod := modName(st.spec)
			if !isMod {
				continue
			}
			m.declared[declaredAt(file, st.within, name)] = true
			if found := m.child(file, st.within, name); len(found) == 1 && found[0] != file {
				m.parents[found[0]] = appendNew(m.parents[found[0]], file)
			}
		}
	}
	for _, files := range m.crates {
		slices.Sort(files)
	}
	for _, files := range m.parents {
		slices.Sort(files)
	}
	return prefix{state: state, own: m.resolve}
}

// rustModules is one side's module tree, read from the mod lines of its Rust files.
type rustModules struct {
	tracked  map[string]bool
	crates   map[string][]string  // an own crate's name to its library roots, sorted
	parents  map[string][]string  // a module file to the files whose mod line names it alone, sorted
	declared map[declaration]bool // every mod line
}

// declaration is a mod line: its file, the inline modules it stands in joined by "/", and the module it declares.
type declaration struct{ file, within, name string }

// modName is the module a mod line's target declares.
func modName(spec string) (string, bool) {
	words := strings.Fields(spec)
	if len(words) != 2 || words[0] != "mod" {
		return "", false
	}
	return words[1], true
}

// declaredAt is the declaration of a module at a place.
func declaredAt(file string, within []string, name string) declaration {
	return declaration{file, strings.Join(within, "/"), name}
}

// resolve gives the tracked files an import names: nil for outside, and for "not found" one path that is not tracked.
func (m rustModules) resolve(from, spec string, within []string) ([]string, bool) {
	if name, isMod := modName(spec); isMod {
		if found := m.child(from, within, name); len(found) > 0 {
			return found, false
		}
		return []string{path.Join(m.folders(from, within)[0], name+".rs")}, false
	}
	written := strings.Fields(spec) // an alias ("as x") is cut
	if len(written) == 0 {
		return nil, false
	}
	names := slices.DeleteFunc(strings.Split(strings.TrimPrefix(written[0], "::"), "::"), func(n string) bool { return n == "" })
	for len(names) > 1 && (names[len(names)-1] == "*" || names[len(names)-1] == "self") {
		names = names[:len(names)-1]
	}
	if len(names) == 0 {
		return nil, false
	}
	var roots []string
	switch first := names[0]; {
	case first == "crate":
		roots = m.roots(from)
	case first == "self" || first == "super" || m.declared[declaredAt(from, within, first)]:
		return m.relative(from, names, within), false
	default:
		roots = m.crates[first]
	}
	var found []string
	for _, root := range roots {
		found = appendNew(found, m.longest([]string{path.Dir(root)}, names[1:], []string{root})...)
	}
	return found, false
}

// relative resolves a path from the statement's place: self, super, or a module that a mod line there declares. Each
// super first leaves the last inline module, then goes one module up, to the files that declare the importing file.
func (m rustModules) relative(from string, names, within []string) []string {
	rest := names
	if rest[0] == "self" {
		rest = rest[1:]
	}
	ups := 0
	for len(rest) > 0 && rest[0] == "super" {
		rest, ups = rest[1:], ups+1
	}
	inside := min(ups, len(within))
	within, ups = within[:len(within)-inside], ups-inside
	if ups == 0 {
		return m.longest(m.folders(from, within), rest, []string{from})
	}
	pastRoot := []string{path.Join(path.Dir(from), "super")}
	base, owners := path.Base(from), m.parents[from]
	if base == "lib.rs" || base == "main.rs" || len(owners) == 0 {
		return pastRoot
	}
	folder := path.Dir(from)
	if base == "mod.rs" {
		folder = path.Dir(folder)
	}
	for range ups - 1 {
		var above []string
		for _, owner := range owners {
			above = appendNew(above, m.parents[owner]...)
		}
		slices.Sort(above)
		owners, folder = above, path.Dir(folder)
	}
	if len(owners) == 0 {
		return pastRoot
	}
	return m.longest([]string{folder}, rest, owners)
}

// folders are the folders of a place, where its submodules lie: the file's module folders, each with the inline
// modules added. A lib.rs, main.rs or mod.rs holds its submodules in its own folder; any other x.rs in x/ first and
// in its own folder second.
func (m rustModules) folders(file string, within []string) []string {
	folder := path.Dir(file)
	own := []string{folder}
	if base := path.Base(file); base != "lib.rs" && base != "main.rs" && base != "mod.rs" {
		own = []string{path.Join(folder, strings.TrimSuffix(base, ".rs")), folder}
	}
	for i, f := range own {
		own[i] = path.Join(append([]string{f}, within...)...)
	}
	return own
}

// files are the tracked files of a module path: base.rs and base/mod.rs.
func (m rustModules) files(base string) []string {
	var found []string
	for _, candidate := range []string{base + ".rs", base + "/mod.rs"} {
		if m.tracked[candidate] {
			found = append(found, candidate)
		}
	}
	return found
}

// child are the files a mod line at a place names: x.rs and x/mod.rs in the first of the place's folders that holds one.
func (m rustModules) child(file string, within []string, name string) []string {
	for _, folder := range m.folders(file, within) {
		if found := m.files(path.Join(folder, name)); len(found) > 0 {
			return found
		}
	}
	return nil
}

// longest gives, for the first folder that holds one, the files of the longest leading run of names; when no folder
// holds a run, the owners: the path names an item of theirs.
func (m rustModules) longest(folders, names, owners []string) []string {
	for _, folder := range folders {
		for k := len(names); k > 0; k-- {
			if found := m.files(path.Join(append([]string{folder}, names[:k]...)...)); len(found) > 0 {
				return found
			}
		}
	}
	return owners
}

// roots are the crate roots of a file: the files without a parent that its parents lead up to.
func (m rustModules) roots(file string) []string {
	seen, frontier := map[string]bool{file: true}, []string{file}
	var roots []string
	for len(frontier) > 0 {
		f := frontier[len(frontier)-1]
		frontier = frontier[:len(frontier)-1]
		if len(m.parents[f]) == 0 {
			roots = append(roots, f)
		}
		for _, parent := range m.parents[f] {
			if !seen[parent] {
				seen[parent] = true
				frontier = append(frontier, parent)
			}
		}
	}
	slices.Sort(roots)
	return roots
}

// appendNew appends the items the list does not hold yet, each once.
func appendNew(list []string, items ...string) []string {
	for _, item := range items {
		if !slices.Contains(list, item) {
			list = append(list, item)
		}
	}
	return list
}

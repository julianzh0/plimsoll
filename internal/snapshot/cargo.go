package snapshot

import (
	"cmp"
	"maps"
	"path"
	"slices"
	"strings"
)

// cargoCrates reads what Rust's own imports name. Every tracked Cargo.toml with a package name is a package: a file
// belongs to the nearest one at or above it. A package's crate roots are its manifest's targets and the ones Cargo
// discovers; its library root is [lib] path, else src/lib.rs. In a package, a dependency with a path to a tracked
// crate is that crate, under the name code uses for it; any other first name is outside, except in the 2015 edition,
// where it is read from the crate root. The mod lines of the language's files, a path attribute on one included, make
// the module tree; a path is followed along it, and is "not found" where a file read whole binds no such name.
func cargoCrates(manifest string, content map[string][]byte, tracked map[string]bool, readings map[string]reading) prefix {
	documents := map[string]map[string]any{} // a manifest's folder to its document; a file that is no TOML is none
	for file := range tracked {
		if path.Base(file) == manifest && content[file] != nil {
			if document, err := decodeTOML(content[file]); err == nil {
				documents[path.Dir(file)] = document
			}
		}
	}
	m := rustModules{
		tracked: tracked, readings: readings, packages: map[string]*rustPackage{}, roots: map[string]bool{},
		parents: map[string][]modPlace{}, declared: map[declaration]bool{}, modPaths: map[declaration]string{},
		exported: map[string]bool{}, bound: map[string]map[string]map[string]bool{}, externs: map[string][]string{},
	}
	state := PrefixAbsent
	for folder, document := range documents {
		if tomlText(document, "package", "name") != "" {
			state = PrefixRead
			m.packages[folder] = newRustPackage(folder, document, documents, tracked, m.roots)
		}
	}
	for _, p := range m.packages {
		p.deps = m.dependencies(documents[p.folder], p.folder, documents)
	}
	for file := range tracked {
		if !strings.HasSuffix(file, ".rs") {
			continue
		}
		if base := path.Base(file); base == "lib.rs" || base == "main.rs" {
			m.roots[file] = true
		} else if p := m.packageOf(file); p != nil && p.discovers(strings.TrimPrefix(file, p.folder+"/")) {
			m.roots[file] = true
		}
	}
	for file, r := range readings {
		for _, name := range r.exports {
			m.exported[name] = true
		}
		m.bound[file] = boundNames(r)
		m.externs[file] = slices.Clip(r.externAliases) // the reading is shared between questions: an append copies
		for _, st := range r.statements {
			if name, isMod := keyworded(st.spec, "mod"); isMod {
				at := declaredAt(file, st.within, name)
				m.declared[at] = true
				if st.modPath != "" {
					m.modPaths[at] = st.modPath
				}
			} else if name, isExtern := keyworded(st.spec, "crate"); isExtern {
				m.externs[file] = append(m.externs[file], name)
			}
		}
	}
	for file, r := range readings {
		for _, st := range r.statements {
			if name, isMod := keyworded(st.spec, "mod"); isMod {
				if found := m.child(file, st.within, name); len(found) == 1 && found[0] != file {
					m.parents[found[0]] = append(m.parents[found[0]], modPlace{file, st.within})
				}
			}
		}
	}
	for _, places := range m.parents {
		slices.SortFunc(places, func(a, b modPlace) int { return cmp.Compare(a.file, b.file) })
	}
	return prefix{state: state, own: m.resolve}
}

// boundNames are, per module place of a file, the names it binds: its items, and the alias or last segment of each
// use, extern crate and mod line there; a raw identifier as its word. A place with a glob use, a macro call or an
// active attribute holds "*": it may bind any name.
func boundNames(r reading) map[string]map[string]bool {
	bound := map[string]map[string]bool{}
	add := func(place, name string) {
		if bound[place] == nil {
			bound[place] = map[string]bool{}
		}
		bound[place][strings.TrimPrefix(name, "r#")] = true
	}
	for place, names := range r.bindings {
		for _, name := range names {
			add(place, name)
		}
	}
	for place := range r.opaque {
		add(place, "*")
	}
	for _, st := range r.statements {
		if name := boundBy(st.spec); name != "" {
			add(strings.Join(st.within, "/"), name)
		}
	}
	return bound
}

// boundBy is the name a statement binds: its alias or last segment ("*" for a glob), the crate of an extern crate line,
// the module of a mod line; "" for none.
func boundBy(spec string) string {
	words := strings.Fields(spec)
	switch {
	case len(words) == 0:
		return ""
	case len(words) >= 3 && words[len(words)-2] == "as":
		return words[len(words)-1]
	case len(words) == 2:
		return words[1] // mod x, crate x
	}
	segments := strings.Split(words[0], "::")
	last := segments[len(segments)-1]
	if last == "self" && len(segments) > 1 {
		last = segments[len(segments)-2]
	}
	return last
}

// rustPackage is one tracked Cargo.toml with a package name.
type rustPackage struct {
	folder      string
	lib         string // its library root, when tracked
	libName     string // the name code uses for its library
	edition2015 bool   // stated, absent, or inherited as such
	deps        map[string][]string
	auto        map[string]bool // the target folders Cargo discovers roots in: src/bin, examples, tests, benches
}

// newRustPackage reads a package's manifest: its library, its edition and its targets, which it adds to roots.
func newRustPackage(folder string, document map[string]any, documents map[string]map[string]any, tracked map[string]bool, roots map[string]bool) *rustPackage {
	p := &rustPackage{folder: folder, auto: map[string]bool{}}
	p.libName = cmp.Or(tomlText(document, "lib", "name"), strings.ReplaceAll(tomlText(document, "package", "name"), "-", "_"))
	add := func(file string) string {
		if file = path.Join(folder, file); tracked[file] {
			roots[file] = true
			return file
		}
		return ""
	}
	p.lib = add(cmp.Or(tomlText(document, "lib", "path"), "src/lib.rs"))
	edition := tomlText(document, "package", "edition")
	if inherits, _ := tomlAt(document, "package", "edition", "workspace"); inherits == true {
		_, workspace := cargoWorkspaceRoot(folder, documents)
		edition = tomlText(workspace, "workspace", "package", "edition")
	}
	p.edition2015 = edition == "" || edition == "2015"
	for _, kind := range []string{"bin", "example", "test", "bench"} {
		targets, _ := tomlAt(document, kind)
		list, _ := targets.([]any)
		for _, target := range list {
			if table, isTable := target.(map[string]any); isTable && tomlText(table, "path") != "" {
				add(tomlText(table, "path"))
			}
		}
	}
	switch build, stated := tomlAt(document, "package", "build"); {
	case !stated || build == true:
		add("build.rs")
	case build != false:
		add(tomlText(document, "package", "build")) // a path; build = false is no build script
	}
	for folder, key := range map[string]string{"src/bin": "autobins", "examples": "autoexamples", "tests": "autotests", "benches": "autobenches"} {
		value, stated := tomlAt(document, "package", key)
		p.auto[folder] = !stated || value != false
	}
	return p
}

// discovers says whether Cargo takes a file, by its path in the package, as a crate root of its own: src/main.rs,
// and x.rs or x/main.rs under src/bin, examples, tests and benches, unless the manifest turns that off.
func (p *rustPackage) discovers(file string) bool {
	parts := strings.Split(file, "/")
	if file == "src/main.rs" {
		return p.auto["src/bin"]
	}
	if len(parts) > 2 && parts[0] == "src" && parts[1] == "bin" {
		parts = append([]string{"src/bin"}, parts[2:]...)
	}
	return p.auto[parts[0]] && (len(parts) == 2 || len(parts) == 3 && parts[2] == "main.rs")
}

// cargoWorkspaceRoot is the folder and document of the nearest manifest at or above a folder with a [workspace]
// table; the folder itself and nil for none.
func cargoWorkspaceRoot(folder string, documents map[string]map[string]any) (string, map[string]any) {
	for f := range foldersUp(folder) {
		if document := documents[f]; document["workspace"] != nil {
			return f, document
		}
	}
	return folder, nil
}

// dependencies are a package's dependencies with a path to a tracked crate, by the name code uses for each: the key
// with "-" as "_" when it is renamed (package = "..."), else the crate's library name. An entry with workspace = true
// takes its path from the workspace's entry. Every dependency table counts: build, dev and target ones too; a name
// that two tables give different crates names both.
func (m rustModules) dependencies(document map[string]any, folder string, documents map[string]map[string]any) map[string][]string {
	var tables []map[string]any
	targets := tomlTableAt(document, "target")
	for _, kind := range []string{"dependencies", "dev-dependencies", "build-dependencies"} {
		tables = append(tables, tomlTableAt(document, kind))
		for _, platform := range slices.Sorted(maps.Keys(targets)) {
			tables = append(tables, tomlTableAt(targets, platform, kind))
		}
	}
	workspaceFolder, workspace := cargoWorkspaceRoot(folder, documents)
	found := map[string][]string{}
	for _, table := range tables {
		for key := range table {
			from, entry := folder, tomlTableAt(table, key)
			if inherits, _ := tomlAt(entry, "workspace"); inherits == true && workspace != nil {
				from, entry = workspaceFolder, tomlTableAt(workspace, "workspace", "dependencies", key)
			}
			crate := m.packages[path.Join(from, tomlText(entry, "path"))]
			if tomlText(entry, "path") == "" || crate == nil || crate.lib == "" {
				continue
			}
			name := crate.libName
			if tomlText(table, key, "package") != "" || tomlText(entry, "package") != "" {
				name = strings.ReplaceAll(key, "-", "_")
			}
			found[name] = appendNew(found[name], crate.lib)
		}
	}
	for _, libs := range found {
		slices.Sort(libs)
	}
	return found
}

// rustModules is one side's module tree, read from the mod lines of its Rust files.
type rustModules struct {
	tracked  map[string]bool
	readings map[string]reading
	packages map[string]*rustPackage // by folder
	roots    map[string]bool         // every crate root a manifest names or Cargo discovers, and every lib.rs and main.rs
	parents  map[string][]modPlace   // a module file to the places whose mod line names it alone, by file
	declared map[declaration]bool    // every mod line
	modPaths map[declaration]string  // the file a path attribute names for a mod line
	exported map[string]bool         // the macros a crate exports at its root
	bound    map[string]map[string]map[string]bool
	externs  map[string][]string // per file, the crates and aliases its extern crate lines name
}

// modPlace is a place in a file where modules are declared: the file and the inline modules around it.
type modPlace struct {
	file   string
	within []string
}

// declaration is a mod line: its file, the inline modules it stands in joined by "/", and the module it declares.
type declaration struct{ file, within, name string }

// keyworded is the name a target of a keyword and a name names ("mod x", "crate x"), a raw identifier as its word.
func keyworded(spec, keyword string) (string, bool) {
	words := strings.Fields(spec)
	if len(words) != 2 || words[0] != keyword {
		return "", false
	}
	return strings.TrimPrefix(words[1], "r#"), true
}

// declaredAt is the declaration of a module at a place.
func declaredAt(file string, within []string, name string) declaration {
	return declaration{file, strings.Join(within, "/"), name}
}

// packageOf is the package a file belongs to: the nearest one at or above it.
func (m rustModules) packageOf(file string) *rustPackage {
	for folder := range foldersUp(path.Dir(file)) {
		if p := m.packages[folder]; p != nil {
			return p
		}
	}
	return nil
}

// builtinCrates are the crates every Rust crate can name without a manifest entry.
func builtinCrates() []string { return []string{"std", "core", "alloc", "proc_macro", "test"} }

// resolve gives the tracked files an import names: nil for outside, and for "not found" one path that is not tracked.
func (m rustModules) resolve(from, spec string, within []string) ([]string, bool) {
	if name, isMod := keyworded(spec, "mod"); isMod {
		return m.follow(modPlace{from, within}, []string{name}), false
	}
	if name, isExtern := keyworded(spec, "crate"); isExtern {
		spec = name
	}
	written := strings.Fields(spec) // an alias ("as x") is cut
	if len(written) == 0 {
		return nil, false
	}
	names := slices.DeleteFunc(strings.Split(strings.TrimPrefix(written[0], "::"), "::"), func(n string) bool { return n == "" })
	for i, n := range names { // a raw identifier names the module of its word
		names[i] = strings.TrimPrefix(n, "r#")
	}
	for len(names) > 1 && (names[len(names)-1] == "*" || names[len(names)-1] == "self") {
		names = names[:len(names)-1]
	}
	if len(names) == 0 {
		return nil, false
	}
	first, p := names[0], m.packageOf(from)
	switch {
	case first == "crate":
		return m.fromRoots(m.rootsOf(from), names[1:]), false
	case first == "self" || first == "super" || m.declared[declaredAt(from, within, first)]:
		return m.relative(from, names, within), false
	case p == nil:
		return nil, false
	case len(p.deps[first]) > 0:
		return m.fromRoots(p.deps[first], names[1:]), false
	case first == p.libName && p.lib != "":
		return m.follow(modPlace{file: p.lib}, names[1:]), false
	case p.edition2015 && !slices.Contains(builtinCrates(), first) && !m.externCrate(from, first):
		return m.fromRoots(m.rootsOf(from), names), false
	}
	return nil, false
}

// externCrate says whether an extern crate line of a file or of its crate roots names a crate, or an alias for one.
func (m rustModules) externCrate(from, name string) bool {
	for _, file := range append([]string{from}, m.rootsOf(from)...) {
		if slices.Contains(m.externs[file], name) {
			return true
		}
	}
	return false
}

// fromRoots follows a path from each crate root.
func (m rustModules) fromRoots(roots, names []string) []string {
	var found []string
	for _, root := range roots {
		found = appendNew(found, m.follow(modPlace{file: root}, names)...)
	}
	return found
}

// relative resolves a path from the statement's place: self, super, or a module that a mod line there declares. Each
// super leaves the last inline module, or goes to the places that declare the file; past a crate root it is not
// found.
func (m rustModules) relative(from string, names, within []string) []string {
	rest := names
	if rest[0] == "self" {
		rest = rest[1:]
	}
	places := []modPlace{{from, within}}
	for len(rest) > 0 && rest[0] == "super" {
		rest = rest[1:]
		var up []modPlace
		for _, place := range places {
			switch {
			case len(place.within) > 0:
				up = append(up, modPlace{place.file, place.within[:len(place.within)-1]})
			case len(m.parents[place.file]) > 0 && !m.roots[place.file]:
				up = append(up, m.parents[place.file]...)
			}
		}
		if len(up) == 0 {
			return []string{path.Join(path.Dir(from), "super")}
		}
		places = up
	}
	var found []string
	for _, place := range places {
		found = appendNew(found, m.follow(place, rest)...)
	}
	return found
}

// follow resolves a path's names from a module place, along the module tree: an inline module or a mod line there
// moves on, and so does a module file in the place's folders that no mod line declares (one a macro may, or a
// folder without a module file of its own); at the first name that is none of these, the path names the place's
// file, or is not found (an untracked path) when that file was read whole and binds no such name there. A path that
// ends at a module names its file.
func (m rustModules) follow(place modPlace, names []string) []string {
	for i := 0; i < len(names); i++ {
		name := names[i]
		if m.declared[declaredAt(place.file, place.within, name)] {
			files := m.child(place.file, place.within, name)
			if len(files) != 1 {
				if len(files) == 0 {
					return []string{path.Join(m.folders(place.file, place.within)[0], name+".rs")}
				}
				return files
			}
			place = modPlace{file: files[0]}
			continue
		}
		if inside, inline := m.readings[place.file].inlines[strings.Join(place.within, "/")][name]; inline {
			place.within = append(slices.Clone(place.within), inside)
			continue
		}
		if files, k := m.moduleFiles(place, names[i:]); len(files) > 0 {
			if len(files) > 1 {
				return files
			}
			place, i = modPlace{file: files[0]}, i+k-1
			continue
		}
		if m.unbound(place, name) {
			return []string{path.Join(m.folders(place.file, place.within)[0], name)}
		}
		return []string{place.file}
	}
	return []string{place.file}
}

// moduleFiles are the files of the longest leading run of names that is a module path in the first of a place's
// folders that holds one, and the length of the run; a folder without a module file of its own is passed through.
func (m rustModules) moduleFiles(place modPlace, names []string) ([]string, int) {
	for _, folder := range m.folders(place.file, place.within) {
		for k := len(names); k > 0; k-- {
			if found := m.files(path.Join(append([]string{folder}, names[:k]...)...)); len(found) > 0 {
				return found, k
			}
		}
	}
	return nil, 0
}

// unbound says whether a place surely binds no such name: its file was read whole and belongs to the module tree (it
// is a crate root, or a mod line declares it; a file only an include or a conditional path names is not), nothing at
// the place may bring names in unseen, and no item, use leaf, extern crate or exported macro has it.
func (m rustModules) unbound(place modPlace, name string) bool {
	r, read := m.readings[place.file]
	if !read || !r.parsed || r.syntaxError || m.exported[name] || !m.roots[place.file] && len(m.parents[place.file]) == 0 {
		return false
	}
	bound := m.bound[place.file][strings.Join(place.within, "/")]
	return !bound["*"] && !bound[name]
}

// folders are the folders of a place, where its submodules lie, each with the inline modules added: a crate root or a
// mod.rs holds them in its own folder; any other x.rs in x/. A crate root that is also mounted as a module is followed
// in both.
func (m rustModules) folders(file string, within []string) []string {
	folder, base := path.Dir(file), path.Base(file)
	sub := path.Join(folder, strings.TrimSuffix(base, ".rs"))
	own := []string{sub, folder}
	if base == "mod.rs" {
		own = []string{folder}
	} else if m.roots[file] {
		own = []string{folder}
		if len(m.parents[file]) > 0 {
			own = append(own, sub)
		}
	}
	for i, f := range own {
		for _, step := range within {
			f = path.Join(f, stepFolder(step))
		}
		own[i] = f
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

// child are the files a mod line at a place names: the file its path attribute names, beside the declaring file
// outside inline modules and under their folders inside them; else x.rs and x/mod.rs in the first of the place's
// folders that holds one.
func (m rustModules) child(file string, within []string, name string) []string {
	if named := m.modPaths[declaredAt(file, within, name)]; named != "" {
		at := path.Join(path.Dir(file), named)
		if len(within) > 0 {
			at = path.Join(m.folders(file, within)[0], named)
		}
		if m.tracked[at] {
			return []string{at}
		}
		return nil
	}
	found, _ := m.moduleFiles(modPlace{file, within}, []string{name})
	return found
}

// rootsOf are the crate roots of a file: the roots and the files without a parent that its parents lead up to.
func (m rustModules) rootsOf(file string) []string {
	seen, frontier := map[string]bool{file: true}, []string{file}
	var roots []string
	for len(frontier) > 0 {
		f := frontier[len(frontier)-1]
		frontier = frontier[:len(frontier)-1]
		if len(m.parents[f]) == 0 || m.roots[f] {
			roots = append(roots, f)
		}
		for _, parent := range m.parents[f] {
			if !seen[parent.file] {
				seen[parent.file] = true
				frontier = append(frontier, parent.file)
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

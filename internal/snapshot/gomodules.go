package snapshot

import (
	"maps"
	"path"
	"slices"
	"strings"
)

// goModuleFile is what the reader needs of one go.mod or go.work: its folder, its module path (go.mod), the modules it
// requires with their versions (go.mod), the folders it uses (go.work), and its replacements by path, each with the
// version it is limited to ("" for every version) and its target.
type goModuleFile struct {
	folder, module string
	requires       map[string]string
	uses           []string
	replaces       []goReplace
}

// goReplace is one replacement: a local one names a folder (target), any other another module, which no folder of
// the repository stands for.
type goReplace struct {
	path, version, target string
	local                 bool
}

// goModules reads every tracked go.mod and go.work. A file belongs to the module of the nearest go.mod at or above it;
// an import under that module's path is the folder under the module's folder, unless a go.mod nearer to that folder
// makes it a module of its own. An import under another tracked module's path is own when the nearest go.work at or
// above the importer uses both modules, or when the replacement that applies names that module's folder: inside such
// a workspace the go.work's, else the importer's go.mod's, only for a module the importer requires (every version, or
// the one required). Any other import is outside.
func goModules(content map[string][]byte, tracked map[string]bool) prefix {
	modules, works := map[string]goModuleFile{}, map[string]goModuleFile{} // by folder
	state := PrefixAbsent
	for file := range tracked {
		switch path.Base(file) {
		case "go.mod":
			if content[file] == nil {
				state = PrefixUnreadable
				continue
			}
			if m := readGoModFile(path.Dir(file), content[file]); m.module != "" {
				modules[m.folder] = m
			} else {
				state = PrefixUnreadable
			}
		case "go.work":
			works[path.Dir(file)] = readGoModFile(path.Dir(file), content[file])
		}
	}
	if len(modules) == 0 {
		return prefix{state: state}
	}
	nearest := func(files map[string]goModuleFile, folder string) (goModuleFile, bool) {
		for f := range foldersUp(folder) {
			if m, found := files[f]; found {
				return m, true
			}
		}
		return goModuleFile{}, false
	}
	folders := slices.Sorted(maps.Keys(modules))
	return prefix{state: PrefixRead, own: func(from, spec string, _ []string) ([]string, bool) {
		importer, inModule := nearest(modules, path.Dir(from))
		if !inModule {
			return nil, false
		}
		work, inWork := nearest(works, path.Dir(from))
		inWork = inWork && slices.Contains(work.uses, importer.folder)
		reaches := func(m goModuleFile) bool {
			return m.folder == importer.folder || inWork && slices.Contains(work.uses, m.folder) || replacedBy(importer, work, inWork, m)
		}
		// The tracked module with the longest path the import lies under; of several with that path (copies, fixtures),
		// the importer's own, else the first in folder order the importer reaches.
		var target goModuleFile
		rest := ""
		for _, folder := range folders {
			m := modules[folder]
			r, under := strings.CutPrefix(spec, m.module)
			if !under || r != "" && r[0] != '/' || len(m.module) < len(target.module) {
				continue
			}
			if len(m.module) == len(target.module) && (target.folder == importer.folder || reaches(target) || !reaches(m)) {
				continue
			}
			target, rest = m, strings.TrimPrefix(r, "/")
		}
		if target.module == "" || !reaches(target) {
			return nil, false
		}
		folder := path.Join(target.folder, rest)
		if owner, _ := nearest(modules, folder); owner.folder != target.folder {
			return nil, false // a module of its own lies in between
		}
		return []string{folder}, false
	}}
}

// replacedBy says whether the replacement that applies to a module the importer requires names that module's folder:
// the workspace's, inside one, before the importer's own.
func replacedBy(importer, work goModuleFile, inWork bool, target goModuleFile) bool {
	version, required := importer.requires[target.module]
	if !required {
		return false
	}
	applies := func(file goModuleFile) (goReplace, bool) {
		for _, r := range file.replaces {
			if r.path == target.module && (r.version == "" || r.version == version) {
				return r, true
			}
		}
		return goReplace{}, false
	}
	r, found := goReplace{}, false
	if inWork {
		r, found = applies(work)
	}
	if !found {
		r, found = applies(importer)
	}
	return found && r.local && r.target == target.folder
}

// readGoModFile reads the module, require, use and replace directives of a go.mod or go.work, single and block form;
// a replacement's local target is resolved against the file's folder.
func readGoModFile(folder string, content []byte) goModuleFile {
	m := goModuleFile{folder: folder, requires: map[string]string{}}
	block := ""
	for line := range strings.Lines(string(content)) {
		line, _, _ = strings.Cut(line, "//")
		words := strings.Fields(line)
		switch {
		case len(words) == 0:
			continue
		case block != "" && words[0] == ")":
			block = ""
			continue
		case block == "" && len(words) >= 2 && words[1] == "(":
			if words[len(words)-1] != ")" { // "require ( )" opens no block
				block = words[0]
			}
			continue
		case block == "":
			m.directive(words[0], words[1:])
			continue
		}
		m.directive(block, words)
	}
	return m
}

// directive reads one directive's words.
func (m *goModuleFile) directive(name string, words []string) {
	unquote := func(s string) string { return strings.Trim(s, "\"`") }
	switch {
	case name == "module" && len(words) > 0:
		m.module = unquote(words[0])
	case name == "require" && len(words) > 0:
		m.requires[unquote(words[0])] = ""
		if len(words) > 1 {
			m.requires[unquote(words[0])] = words[1]
		}
	case name == "use" && len(words) > 0:
		m.uses = append(m.uses, path.Join(m.folder, unquote(words[0])))
	case name == "replace":
		at := slices.Index(words, "=>")
		if at < 1 || at+1 >= len(words) {
			return
		}
		target := strings.ReplaceAll(unquote(words[at+1]), `\`, "/") // .\x and ..\x are local folders too
		r := goReplace{path: unquote(words[0])}
		if r.local = strings.HasPrefix(target, "./") || strings.HasPrefix(target, "../") || target == "." || target == ".."; r.local {
			r.target = path.Join(m.folder, target)
		}
		if at == 2 {
			r.version = words[1]
		}
		m.replaces = append(m.replaces, r)
	}
}

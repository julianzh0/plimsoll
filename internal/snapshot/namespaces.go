package snapshot

import (
	"path"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// declaredNamespaces finds the namespaces that the language's tracked files declare: a namespace is the files that
// declare it, wherever they lie, a block inside another's declaring the joined name, as the grammar reads them; a
// file the grammar cannot read declares what its namespace lines say. An import names exactly one namespace; an
// alias (using N = A.B) names A.B when it is declared, else A. One that no tracked file declares is outside when it
// only holds declared namespaces; it is the project's own, and not found, when it begins with the name of a tracked
// project file; else it is outside. RootNamespace in a project file names embedded resources, not code: not read.
// A using line under an inactive #if is read, as every branch is: the build's defines decide which is taken.
func declaredNamespaces(language conventions.Language, content map[string][]byte, tracked map[string]bool, readings map[string]reading) prefix {
	imports := language.Imports
	declared := imports.PackageLine.SubexpIndex("package")
	files := map[string][]string{}  // a namespace to the files that declare it
	projects := map[string]string{} // a project file's name without its suffix to the first such file by path
	for file := range tracked {
		if matched, _ := path.Match(imports.Prefix.Projects, path.Base(file)); matched {
			name := strings.TrimSuffix(path.Base(file), conventions.Suffix(file))
			if first, seen := projects[name]; !seen || file < first {
				projects[name] = file
			}
		}
		if !slices.Contains(language.Extensions, conventions.Suffix(file)) {
			continue
		}
		names := readings[file].namespaces
		if r, read := readings[file]; !read || !r.parsed || r.syntaxError {
			names = nil
			for _, m := range imports.PackageLine.FindAllSubmatch(content[file], -1) {
				names = append(names, string(m[declared]))
			}
		}
		for _, name := range names {
			name = strings.ReplaceAll(strings.Join(strings.Fields(name), ""), "@", "")
			files[name] = append(files[name], file)
		}
	}
	if len(files) == 0 {
		return prefix{state: PrefixAbsent}
	}
	parents := map[string]bool{} // every name that holds a declared namespace
	for name := range files {
		slices.Sort(files[name])
		files[name] = slices.Compact(files[name])
		for i, r := range name {
			if r == '.' {
				parents[name[:i]] = true
			}
		}
	}
	return prefix{state: PrefixRead, own: func(_, spec string, _ []string) ([]string, bool) {
		name := strings.ReplaceAll(spec, "@", "")
		if aliased, isAlias := strings.CutPrefix(name, "= "); isAlias {
			aliased, _, _ = strings.Cut(aliased, "<") // a generic type's arguments
			name = aliased
			if found := files[name]; len(found) == 0 && strings.Contains(name, ".") {
				name = name[:strings.LastIndexByte(name, '.')] // a type: its namespace
			}
		}
		if found := files[name]; len(found) > 0 {
			return found, false
		}
		if parents[name] {
			return nil, false // it holds namespaces and no file of its own
		}
		for i := range len(name) + 1 { // the name and each of its leading dotted parts
			if i < len(name) && name[i] != '.' {
				continue
			}
			if project, own := projects[name[:i]]; own {
				return []string{project}, false // the project file is no folder, so the import is not found
			}
		}
		return nil, false
	}}
}

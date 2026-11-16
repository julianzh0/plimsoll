package snapshot

import (
	"path"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// declaredNamespaces finds the namespaces that the language's tracked files declare: a namespace is all folders
// whose files declare it, wherever they lie. An import names exactly one namespace. One that no tracked file
// declares is outside when it only holds declared namespaces; it is the project's own, and not found, when it
// begins with the name of a tracked project file; else it is outside.
// NOTE: a folder whose files declare several namespaces is reached whole. Upgrade: reach only the declaring files.
// NOTE: a namespace declared inside another's block is not read, and a root namespace set in a project file is not
// used. Upgrade: join a nested name to its enclosing one; read RootNamespace.
// NOTE: a using line under an inactive #if is read: every branch is. Upgrade: evaluate the build's defines.
func declaredNamespaces(language conventions.Language, content map[string][]byte, tracked map[string]bool) prefix {
	imports := language.Imports
	declared := imports.PackageLine.SubexpIndex("package")
	folders := map[string][]string{} // a namespace to its folders
	projects := map[string]string{}  // a project file's name without its suffix to the first such file by path
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
		for _, m := range imports.PackageLine.FindAllSubmatch(content[file], -1) {
			name := strings.ReplaceAll(string(m[declared]), "@", "")
			folders[name] = append(folders[name], path.Dir(file))
		}
	}
	if len(folders) == 0 {
		return prefix{state: PrefixAbsent}
	}
	parents := map[string]bool{} // every name that holds a declared namespace
	for name := range folders {
		slices.Sort(folders[name])
		folders[name] = slices.Compact(folders[name])
		for i, r := range name {
			if r == '.' {
				parents[name[:i]] = true
			}
		}
	}
	return prefix{state: PrefixRead, own: func(_, spec string, _ []string) ([]string, bool) {
		name := strings.ReplaceAll(spec, "@", "")
		if found := folders[name]; len(found) > 0 {
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

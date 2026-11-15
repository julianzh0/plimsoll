package snapshot

import (
	"path"
	"slices"
	"strings"

	"github.com/juzhxng/plimsoll/internal/conventions"
)

// declaredNamespaces finds the namespaces that the language's tracked files declare: a namespace is all folders
// whose files declare it, wherever they lie. An import names exactly one namespace. One that no tracked file
// declares is outside when it only holds declared namespaces; it is the project's own, and not found, when it
// begins with the name of a tracked project file; else it is outside.
// NOTE: a folder whose files declare several namespaces is reached whole. Upgrade: reach only the declaring files.
// NOTE: a namespace declared inside another's block is not read, and a root namespace set in a project file is not
// used. Upgrade: join a nested name to its enclosing one; read RootNamespace.
// NOTE: a using line inside a multi-line string or under an inactive #if is read. Upgrade: a parser.
func declaredNamespaces(language conventions.Language, content map[string][]byte, tracked map[string]bool) prefix {
	imports := language.Imports
	declared := imports.PackageLine.SubexpIndex("package")
	folders := map[string][]string{} // a namespace to its folders
	var projects []string            // the tracked project files
	for file := range tracked {
		if matched, _ := path.Match(imports.Prefix.Projects, path.Base(file)); matched {
			projects = append(projects, file)
		}
		if !slices.Contains(language.Extensions, conventions.Suffix(file)) {
			continue
		}
		for _, m := range imports.PackageLine.FindAllSubmatch(content[file], -1) {
			name := strings.ReplaceAll(string(m[declared]), "@", "")
			if folder := path.Dir(file); !slices.Contains(folders[name], folder) {
				folders[name] = append(folders[name], folder)
			}
		}
	}
	if len(folders) == 0 {
		return prefix{state: PrefixAbsent}
	}
	parents := map[string]bool{} // every name that holds a declared namespace
	for name := range folders {
		slices.Sort(folders[name])
		for i, r := range name {
			if r == '.' {
				parents[name[:i]] = true
			}
		}
	}
	slices.Sort(projects)
	return prefix{state: PrefixRead, own: func(_, spec string, _ []string) ([]string, bool) {
		name := strings.ReplaceAll(spec, "@", "")
		if found := folders[name]; len(found) > 0 {
			return found, false
		}
		if parents[name] {
			return nil, false // it holds namespaces and no file of its own
		}
		for _, project := range projects {
			root := strings.TrimSuffix(path.Base(project), conventions.Suffix(project))
			if name == root || strings.HasPrefix(name, root+".") {
				return []string{project}, false // the project file is no folder, so the import is not found
			}
		}
		return nil, false
	}}
}

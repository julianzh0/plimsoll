package snapshot

import (
	"path"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/juzhxng/plimsoll/internal/conventions"
)

// sourcePackages finds the packages that the files under the language's source roots declare: a package is all
// folders whose files declare it. An import names a member of the longest leading run of its names that is a
// package; when the rest goes on with a lower-case name, it names a package below that holds no tracked file.
// NOTE: a package that lies in several modules is reached in each, so a link may name a module the import does not
// use. Upgrade: narrow to the folders that hold a file named after the imported class, or read the modules'
// dependencies.
// NOTE: a file under a test source set reaches the test source sets of every module. Upgrade: only its own module's,
// once testFixtures can be told apart.
// NOTE: an import line inside a multi-line string is read as an import. Upgrade: stop at the first declaration.
func sourcePackages(rules conventions.Rules, language conventions.Language, content map[string][]byte, tracked map[string]bool) prefix {
	imports := language.Imports
	extensions := slices.Concat(language.Extensions, imports.FolderAlso)
	folders := map[string][]string{} // a package, as a path, to its folders
	for file := range tracked {
		if !slices.Contains(extensions, conventions.Suffix(file)) || !underSourceRoot(imports.Prefix.Roots, file) {
			continue
		}
		m := imports.PackageLine.FindSubmatch(content[file])
		if m == nil {
			continue
		}
		name := strings.ReplaceAll(strings.ReplaceAll(string(m[imports.PackageLine.SubexpIndex("package")]), "`", ""), ".", "/")
		if folder := path.Dir(file); !slices.Contains(folders[name], folder) {
			folders[name] = append(folders[name], folder)
		}
	}
	if len(folders) == 0 {
		return prefix{state: PrefixAbsent}
	}
	for name := range folders {
		slices.Sort(folders[name])
	}
	return prefix{state: PrefixRead, own: func(from, spec string, _ []string) ([]string, bool) {
		names := strings.Split(strings.ReplaceAll(spec, "`", ""), ".")
		for n := len(names); n > 0; n-- {
			found := folders[strings.Join(names[:n], "/")]
			if len(found) == 0 {
				continue
			}
			if rest := names[n:]; len(rest) > 1 && startsLower(rest[0]) {
				var below []string
				for _, folder := range found {
					below = append(below, path.Join(folder, rest[0]))
				}
				return below, false // a package below that holds no tracked source file: not found
			}
			fromTests := underTestSourceSet(rules, path.Dir(from))
			return slices.DeleteFunc(slices.Clone(found), func(folder string) bool {
				return !fromTests && underTestSourceSet(rules, folder)
			}), false // none usable: outside
		}
		return nil, false
	}}
}

// underSourceRoot says whether a file lies below a folder that one of the root patterns matches, at any depth.
func underSourceRoot(roots []string, file string) bool {
	folders := strings.Split(path.Dir(file), "/")
	for _, root := range roots {
		parts := strings.Split(root, "/")
		for start := 0; start+len(parts) <= len(folders); start++ {
			if matched, _ := path.Match(root, strings.Join(folders[start:start+len(parts)], "/")); matched {
				return true
			}
		}
	}
	return false
}

// startsLower says whether a name begins with a lower-case letter.
func startsLower(name string) bool {
	r, _ := utf8.DecodeRuneInString(name)
	return unicode.IsLower(r)
}

package snapshot

import (
	"path"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// sourcePackages finds the packages that the files under the language's source roots declare: a package is all
// folders whose files declare it. An import names a member of the longest leading run of its names that is a
// package; when the rest goes on with a lower-case name, it names a package below that holds no tracked file. Of the
// package's folders an importing file sees (testSees), the import reaches the files that declare its next name at
// the top level, when every file of those folders was read whole; else all of them. A wildcard reaches all.
func sourcePackages(rules conventions.Rules, language conventions.Language, content map[string][]byte, tracked map[string]bool, readings map[string]reading) prefix {
	imports := language.Imports
	extensions := slices.Concat(language.Extensions, imports.FolderAlso)
	folders := map[string][]string{}  // a package, as a path, to its folders
	inFolder := map[string][]string{} // a folder to its files of the entry or its other extensions
	for file := range tracked {
		if !slices.Contains(extensions, conventions.Suffix(file)) || !underSourceRoot(imports.Prefix.Roots, file) {
			continue
		}
		inFolder[path.Dir(file)] = append(inFolder[path.Dir(file)], file)
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
	sees := testSees(content, tracked)
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
			visible := slices.DeleteFunc(slices.Clone(found), func(folder string) bool {
				return underTestSourceSet(rules, folder) && (!fromTests || !sees(path.Dir(from), folder))
			}) // none usable: outside
			if rest := names[n:]; len(rest) > 0 && rest[0] != "*" {
				return declaring(visible, inFolder, readings, rest[0]), false
			}
			return visible, false
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

// declaring narrows a package's folders to the files that declare a name at the top level, when every file of them
// was read whole and at least one declares it; else it keeps the folders.
func declaring(folders []string, inFolder map[string][]string, readings map[string]reading, name string) []string {
	var found []string
	for _, folder := range folders {
		for _, file := range inFolder[folder] {
			r, read := readings[file]
			if !read || !r.parsed || r.syntaxError {
				return folders
			}
			if slices.Contains(r.bindings[""], name) {
				found = append(found, file)
			}
		}
	}
	if len(found) == 0 {
		return folders
	}
	slices.Sort(found)
	return found
}

// testSees says whether a folder under a test source set sees a test folder: one of its own module (the folder that
// holds its src), another module's testFixtures when its module's Gradle build file asks for them, and another
// module's tests when its module's pom depends on them as a test-jar.
func testSees(content map[string][]byte, tracked map[string]bool) func(from, folder string) bool {
	byPath, byAccessor := conventions.JVMTestDependencies()
	modules := map[string]bool{} // the folders that hold a build file
	for file := range tracked {
		if base := path.Base(file); base == "build.gradle" || base == "build.gradle.kts" || base == "pom.xml" {
			modules[moduleFolder(path.Dir(file))] = true
		}
	}
	fixtures := map[string][]string{} // a module to the modules whose test fixtures it uses
	testJars := map[string][]string{} // a module to the modules whose tests it uses
	coordinates := map[string]string{}
	jars := map[string][]string{}
	for file := range tracked {
		module := moduleFolder(path.Dir(file))
		switch path.Base(file) {
		case "build.gradle", "build.gradle.kts":
			for _, m := range byPath.FindAllSubmatch(content[file], -1) {
				fixtures[module] = append(fixtures[module], strings.ReplaceAll(string(m[1]), ":", "/"))
			}
			for _, m := range byAccessor.FindAllSubmatch(content[file], -1) {
				if folder, unique := accessorFolder(string(m[1]), modules); unique {
					fixtures[module] = append(fixtures[module], folder)
				}
			}
		case "pom.xml":
			coordinate, uses := pomTestJars(content[file])
			coordinates[coordinate], jars[module] = module, uses
		}
	}
	for module, uses := range jars {
		for _, c := range uses {
			if other, tracked := coordinates[c]; tracked {
				testJars[module] = append(testJars[module], other)
			}
		}
	}
	return func(from, folder string) bool {
		mine, theirs := moduleOf(from), moduleOf(folder)
		return mine == theirs || slices.Contains(testJars[mine], theirs) ||
			slices.Contains(fixtures[mine], theirs) && sourceSetOf(folder) == "testFixtures"
	}
}

// moduleOf is the module of a folder under a source set: the folder that holds its src, "" at the root.
func moduleOf(folder string) string {
	if strings.HasPrefix(folder, "src/") {
		return ""
	}
	before, _, _ := strings.Cut(folder, "/src/")
	return before
}

// sourceSetOf is the source set a folder lies in: the folder after src.
func sourceSetOf(folder string) string {
	_, after, _ := strings.Cut("/"+folder, "/src/")
	set, _, _ := strings.Cut(after, "/")
	return set
}

// moduleFolder is a build file's folder as a module, "" at the root.
func moduleFolder(folder string) string {
	if folder == "." {
		return ""
	}
	return folder
}

// accessorFolder is the module folder a Gradle type-safe accessor (a.bCd) names: the one whose segments match its
// parts, compared without case, dashes and underscores; found only when exactly one does.
func accessorFolder(accessor string, modules map[string]bool) (string, bool) {
	plain := func(s string) string { return strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(s)) }
	want := strings.Split(accessor, ".")
	var found []string
	for module := range modules {
		segments := strings.Split(module, "/")
		if len(segments) != len(want) {
			continue
		}
		match := true
		for i := range segments {
			match = match && plain(segments[i]) == plain(want[i])
		}
		if match {
			found = append(found, module)
		}
	}
	if len(found) != 1 {
		return "", false
	}
	return found[0], true
}

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
	folders := map[string][]string{}        // a package, as a path, to its folders
	members := map[packageFolder][]string{} // the files of a package in one folder
	for file := range tracked {
		if !slices.Contains(extensions, conventions.Suffix(file)) || !underSourceRoot(imports.Prefix.Roots, file) {
			continue
		}
		m := imports.PackageLine.FindSubmatch(content[file])
		if m == nil {
			continue
		}
		name := strings.ReplaceAll(strings.ReplaceAll(string(m[imports.PackageLine.SubexpIndex("package")]), "`", ""), ".", "/")
		folder := path.Dir(file)
		if !slices.Contains(folders[name], folder) {
			folders[name] = append(folders[name], folder)
		}
		members[packageFolder{name, folder}] = append(members[packageFolder{name, folder}], file)
	}
	declared := declaredIn(members, readings)
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
				return declaring(strings.Join(names[:n], "/"), visible, declared, rest[0]), false
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

// packageFolder is one folder of a package.
type packageFolder struct{ name, folder string }

// declaredIn are, per folder of a package whose files were all read whole, the files that declare each top-level
// name; a folder with a file not read whole is absent.
func declaredIn(members map[packageFolder][]string, readings map[string]reading) map[packageFolder]map[string][]string {
	found := map[packageFolder]map[string][]string{}
next:
	for at, files := range members {
		names := map[string][]string{}
		for _, file := range files {
			r, read := readings[file]
			if !read || !r.parsed || r.syntaxError {
				continue next
			}
			for _, name := range r.bindings[""] {
				names[name] = append(names[name], file)
			}
		}
		found[at] = names
	}
	return found
}

// declaring narrows a package's folders to the files that declare a name at the top level, when every file of them
// was read whole and at least one declares it; else it keeps the folders.
func declaring(pkg string, folders []string, declared map[packageFolder]map[string][]string, name string) []string {
	var found []string
	for _, folder := range folders {
		names, whole := declared[packageFolder{pkg, folder}]
		if !whole {
			return folders
		}
		found = append(found, names[name]...)
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
				fixtures[module] = append(fixtures[module], strings.ReplaceAll(strings.TrimPrefix(string(m[1]), ":"), ":", "/"))
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
		mine, _ := sourceSet(from)
		theirs, set := sourceSet(folder)
		return mine == theirs || slices.Contains(testJars[mine], theirs) ||
			slices.Contains(fixtures[mine], theirs) && set == "testFixtures"
	}
}

// sourceSet are the module and the source set of a folder under a source root: the last src/<set>/<java or kotlin>
// in its path, the module being the folder that holds that src ("" at the root).
func sourceSet(folder string) (module, set string) {
	parts := strings.Split(folder, "/")
	for i := len(parts) - 2; i >= 1; i-- {
		if parts[i-1] == "src" && (parts[i+1] == "java" || parts[i+1] == "kotlin") {
			return strings.Join(parts[:i-1], "/"), parts[i]
		}
	}
	return "", ""
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

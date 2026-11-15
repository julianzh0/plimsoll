package conventions

import "regexp"

// javaLanguage is the entry for Java. Its imports name a package: the folders, under the source roots, whose files
// declare it.
func javaLanguage() Language {
	return Language{
		Name: "java", Extensions: []string{".java"}, DynamicRead: jvmDynamicRead,
		Imports: jvmImports([]string{".kt", ".kts"}, []string{"package-info.java", "module-info.java"}),
	}
}

var (
	// jvmDynamicRead is a settings read whose name is not a quoted literal.
	jvmDynamicRead = regexp.MustCompile(`System\.getenv\([ \t]*[^" \t)]`)
	// jvmImport is an import of Java or Kotlin: plain, static, a wildcard, Kotlin's alias and backticks.
	jvmImport = regexp.MustCompile("(?m)^[ \\t]*import[ \\t]+(?:static[ \\t]+)?(?P<spec>(?:[A-Za-z_$][\\w$]*|`[^`\\n]+`)(?:\\.(?:[A-Za-z_$][\\w$]*|`[^`\\n]+`|\\*))*)")
	// jvmPackage is the package a file declares, its first statement.
	jvmPackage = regexp.MustCompile("(?m)^[ \\t]*package[ \\t]+(?P<package>(?:[A-Za-z_$][\\w$]*|`[^`\\n]+`)(?:\\.(?:[A-Za-z_$][\\w$]*|`[^`\\n]+`))*)")
)

// jvmImports is what the Java and Kotlin entries share: also names the other entry's extensions that share their
// package folders, implicit the files that run without an import of them.
func jvmImports(also, implicit []string) Imports {
	return Imports{
		Statements:  []ImportForm{{Pattern: jvmImport}},
		Relative:    NoRelative,
		Prefix:      Prefix{Roots: []string{"src/*/java", "src/*/kotlin"}, Format: SourceRoots, Required: true},
		Target:      FolderTarget,
		FolderAlso:  also,
		PackageLine: jvmPackage,
		NoRings:     "an import names a package, not a file",
		Implicit:    implicit,
	}
}

package conventions

import "regexp"

// javaLanguage is the entry for Java. Its imports name a package: the folders, under the source roots, whose files
// declare it.
func javaLanguage() Language {
	return Language{
		Name: "java", Extensions: []string{".java"}, Grammar: "java", Query: javaQuery, Declarations: javaDeclarations,
		Imports: jvmImports([]string{".kt", ".kts"}, []string{"package-info.java", "module-info.java"}),
	}
}

// javaQuery reads an import (plain, static or a wildcard, the wildcard part of its target; an import of a module is
// none of the repository's code).
const javaQuery = `
((import_declaration [(identifier) (scoped_identifier)] @spec (asterisk)? @spec) @import
  (#not-match? @import "^import[ \t]+module[ \t]"))
`

// jvmPackage is the package a file declares, its first statement. It stays a pattern: it is also read from generated
// files, which are not code files and are not parsed.
var jvmPackage = regexp.MustCompile("(?m)^[ \\t]*package[ \\t]+(?P<package>(?:[A-Za-z_$][\\w$]*|`[^`\\n]+`)(?:\\.(?:[A-Za-z_$][\\w$]*|`[^`\\n]+`))*)")

// jvmImports is what the Java and Kotlin entries share: also names the other entry's extensions that share their
// package folders, implicit the files that run without an import of them.
func jvmImports(also, implicit []string) Imports {
	return Imports{
		Relative:    NoRelative,
		Prefix:      Prefix{Roots: []string{"src/*/java", "src/*/kotlin"}, Format: SourceRoots, Required: true},
		Target:      FolderTarget,
		FolderAlso:  also,
		PackageLine: jvmPackage,
		NoRings:     "an import names a package, not a file",
		Implicit:    implicit,
	}
}

// javaDeclarations reads the top-level names a file declares, for the list of code a change added that nothing uses: run
// only on the files a change names.
const javaDeclarations = `
(program [(class_declaration name: (identifier) @declared.type) (interface_declaration name: (identifier) @declared.type)
  (enum_declaration name: (identifier) @declared.type) (record_declaration name: (identifier) @declared.type)
  (annotation_type_declaration name: (identifier) @declared.type)])
(program [(class_declaration (modifiers [(marker_annotation) (annotation)]) name: (identifier) @implicit)
  (interface_declaration (modifiers [(marker_annotation) (annotation)]) name: (identifier) @implicit)
  (record_declaration (modifiers [(marker_annotation) (annotation)]) name: (identifier) @implicit)])
`

package conventions

import "regexp"

// cLanguage is the entry for C. Its includes are read as C++'s are; a header named .h belongs to the one of the two
// entries that has more files of its own in the repository.
func cLanguage() Language {
	return cFamily("c", []string{".c", ".h"}, []string{"*.c"})
}

var (
	// cDynamicRead is a settings read whose name is not a quoted literal.
	cDynamicRead = regexp.MustCompile(`getenv\([ \t]*[^" \t)]`)
	// cComments are a line comment and a block comment.
	// NOTE: a "/*" inside a string or a character literal starts a comment for this reader, and an include line
	// inside a raw string is read. Upgrade: blank strings too, with the include directive's own quotes kept.
	cComments = regexp.MustCompile(`//[^\n]*|/\*(?:[^*]|\*+[^*/])*\*+/`)
	// cIncludeQuoted is an include in quotes: looked for beside the including file first, then in the include folders.
	// NOTE: an include under #if is read whatever its condition, so the includes of every platform and option count.
	// Upgrade: evaluate the conditions for the flags a build file names.
	cIncludeQuoted = regexp.MustCompile(`(?m)^[ \t]*#[ \t]*include[ \t]*"(?P<spec>[^"\n]+)"`)
	// cIncludeAngled is an include in angle brackets: looked for in the include folders only. The target keeps its
	// brackets, so the two forms are told apart.
	cIncludeAngled = regexp.MustCompile(`(?m)^[ \t]*#[ \t]*include[ \t]*(?P<spec><[^>\n]+>)`)
	// cIncludeComputed is an include whose target a macro names.
	cIncludeComputed = regexp.MustCompile(`(?m)^[ \t]*#[ \t]*include[ \t]+[A-Za-z_]\w*`)
)

// cFamily is what the C and C++ entries share: units are the file names the compiler takes as a unit of their own,
// which no file includes.
func cFamily(name string, extensions, units []string) Language {
	return Language{
		Name: name, Extensions: extensions, DynamicRead: cDynamicRead, Literals: cComments,
		Imports: Imports{
			Statements: []ImportForm{{Pattern: cIncludeQuoted}, {Pattern: cIncludeAngled}},
			Computed:   ImportForm{Pattern: cIncludeComputed},
			Relative:   NoRelative,
			Prefix:     Prefix{File: "CMakeLists.txt", Members: true, Format: IncludeFolders, Folders: []string{"include"}},
			Target:     FileTarget,
			Implicit:   units,
			NotSeen: "an include reached only through an include folder that the build alone knows (a variable in a CMake file, " +
				"another build system): it counts as not found when a tracked file ends in its path, else as outside; C++ modules (`import x;`)",
		},
	}
}

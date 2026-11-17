package conventions

import "regexp"

// cLanguage is the entry for C. Its includes are read as C++'s are; a header named .h belongs to the one of the two
// entries that has more files of its own in the repository.
func cLanguage() Language {
	return cFamily("c", []string{".c", ".h"}, []string{"*.c"})
}

var (
	// cComments are a string, a character literal, a line comment and a block comment; the strings are matched so a
	// comment marker inside one starts no comment.
	// NOTE: an include line inside a raw string is read. Upgrade: match raw strings too.
	cComments = regexp.MustCompile(`"(?:[^"\\\n]|\\.)*"|'(?:[^'\\\n]|\\.)*'|//[^\n]*|/\*(?:[^*]|\*+[^*/])*\*+/`)
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
	query := cQuery + cDeclarations
	if name == "cpp" {
		query += cppDeclarations
	}
	return Language{
		Name: name, Extensions: extensions, Literals: cComments,
		Grammar: name, Query: query, LineReadWhenBroken: true,
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

// cQuery reads the includes of a file the grammar reads without an error: a quoted target is unquoted, an angled one
// keeps its brackets, and an include a macro names is built at run time. A define whose value is a directive is a misread the grammar reports no error for (a define with no value and a
// trailing space takes the next directive line as its value), so it counts as one.
const cQuery = `
(preproc_include path: [(string_literal) (system_lib_string)] @spec) @import
(preproc_include path: (identifier) @computed)
(comment) @blank
((preproc_def value: (preproc_arg) @error) (#match? @error "^#[ \t]*[a-z]"))
`

// cDeclarations reads the functions a file defines and the structs, enums and unions it declares with a body, at
// the top level; a type that only a typedef names is no declaration of its own, and a method defined outside its
// class has a qualified name.
const cDeclarations = `
(translation_unit (function_definition declarator: [(function_declarator declarator: (identifier) @declared.function)
  (pointer_declarator declarator: (function_declarator declarator: (identifier) @declared.function))]))
(translation_unit [(struct_specifier name: (type_identifier) @declared.type body: (_))
  (enum_specifier name: (type_identifier) @declared.type body: (_)) (union_specifier name: (type_identifier) @declared.type body: (_))])
(translation_unit (declaration type: [(struct_specifier name: (type_identifier) @declared.type body: (_))
  (enum_specifier name: (type_identifier) @declared.type body: (_)) (union_specifier name: (type_identifier) @declared.type body: (_))]))
`

// cppDeclarations reads, besides, the classes, and what a namespace holds.
const cppDeclarations = `
(translation_unit (class_specifier name: (type_identifier) @declared.type body: (_)))
(namespace_definition body: (declaration_list (function_definition declarator: (function_declarator declarator: (identifier) @declared.function))))
(namespace_definition body: (declaration_list [(struct_specifier name: (type_identifier) @declared.type body: (_))
  (class_specifier name: (type_identifier) @declared.type body: (_)) (enum_specifier name: (type_identifier) @declared.type body: (_))]))
`

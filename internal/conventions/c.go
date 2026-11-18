package conventions

import (
	"regexp"
	"slices"
)

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
	declarations, unitQuery, functions, macros, literals := cDeclarations, cUnits, cFunctions(), cMacros(), doubleQuoted+"|"+character
	if name == "cpp" {
		declarations, unitQuery = declarations+cppDeclarations, unitQuery+cppUnits
		functions, macros, literals = cppFunctions(), cppMacros(), cppRawString+"|"+literals
	}
	return Language{
		Name: name, Extensions: extensions, Literals: cComments, Functions: functions, Macros: macros, Tokens: cLikeTokens(`(?:u8|[uUL])?(?:` + literals + `)`),
		Grammar: name, Query: cQuery, Declarations: declarations, Units: unitQuery, Peers: HeaderPeers, LineReadWhenBroken: true,
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

// cFunctions is C's function table: an else sits in an else_clause, a goto counts 1 without nesting, and a function's
// name lies in its innermost declarator (int *f() names f under a pointer_declarator).
func cFunctions() FunctionKinds {
	return FunctionKinds{
		Declared:     []string{"function_definition"},
		Bodies:       []string{"compound_statement"},
		Ifs:          []string{"if_statement"},
		ElseClauses:  []string{"else_clause"},
		FlatBranches: []string{"goto_statement"},
		Nesting:      []string{"for_statement", "while_statement", "do_statement", "switch_statement", "conditional_expression"},
		Logic:        []string{"&&", "||"},
		Parentheses:  []string{"parenthesized_expression"},
		DepthIfs:     []string{"if_statement"},
		Depth:        []string{"for_statement", "while_statement", "do_statement", "switch_statement"},
		Blocks:       []string{"compound_statement", "case_statement"},
		SwitchBlocks: []string{"case_statement"},
		Declarators:  []string{"function_declarator", "pointer_declarator"},
		MisreadIn:    []string{"compound_statement"},
	}
}

// cppFunctions is C++'s: C's, with lambdas, range loops, try and catch, and names through references, destructors and
// operators.
func cppFunctions() FunctionKinds {
	k := cFunctions()
	k.Closures = []string{"lambda_expression"}
	k.NamedClosures, k.NamedBy = []string{"lambda_expression"}, []string{"init_declarator"}
	k.Nesting = append(k.Nesting, "for_range_loop", "catch_clause")
	k.Depth = append(k.Depth, "for_range_loop", "try_statement")
	k.Declarators = append(k.Declarators, "reference_declarator")
	k.Names = []string{"destructor_name", "operator_name"}
	return k
}

// cLoopMacro is a loop macro read as while.
func cLoopMacro() MacroRule {
	return MacroRule{Pattern: regexp.MustCompile(`\b(?P<macro>[A-Z][A-Z0-9_]*FOR_?EACH[A-Z0-9_]*)\s*\(`), As: "while"}
}

// NOTE: C++ raw strings are matched with an empty delimiter only (R"(...)"), since Go's regexp has no back-reference;
// one with a delimiter is split into ordinary tokens. Upgrade: a small scanner for C++ literals.
const cppRawString = `R"\([\s\S]*?\)"`

// cMacros is the macro uses the C grammar cannot read, in the order they are rewritten, each in one pass:
//   - a run of attribute-like names in capitals with an underscore, whole, before another word, where a declaration
//     starts: at a line's start, after a specifier, or after a template's closing bracket (FMT_CONSTEXPR void f());
//   - an annotation with underscores around its name, and its arguments (_In_, _Out_opt_, _In_reads_(n)), and
//     __declspec(...);
//   - a loop macro, *_FOREACH or *_FOR_EACH, read as while: its arguments are one condition (LIST_FOREACH(x, l, link));
//   - GLib's cleanup attributes and types, type-definition and attribute macros, and assertions that take an operator.
//
// NOTE: conventions, not rules of the language; a capitalised type or constant written where an attribute stands is
// blanked as well, which only the functions that could not be read as written ever see. Other frameworks' macros are
// not rewritten. Upgrade: learn the names from the repository's own #define lines.
func cMacros() []MacroRule {
	return slices.Concat(cDeclarationMacros(), []MacroRule{cLoopMacro()}, glibMacros())
}

// cppMacros is C's, with Qt's and Boost's: foreach (decl, list) read as a range for, before the loop macros that
// would take Q_FOREACH; emit before a call; the class, property and d-pointer macros; signal and slot sections.
func cppMacros() []MacroRule {
	return slices.Concat(cDeclarationMacros(), []MacroRule{
		{Pattern: regexp.MustCompile(`\b(?:foreach|Q_FOREACH|BOOST_FOREACH)\s*\((?:[^,()<>]|\([^()]*\)|<(?:[^<>]|<[^<>]*>)*>)*(?P<macro>,)`), As: ":"},
		{Pattern: regexp.MustCompile(`\b(?P<macro>foreach|Q_FOREACH|BOOST_FOREACH)\s*\(`), As: "for"},
		cLoopMacro(),
		{Pattern: regexp.MustCompile(`\b(?P<macro>emit|Q_EMIT)\s+[A-Za-z_]`)},
		{Pattern: regexp.MustCompile(`(?m)^[ \t]*(?P<macro>Q_(?:OBJECT|GADGET|[A-Z_]+\s*\((?:[^()]|\([^()]*\))*\))\s*;?)[ \t]*$`)},
		{Pattern: regexp.MustCompile(`\b(?:public|protected|private)[ \t]+(?P<macro>slots|Q_SLOTS)[ \t]*:`)},
		{Pattern: regexp.MustCompile(`(?m)^[ \t]*(?P<macro>signals|Q_SIGNALS)[ \t]*:`), As: "public"},
	}, glibMacros())
}

func cDeclarationMacros() []MacroRule {
	return []MacroRule{
		{Pattern: regexp.MustCompile(`(?m)(?:^[ \t]*|\b(?:static|inline|constexpr|virtual|explicit|const|extern|friend)[ \t]+|>[ \t]*)(?P<macro>(?:[A-Z][A-Z0-9]*_[A-Z0-9_]+\b[ \t]*\n?[ \t]*)+)[A-Za-z_~]`)},
		{Pattern: regexp.MustCompile(`(?:^|[^\w])(?P<macro>(?:__declspec|_[A-Z][A-Za-z0-9]*_(?:[A-Za-z0-9]+_)*\b)(?:\s*\((?:[^()]|\([^()]*\))*\))?)`)},
	}
}

func glibMacros() []MacroRule {
	return []MacroRule{
		{Pattern: regexp.MustCompile(`\b(?P<macro>g_autofree)\b`)},
		{Pattern: regexp.MustCompile(`\b(?P<macro>g_auto(?:ptr|list|slist)?\s*\([^()]*\))`), As: "void*"},
		{Pattern: regexp.MustCompile(`(?m)^[ \t]*(?P<macro>G_(?:DEFINE|DECLARE)_\w+\s*\((?:[^()]|\([^()]*\))*\))[ \t]*;?`)},
		{Pattern: regexp.MustCompile(`\b(?P<macro>G_GNUC_\w+(?:\s*\([^()]*\))?|G_BEGIN_DECLS|G_END_DECLS)`)},
		{Pattern: regexp.MustCompile(`\b(?P<macro>g_assert_cmp\w+\s*\((?:[^()]|\([^()]*\))*\)\s*;)`)},
	}
}

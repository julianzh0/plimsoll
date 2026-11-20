package conventions

import "regexp"

// csharpLanguage is the entry for C#. A using names a namespace: the folders whose files declare it.
func csharpLanguage() Language {
	return Language{
		Name: "csharp", Functions: csharpFunctions(), Tokens: cLikeTokens(doubleQuoted+"|"+character, CSharpStrings), Extensions: []string{".cs"}, Imports: csharpImports(), Grammar: "csharp", Query: csharpQuery, Declarations: csharpDeclarations,
		Units: csharpUnits, Peers: PackagePeers,
		// ASP.NET loads the application class of Global.asax.
		Loaded: []Convention{{File: "Global.asax.cs"}},
		// A project is a published library unless it says it is not packable, builds a program, or uses an
		// application SDK.
		Library: &Library{Manifest: "*.csproj", Not: regexp.MustCompile(
			`(?i)<IsPackable>\s*false|<OutputType>\s*(?:Exe|WinExe)\s*<|Sdk="Microsoft\.NET\.Sdk\.(?:Web|Worker|BlazorWebAssembly)"`)},
	}
}

// csharpQuery reads using directives: one that names a namespace, and a using static or a using alias, which names a
// type whose namespace is the part before its last name; global:: is no part of the name, and a global using is
// written once for every file of its project.
// NOTE: an alias of a namespace (using N = A.B;) is read as the type B of A. Upgrade: try the whole name first.
const csharpQuery = `
((using_directive "global"? @everywhere !name [(identifier) (qualified_name) (alias_qualified_name)] @spec) @import
  (#not-match? @import "^(global[ \t]+)?using[ \t]+static[ \t]") (#strip-prefix! @spec "global::"))
((using_directive "global"? @everywhere "static" (qualified_name qualifier: (_) @spec)) @import
  (#strip-prefix! @spec "global::"))
((using_directive "global"? @everywhere name: (identifier) (qualified_name qualifier: (_) @spec)) @import
  (#strip-prefix! @spec "global::"))
`

// dotnetProject is the file-name pattern of a .NET project file: a manifest of dependencies, and a name of the
// project's own code.
const dotnetProject = "*.csproj"

// The parts of the C# forms: a name, with the @ that lets a keyword be one, and a dotted name.
const (
	csName   = `@?[A-Za-z_]\w*`
	csDotted = csName + `(?:\.` + csName + `)*`
)

var (
	// csNamespace is a namespace declaration at the first column, file-scoped or the head of a block.
	csNamespace = regexp.MustCompile(`(?m)^namespace[ \t]+(?P<package>` + csDotted + `)`)
)

func csharpImports() Imports {
	return Imports{
		Relative:    NoRelative,
		Prefix:      Prefix{Format: DeclaredNamespaces, Projects: dotnetProject, Required: true},
		Target:      FolderTarget,
		PackageLine: csNamespace,
		NoRings:     "a using names a namespace, not a file",
		NotSeen: "use of the same namespace or of a parent namespace, which needs no `using`, and the files that a `global using` serves: " +
			"one that names the repository's own code counts as not readable and gives no link",
		Implicit: []string{"Program.cs"},
	}
}

// csharpDeclarations reads the top-level names a file declares, for the list of code a change added that nothing uses: run
// only on the files a change names.
const csharpDeclarations = `
(compilation_unit [(class_declaration name: (identifier) @declared.type) (interface_declaration name: (identifier) @declared.type)
  (enum_declaration name: (identifier) @declared.type) (record_declaration name: (identifier) @declared.type)
  (struct_declaration name: (identifier) @declared.type)])
(namespace_declaration body: (declaration_list [(class_declaration name: (identifier) @declared.type) (interface_declaration name: (identifier) @declared.type)
  (enum_declaration name: (identifier) @declared.type) (record_declaration name: (identifier) @declared.type)
  (struct_declaration name: (identifier) @declared.type)]))
(class_declaration name: (identifier) @implicit body: (declaration_list (method_declaration parameters: (parameter_list (parameter (modifier) @_m (#eq? @_m "this"))))))
((compilation_unit [(class_declaration (modifier) @_p name: (identifier) @exported) (interface_declaration (modifier) @_p name: (identifier) @exported)
  (enum_declaration (modifier) @_p name: (identifier) @exported) (record_declaration (modifier) @_p name: (identifier) @exported)
  (struct_declaration (modifier) @_p name: (identifier) @exported)]) (#eq? @_p "public"))
((namespace_declaration body: (declaration_list [(class_declaration (modifier) @_p name: (identifier) @exported) (interface_declaration (modifier) @_p name: (identifier) @exported)
  (enum_declaration (modifier) @_p name: (identifier) @exported) (record_declaration (modifier) @_p name: (identifier) @exported)
  (struct_declaration (modifier) @_p name: (identifier) @exported)])) (#eq? @_p "public"))
`

// csharpFunctions names what a function is and what counts in it, in the grammar's node kinds.
func csharpFunctions() FunctionKinds {
	return FunctionKinds{
		Declared:     []string{"method_declaration", "constructor_declaration", "destructor_declaration", "operator_declaration", "conversion_operator_declaration", "local_function_statement", "accessor_declaration"},
		Bodies:       []string{"block", "arrow_expression_clause"},
		Closures:     []string{"lambda_expression", "anonymous_method_expression"},
		Ifs:          []string{"if_statement"},
		Nesting:      []string{"for_statement", "foreach_statement", "while_statement", "do_statement", "switch_statement", "switch_expression", "catch_clause", "conditional_expression"},
		Logic:        []string{"&&", "||"},
		Parentheses:  []string{"parenthesized_expression"},
		DepthIfs:     []string{"if_statement"},
		Depth:        []string{"for_statement", "foreach_statement", "while_statement", "do_statement", "switch_statement", "switch_expression", "try_statement"},
		Blocks:       []string{"block", "switch_section"},
		SwitchBlocks: []string{"switch_section"},
	}
}

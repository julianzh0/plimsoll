package conventions

import "regexp"

// csharpLanguage is the entry for C#. A using names a namespace: the folders whose files declare it.
func csharpLanguage() Language {
	return Language{Name: "csharp", Extensions: []string{".cs"}, Imports: csharpImports(), Grammar: "csharp", Query: csharpQuery}
}

// csharpQuery reads using directives: one that names a namespace, and a using static or a using alias, which names a
// type whose namespace is the part before its last name; global:: is no part of the name, and a global using is
// written once for every file of its project. It also reads a settings read whose name is not a quoted literal: an
// environment variable, or an index into a configuration object.
// NOTE: an alias of a namespace (using N = A.B;) is read as the type B of A. Upgrade: try the whole name first.
const csharpQuery = `
((using_directive "global"? @everywhere !name [(identifier) (qualified_name) (alias_qualified_name)] @spec) @import
  (#not-match? @import "^(global[ \t]+)?using[ \t]+static[ \t]") (#strip-prefix! @spec "global::"))
((using_directive "global"? @everywhere "static" (qualified_name qualifier: (_) @spec)) @import
  (#strip-prefix! @spec "global::"))
((using_directive "global"? @everywhere name: (identifier) (qualified_name qualifier: (_) @spec)) @import
  (#strip-prefix! @spec "global::"))
((invocation_expression
   function: (member_access_expression expression: (identifier) @o name: (identifier) @m)
   arguments: (argument_list . (argument (_) @dynamic)))
  (#eq? @o "Environment") (#eq? @m "GetEnvironmentVariable") (#not-match? @dynamic "^[\"@]"))
((element_access_expression
   expression: [(identifier) @o (member_access_expression name: (identifier) @o)]
   subscript: (bracketed_argument_list . (argument (_) @dynamic)))
  (#match? @o "(?i)^(.*_)?config(uration)?$") (#not-match? @dynamic "^[\"@0-9]"))
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

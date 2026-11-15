package conventions

import "regexp"

// csharpLanguage is the entry for C#. A using names a namespace: the folders whose files declare it.
func csharpLanguage() Language {
	return Language{Name: "csharp", Extensions: []string{".cs"}, DynamicRead: csDynamicRead, Imports: csharpImports()}
}

// dotnetProject is the file-name pattern of a .NET project file: a manifest of dependencies, and a name of the
// project's own code.
const dotnetProject = "*.csproj"

// The parts of the C# forms: a name, with the @ that lets a keyword be one, and a dotted name.
const (
	csName   = `@?[A-Za-z_]\w*`
	csDotted = csName + `(?:\.` + csName + `)*`
)

var (
	// csDynamicRead is a settings read whose name is not a quoted literal: an environment variable, or an index into
	// a configuration object.
	csDynamicRead = regexp.MustCompile(`Environment\.GetEnvironmentVariable\([ \t]*[^"@ \t)]|(?i:(?:\b|_)config(?:uration)?)\[[ \t]*[^"@ \t\]0-9]`)
	// csUsing is a using directive that names a namespace. Its end keeps it from a using statement of a method.
	csUsing = regexp.MustCompile(`(?m)^[ \t]*(?P<everywhere>global[ \t]+)?using[ \t]+(?:global::)?(?P<spec>` + csDotted + `)[ \t]*;`)
	// csUsingMember is a using static or a using alias: it names a type, whose namespace is the part before its
	// last name.
	// NOTE: an alias of a namespace (using N = A.B;) is read as the type B of A. Upgrade: try the whole name first.
	csUsingMember = regexp.MustCompile(`(?m)^[ \t]*(?P<everywhere>global[ \t]+)?using[ \t]+(?:static[ \t]+|` + csName +
		`[ \t]*=[ \t]*)(?:global::)?(?P<spec>` + csDotted + `)\.` + csName + `[ \t]*[;<]`)
	// csNamespace is a namespace declaration at the first column, file-scoped or the head of a block.
	csNamespace = regexp.MustCompile(`(?m)^namespace[ \t]+(?P<package>` + csDotted + `)`)
)

func csharpImports() Imports {
	return Imports{
		Statements:  []ImportForm{{Pattern: csUsing}, {Pattern: csUsingMember}},
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

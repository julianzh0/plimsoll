package conventions

import "regexp"

// The Go entry: what the shared rules cannot know for Go files.

// goLanguage is the entry of Go.
func goLanguage() Language {
	return Language{Name: "go", Extensions: []string{".go"}, DynamicRead: goDynamicRead, Imports: goImports()}
}

// A settings read whose name is computed at run time: not a quoted name after the opening.
var goDynamicRead = regexp.MustCompile(`os\.(?:Getenv|LookupEnv)\([ \t]*[^" \t)]`)

var (
	// Before the first declaration, a line that starts with a quoted string, with or without import and an alias, can
	// only be an import: one pattern covers the single form and every line of a block.
	goImportLine   = regexp.MustCompile("(?m)^[ \\t]*(?:import[ \\t]+)?(?:[\\w.]+[ \\t]+)?[\"`](?P<spec>[^\"`\\n]+)[\"`]")
	goUntil        = regexp.MustCompile(`(?m)^(?:func|type|var|const)\b`)
	goFolderExcept = regexp.MustCompile(`_test\.go$`)
)

func goImports() Imports {
	return Imports{
		Statements:   []ImportForm{{Pattern: goImportLine}},
		Until:        goUntil,
		Relative:     NoRelative,
		Prefix:       Prefix{File: "go.mod", Format: GoModule, Required: true},
		Target:       FolderTarget,
		FolderExcept: goFolderExcept,
		NoRings:      "the Go compiler rejects import cycles",
	}
}

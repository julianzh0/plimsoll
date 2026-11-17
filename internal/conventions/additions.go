package conventions

// Additions are the shared rules of the list of code a change added that nothing uses: what never counts as an
// addition, and which files are no use of a name.
type Additions struct {
	SkippedNames   []string // names a runtime calls without their being written: never listed
	SkippedFolders []string // folder-name patterns, at any depth: code below declares nothing for the list
	SkippedFiles   []string // file-name patterns: a code file of such a name declares nothing for the list
	DataFiles      []string // file-name patterns of files whose words are no use: lockfiles, minified code, source maps
	Docs           []string // suffixes of documentation: a name there is a mention, no use
	Config         []string // suffixes of configuration: a name there is a use
	ConfigNames    []string // file names of configuration without a suffix of Config
}

// defaultAdditions are the rules measured on agent pull requests and human history: entry points a runtime calls,
// fixtures, translations, generated and vendored code, test helpers outside test folders and example projects.
func defaultAdditions() Additions {
	return Additions{
		SkippedNames: []string{"main", "init", "new", "default", "setup", "teardown", "Main"},
		SkippedFolders: []string{
			"fixtures", "fixture", "__snapshots__", "snapshots", "test-data", "locales", "locale", "i18n", "translations",
			"third_party", "thirdparty", "generated", "__generated__", "gen", "alembic", "*[Uu]nit[Tt]est*",
			"*[Tt]est[Hh]elper*", "*[Ii]ntegration[Tt]est*",
			"examples", "example", "samples", "sample", "*.Examples", "*.Examples.*", "*.Samples", "*.Samples.*",
		},
		SkippedFiles: []string{"*.d.ts", "*.min.js", "*_generated.*", "*.generated.*", "*.gen.*", "conftest.py"},
		DataFiles: []string{
			"package-lock.json", "pnpm-lock.yaml", "yarn.lock", "Cargo.lock", "go.sum", "poetry.lock", "uv.lock", "pubspec.lock",
			"Package.resolved", "*.lock", "*.min.js", "*.map", "*.snap",
		},
		Docs: []string{".md", ".mdx", ".rst", ".adoc", ".txt"},
		Config: []string{
			".json", ".yaml", ".yml", ".toml", ".xml", ".html", ".cfg", ".ini", ".csproj", ".props", ".targets", ".sln",
			".properties", ".example",
		},
		ConfigNames: []string{"Dockerfile", "Makefile", "Procfile", "Justfile", "justfile", "BUILD", "WORKSPACE", "Taskfile", "Containerfile"},
	}
}

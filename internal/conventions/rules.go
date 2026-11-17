// Package conventions holds what plimsoll knows about code as data: the shared rules that hold for every
// language, the thin entry of each language where it differs, and the limits every mark compares with.
package conventions

import (
	"bytes"
	"path"
	"regexp"
	"slices"
	"strings"
)

// The two shared patterns are compiled once; everything else is built by Default. Each language's own patterns live
// in its file: typescript.go, golang.go, python.go, dart.go, java.go, kotlin.go, rust.go,
// csharp.go, swift.go, c.go, cpp.go. A test is defined by a Go test function or an XCTest method, a Python test
// function or test class, a JUnit-family annotation, a .NET test attribute, a Rust attribute whose last name is test,
// a test-runner call (test, testWidgets, describe, it), or a test macro of GoogleTest, Catch2 or doctest (TEST, TEST_F,
// TEST_CASE, SCENARIO, and their template forms).
var (
	testDefinition = regexp.MustCompile(`(?m)^[ \t]*(?:export[ \t]+)?(?:async[ \t]+)?(?:func[ \t]+(?:(?:Test|Benchmark|Fuzz|Example)\w*[ \t]*\(|test\w*[ \t]*\([ \t]*\))|def[ \t]+test_\w*[ \t]*\(|class[ \t]+Test\w*[ \t]*(?:\([^)]*\))?[ \t]*:[ \t\r]*(?:#[^\n]*)?$|@(?:\w+\.)*(?:Test|ParameterizedTest|RepeatedTest|TestFactory|TestTemplate)\b|\[(?:\w+\.)*(?:Fact|Theory|Test|TestMethod|TestCase)(?:Attribute)?[ \t]*[\](,]|#\[(?:\w+::)*test[(\]]|(?:await[ \t]+)?(?:t\.)?(?:(?:test|testWidgets|describe)(?:\.\w+)*|it(?:\.(?:only|skip|each|todo|concurrent|failing|fails|skipIf|runIf|sequential|for))*)[ \t]*\(|(?:TEST(?:_[FP])?|TYPED_TEST(?:_P)?|TEST_CASE(?:_METHOD|_FIXTURE|_TEMPLATE)?|TEMPLATE_(?:PRODUCT_|LIST_)?TEST_CASE(?:_SIG)?|SCENARIO(?:_TEMPLATE)?)[ \t]*\()`)
	generated      = regexp.MustCompile(`(?i)generated.*do not (?:edit|modify)|<auto-?generated`)
)

const (
	// BinarySniffBytes is how many first bytes are searched for a NUL byte, which makes a file binary.
	BinarySniffBytes = 8192
	// GeneratedHeaderLines is how many first lines are searched for the generated-file marker.
	GeneratedHeaderLines = 5
)

// Rules are the shared rules: which files are code, which code is test, product or other, and the limits.
type Rules struct {
	NotCode        []string // suffixes of documentation, data and lockfiles; "" stands for no suffix
	SkippedFolders []string // folder names skipped at any depth
	OtherFolders   []string // top-level folders whose code is other code
	DocFolders     []string // top-level folders that are documentation
	DocSuffixes    []string // suffixes that are documentation
	TestFolders    []string // folder names, at any depth, that hold test code
	TestSourceSets []string // name patterns of a folder directly in a folder src that holds test code, beside TestFolders
	// TestFolderPatterns are name patterns of a folder, at any depth, that make it hold test code.
	TestFolderPatterns []string
	TestPaths          []string       // folders, as paths from the repository root, that hold test code; empty in the shared rules
	TestEndings        []string       // endings of a file name without its suffix
	TestBeginnings     []string       // beginnings of a file name
	TestDefinition     *regexp.Regexp // a line that defines a test, anchored to the start of a statement
	Generated          *regexp.Regexp // the generated-file marker, searched in the first GeneratedHeaderLines lines
	// GeneratedNames are file-name patterns of generated files: such a file is read, and it is no code file.
	GeneratedNames []string
	Manifests      []Manifest // the known dependency manifests, read at the repository root
	CommitPaths    int        // the most paths a commit may touch and still count for a history detector
	// OwnFile is the file at a repository's root in which it sets its own limits and declares its layout. LimitsTable
	// is its table of limits and LimitKeys the keys of that table, in the order an answer names them; OwnKeys are the
	// keys that declare the layout, in the same order.
	OwnFile          string
	LimitsTable      string
	LimitKeys        []LimitKey
	OwnKeys          []OwnKey
	TestNames        []string // file-name patterns of test code; empty in the shared rules
	PlanningPaths    []string // planning and review records: an entry ending in "/" is a folder, at the root when it is one name, at any depth when it is a chain; any other a file-name pattern
	MeasurementPaths []string // measurement records, in the syntax of PlanningPaths
	ComposeFiles     []string // file-name patterns of compose files, read for their environment keys
	Limits           Limits   // the shared defaults
	// ReportOnly are the kinds of mark that report lists and status only counts, as "<detector>.<rule>", or with a
	// change, "<detector>.<rule> <change>": kinds outside reviewers judged too noisy to show on every change.
	ReportOnly []string
	Languages  []Language // the language entries
}

// Manifest is a known dependency manifest and how its runtime dependencies are read.
type Manifest struct {
	File      string
	Format    ManifestFormat
	Lockfiles []string // the lockfiles at the root that record what the manifest's dependencies resolved to
	// Nested makes File a pattern for the file name: the manifest is read wherever it is tracked outside skipped
	// folders, test code and other code.
	Nested bool
}

// ManifestFormat says how a manifest lists its runtime dependencies.
type ManifestFormat int

const (
	// JSONDependencies reads the keys of the top-level "dependencies" object.
	JSONDependencies ManifestFormat = iota + 1
	// GoModRequire reads the require entries that are not marked "// indirect".
	GoModRequire
	// PyprojectDependencies reads the names of the [project] table's dependencies array.
	PyprojectDependencies
	// PubspecDependencies reads the keys of the top-level dependencies block of a pubspec.yaml.
	PubspecDependencies
	// PomDependencies reads the dependencies a pom.xml declares for itself, in compile or runtime scope.
	PomDependencies
	// GradleDependencies reads the implementation, api and runtimeOnly lines of the top-level dependencies block.
	GradleDependencies
	// CsprojDependencies reads the PackageReference items of a .NET project file, without those whose assets are
	// private.
	CsprojDependencies
	// SwiftDependencies reads the .package(...) dependencies of a Package.swift.
	SwiftDependencies
	// CargoDependencies reads the keys of [dependencies] and of the target tables of a Cargo.toml.
	CargoDependencies
	// VcpkgDependencies reads the names of the top-level dependencies of a vcpkg.json, without host tools.
	VcpkgDependencies
)

// Limits are every number a mark compares with. No detector holds one.
type Limits struct {
	FileLines     Limit // per code file
	TestRatio     Limit // over the repository
	SharedCommits Limit // per pair of files
	// PairShareRise is how much a pair's share of the commits that touched either file must rise for the pair to
	// have grown: a pair that keeps changing together at the same rate has not grown by one more shared commit.
	PairShareRise Limit
	HotspotFiles  Limit // the files a report lists as hotspots
	// ImportsNotFollowed is the share of a language's own imports that may stay not followed before its link
	// numbers are not given. It gates the link numbers and is no mark, so it has no step.
	ImportsNotFollowed Limit
}

// OwnKey is a key of a repository's own file that declares its layout: its table, its key, what an answer calls it
// and what its values hold.
type OwnKey struct {
	Table, Key, Name string
	Holds            OwnValue
}

// OwnValue is what the values of an own key hold.
type OwnValue int

const (
	// TestFolderPaths are folders of test code, as paths from the repository root.
	TestFolderPaths OwnValue = iota + 1
	// TestFileNames are file-name patterns of test code.
	TestFileNames
	// BaseName is one name of a branch, a remote branch or a tag: the base of a question that names none.
	BaseName
)

// OwnValueLength is the longest value an own key may hold: the longest tracked folder path of sixteen repositories is
// 109 characters, the longest file name 72 (2026-11-16).
const OwnValueLength = 200

var (
	// ownFolder is a folder path from the root: segments of letters, digits and ._+- joined by "/".
	// NOTE: a folder whose name holds another character (a space, a letter outside ASCII) cannot be declared. Upgrade:
	// widen the set once a field case needs it.
	ownFolder = regexp.MustCompile(`^[A-Za-z0-9._+-]+(/[A-Za-z0-9._+-]+)*$`)
	// ownName is a file-name pattern of path.Match: letters, digits, ._+- and the wildcards * and ?.
	// NOTE: a pattern matches a file name, never a path across folders. Upgrade: match a folder path segment by
	// segment.
	ownName = regexp.MustCompile(`^[A-Za-z0-9._+*?-]+$`)
	// ownBase is a branch, remote-branch or tag name: segments of letters, digits and ._- joined by "/", so no
	// revision expression (~, ^, @{, :) and nothing that depends on the user's git configuration.
	ownBase = regexp.MustCompile(`^[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+)*$`)
)

// Form brings a value as written to the form an answer may print, and says whether it has that form: a folder loses
// one leading "./" and one trailing "/", and "." or "./" is the root.
func (v OwnValue) Form(value string) (formed string, usable bool) {
	if value == "" || len(value) > OwnValueLength {
		return "", false
	}
	if v == BaseName {
		if !ownBase.MatchString(value) || strings.Contains(value, "..") || strings.HasPrefix(value, "-") {
			return "", false
		}
		return value, true
	}
	if v.Pattern() {
		if !ownName.MatchString(value) {
			return "", false
		}
		return value, true
	}
	if value == "." || value == "./" {
		return ".", true
	}
	formed = strings.TrimSuffix(strings.TrimPrefix(value, "./"), "/")
	if !ownFolder.MatchString(formed) {
		return "", false
	}
	for segment := range strings.SplitSeq(formed, "/") {
		if segment == "." || segment == ".." {
			return "", false
		}
	}
	return formed, true
}

// Pattern says whether the values are file-name patterns rather than folders.
func (v OwnValue) Pattern() bool { return v == TestFileNames }

// Single says whether the key holds one string rather than a list.
func (v OwnValue) Single() bool { return v == BaseName }

// LimitKey is a key of a repository's limits file: the limit it sets, as an answer names it, and what the value
// counts ("" for a ratio, printed with one decimal).
type LimitKey struct {
	Key, Name, Unit string
}

// ByKey is the limit a repository's limits file names by its key, or nil for a key that is no limit.
func (l *Limits) ByKey(key string) *Limit {
	switch key {
	case "file_lines":
		return &l.FileLines
	case "test_ratio":
		return &l.TestRatio
	case "shared_commits":
		return &l.SharedCommits
	case "hotspot_files":
		return &l.HotspotFiles
	}
	return nil
}

// Limit is one limit: its value, the smallest rise that counts as grown, and where the value comes from.
type Limit struct {
	Value  float64
	Step   float64
	Source string
}

// Language is a language's entry: only what the shared rules cannot know for that language.
type Language struct {
	Name        string
	Extensions  []string       // the suffixes the entry speaks for
	DynamicRead *regexp.Regexp // a settings read whose name is computed at run time
	// Literals are comments, strings and character literals; comments are blanked before imports are read, and
	// strings and character literals are kept as written, so a comment marker inside one starts no comment. Nil reads
	// the text as written.
	Literals *regexp.Regexp
	Imports  Imports       // how its imports are written and what they name
	Layout   []Declaration // the standard files and keys in which a project declares its layout; nil for none
	// Grammar is the tree-sitter grammar that reads the entry's files; "" reads them by the text patterns above.
	Grammar string
	// GrammarBySuffix names another grammar for some of the entry's suffixes.
	GrammarBySuffix map[string]string
	// Query is the tree-sitter query that reads, in one parse, everything the entry takes from a file. Its captures:
	//   @import   an import statement; the @name captures inside it are the names it imports
	//   @spec.mod a module declared in its own file: the target is "mod " and the name
	//   @module, @module.name  a module a file holds in braces, and its name; a statement inside is resolved from it
	//   @test, @test.end  test code inside a product file, from the start of the first to the end of the second
	//   @text     a stretch the grammar keeps as tokens (a macro body): the entry's text forms read it
	//   @blank    a comment or a string, blanked inside a target and inside @text
	//   @spec     the target as written, one pair of quotes removed; several in one match are one target
	//   @type     the statement imports types only
	//   @everywhere  the statement is written once for every file of its project
	//   @computed an import whose target is built at run time; counts unless a @spec lies inside it
	//   @pass     a top-level node a file that only passes imports on may hold; a file all of whose top-level nodes
	//             are marked only passes imports on, and an entry whose query marks none never does
	//   @name     one name a statement imports out of its target, inside the statement's @import
	//   @type_block  a block whose statements import types only
	//   @prefix   a declared prefix of settings names
	//   @dynamic  a settings read whose name is computed at run time; each match counts once
	// No fact is read both by a pattern and by the query, except under LineReadWhenBroken.
	Query string
	// LineReadWhenBroken reads a file the grammar cannot read without an error by the entry's text patterns instead:
	// for a language whose files the grammar often misreads (macros), the patterns read every line on its own.
	LineReadWhenBroken bool
}

// GrammarOf names the grammar that reads a file of the entry, or "" when the entry reads by text.
func (l Language) GrammarOf(filePath string) string {
	if grammar, found := l.GrammarBySuffix[Suffix(filePath)]; found {
		return grammar
	}
	return l.Grammar
}

// LayoutKind is what a declaration says about a project's layout.
type LayoutKind int

const (
	// TestFolders are folders that hold test code.
	TestFolders LayoutKind = iota + 1
	// Roots are folders whose direct children are the project's own packages and modules.
	Roots
)

// Declaration is one key of a standard file in which a project declares part of its layout: a key of a TOML table
// whose value is an array of strings, or one string that holds the values separated by white space.
type Declaration struct {
	File     string // read at the repository root
	Table    string // the TOML table that holds the key, as its header names it
	Key      string
	Means    LayoutKind
	Tool     string   // the tool that reads File, as a sentence names it
	Shadowed []string // root files the tool reads instead of File, the one it reads first first
}

// Default returns the rules of the first version.
func Default() Rules {
	return Rules{
		NotCode: []string{
			".md", ".rst", ".txt", ".json", ".arb", ".properties", ".pdf", ".yaml", ".yml", ".toml", ".ini", ".cfg", ".lock", ".csv", ".tsv",
			".svg", ".xml", ".html", ".css", ".snap", ".golden",
			"", ".mod", ".sum", ".example", ".crt", ".pem", ".key", ".diff", ".patch", ".sha256",
			".resolved", ".natvis", ".csproj", ".props", ".targets", ".sln", ".slnx", ".config", ".Config", ".DotSettings", ".resx",
		},
		SkippedFolders:     []string{"node_modules", "dist", "build", "vendor", "migrations", ".venv", "coverage", "testdata", "__pycache__", ".dart_tool", "META-INF"},
		OtherFolders:       []string{"scripts", "examples", ".github", "docs"},
		DocFolders:         []string{"docs"},
		DocSuffixes:        []string{".md"},
		TestFolders:        []string{"tests", "Tests", "test", "__tests__", "spec", "integration_test"},
		TestSourceSets:     []string{"test[A-Z]*", "*Test*"},
		TestFolderPatterns: []string{"*.Tests", "*.Test"},
		GeneratedNames:     []string{"*.g.cs", "*.Designer.cs"},
		TestEndings:        []string{".test", ".spec", "_test", "-test"},
		TestBeginnings:     []string{"test_"},
		TestDefinition:     testDefinition,
		Generated:          generated,
		Manifests: []Manifest{
			{File: "package.json", Format: JSONDependencies, Lockfiles: []string{"package-lock.json", "pnpm-lock.yaml", "yarn.lock"}},
			{File: "go.mod", Format: GoModRequire, Lockfiles: []string{"go.sum"}},
			{File: "pyproject.toml", Format: PyprojectDependencies, Lockfiles: []string{"uv.lock", "poetry.lock"}},
			{File: "pubspec.yaml", Format: PubspecDependencies, Lockfiles: []string{"pubspec.lock"}},
			{File: "pom.xml", Format: PomDependencies},
			{File: "build.gradle", Format: GradleDependencies},
			{File: "build.gradle.kts", Format: GradleDependencies},
			{File: "Cargo.toml", Format: CargoDependencies, Lockfiles: []string{"Cargo.lock"}},
			{File: dotnetProject, Format: CsprojDependencies, Nested: true},
			{File: "Package.swift", Format: SwiftDependencies, Lockfiles: []string{"Package.resolved"}},
			{File: "vcpkg.json", Format: VcpkgDependencies},
		},
		OwnFile:     ".plimsoll.toml",
		LimitsTable: "limits",
		OwnKeys: []OwnKey{
			{Table: "layout", Key: "test_folders", Name: "test folders", Holds: TestFolderPaths},
			{Table: "layout", Key: "test_files", Name: "test files", Holds: TestFileNames},
			{Table: "defaults", Key: "base", Name: "default base", Holds: BaseName},
		},
		LimitKeys: []LimitKey{
			{Key: "file_lines", Name: "file-size limit", Unit: "lines"},
			{Key: "test_ratio", Name: "test-ratio limit"},
			{Key: "shared_commits", Name: "co-change limit", Unit: "shared commits"},
			{Key: "hotspot_files", Name: "hotspot count", Unit: "files"},
		},
		PlanningPaths: []string{
			"docs/superpowers/", "docs/stories/", "docs/plans/", "docs/specs/", "projects/", ".review-work/",
			"*review-packet*", "REQUIREMENTS.md", "SPEC.md",
		},
		MeasurementPaths: []string{"docs/qualification/", "evidence/"},
		ComposeFiles:     []string{"compose*.yml", "compose*.yaml", "docker-compose*.yml", "docker-compose*.yaml"},
		// A commit that touches more files says little about which files belong together (requirements, R-16).
		CommitPaths: 20,
		// Seven outside reviewers judged real answers per kind of mark (2026-11-17); these they judged too noisy for
		// status and worth keeping in report.
		ReportOnly: []string{
			"shape.file-size grown", "shape.dependencies", "shape.settings", "process.doc-churn", "settings.compose-only",
			"co-change.pair", "links.test-import", "links.folder-cycle",
		},
		Limits: Limits{
			FileLines: Limit{Value: 1000, Step: 1, Source: "the default of Pylint and SonarQube"},
			TestRatio: Limit{Value: 2.0, Step: 0.1, Source: "plimsoll's rule: test code at most twice the product code"},
			// The requirements set the floor for a pair of files that changed together (R-16).
			SharedCommits: Limit{Value: 5, Step: 1, Source: "plimsoll's rule"},
			// Read blind on 1,180 changes, growth by one shared commit was most pair marks and rarely worth a look.
			PairShareRise: Limit{Value: 0.1, Source: "plimsoll's rule: one more shared commit is not growth"},
			// The requirements name the three files changed most often times their size (R-15).
			HotspotFiles: Limit{Value: 3, Step: 1, Source: "the three files changed most often times their size"},
			// The share of own imports not followed was 1.4% on the BFF and 0% on plimsoll, where the links equalled those
			// of dependency-cruiser and go list; 10% leaves room for code that embeds program text in strings.
			ImportsNotFollowed: Limit{Value: 0.10, Source: "links checked against go list and dependency-cruiser, 2026-11"},
		},
		// One entry per language, each in its own file. A new language is one file and one line here.
		Languages: []Language{typescriptLanguage(), goLanguage(), pythonLanguage(), dartLanguage(), javaLanguage(), kotlinLanguage(), rustLanguage(), csharpLanguage(), swiftLanguage(), cLanguage(), cppLanguage()},
	}
}

// Suffix returns the last extension of a path's file name, as written; "" when it has none or is a dotfile.
func Suffix(filePath string) string {
	return path.Ext(strings.TrimPrefix(path.Base(filePath), "."))
}

// IsBinary says whether the first bytes of a file make it binary: a NUL byte in the first BinarySniffBytes.
func IsBinary(head []byte) bool {
	return bytes.IndexByte(head[:min(len(head), BinarySniffBytes)], 0) >= 0
}

// LanguageOf returns the entry that speaks for a path's suffix.
func (r Rules) LanguageOf(filePath string) (Language, bool) {
	suffix := Suffix(filePath)
	for _, language := range r.Languages {
		if slices.Contains(language.Extensions, suffix) {
			return language, true
		}
	}
	return Language{}, false
}

// PlanningRule lists the planning and review paths, as an answer prints them.
func (r Rules) PlanningRule() string {
	parts := make([]string, 0, len(r.PlanningPaths))
	for _, entry := range r.PlanningPaths {
		parts = append(parts, "`"+entry+"`")
	}
	return strings.Join(parts, ", ")
}

// TestRule describes how test code is recognized, as an answer prints it.
func (r Rules) TestRule() string {
	var parts []string
	for _, folder := range r.TestFolders {
		parts = append(parts, "`"+folder+"/`")
	}
	for _, pattern := range r.TestSourceSets {
		parts = append(parts, "`src/"+pattern+"/`")
	}
	for _, pattern := range r.TestFolderPatterns {
		parts = append(parts, "`"+pattern+"/`")
	}
	for _, ending := range r.TestEndings {
		parts = append(parts, "`*"+ending+".*`")
	}
	for _, beginning := range r.TestBeginnings {
		parts = append(parts, "`"+beginning+"*`")
	}
	return strings.Join(parts, ", ")
}

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
// in its file: typescript.go, golang.go, python.go, dart.go. A test is defined by a Go test function, a Python test
// function or test class, a JUnit-family annotation, or a test-runner call (test, testWidgets, describe, it).
var (
	testDefinition = regexp.MustCompile(`(?m)^[ \t]*(?:export[ \t]+)?(?:async[ \t]+)?(?:func[ \t]+(?:Test|Benchmark|Fuzz|Example)\w*[ \t]*\(|def[ \t]+test_\w*[ \t]*\(|class[ \t]+Test\w*[ \t]*(?:\([^)]*\))?[ \t]*:[ \t\r]*(?:#[^\n]*)?$|@(?:\w+\.)*(?:Test|ParameterizedTest|RepeatedTest|TestFactory|TestTemplate)\b|(?:await[ \t]+)?(?:t\.)?(?:(?:test|testWidgets|describe)(?:\.\w+)*|it(?:\.(?:only|skip|each|todo|concurrent|failing|fails|skipIf|runIf|sequential|for))*)[ \t]*\()`)
	generated      = regexp.MustCompile(`(?i)generated.*do not (?:edit|modify)`)
)

const (
	// BinarySniffBytes is how many first bytes are searched for a NUL byte, which makes a file binary.
	BinarySniffBytes = 8192
	// GeneratedHeaderLines is how many first lines are searched for the generated-file marker.
	GeneratedHeaderLines = 5
)

// Rules are the shared rules: which files are code, which code is test, product or other, and the limits.
type Rules struct {
	NotCode          []string       // suffixes of documentation, data and lockfiles; "" stands for no suffix
	SkippedFolders   []string       // folder names skipped at any depth
	OtherFolders     []string       // top-level folders whose code is other code
	DocFolders       []string       // top-level folders that are documentation
	DocSuffixes      []string       // suffixes that are documentation
	TestFolders      []string       // folder names, at any depth, that hold test code
	TestSourceSets   []string       // name patterns of a folder directly in a folder src that holds test code, beside TestFolders
	TestPaths        []string       // folders, as paths from the repository root, that hold test code; empty in the shared rules
	TestEndings      []string       // endings of a file name without its suffix
	TestBeginnings   []string       // beginnings of a file name
	TestDefinition   *regexp.Regexp // a line that defines a test, anchored to the start of a statement
	Generated        *regexp.Regexp // the generated-file marker, searched in the first GeneratedHeaderLines lines
	Manifests        []Manifest     // the known dependency manifests, read at the repository root
	CommitPaths      int            // the most paths a commit may touch and still count for a history detector
	PlanningPaths    []string       // planning and review records: an entry ending in "/" is a folder, at the root when it is one name, at any depth when it is a chain; any other a file-name pattern
	MeasurementPaths []string       // measurement records, in the syntax of PlanningPaths
	ComposeFiles     []string       // file-name patterns of compose files, read for their environment keys
	Limits           Limits         // the shared defaults
	Languages        []Language     // the language entries
}

// Manifest is a known dependency manifest and how its runtime dependencies are read.
type Manifest struct {
	File   string
	Format ManifestFormat
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
)

// Limits are every number a mark compares with. No detector holds one.
type Limits struct {
	FileLines     Limit // per code file
	TestRatio     Limit // over the repository
	SharedCommits Limit // per pair of files
	// ImportsNotFollowed is the share of a language's own imports that may stay not followed before its link
	// numbers are not given. It gates the link numbers and is no mark, so it has no step.
	ImportsNotFollowed Limit
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
	FileLines   *Limit         // replaces the shared line limit for this language's files; nil keeps the default
	DynamicRead *regexp.Regexp // a settings read whose name is computed at run time
	// SettingsPrefix is a declaration of a prefix of settings names; its group "prefix" holds it. Nil for none.
	SettingsPrefix *regexp.Regexp
	Imports        Imports       // how its imports are written and what they name
	Layout         []Declaration // the standard files and keys in which a project declares its layout; nil for none
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
		},
		SkippedFolders: []string{"node_modules", "dist", "build", "vendor", "migrations", ".venv", "coverage", "testdata", "__pycache__", ".dart_tool", "META-INF"},
		OtherFolders:   []string{"scripts", "examples", ".github", "docs"},
		DocFolders:     []string{"docs"},
		DocSuffixes:    []string{".md"},
		TestFolders:    []string{"tests", "test", "__tests__", "spec", "integration_test"},
		TestSourceSets: []string{"test[A-Z]*", "*Test*"},
		TestEndings:    []string{".test", ".spec", "_test", "-test"},
		TestBeginnings: []string{"test_"},
		TestDefinition: testDefinition,
		Generated:      generated,
		Manifests: []Manifest{
			{File: "package.json", Format: JSONDependencies},
			{File: "go.mod", Format: GoModRequire},
			{File: "pyproject.toml", Format: PyprojectDependencies},
			{File: "pubspec.yaml", Format: PubspecDependencies},
			{File: "pom.xml", Format: PomDependencies},
			{File: "build.gradle", Format: GradleDependencies},
			{File: "build.gradle.kts", Format: GradleDependencies},
		},
		PlanningPaths: []string{
			"docs/superpowers/", "docs/stories/", "docs/plans/", "docs/specs/", "projects/", ".review-work/",
			"*review-packet*", "REQUIREMENTS.md", "SPEC.md",
		},
		MeasurementPaths: []string{"docs/qualification/", "evidence/"},
		ComposeFiles:     []string{"compose*.yml", "compose*.yaml", "docker-compose*.yml", "docker-compose*.yaml"},
		// A commit that touches more files says little about which files belong together (requirements, R-16).
		CommitPaths: 20,
		Limits: Limits{
			FileLines: Limit{Value: 1000, Step: 1, Source: "the default of Pylint and SonarQube"},
			TestRatio: Limit{Value: 2.0, Step: 0.1, Source: "plimsoll's rule: test code at most twice the product code"},
			// The requirements set the floor for a pair of files that changed together (R-16).
			SharedCommits: Limit{Value: 5, Step: 1, Source: "at least 5 shared commits"},
			// The share of own imports not followed was 1.4% on the BFF and 0% on plimsoll, where the links equalled those
			// of dependency-cruiser and go list; 10% leaves room for code that embeds program text in strings.
			ImportsNotFollowed: Limit{Value: 0.10, Source: "links checked against go list and dependency-cruiser, 2026-11"},
		},
		// One entry per language, each in its own file. A new language is one file and one line here.
		Languages: []Language{typescriptLanguage(), goLanguage(), pythonLanguage(), dartLanguage(), javaLanguage(), kotlinLanguage()},
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

// FileLimit is the line limit of a path: its language entry's replacement, else the shared default.
func (r Rules) FileLimit(filePath string) Limit {
	if language, found := r.LanguageOf(filePath); found && language.FileLines != nil {
		return *language.FileLines
	}
	return r.Limits.FileLines
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
	for _, ending := range r.TestEndings {
		parts = append(parts, "`*"+ending+".*`")
	}
	for _, beginning := range r.TestBeginnings {
		parts = append(parts, "`"+beginning+"*`")
	}
	return strings.Join(parts, ", ")
}

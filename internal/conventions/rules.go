// Package conventions holds what plimsoll knows about code as data: the shared rules that hold for every
// language, the thin entry of each language where it differs, and the limits every mark compares with.
package conventions

import (
	"bytes"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
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
	// FunctionRules are the shared rules of the function marks: what is left out, and their other numbers.
	FunctionRules  FunctionRules
	NotCode        []string // suffixes of documentation, data and lockfiles; "" stands for no suffix
	SkippedFolders []string // folder names skipped at any depth
	OtherFolders   []string // top-level folders whose code is other code
	DocFolders     []string // top-level folders that are documentation
	DocSuffixes    []string // suffixes that are documentation
	TestFolders    []string // folder names, at any depth, that hold test code
	TestSourceSets []string // name patterns of a folder directly in a folder src that holds test code, beside TestFolders
	// TestFolderPatterns are name patterns of a folder, at any depth, that make it hold test code.
	TestFolderPatterns []string
	TestPaths          []string // folders, as paths from the repository root, that hold test code; empty in the shared rules
	// TestFileFolders are folders, as paths from the repository root, in which the files of certain names are test code;
	// empty in the shared rules.
	TestFileFolders []TestFileFolder
	TestEndings     []string       // endings of a file name without its suffix
	TestBeginnings  []string       // beginnings of a file name
	TestDefinition  *regexp.Regexp // a line that defines a test, anchored to the start of a statement
	Generated       *regexp.Regexp // the generated-file marker, searched in the first GeneratedHeaderLines lines
	// GeneratedNames are file-name patterns of generated files: such a file is read, and it is no code file.
	GeneratedNames []string
	Manifests      []Manifest // the known dependency manifests, read wherever tracked
	CommitPaths    int        // the most paths a commit may touch and still count for a history detector
	// OwnFile is the file at a repository's root in which it sets its own limits and declares its layout. LimitsTable
	// is its table of limits and LimitKeys the keys of that table, in the order an answer names them; OwnKeys are the
	// keys that declare the layout, in the same order.
	OwnFile     string
	LimitsTable string
	LimitKeys   []LimitKey
	OwnKeys     []OwnKey
	// AcknowledgedTable is the table of the own file whose keys are mark identifiers the repository acknowledges, each
	// with a reason; AcknowledgedName is what an answer calls its entries.
	AcknowledgedTable, AcknowledgedName string
	TestNames                           []string  // file-name patterns of test code; empty in the shared rules
	IgnoredPaths                        []string  // folders, as paths from the repository root, whose files are left out; empty in the shared rules
	IgnoredNames                        []string  // file-name patterns of files left out; empty in the shared rules
	PlanningPaths                       []string  // planning and review records: an entry ending in "/" is a folder, at the root when it is one name, at any depth when it is a chain; any other a file-name pattern
	PlanningFolders                     []string  // planning folders a repository declares, as paths from the root; empty in the shared rules
	PlanningNames                       []string  // file-name patterns of planning files a repository declares; empty in the shared rules
	NoteNames                           NoteNames // the words that make an added Markdown file's name a working note
	MeasurementPaths                    []string  // measurement records, in the syntax of PlanningPaths
	ComposeFiles                        []string  // file-name patterns of compose files, read for their environment keys
	Limits                              Limits    // the shared defaults
	// ReportOnly are the kinds of mark that report lists and status only counts, as "<detector>.<rule>": kinds
	// outside reviewers or a blind reading judged too noisy to show on every change.
	ReportOnly []string
	// Areas are the areas of the headline, in the order an answer shows them; every kind of mark is in exactly one.
	Areas     []Area
	Additions Additions  // what never counts as code a change added, and which files are no use of a name
	Languages []Language // the language entries
}

// Manifest is a known dependency manifest and how its runtime dependencies are read.
// A manifest is read wherever it is tracked outside skipped folders, test code and other code.
type Manifest struct {
	File      string // the file name, or a pattern for it
	Format    ManifestFormat
	Lockfiles []string // the names of the lockfiles that record what its dependencies resolved to, in its folder or above
	// Recipes are folders whose files of this name describe other packages, not the project's own: vcpkg's ports.
	Recipes []string
	// Reads are files, as paths below a folder at or above the manifest, its reader takes too: a Gradle catalogue.
	Reads []string
	// Name is the form of a dependency name in its ecosystem. A manifest that lists a name outside it is unreadable,
	// so no text of a file reaches an answer as a dependency name.
	Name *regexp.Regexp
}

// coordinatePattern is a Maven coordinate, group and artifact and an optional third part, each plain or an unresolved
// property reference with a plain name.
const coordinatePattern = `(?:[A-Za-z0-9._-]+|\$\{[A-Za-z0-9._-]+\}|\$[A-Za-z_][A-Za-z0-9_]*)(?::(?:[A-Za-z0-9._-]+|\$\{[A-Za-z0-9._-]+\}|\$[A-Za-z_][A-Za-z0-9_]*)){1,2}`

// GradleCatalogue is where a Gradle build keeps its version catalogue, below its root folder.
const GradleCatalogue = "gradle/libs.versions.toml"

// Dependency name forms, by ecosystem. Maven and Gradle coordinates may hold an unresolved property reference with a
// plain name, Gradle an accessor (of a catalogue or of constants in buildSrc) or a project path; each is printed as
// written.
var (
	npmName    = regexp.MustCompile(`^(@[A-Za-z0-9._~-]+/)?[A-Za-z0-9._~-]+$`)
	goModule   = regexp.MustCompile(`^[A-Za-z0-9._~+-]+(/[A-Za-z0-9._~+-]+)*$`)
	pythonName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`) // after PEP 503 normalisation
	pubName    = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)
	coordinate = regexp.MustCompile(`^` + coordinatePattern + `$`)
	gradleName = regexp.MustCompile(`^(?:` + coordinatePattern + `|[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z0-9_]+)+|:[A-Za-z0-9._-]+(?::[A-Za-z0-9._-]+)*)$`)
	crateName  = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	nugetID    = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]*$`)
	swiftID    = regexp.MustCompile(`^[a-z0-9._-]+$`)
	vcpkgPort  = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
)

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
	// FunctionComplexity is the cognitive complexity over which a function the change made is marked; its step is the
	// rise a function that existed needs.
	FunctionComplexity Limit
	// FunctionNesting is the control nesting a function reaches for the nesting list; its step is the rise it needs.
	FunctionNesting Limit
	// RadiusPaths is how many paths of the blast radius a report lists; the rest is counted.
	RadiusPaths Limit
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
	// IgnoredFolderPaths are folders, as paths from the repository root, whose files plimsoll leaves out.
	IgnoredFolderPaths
	// IgnoredFileNames are file-name patterns of files plimsoll leaves out.
	IgnoredFileNames
	// NeverImportRules are import rules "a -> b": product code under folder a imports no code under folder b.
	NeverImportRules
	// OnlyImportRules are import rules "a -> b": product code under folder a imports code only under a itself and
	// under the folders its rules of this kind name.
	OnlyImportRules
	// PlanningFolderPaths are folders, as paths from the repository root, whose files are planning or review records.
	PlanningFolderPaths
	// PlanningFileNames are file-name patterns of planning or review records.
	PlanningFileNames
)

// OwnValueLength is the longest value an own key may hold: the longest tracked folder path of sixteen repositories is
// 109 characters, the longest file name 72 (2026-11-16).
const OwnValueLength = 200

// importArrow joins the two folders of an import rule, as it is written and as an answer prints it.
const importArrow = "->"

// importRuleLength is the longest import rule as written: two folders at OwnValueLength, each with a leading "./"
// and a trailing "/", joined by the arrow with up to three spaces on each side.
const importRuleLength = 2*(OwnValueLength+len("./")+len("/")) + 2*len(" -> ")

// MarkIDLength is the longest mark identifier the acknowledged table may hold: a pair names two paths, and the
// longest tracked path of the 193 local validation clones is 257 characters, so a pair identifier stays under 530.
const MarkIDLength = 600

// ReasonLength is the longest reason an acknowledged entry may give, in characters: the reason is never kept or
// printed, so the cap only keeps an entry to one readable line.
const ReasonLength = 200

// markID is a mark identifier as answers print it: a detector and a rule, each lower-case words joined by single
// hyphens, and a subject of letters, digits and the punctuation paths, functions and pairs hold. A space or a quote
// has no place in it, so an identifier cannot carry a sentence into an answer.
// A path with a space or a letter outside ASCII has no identifier in this form; WideMarkID admits it.
var markID = regexp.MustCompile(`^` + markPrefix + `[A-Za-z0-9._\-+/:#~@$()\[\],=>]+$`)

// markPrefix is the detector and rule of a mark identifier, as answers print them.
const markPrefix = `[a-z]+(?:-[a-z]+)*\.[a-z]+(?:-[a-z]+)*:`

// MarkID says whether an acknowledged entry's identifier has the form of a mark identifier, within its cap.
func MarkID(id string) bool {
	return len(id) <= MarkIDLength && markID.MatchString(id)
}

var (
	// ownFolder is a folder path from the root: segments of letters, digits and ._+- joined by "/". A folder whose name
	// holds another character has the widened form instead (WideForm).
	ownFolder = regexp.MustCompile(`^[A-Za-z0-9._+-]+(/[A-Za-z0-9._+-]+)*$`)
	// ownName is a file-name pattern of path.Match: letters, digits, ._+- and the wildcards * and ?.
	// A pattern matches a file name, never a path across folders, as docs/rules.md states.
	ownName = regexp.MustCompile(`^[A-Za-z0-9._+*?-]+$`)
	// ownBase is a branch, remote-branch or tag name: segments of letters, digits and ._- joined by "/", so no
	// revision expression (~, ^, @{, :) and nothing that depends on the user's git configuration.
	ownBase = regexp.MustCompile(`^[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+)*$`)
)

// Form brings a value as written to the form an answer may print, and says whether it has that form: a folder loses
// one leading "./" and one trailing "/", and "." or "./" is the root.
func (v OwnValue) Form(value string) (formed string, usable bool) {
	if v.ImportRule() {
		return v.ruleForm(value, folderForm)
	}
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
	return folderForm(value)
}

// folderForm is a folder path from the root without one leading "./" and one trailing "/"; "." or "./" is the root.
func folderForm(value string) (string, bool) {
	if value == "." || value == "./" {
		return ".", true
	}
	formed := strings.TrimSuffix(strings.TrimPrefix(value, "./"), "/")
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

// ruleForm brings an import rule to "a -> b", rebuilt from its two folders so that nothing else of the value
// as written is kept: one arrow, two folders from the root other than the root itself, each within OwnValueLength.
// Under NeverImportRules the two folders differ and neither lies under the other.
// folder is the form of each of the two folders: the plain or the widened one.
func (v OwnValue) ruleForm(value string, folder func(string) (string, bool)) (string, bool) {
	if len(value) > importRuleLength || strings.Count(value, importArrow) != 1 {
		return "", false
	}
	from, to, _ := strings.Cut(value, importArrow)
	var ends [2]string
	for i, end := range []string{from, to} {
		formed, usable := folder(strings.Trim(end, " \t"))
		if !usable || formed == "." || len(formed) > OwnValueLength {
			return "", false
		}
		ends[i] = formed
	}
	if v == NeverImportRules && (ends[0] == ends[1] || strings.HasPrefix(ends[0], ends[1]+"/") || strings.HasPrefix(ends[1], ends[0]+"/")) {
		return "", false
	}
	return ImportRule(ends[0], ends[1]), true
}

// WideForm brings a folder or an import rule written with other characters (a space, a letter outside ASCII) to its
// widened form, and says whether it has it: segments that are neither empty, "." nor "..", no quote, no backtick and
// no control, format or line-separating character, within the same caps. A widened value is used only when it names
// a tracked path, so it is never printed unless it is the leading part of one. File-name patterns and the base have
// no widened form.
func (v OwnValue) WideForm(value string) (formed string, usable bool) {
	switch {
	case v.ImportRule():
		return v.ruleForm(value, wideFolderForm)
	case v.Pattern() || v.Single() || len(value) > OwnValueLength:
		return "", false
	}
	return wideFolderForm(value)
}

// wideFolderForm is a folder path from the root in the widened form, without one leading "./" and one trailing "/".
func wideFolderForm(value string) (string, bool) {
	formed := strings.TrimSuffix(strings.TrimPrefix(value, "./"), "/")
	if formed == "" || !wideText(formed) {
		return "", false
	}
	for segment := range strings.SplitSeq(formed, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", false
		}
	}
	return formed, true
}

// wideText says whether a text holds no character a widened value may not hold: a quote, a backtick, or a control,
// format, line or paragraph separator character, which could break the line or the markup of an answer.
func wideText(text string) bool {
	return utf8.ValidString(text) && !strings.ContainsFunc(text, func(r rune) bool {
		return r == '"' || r == '`' || unicode.In(r, unicode.Cc, unicode.Cf, unicode.Zl, unicode.Zp)
	})
}

// wideMarkPrefix finds the detector and rule at the start of an identifier.
var wideMarkPrefix = regexp.MustCompile(`^` + markPrefix)

// WideMarkID says whether an acknowledged entry's identifier has the widened form of a mark identifier: the plain
// detector and rule, then a subject of any characters a widened value may hold. Such an entry applies only when it
// equals the identifier of an item an answer holds, so its subject is never printed.
func WideMarkID(id string) bool {
	prefix := wideMarkPrefix.FindString(id)
	return prefix != "" && len(id) > len(prefix) && len(id) <= MarkIDLength && wideText(id[len(prefix):])
}

// ImportRule prints an import rule from its two folders.
func ImportRule(from, to string) string { return from + " " + importArrow + " " + to }

// ImportRuleEnds are the two folders of an import rule in its form.
func ImportRuleEnds(rule string) (from, to string) {
	from, to, _ = strings.Cut(rule, " "+importArrow+" ")
	return from, to
}

// Shape says what a usable value of an import rule kind looks like, as the sentence on unusable values names it;
// "" for any other kind.
func (v OwnValue) Shape() string {
	switch v {
	case NeverImportRules:
		return "import rule of the form `folder -> folder` (two different folders from the root, neither inside the other)"
	case OnlyImportRules:
		return "import rule of the form `folder -> folder` (two folders from the root)"
	case TestFolderPaths, TestFileNames, BaseName, IgnoredFolderPaths, IgnoredFileNames, PlanningFolderPaths, PlanningFileNames:
	}
	return ""
}

// ImportRule says whether the values are import rules.
func (v OwnValue) ImportRule() bool { return v == NeverImportRules || v == OnlyImportRules }

// Pattern says whether the values are file-name patterns rather than folders.
func (v OwnValue) Pattern() bool {
	return v == TestFileNames || v == IgnoredFileNames || v == PlanningFileNames
}

// Planning says whether the values are planning folders or planning file names.
func (v OwnValue) Planning() bool { return v == PlanningFolderPaths || v == PlanningFileNames }

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
	Name       string
	Extensions []string // the suffixes the entry speaks for
	// Literals are comments, strings and character literals; comments are blanked before imports are read, and
	// strings and character literals are kept as written, so a comment marker inside one starts no comment. Nil reads
	// the text as written.
	Literals *regexp.Regexp
	Imports  Imports       // how its imports are written and what they name
	Layout   []Declaration // the standard files and keys in which a project declares its layout; nil for none
	// Grammar is the tree-sitter grammar that reads the entry's files; "" reads them by the text patterns above.
	Grammar string
	// Functions says what a function is and what counts in it, in the grammar's node kinds; every entry has one.
	Functions FunctionKinds
	// Tokens splits a function's text into tokens; set with Functions.
	Tokens Tokens
	// GrammarBySuffix names another grammar for some of the entry's suffixes.
	GrammarBySuffix map[string]string
	// Query is the tree-sitter query that reads, in one parse, everything the entry takes from a file. Its captures:
	//   @import   an import statement; the @name captures inside it are the names it imports
	//   @spec.mod a module declared in its own file: the target is "mod " and the name
	//   @spec.extern  an external crate: the target is "crate " and the name
	//   @path, @path.of  the file a path attribute names (quoted) for the module whose name is @path.of
	//   @module, @module.name  a module a file holds in braces, and its name; a statement inside is resolved from it
	//   @bind, @bind.at  a name an item binds in the module it stands in, and the item
	//   @opaque   what may bind names in its module that no item shows (a macro call, an active attribute)
	//   @export   a macro the crate exports at its root
	//   @alias    the name an external crate is given
	//   @aliasing the statement gives its target a name of its own (C#'s using N = A.B): the target is "= " and the name
	//   @namespace, @namespace.name  a namespace a file declares in a block, and its name; a block inside another's
	//             declares the joined name
	//   @namespace.file  a namespace a file declares for the whole file
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
	// No fact is read both by a pattern and by the query, except under LineReadWhenBroken.
	Query string
	// LineReadWhenBroken reads the facts of a file the grammar cannot read without an error (its imports, its
	// declarations) by the entry's text patterns instead: for a language whose files the grammar often misreads
	// (macros), the patterns read every line on its own. Its functions still come from the tree.
	LineReadWhenBroken bool
	// Macros rewrite, in a copy of a file whose tree holds an error, what the grammar cannot read as a macro is used
	// (FMT_CONSTEXPR void f(), _In_ const char* p, LIST_FOREACH(x, l, link)); the function walk reads that copy for the
	// functions it could not read as written. None for a language without a preprocessor.
	Macros []MacroRule
	// Declarations is the query that reads the top-level names a file declares, run only on the files a change names,
	// with these captures:
	//   @declared.function, @declared.type, @declared.value  the name of a top-level function, type or value
	//   @exported a declared name the file exports: public to code outside it
	//   @implicit a declared name a runtime or framework calls without its being written (a default export, a
	//             decorated class); its declaration is never listed
	Declarations string
	// Units is the query of the comparison units of the blast radius, read with Declarations (units.go).
	Units string
	// LayoutCode says which of a file's layout is code, for comparing a changed file's units.
	LayoutCode LayoutCode
	// Peers says which files use the file's declarations without an import of it.
	Peers Peers
	// Loaded are the files a framework of the language loads by convention: their declarations are no added code.
	Loaded []Convention
	// Library says when a file is part of a published library, whose exported declarations are no added code; nil
	// when the language has none.
	Library *Library
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
// whose value is an array of strings or one string, or a key of an INI section whose value holds the values
// separated as a shell separates arguments.
type Declaration struct {
	Config
	Key      string
	Means    LayoutKind
	Tool     string   // the tool that reads File, as a sentence names it
	Shadowed []Config // configurations the tool reads instead of this one, the one it reads first first
	Rival    string   // a TOML table that, standing beside Table in File, makes the tool refuse the file; "" for none
}

// Config is a section of a configuration file at the repository root. Always is true for a file the tool reads
// whenever it is tracked, whatever it holds; Present for a section the tool reads whenever it is there, even empty;
// otherwise the tool reads it only when the section holds a key of its own.
type Config struct {
	File    string
	Table   string // the TOML table, as its header names it, or the INI section
	INI     bool
	Always  bool
	Present bool
}

// Default returns the rules of the first version.
func Default() Rules {
	return Rules{
		NotCode: []string{
			".md", ".mdx", ".rst", ".txt", ".json", ".arb", ".properties", ".pdf", ".yaml", ".yml", ".toml", ".ini", ".cfg", ".lock", ".csv", ".tsv",
			".svg", ".xml", ".html", ".css", ".snap", ".golden",
			"", ".mod", ".sum", ".example", ".crt", ".pem", ".key", ".diff", ".patch", ".sha256",
			".resolved", ".natvis", ".csproj", ".props", ".targets", ".sln", ".slnx", ".config", ".Config", ".DotSettings", ".resx",
		},
		SkippedFolders:     []string{"node_modules", "dist", "build", "vendor", "migrations", ".venv", "coverage", "testdata", "__pycache__", ".dart_tool", "META-INF", "single_include"},
		OtherFolders:       []string{"scripts", "examples", ".github", "docs"},
		DocFolders:         []string{"docs"},
		DocSuffixes:        []string{".md", ".mdx"},
		TestFolders:        []string{"tests", "Tests", "test", "__tests__", "spec", "integration_test"},
		TestSourceSets:     []string{"test[A-Z]*", "*Test*"},
		TestFolderPatterns: []string{"*.Tests", "*.Test"},
		GeneratedNames:     []string{"*.g.cs", "*.Designer.cs"},
		TestEndings:        []string{".test", ".spec", "_test", "-test"},
		TestBeginnings:     []string{"test_"},
		TestDefinition:     testDefinition,
		Generated:          generated,
		Manifests: []Manifest{
			{File: "package.json", Format: JSONDependencies, Lockfiles: []string{"package-lock.json", "pnpm-lock.yaml", "yarn.lock"}, Name: npmName},
			{File: "go.mod", Format: GoModRequire, Lockfiles: []string{"go.sum"}, Name: goModule},
			{File: "pyproject.toml", Format: PyprojectDependencies, Lockfiles: []string{"uv.lock", "poetry.lock"}, Name: pythonName},
			{File: "pubspec.yaml", Format: PubspecDependencies, Lockfiles: []string{"pubspec.lock"}, Name: pubName},
			{File: "pom.xml", Format: PomDependencies, Name: coordinate},
			{File: "build.gradle", Format: GradleDependencies, Name: gradleName, Reads: []string{GradleCatalogue}},
			{File: "build.gradle.kts", Format: GradleDependencies, Name: gradleName, Reads: []string{GradleCatalogue}},
			{File: "Cargo.toml", Format: CargoDependencies, Lockfiles: []string{"Cargo.lock"}, Name: crateName},
			{File: dotnetProject, Format: CsprojDependencies, Name: nugetID},
			{File: "Directory.Build.props", Format: CsprojDependencies, Name: nugetID},
			{File: "Directory.Build.targets", Format: CsprojDependencies, Name: nugetID},
			{File: "Package.swift", Format: SwiftDependencies, Lockfiles: []string{"Package.resolved"}, Name: swiftID},
			// a manifest for one Swift toolchain version; which one a build picks is the toolchain's choice
			{File: "Package@swift-*.swift", Format: SwiftDependencies, Lockfiles: []string{"Package.resolved"}, Name: swiftID},
			{File: "vcpkg.json", Format: VcpkgDependencies, Recipes: []string{"ports"}, Name: vcpkgPort},
		},
		OwnFile:           ".plimsoll.toml",
		LimitsTable:       "limits",
		AcknowledgedTable: "acknowledged", AcknowledgedName: "acknowledged items",
		OwnKeys: []OwnKey{
			{Table: "layout", Key: "test_folders", Name: "test folders", Holds: TestFolderPaths},
			{Table: "layout", Key: "test_files", Name: "test files", Holds: TestFileNames},
			{Table: "layout", Key: "planning_folders", Name: "planning folders", Holds: PlanningFolderPaths},
			{Table: "layout", Key: "planning_files", Name: "planning files", Holds: PlanningFileNames},
			{Table: "defaults", Key: "base", Name: "default base", Holds: BaseName},
			{Table: "ignore", Key: "folders", Name: "ignored folders", Holds: IgnoredFolderPaths},
			{Table: "ignore", Key: "files", Name: "ignored files", Holds: IgnoredFileNames},
			{Table: "imports", Key: "never", Name: "`never` import rules", Holds: NeverImportRules},
			{Table: "imports", Key: "only", Name: "`only` import rules", Holds: OnlyImportRules},
		},
		LimitKeys: []LimitKey{
			{Key: "file_lines", Name: "file-size limit", Unit: "lines"},
			{Key: "test_ratio", Name: "test-ratio limit"},
			{Key: "shared_commits", Name: "co-change limit", Unit: "shared commits"},
			{Key: "hotspot_files", Name: "hotspot count", Unit: "files"},
		},
		// Each place an agent tool writes plans to is backed by the tool's own documentation (docs/rules.md); GSD's
		// folders are those its pull-request filter leaves out of review, not its lasting project files.
		PlanningPaths: []string{
			"docs/superpowers/", "docs/stories/", "docs/plans/", "docs/specs/", ".claude/plans/", ".cursor/plans/",
			".planning/phases/", ".planning/quick/", ".planning/research/", ".planning/threads/", ".planning/todos/",
			".planning/debug/", ".planning/seeds/", ".planning/codebase/", ".planning/ui-reviews/",
			"REQUIREMENTS.md", "SPEC.md", "PLAN.md", "*_PLAN.md",
		},
		MeasurementPaths: []string{"docs/qualification/", "evidence/"},
		NoteNames:        defaultNoteNames(),
		ComposeFiles:     []string{"compose*.yml", "compose*.yaml", "docker-compose*.yml", "docker-compose*.yaml"},
		// A commit that touches more files says little about which files belong together (requirements, R-16).
		CommitPaths: 20,
		Additions:   defaultAdditions(),
		// Seven outside reviewers judged real answers per kind of mark (2026-11-17); these they judged too noisy for
		// status and worth keeping in report. A file crossing its line limit was worth acting on in 45 of 150 agent pull
		// requests read blind (2026-11-18), under the bar of one in two.
		ReportOnly: []string{
			"shape.file-size", "shape.settings", "process.doc-churn", "settings.compose-only",
			"co-change.pair", "links.test-import", "links.folder-cycle", "shape.function-nesting",
		},
		Areas: []Area{
			{Name: "size", Kinds: []string{"shape.test-ratio", "shape.file-size"}},
			{Name: "dependencies", Kinds: []string{"shape.dependencies"}},
			{Name: "wiring", Kinds: []string{"links.cycle", "links.never", "links.only", "links.test-import", "links.folder-cycle", "co-change.pair"}},
			{Name: "functions", Kinds: []string{"shape.function-complexity", "shape.function-nesting"}},
			{Name: "settings", Kinds: []string{"settings.defined-twice", "settings.compose-only", "shape.settings"}},
			{Name: "process", Kinds: []string{"process.planning-file", "process.limit-change", "process.declaration-change", "process.doc-churn"}},
		},
		FunctionRules: defaultFunctionRules(),
		Limits: Limits{
			FileLines:          Limit{Value: 1000, Step: 1, Source: "plimsoll's default, the same limit as Pylint's and SonarQube's"},
			FunctionComplexity: Limit{Value: 25, Step: 8, Source: "plimsoll's rule, read blind on agent pull requests; measured with SonarSource's cognitive complexity metric"},
			FunctionNesting:    Limit{Value: 4, Step: 1, Source: "CodeScene's threshold for deep nesting"},
			TestRatio:          Limit{Value: 2.0, Step: 0.1, Source: "plimsoll's rule: test code at most twice the product code"},
			// The requirements set the floor for a pair of files that changed together (R-16).
			SharedCommits: Limit{Value: 5, Step: 1, Source: "plimsoll's rule"},
			// Read blind on 1,180 changes, growth by one shared commit was most pair marks and rarely worth a look.
			PairShareRise: Limit{Value: 0.1, Source: "plimsoll's rule: one more shared commit is not growth"},
			// The requirements name the three files changed most often times their size (R-15).
			HotspotFiles: Limit{Value: 3, Step: 1, Source: "the three files changed most often times their size"},
			// The share of own imports not followed was 1.4% on the BFF and 0% on plimsoll, where the links equalled those
			// of dependency-cruiser and go list; 10% leaves room for code that embeds program text in strings.
			ImportsNotFollowed: Limit{Value: 0.10, Source: "links checked against go list and dependency-cruiser, 2026-11"},
			RadiusPaths:        Limit{Value: 18, Source: "the smallest number that lists three in four of 170 field radii in full"},
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

// MacroRule rewrites what its pattern's group "macro" finds: blanked when As is "", else replaced by As padded with
// spaces (As is never longer than what it replaces). Offsets and line breaks stay where they were.
type MacroRule struct {
	Pattern *regexp.Regexp
	As      string
}

// Area is one area of the headline: its name and the kinds of mark it holds, as "<detector>.<rule>". A kind that is
// not report-only decides the area's state.
type Area struct {
	Name  string
	Kinds []string
}

// TestFileFolder is a folder in which only the files whose names match one of Names are test code.
type TestFileFolder struct {
	Folder string
	Names  []string
}

// PytestFiles are the file names pytest collects as tests when its configuration sets no python_files, and the
// file it always reads.
func PytestFiles() []string { return []string{"test_*.py", "*_test.py", "conftest.py"} }

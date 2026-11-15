package conventions

import "regexp"

// Imports says how a language writes imports and what an import of the project's own code names.
type Imports struct {
	Statements   []ImportForm   // the forms an import statement takes; every match is one import
	Until        *regexp.Regexp // imports are read only before its first match; nil reads the whole file
	Computed     ImportForm     // an import whose target is built at run time; its Pattern is nil when the language has none
	PassesOn     *regexp.Regexp // matches the whole content of a file that only passes imports on; nil for none
	Relative     RelativeForm   // how a relative import is written
	Prefix       Prefix         // where the project's own prefix is declared
	Target       TargetKind     // what an own import names
	Candidates   Candidates     // for a file target: the tracked paths an import may mean
	FolderExcept *regexp.Regexp // for a folder target: file names an import of the folder does not reach
	NoRings      string         // why import rings are not looked for among its files; "" looks for them
	Implicit     []string       // file names the language runs without an import of them
	// FolderAlso, for a folder target, names extensions of other entries' files that share its folders; an import of
	// the folder reaches them too.
	FolderAlso []string
	// PackageLine, for SourceRoots, is the declaration of a file's package; its group "package" holds the dotted name.
	PackageLine *regexp.Regexp
	// TypeOnlyBlock matches a block whose imports are of types only; a statement whose target starts inside one is
	// type-only. Its Pattern is nil for a language without such blocks.
	TypeOnlyBlock ImportForm
}

// ImportForm is a pattern, tried only on text that holds one of its words. As a statement form, its named group
// "spec" holds the target as written; its optional named group "type" takes part only in a statement that imports
// types alone; its optional named group "names" holds the names a statement imports out of its target, each of
// which may itself be a module under the target, tried before the target.
type ImportForm struct {
	Pattern *regexp.Regexp
	Words   []string // the pattern is tried only on content that holds one of these; nil tries every file
}

// RelativeForm is how a language writes an import relative to the importing file.
type RelativeForm int

const (
	// NoRelative is a language whose imports are never relative.
	NoRelative RelativeForm = iota + 1
	// DotSlash is "./x" and "../x", starting at the importing file's folder.
	DotSlash
	// LeadingDots is ".x" and "..x": the importing file's folder, one folder up per dot after the first.
	LeadingDots
	// NoScheme is any target without a scheme ("x.dart", "../x.dart"), starting at the importing file's folder.
	NoScheme
)

// TargetKind is what an import of the project's own code names.
type TargetKind int

const (
	// FileTarget is an import that names one file.
	FileTarget TargetKind = iota + 1
	// FolderTarget is an import that names a folder, whose files it reaches.
	FolderTarget
)

// Prefix is where the project's own prefix is declared, and how it is read: a file at the repository root, or, for
// OwnPackages, the packages and modules under the roots. OwnPackages reads Candidates.Add as what makes a file a
// module and Candidates.InFolder as what makes a folder a package; an import that names none of them may still mean
// a module beside the importing file when that file's folder is no package.
type Prefix struct {
	File     string   // read at the repository root; "" when the language has none
	Roots    []string // for OwnPackages: the folders whose packages and modules are the project's own
	Format   PrefixFormat
	Required bool // without it the language's own imports cannot be told from outside ones, so no link numbers
	Members  bool // the prefix file in other folders is read too: the members of a workspace the root file lists
}

// PrefixFormat is how a prefix file declares the project's own imports.
type PrefixFormat int

const (
	// GoModule is the module line of go.mod.
	GoModule PrefixFormat = iota + 1
	// TSConfigPaths are the paths aliases of tsconfig.json.
	TSConfigPaths
	// OwnPackages are the packages and modules found under the prefix's roots in the tracked paths.
	OwnPackages
	// PubspecName is the name of a pubspec.yaml: "package:<name>/x" is the file x under lib/.
	PubspecName
	// SourceRoots are the packages that files under the source roots declare. Roots holds the last folders of a
	// source root as a pattern, each part a name or "*": "src/*/java" is a folder java in any folder directly in a
	// folder src, at any depth.
	SourceRoots
)

// Candidates are the tracked paths a file import may mean when the path as written is not a tracked file.
type Candidates struct {
	Swap     []Swap   // an extension as written, and the extensions a source file may carry in its place
	Add      []string // extensions added to the path as written; for OwnPackages also what makes a file a module
	InFolder []string // file names tried inside the path, when it names a folder; for OwnPackages also what makes a folder a package
}

// Swap is an extension as written and the extensions a source file may carry in its place.
type Swap struct {
	From string
	To   []string
}

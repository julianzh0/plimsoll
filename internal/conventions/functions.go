package conventions

import "regexp"

// FunctionKinds names, by the kinds of a grammar's nodes, what a function is and what counts in it.
type FunctionKinds struct {
	Declared      []string // kinds that declare a function when they hold a body
	Bodies        []string // kinds of a function's body
	BodyAfter     bool     // the body is the node after the signature, not a child of it (Dart)
	Closures      []string // functions written inside an expression: measured on their own, never counted in the one around
	NamedClosures []string // closures that are listed when a NamedBy parent names them outside every function
	NamedBy       []string // parents that give such a closure its name
	Ifs           []string
	ElseClauses   []string // a node that holds an else branch, where the grammar has one
	FlatBranches  []string // branches that count 1 without nesting (Python's elif)
	Nesting       []string // loops, switches, catches, conditional expressions: 1 plus their nesting, and one level deeper inside
	Logic         []string // boolean operator tokens
	Parentheses   []string // kinds a tree of boolean operators continues through
	Markup        []string // kinds inside which nothing counts
	DepthIfs      []string // for the nesting measure: the if kinds
	Depth         []string // for the nesting measure: the other kinds that open a level
	Blocks        []string // kinds whose named children are a block's statements
	SwitchBlocks  []string // of Blocks, those whose children count only when their kind ends in "statement" or "declaration"
	Declarators   []string // kinds the name search descends through to the innermost one (C, C++: int *f(), T& A::f())
	Names         []string // kinds that name a function besides identifiers (C++: ~Foo, operator+)
}

// FunctionRules are the shared rules of the function marks: what is left out, and every number but the two limits.
type FunctionRules struct {
	BlockStatements, BlockTokens      int     // a repeated block: at least this many sibling statements and tokens
	Run                               int     // tokens in one compared run
	MovedShare, CopyShare             float64 // the share of a function's tokens found moved, or copied, that leaves it out
	ExtractedShare, LikeBody          float64 // a new function with fewer added lines than this share is extracted; bodies this alike match
	CopyFiles, LongNames              int     // files compared per copy search; long names searched for
	LongNameLength, NameLength        int     // a long name has at least this many characters; a shorter function name is not searched
	PerMeasure                        int     // items per measure and change
	MinifiedLineBytes                 int     // a file whose lines are longer than this on average is minified
	BenchmarkWords                    []string
	SkippedFolders                    []string // folders whose functions are not measured, beside Additions.SkippedFolders
	VendorFolders                     []string // folders that hold another project's code when they also hold a licence file
	VendorLicence, RootLicence        *regexp.Regexp
	HeaderLines, NearFiles, NearAgree int // the lines of a header read; base files compared for the own holder; how many must agree
	Copyright, LicensedTo, PartOf     *regexp.Regexp
	CodeHost, Ported                  *regexp.Regexp
	HolderStopWords                   []string
}

func defaultFunctionRules() FunctionRules {
	return FunctionRules{
		BlockStatements: 4, BlockTokens: 40, Run: 10,
		MovedShare: 0.70, CopyShare: 0.70, ExtractedShare: 0.5, LikeBody: 0.6,
		CopyFiles: 20, LongNames: 4, LongNameLength: 8, NameLength: 3, PerMeasure: 3, MinifiedLineBytes: 110,
		BenchmarkWords: []string{"bench", "benches", "benchmark", "benchmarks"},
		SkippedFolders: []string{"scripts", "docs"},
		VendorFolders:  []string{"third_party", "third-party", "thirdparty", "3rdparty", "vendor", "external", "upstream"},
		VendorLicence:  regexp.MustCompile(`(?i)^(?:license|licence|copying)`),
		RootLicence:    regexp.MustCompile(`(?i)^(?:license|licence|copying|notice)`),
		HeaderLines:    40, NearFiles: 300, NearAgree: 3,
		// a notice starts its comment line: prose that speaks of copyright is no notice
		Copyright:  regexp.MustCompile(`(?i)^[\s/*#!;\-'"]*@?(?:copyright\b|©|\(c\)\s*\d{4}|spdx-filecopyrighttext)`),
		LicensedTo: regexp.MustCompile(`(?i)licensed to (.+?) under`),
		PartOf:     regexp.MustCompile(`(?i)this file is part of (.+)`),
		CodeHost:   regexp.MustCompile(`https?://(?:www\.)?(?:github\.com|gitlab\.com|bitbucket\.org|gist\.github\.com|raw\.githubusercontent\.com|codeberg\.org|sourceforge\.net|[^/\s]*googlesource\.com)/([^/\s)"'>]+)/([^/\s#?)"'>]+)`),
		Ported:     regexp.MustCompile(`(?i)\b(ported from|copied from)\b`),
		HolderStopWords: []string{"copyright", "c", "all", "rights", "reserved", "by", "the", "and", "inc", "llc", "ltd", "gmbh",
			"corp", "co", "authors", "author", "contributors", "contributor", "project", "licensed", "to", "under", "this", "file",
			"is", "part", "of", "its", "affiliates", "other", "others", "et", "al", "see", "for", "details", "license", "licence",
			"spdx", "identifier"},
	}
}

// The string forms the token patterns are built from.
const (
	doubleQuoted = `"(?:\\.|[^"\\\n])*"`
	singleQuoted = `'(?:\\.|[^'\\\n])*'`
	character    = `'(?:\\[^'\n]{1,10}|[^'\\\n])'`
	backQuoted   = "`(?:\\\\.|[^`\\\\])*`"
	tripleQuoted = `"""[\s\S]*?"""`
)

// cLikeTokens splits code with // and /* */ comments into tokens; literals are the language's string forms.
func cLikeTokens(literals string) *regexp.Regexp {
	return regexp.MustCompile(`(?P<ws>\s+)|(?P<com>//[^\n]*|/\*[\s\S]*?(?:\*/|\z))|(?P<lit>` + literals +
		`|[0-9][\pL\pN_.]*|\.[0-9][\pL\pN_]*)|(?P<id>[A-Za-z_$][\pL\pN_$]*)|(?P<p>.)`)
}

var pythonTokens = regexp.MustCompile(`(?P<ws>\s+)|(?P<com>#[^\n]*)|(?P<lit>(?i:[rbuf]{0,2})(?:"""[\s\S]*?"""|'''[\s\S]*?'''|"(?:\\.|[^"\\\n])*"|'(?:\\.|[^'\\\n])*')|[0-9][\pL\pN_.]*|\.[0-9][\pL\pN_]*)|(?P<id>[A-Za-z_][\pL\pN_]*)|(?P<p>.)`)

// NOTE: Rust raw strings are matched with up to three `#`, since Go's regexp has no back-reference; a raw string with
// four or more is split into ordinary tokens. Upgrade: a small scanner for Rust literals.
const rustLiterals = `b?r###"[\s\S]*?"###|b?r##"[\s\S]*?"##|b?r#"[\s\S]*?"#|b?r"[\s\S]*?"|b?` + doubleQuoted + `|b?` + character

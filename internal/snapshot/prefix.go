package snapshot

import (
	"bytes"
	"encoding/json"
	"path"
	"slices"
	"strings"

	"github.com/juzhxng/plimsoll/internal/conventions"
)

// PrefixState is what became of a language's prefix file on one side.
type PrefixState int

const (
	// PrefixRead is a prefix file that was read, or a language that names none.
	PrefixRead PrefixState = iota + 1
	// PrefixAbsent is a prefix file that is not tracked at the root.
	PrefixAbsent
	// PrefixUnreadable is a prefix file that declares no prefix or could not be decoded.
	PrefixUnreadable
	// PrefixExtends is a tsconfig.json that extends another file; its own paths are still used.
	PrefixExtends
)

// byteOrderMark is skipped at the start of a source file or a config.
const byteOrderMark = "\uFEFF"

// ownImports gives the paths from the root that an import that is not relative may mean, when it is the project's
// own; no path means outside. catchAll marks a match that would take any name, so finding no file makes it outside,
// not "not found".
// within are the inline modules the statement stands in, the outermost first; only the Rust reader reads them.
type ownImports func(from, spec string, within []string) (bases []string, catchAll bool)

// prefix is what one side's prefix file declares about the project's own imports.
type prefix struct {
	state PrefixState
	own   ownImports // nil when no prefix was read: every import that is not relative is outside
}

// readPrefix reads a language's prefix at the root of one side: its file, its own packages, or, from the readings of the
// language's files, the modules they declare. Each format has one
// reader; resolving an import needs only the function it returns.
// NOTE: only the root config is read; an extended or nested config, replace, go.work and workspaces are not followed,
// so imports through them count as outside. Upgrade: follow extends through tracked files.
func readPrefix(rules conventions.Rules, language conventions.Language, content map[string][]byte, tracked map[string]bool, readings map[string]reading) prefix {
	imports := language.Imports
	fromFile := func(parse func([]byte) prefix) prefix {
		file, present := content[imports.Prefix.File]
		switch {
		case !present:
			return prefix{state: PrefixAbsent}
		case file == nil:
			return prefix{state: PrefixUnreadable}
		}
		return parse(file)
	}
	switch imports.Prefix.Format {
	case conventions.GoModule:
		return fromFile(goModule)
	case conventions.TSConfigPaths:
		return fromFile(func(file []byte) prefix { return tsconfigPaths(rules, file) })
	case conventions.OwnPackages:
		return ownPackages(imports, tracked)
	case conventions.SourceRoots:
		return sourcePackages(rules, language, content, tracked)
	case conventions.PubspecName:
		return fromFile(func(file []byte) prefix { return pubspecPackages(file, content) })
	case conventions.DeclaredNamespaces:
		return declaredNamespaces(language, content, tracked)
	case conventions.CargoCrates:
		return cargoCrates(imports.Prefix.File, content, tracked, readings)
	}
	return prefix{state: PrefixRead} // the language declares no prefix
}

// goModule reads the module path from the first line that starts with "module". An import is own when it is the
// module path or lies under it; its path is the rest.
func goModule(content []byte) prefix {
	for line := range strings.Lines(string(content)) {
		rest, isModule := strings.CutPrefix(strings.TrimSpace(line), "module")
		if !isModule {
			continue
		}
		rest, _, _ = strings.Cut(rest, "//")
		module := strings.Trim(strings.TrimSpace(rest), `"`)
		if module == "" {
			break
		}
		return prefix{state: PrefixRead, own: func(_, spec string, _ []string) ([]string, bool) {
			rest, under := strings.CutPrefix(spec, module)
			if !under || rest != "" && !strings.HasPrefix(rest, "/") {
				return nil, false
			}
			return []string{path.Join(".", strings.TrimPrefix(rest, "/"))}, false
		}}
	}
	return prefix{state: PrefixUnreadable}
}

// tsconfigPaths reads the paths aliases and baseUrl of a tsconfig.json, after its comments and trailing commas.
func tsconfigPaths(rules conventions.Rules, content []byte) prefix {
	var config struct {
		Extends         json.RawMessage `json:"extends"`
		CompilerOptions struct {
			BaseURL string              `json:"baseUrl"`
			Paths   map[string][]string `json:"paths"`
		} `json:"compilerOptions"`
	}
	if err := json.Unmarshal(plainJSON(content), &config); err != nil {
		return prefix{state: PrefixUnreadable}
	}
	baseURL := "."
	if config.CompilerOptions.BaseURL != "" {
		baseURL = path.Clean(config.CompilerOptions.BaseURL)
	}
	state := PrefixRead
	if len(config.Extends) > 0 && string(config.Extends) != "null" {
		state = PrefixExtends
	}
	aliases := config.CompilerOptions.Paths
	return prefix{state: state, own: func(_, spec string, _ []string) ([]string, bool) {
		pattern, star, matched := bestAlias(aliases, spec)
		if !matched {
			return nil, false
		}
		var bases []string
		for _, target := range aliases[pattern] {
			base := path.Join(baseURL, strings.Replace(target, "*", star, 1))
			if !leaves(base) && !inFolders(base, rules.SkippedFolders) {
				bases = append(bases, base)
			}
		}
		return bases, strings.HasPrefix(pattern, "*")
	}}
}

// bestAlias picks the paths pattern for an import: an exact pattern first, else the one with the longest text before
// its `*`, ties by the pattern. star is the text the `*` stands for.
func bestAlias(aliases map[string][]string, spec string) (pattern, star string, matched bool) {
	if _, exact := aliases[spec]; exact && !strings.Contains(spec, "*") {
		return spec, "", true
	}
	longest := -1
	for candidate := range aliases {
		before, after, hasStar := strings.Cut(candidate, "*")
		if !hasStar || len(spec) < len(before)+len(after) || !strings.HasPrefix(spec, before) || !strings.HasSuffix(spec, after) {
			continue
		}
		if len(before) > longest || len(before) == longest && candidate < pattern {
			pattern, longest, star = candidate, len(before), spec[len(before):len(spec)-len(after)]
		}
	}
	return pattern, star, longest >= 0
}

// plainJSON drops // and /* */ comments and the trailing commas before a closing brace or bracket, outside strings, in
// one pass: what tsconfig.json allows and encoding/json does not.
func plainJSON(content []byte) []byte {
	out := make([]byte, 0, len(content))
	inString := false
	for i := 0; i < len(content); i++ {
		c := content[i]
		switch {
		case inString:
			out = append(out, c)
			if c == '\\' && i+1 < len(content) {
				i++
				out = append(out, content[i])
			} else if c == '"' {
				inString = false
			}
		case c == '"':
			inString = true
			out = append(out, c)
		case c == '/' && i+1 < len(content) && content[i+1] == '/':
			for i+1 < len(content) && content[i+1] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(content) && content[i+1] == '*':
			end := bytes.Index(content[i+2:], []byte("*/"))
			if end < 0 {
				return out
			}
			i += end + 3
		case c == ',' && closesNext(content[i+1:]):
		default:
			out = append(out, c)
		}
	}
	return out
}

// closesNext says whether a closing brace or bracket comes next, after white space and comments.
func closesNext(rest []byte) bool {
	for {
		rest = bytes.TrimLeft(rest, " \t\r\n")
		switch {
		case bytes.HasPrefix(rest, []byte("//")):
			end := bytes.IndexByte(rest, '\n')
			if end < 0 {
				return false
			}
			rest = rest[end:]
		case bytes.HasPrefix(rest, []byte("/*")):
			end := bytes.Index(rest[2:], []byte("*/"))
			if end < 0 {
				return false
			}
			rest = rest[end+4:]
		default:
			return len(rest) > 0 && (rest[0] == '}' || rest[0] == ']')
		}
	}
}

// leaves says whether a cleaned path from the root leaves the repository.
func leaves(p string) bool {
	return p == ".." || strings.HasPrefix(p, "../")
}

// NamedPaths cleans paths a caller names, relative to the repository's top folder, sorts them and drops repeats.
// ok is false when one is absolute or leaves the repository: it cannot be a tracked path.
func NamedPaths(paths []string) (named []string, ok bool) {
	for _, p := range paths {
		cleaned := path.Clean(p)
		if path.IsAbs(cleaned) || leaves(cleaned) {
			return nil, false
		}
		named = append(named, cleaned)
	}
	slices.Sort(named)
	return slices.Compact(named), true
}

// ownPackages finds the project's own names under each root of the prefix, in one side's tracked paths. A regular
// name is a folder that directly holds a package file, or a module file; a loose name is any other folder that holds
// a module file at some depth. An import whose first name is the project's own may mean that path under each root
// that holds the name; failing that, a file beside the importing one, when its folder is no package.
func ownPackages(imports conventions.Imports, tracked map[string]bool) prefix {
	const loose, regular = 1, 2
	isModule := func(p string) bool { return slices.Contains(imports.Candidates.Add, conventions.Suffix(p)) }
	names := map[string]map[string]int{}
	found := false
	for _, root := range imports.Prefix.Roots {
		names[root] = map[string]int{}
		for p := range tracked {
			rel, under := p, root == "."
			if !under {
				rel, under = strings.CutPrefix(p, root+"/")
			}
			if !under || !isModule(rel) {
				continue
			}
			first, rest, nested := strings.Cut(rel, "/")
			switch {
			case !nested:
				names[root][strings.TrimSuffix(rel, conventions.Suffix(rel))], found = regular, true
			case slices.Contains(imports.Candidates.InFolder, rest):
				names[root][first], found = regular, true
			case names[root][first] == 0:
				names[root][first] = loose
			}
		}
	}
	if !found {
		return prefix{state: PrefixAbsent}
	}
	isPackage := func(folder string) bool {
		return slices.ContainsFunc(imports.Candidates.InFolder, func(name string) bool { return tracked[path.Join(folder, name)] })
	}
	return prefix{state: PrefixRead, own: func(from, spec string, _ []string) ([]string, bool) {
		first, _, _ := strings.Cut(spec, ".")
		asPath := dottedPath(spec)
		var bases []string
		isRegular := false
		for _, root := range imports.Prefix.Roots {
			kind := names[root][first]
			if kind == 0 {
				continue
			}
			isRegular = isRegular || kind == regular
			bases = append(bases, path.Join(root, asPath))
		}
		if len(bases) > 0 {
			return bases, !isRegular
		}
		if folder := path.Dir(from); !isPackage(folder) {
			return []string{path.Join(folder, asPath)}, true
		}
		return nil, false
	}}
}

// dottedPath is a dotted module name as a path: every "." a "/".
func dottedPath(name string) string { return strings.ReplaceAll(name, ".", "/") }

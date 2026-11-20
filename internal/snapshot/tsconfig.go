package snapshot

import (
	"cmp"
	"encoding/json"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// unreadableTarget is the base a prefix gives for an import that is the project's own but whose target has no tracked
// source: the import counts as not readable.
const unreadableTarget = "\x00unreadable"

// tsConfig is one tracked tsconfig file with what it inherits along its extends chain: each option with the folder of
// the file that set it.
type tsConfig struct {
	file                    string
	baseURL                 string // a folder, "" when none in the chain sets it
	paths                   map[string][]string
	pathsFolder             string // the folder of the file that set paths
	outDir, rootDir         string // folders, "" when unset
	files, include, exclude []string
	filesSet, includeSet    bool
	excludeSet              bool
	references              []string // the config files it references
	baseUnread              bool     // an extends names no tracked file
}

// tsConfigJSON is what plimsoll reads of a tsconfig.json, after its comments and trailing commas.
type tsConfigJSON struct {
	Extends         json.RawMessage `json:"extends"`
	CompilerOptions struct {
		BaseURL *string             `json:"baseUrl"`
		Paths   map[string][]string `json:"paths"`
		OutDir  *string             `json:"outDir"`
		RootDir *string             `json:"rootDir"`
	} `json:"compilerOptions"`
	Files      *[]string `json:"files"`
	Include    *[]string `json:"include"`
	Exclude    *[]string `json:"exclude"`
	References []struct {
		Path string `json:"path"`
	} `json:"references"`
}

// tsConfigs reads tracked tsconfig files on demand, each once, its extends chain followed through tracked files.
type tsConfigs struct {
	content map[string][]byte
	tracked map[string]bool
	read    map[string]*tsConfig // nil for a file that is no readable config
}

// load reads a config and its chain: the bases of extends, one text or a list, in order, a later one replacing an
// earlier, the extending file replacing both. A base that names no tracked file is not read; its own options stay.
func (c *tsConfigs) load(file string, chain []string) *tsConfig {
	if loaded, done := c.read[file]; done || slices.Contains(chain, file) {
		return loaded
	}
	var raw tsConfigJSON
	if err := json.Unmarshal(plainJSON(c.content[file]), &raw); err != nil {
		c.read[file] = nil
		return nil
	}
	folder := path.Dir(file)
	merged := &tsConfig{file: file}
	var bases []string
	if json.Unmarshal(raw.Extends, &bases) != nil {
		var one string
		if json.Unmarshal(raw.Extends, &one) == nil && one != "" {
			bases = []string{one}
		}
	}
	for _, base := range bases {
		at, found := c.extended(folder, base)
		inherited := c.load(at, append(chain, file))
		if !found || inherited == nil {
			merged.baseUnread = true
			continue
		}
		merged.override(inherited)
		merged.baseUnread = merged.baseUnread || inherited.baseUnread
	}
	own := tsConfig{}
	options := raw.CompilerOptions
	if options.BaseURL != nil {
		own.baseURL = path.Join(folder, *options.BaseURL)
	}
	if options.Paths != nil {
		own.paths, own.pathsFolder = options.Paths, folder
	}
	if options.OutDir != nil {
		own.outDir = path.Join(folder, *options.OutDir)
	}
	if options.RootDir != nil {
		own.rootDir = path.Join(folder, *options.RootDir)
	}
	relative := func(patterns []string) []string {
		var out []string
		for _, p := range patterns {
			out = append(out, path.Join(folder, p))
		}
		return out
	}
	if raw.Files != nil {
		own.files, own.filesSet = relative(*raw.Files), true
	}
	if raw.Include != nil {
		own.include, own.includeSet = relative(*raw.Include), true
	}
	if raw.Exclude != nil {
		own.exclude, own.excludeSet = relative(*raw.Exclude), true
	}
	merged.override(&own)
	for _, r := range raw.References {
		if at, found := c.extended(folder, r.Path); found {
			merged.references = append(merged.references, at)
		}
	}
	c.read[file] = merged
	return merged
}

// override takes every option the other config sets.
func (cfg *tsConfig) override(other *tsConfig) {
	cfg.baseURL = cmp.Or(other.baseURL, cfg.baseURL)
	cfg.outDir, cfg.rootDir = cmp.Or(other.outDir, cfg.outDir), cmp.Or(other.rootDir, cfg.rootDir)
	if other.paths != nil {
		cfg.paths, cfg.pathsFolder = other.paths, other.pathsFolder
	}
	if other.filesSet {
		cfg.files, cfg.filesSet = other.files, true
	}
	if other.includeSet {
		cfg.include, cfg.includeSet = other.include, true
	}
	if other.excludeSet {
		cfg.exclude, cfg.excludeSet = other.exclude, true
	}
}

// extended is the tracked config a relative extends or reference names: the file, the file with .json, or the
// tsconfig.json of the folder; a package name is no tracked file.
func (c *tsConfigs) extended(folder, name string) (string, bool) {
	if !strings.HasPrefix(name, ".") && !strings.HasPrefix(name, "/") {
		return "", false
	}
	at := path.Join(folder, name)
	for _, candidate := range []string{at, at + ".json", path.Join(at, "tsconfig.json")} {
		if c.tracked[candidate] {
			return candidate, true
		}
	}
	return "", false
}

// takes says whether a config takes a file: listed in its files, or matched by its include patterns (every file when
// it has neither files nor include) and by none of its exclude patterns.
func (cfg *tsConfig) takes(file string) bool {
	if slices.Contains(cfg.files, file) {
		return true
	}
	include := cfg.include
	if !cfg.filesSet && !cfg.includeSet {
		include = []string{path.Join(path.Dir(cfg.file), "**/*")}
	}
	return slices.ContainsFunc(include, func(p string) bool { return globMatches(p, file) }) &&
		!slices.ContainsFunc(cfg.exclude, func(p string) bool { return globMatches(p, file) })
}

// configOf is a file's config: the nearest tracked tsconfig.json at or above it when it takes the file; else, when that
// one references others, the one referenced config that takes it; nil when none or several do.
func (c *tsConfigs) configOf(file string) *tsConfig {
	for folder := range foldersUp(path.Dir(file)) {
		at := path.Join(folder, "tsconfig.json")
		if !c.tracked[at] {
			continue
		}
		cfg := c.load(at, nil)
		if cfg == nil || cfg.takes(file) {
			return cfg
		}
		var taking []*tsConfig
		for _, ref := range cfg.references {
			if r := c.load(ref, nil); r != nil && r.takes(file) {
				taking = append(taking, r)
			}
		}
		if len(taking) == 1 {
			return taking[0]
		}
		return nil
	}
	return nil
}

// globMatches matches a tsconfig pattern (*, ** and ?; a pattern without a wildcard or a suffix names a folder and
// everything below it) against a path, segment by segment.
func globMatches(pattern, file string) bool {
	if !strings.ContainsAny(pattern, "*?") && path.Ext(pattern) == "" {
		pattern = path.Join(pattern, "**/*")
	}
	var match func(p, f []string) bool
	match = func(p, f []string) bool {
		for len(p) > 0 {
			if p[0] == "**" {
				for i := 0; i <= len(f); i++ {
					if match(p[1:], f[i:]) {
						return true
					}
				}
				return false
			}
			if len(f) == 0 {
				return false
			}
			if ok, _ := path.Match(p[0], f[0]); !ok {
				return false
			}
			p, f = p[1:], f[1:]
		}
		return len(f) == 0
	}
	return match(strings.Split(path.Clean(pattern), "/"), strings.Split(file, "/"))
}

// workspacePackage is a tracked package.json with a name: its folder and what it says of its entry points.
type workspacePackage struct {
	folder  string
	exports any      // nil when it has none
	fields  []string // types, typings, module and main, as written, in that order
}

// typescriptImports reads, per importing file, the aliases of its config (paths, from the config's baseUrl, else the
// folder of the file that set them), and the workspace packages: an import of a tracked package's name, or of a path
// under it, is own. With exports, Node's rules pick the subpath (an exact one first, else the pattern with the longest
// text before its *, then the longer key; null excludes), and every condition's target counts; without, types,
// typings, module and main name the bare import, then index, and a subpath is the path under the package. A target
// under the package's outDir is looked for under its rootDir, else under src, lib and the package folder. An own
// import whose target has no tracked source is not readable, never outside.
func typescriptImports(rules conventions.Rules, language conventions.Language, content map[string][]byte, tracked map[string]bool) prefix {
	configs := &tsConfigs{content: content, tracked: tracked, read: map[string]*tsConfig{}}
	packages := map[string]workspacePackage{}
	for file := range tracked {
		if path.Base(file) != "package.json" || inFolders(file, rules.SkippedFolders) || inFolders(file, []string{"node_modules"}) {
			continue
		}
		var manifest map[string]any
		if json.Unmarshal(content[file], &manifest) != nil {
			continue
		}
		name, _ := manifest["name"].(string)
		if name == "" {
			continue
		}
		p := workspacePackage{folder: path.Dir(file), exports: manifest["exports"]}
		for _, field := range []string{"types", "typings", "module", "main"} {
			if value, isText := manifest[field].(string); isText {
				p.fields = append(p.fields, value)
			}
		}
		if other, taken := packages[name]; !taken || p.folder < other.folder {
			packages[name] = p
		}
	}
	state := PrefixAbsent
	if tracked["tsconfig.json"] {
		state = PrefixRead
		if root := configs.load("tsconfig.json", nil); root == nil {
			state = PrefixUnreadable
		} else if root.baseUnread {
			state = PrefixExtends
		}
	} else if len(packages) > 0 {
		state = PrefixRead
	}
	exists := func(base string) (string, bool) {
		for _, candidate := range append([]string{base}, candidatesOf(language.Imports.Candidates, base)...) {
			if tracked[candidate] {
				return candidate, true
			}
		}
		return "", false
	}
	return prefix{state: state, own: func(from, spec string, _ []string) ([]string, bool) {
		if cfg := configs.configOf(from); cfg != nil && cfg.paths != nil {
			if pattern, star, matched := bestAlias(cfg.paths, spec); matched {
				var bases []string
				for _, target := range cfg.paths[pattern] {
					base := path.Join(cmp.Or(cfg.baseURL, cfg.pathsFolder), strings.Replace(target, "*", star, 1))
					if !leaves(base) && !inFolders(base, rules.SkippedFolders) {
						bases = append(bases, base)
					}
				}
				return bases, strings.HasPrefix(pattern, "*")
			}
		}
		name, p, found := workspacePackageOf(packages, spec)
		if !found {
			return nil, false
		}
		targets, exported := p.targets(strings.TrimPrefix(spec, name))
		if !exported {
			return []string{path.Join(p.folder, "\x00not exported")}, false
		}
		cfg := configs.configOf(path.Join(p.folder, "package.json"))
		var sources []string
		for _, target := range targets {
			for _, base := range sourcesOf(cfg, p.folder, path.Join(p.folder, target)) {
				if file, ok := exists(base); ok {
					sources = appendNew(sources, file)
					break
				}
			}
		}
		if len(sources) == 0 {
			return []string{unreadableTarget}, false
		}
		return sources, false
	}}
}

// workspacePackageOf is the workspace package an import names: the one whose name it is, or lies under.
func workspacePackageOf(packages map[string]workspacePackage, spec string) (string, workspacePackage, bool) {
	var best string
	for name := range packages {
		if (spec == name || strings.HasPrefix(spec, name+"/")) && len(name) > len(best) {
			best = name
		}
	}
	return best, packages[best], best != ""
}

// targets are the paths, relative to the package, that an import of a subpath ("" or "/x") of the package names, and
// whether the package exports it.
func (p workspacePackage) targets(subpath string) ([]string, bool) {
	if p.exports == nil {
		if subpath != "" {
			return []string{"." + subpath}, true
		}
		return append(slices.Clone(p.fields), "index"), true
	}
	entries, isMap := p.exports.(map[string]any)
	if !isMap || !slices.ContainsFunc(keysOf(entries), func(k string) bool { return strings.HasPrefix(k, ".") }) {
		entries = map[string]any{".": p.exports}
	}
	key := "." + subpath
	if value, exact := entries[key]; exact {
		return conditionTargets(value, ""), value != nil
	}
	best, star := "", ""
	for candidate := range entries {
		before, after, hasStar := strings.Cut(candidate, "*")
		if !hasStar || !strings.HasPrefix(key, before) || !strings.HasSuffix(key, after) || len(key) < len(before)+len(after) {
			continue
		}
		if b, _, _ := strings.Cut(best, "*"); best == "" || len(before) > len(b) || len(before) == len(b) && len(candidate) > len(best) {
			best, star = candidate, key[len(before):len(key)-len(after)]
		}
	}
	if best == "" || entries[best] == nil {
		return nil, false
	}
	return conditionTargets(entries[best], star), true
}

// conditionTargets are every target an exports value gives: a text, each entry of a list, each condition's value;
// null none. A * is the matched text.
func conditionTargets(value any, star string) []string {
	switch v := value.(type) {
	case string:
		return []string{strings.ReplaceAll(v, "*", star)}
	case []any:
		var out []string
		for _, item := range v {
			out = append(out, conditionTargets(item, star)...)
		}
		return out
	case map[string]any:
		var out []string
		for _, key := range slices.Sorted(maps.Keys(v)) {
			out = append(out, conditionTargets(v[key], star)...)
		}
		return out
	}
	return nil
}

func keysOf(m map[string]any) []string { return slices.Collect(maps.Keys(m)) }

// sourcesOf are the paths to look for a package's target at, in order: under the config's outDir, the same path under
// its rootDir, else under src, lib and the package folder; a declaration file also as its source.
func sourcesOf(cfg *tsConfig, folder, target string) []string {
	var bases []string
	if cfg != nil && cfg.outDir != "" {
		if rel, under := strings.CutPrefix(target, cfg.outDir+"/"); under {
			if cfg.rootDir != "" {
				bases = append(bases, path.Join(cfg.rootDir, rel))
			} else {
				bases = append(bases, path.Join(folder, "src", rel), path.Join(folder, "lib", rel), path.Join(folder, rel))
			}
		}
	}
	bases = append(bases, target)
	for _, b := range slices.Clone(bases) {
		if trimmed, isDeclaration := strings.CutSuffix(b, ".d.ts"); isDeclaration {
			bases = append(bases, trimmed+".ts", trimmed)
		}
	}
	return bases
}

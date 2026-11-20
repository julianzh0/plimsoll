package snapshot

import (
	"path"
	"regexp"
	"slices"
	"strings"
)

var (
	// swiftPackageCall is the start of a dependency in a Package.swift.
	swiftPackageCall = regexp.MustCompile(`\.package\(`)
	// swiftPackage is a dependency whose place is a string literal: a url, a local path or a registry id, after an
	// optional old-style name argument.
	swiftPackage = regexp.MustCompile(`\.package\([ \t\r\n]*(?:name:[ \t\r\n]*"[^"\n]*"[ \t\r\n]*,[ \t\r\n]*)?(?P<kind>url|path|id):[ \t\r\n]*"(?P<place>[^"\n]+)"`)
	// swiftTarget is the start of a target a Package.swift declares, and its kind.
	swiftTarget = regexp.MustCompile(`\.(?P<kind>target|executableTarget|testTarget|macro|plugin|systemLibrary)\(`)
	// swiftName is the name a target's call opens with, as SwiftPM requires it first.
	swiftName = regexp.MustCompile(`^[ \t\r\n]*name:[ \t\r\n]*"(?P<value>[^"\n]*)"`)
	// swiftPath is a path argument.
	swiftPath = regexp.MustCompile(`\bpath:[ \t\r\n]*"(?P<value>[^"\n]*)"`)
	// swiftComment is a comment of a manifest, after the strings that may hold "//" (a url).
	swiftComment = regexp.MustCompile(`"(?:[^"\\\n]|\\.)*"|//[^\n]*|/\*(?s:.*?)\*/`)
	// swiftPackageDeclaration is the Package(...) a manifest declares.
	swiftPackageDeclaration = regexp.MustCompile(`\bPackage\(`)
)

// swiftDependencies reads the packages a root Package.swift depends on, by text, each by its identity as SwiftPM
// names it and as Package.resolved records it: the last part of a url or a path without ".git", or a registry id,
// in lower case. manifest is false when the file declares no Package; readable is false when a dependency's place
// is not a string literal. When every target's dependencies are literal, a package only test targets reach is
// test-only and left out (swiftRuntime); conditions are not evaluated: a conditional use counts.
func swiftDependencies(content []byte) (names []string, manifest, readable bool) {
	text, declared := swiftManifest(content)
	if !declared {
		return nil, false, true
	}
	found := swiftPackage.FindAllStringSubmatch(text, -1)
	if len(found) != len(swiftPackageCall.FindAllStringIndex(text, -1)) {
		return nil, true, false
	}
	names = []string{}
	for _, m := range found {
		identity := m[swiftPackage.SubexpIndex("place")]
		if m[swiftPackage.SubexpIndex("kind")] != "id" {
			identity = strings.TrimSuffix(path.Base(strings.ReplaceAll(strings.TrimSuffix(identity, "/"), ":", "/")), ".git")
		}
		if identity = strings.ToLower(identity); !slices.Contains(names, identity) {
			names = append(names, identity)
		}
	}
	if runtime, tests, literal := swiftReached(text); literal {
		names = slices.DeleteFunc(names, func(name string) bool { return tests[name] && !runtime[name] })
	}
	return names, true, true
}

// swiftTargetCall is one target of a manifest: its kind, its name and the entries of its dependencies list.
type swiftTargetCall struct {
	kind, name   string
	dependencies []string
}

var (
	// swiftDependencyEntry is one literal entry of a target's dependencies: a name, a product of a package, a target or
	// a name by its call, each with or without a condition.
	swiftDependencyEntry = regexp.MustCompile(`^(?:"(?P<plain>[^"]+)"|\.product\(\s*name:\s*"[^"]+"\s*,\s*package:\s*"(?P<package>[^"]+)"\s*(?:,\s*(?:moduleAliases|condition):[\s\S]*)?\)|\.(?:target|byName)\(\s*name:\s*"(?P<named>[^"]+)"\s*(?:,\s*condition:[\s\S]*)?\))$`)
	// swiftProductTargets are the targets a product lists.
	swiftProductTargets = regexp.MustCompile(`\.(?:library|executable|plugin)\([^)]*?targets:\s*\[([^\]]*)\]`)
)

// swiftReached are the packages, by lower-case identity, that the production roots reach over the literal local target
// dependencies (the targets the products list and every executable target; every target but tests when the manifest
// declares no product), and those the test targets reach. literal is false when a dependencies list is no literal
// list, or a by-name entry is no local target: then every package counts.
func swiftReached(text string) (runtime, tests map[string]bool, literal bool) {
	targets := map[string]swiftTargetCall{}
	end := -1 // the end of the last target read: a target named inside it is a reference
	for _, m := range swiftTarget.FindAllStringSubmatchIndex(text, -1) {
		if m[0] < end {
			continue
		}
		end = balanced(text, m[1]-1)
		args := text[m[1]:end]
		name := swiftName.FindStringSubmatch(args)
		if name == nil {
			return nil, nil, false
		}
		t := swiftTargetCall{kind: text[m[2]:m[3]], name: name[1]}
		if at := strings.Index(args, "dependencies:"); at >= 0 {
			open := strings.Index(args[at:], "[")
			if open < 0 {
				return nil, nil, false
			}
			t.dependencies = splitTopLevel(args[at+open+1 : balanced(args, at+open)])
		}
		for _, entry := range t.dependencies { // every list must be literal, also those no root reaches
			if !swiftDependencyEntry.MatchString(entry) {
				return nil, nil, false
			}
		}
		targets[t.name] = t
	}
	var roots, testRoots []string
	products := swiftProductTargets.FindAllStringSubmatch(text, -1)
	for _, t := range targets {
		switch {
		case t.kind == "testTarget":
			testRoots = append(testRoots, t.name)
		case t.kind == "executableTarget" || len(products) == 0:
			roots = append(roots, t.name)
		}
	}
	for _, p := range products {
		for name := range strings.SplitSeq(p[1], ",") {
			roots = append(roots, strings.Trim(strings.TrimSpace(name), `"`))
		}
	}
	runtime, ok := reachedPackages(targets, roots)
	tests, okTests := reachedPackages(targets, testRoots)
	return runtime, tests, ok && okTests
}

// reachedPackages are the packages the roots reach over the local target dependencies; false at a by-name entry
// that is no local target.
func reachedPackages(targets map[string]swiftTargetCall, roots []string) (map[string]bool, bool) {
	runtime, seen := map[string]bool{}, map[string]bool{}
	for len(roots) > 0 {
		name := roots[len(roots)-1]
		roots = roots[:len(roots)-1]
		t, local := targets[name]
		if !local || seen[name] {
			continue
		}
		seen[name] = true
		for _, entry := range t.dependencies {
			m := swiftDependencyEntry.FindStringSubmatch(entry)
			if m == nil {
				return nil, false
			}
			named := m[swiftDependencyEntry.SubexpIndex("plain")] + m[swiftDependencyEntry.SubexpIndex("named")]
			if pkg := m[swiftDependencyEntry.SubexpIndex("package")]; pkg != "" {
				runtime[strings.ToLower(pkg)] = true
			} else if _, isTarget := targets[named]; isTarget {
				roots = append(roots, named)
			} else {
				return nil, false // a by-name product: its package is unknown
			}
		}
	}
	return runtime, true
}

// balanced is the offset of the bracket that closes the one at open, strings respected; the text's end when none does.
func balanced(text string, open int) int {
	depth, inString := 0, false
	for i := open; i < len(text); i++ {
		switch c := text[i]; {
		case inString:
			switch c {
			case '\\':
				i++
			case '"':
				inString = false
			}
		case c == '"':
			inString = true
		case c == '(' || c == '[':
			depth++
		case c == ')' || c == ']':
			if depth--; depth == 0 {
				return i
			}
		}
	}
	return len(text)
}

// splitTopLevel splits a list's text at the commas outside brackets and strings, each entry trimmed; none for an
// empty list.
func splitTopLevel(list string) []string {
	var entries []string
	start, depth, inString := 0, 0, false
	for i := 0; i <= len(list); i++ {
		if i == len(list) || !inString && depth == 0 && list[i] == ',' {
			if entry := strings.TrimSpace(list[start:i]); entry != "" {
				entries = append(entries, entry)
			}
			start = i + 1
			continue
		}
		switch c := list[i]; {
		case inString:
			switch c {
			case '\\':
				i++
			case '"':
				inString = false
			}
		case c == '"':
			inString = true
		case c == '(' || c == '[':
			depth++
		case c == ')' || c == ']':
			depth--
		}
	}
	return entries
}

// swiftTargets reads the targets a root Package.swift declares, each in its folder: the path it names, else SwiftPM's
// default, Tests/<name> for a test target, Plugins/<name> for a plugin and Sources/<name> for any other. An import is
// own when it names a target; its folder holds the module's files at any depth.
// A target's name or path built from a variable is not read, and such a module's imports count as outside: only
// running the manifest knows it.
func swiftTargets(content []byte) prefix {
	folders, declared := swiftTargetFolders(content)
	if !declared {
		return prefix{state: PrefixUnreadable}
	}
	return prefix{state: PrefixRead, own: func(_, spec string, _ []string) ([]string, bool) {
		if folder, own := folders[spec]; own {
			return []string{folder}, false
		}
		return nil, false
	}}
}

// swiftTargetFolders are the folders of the targets a root Package.swift declares, by target name, and whether it
// declares a Package.
func swiftTargetFolders(content []byte) (map[string]string, bool) {
	text, declared := swiftManifest(content)
	if !declared {
		return nil, false
	}
	folders := map[string]string{} // a target's name to its folder
	end := -1                      // the end of the last target read: a target named inside it is a reference
	for _, at := range swiftTarget.FindAllStringSubmatchIndex(text, -1) {
		if at[0] < end {
			continue
		}
		end = closing([]byte(text), at[1]-1)
		call := text[at[1]:end]
		m := swiftName.FindStringSubmatch(call)
		if m == nil {
			continue
		}
		name, folder := m[1], ""
		if p := swiftPath.FindStringSubmatch(topLevel(call)); p != nil {
			folder = path.Clean(p[1])
		}
		if folder == "" {
			switch text[at[2]:at[3]] {
			case "testTarget":
				folder = path.Join("Tests", name)
			case "plugin":
				folder = path.Join("Plugins", name)
			default:
				folder = path.Join("Sources", name)
			}
		}
		folders[name] = folder
	}
	return folders, true
}

// swiftManifest is a Package.swift's text with its comments blanked, and whether it declares a Package.
func swiftManifest(content []byte) (string, bool) {
	text := withoutComments(swiftComment, string(content))
	return text, swiftPackageDeclaration.MatchString(text)
}

// topLevel is a call's argument text with everything inside nested parentheses and brackets blanked.
func topLevel(call string) string {
	out := []byte(call)
	depth := 0
	for i, c := range out {
		switch c {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		}
		if depth > 0 {
			out[i] = ' '
		}
	}
	return string(out)
}

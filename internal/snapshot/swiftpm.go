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

// swiftDependencies reads the packages a Package.swift depends on, by text, each by its identity as SwiftPM
// names it and as Package.resolved records it: the last part of a url or a path without ".git", or a registry id,
// in lower case. manifest is false when the file declares no Package; readable is false when a dependency's place
// is not a string literal.
// Products a target uses, packages only test targets use and dependencies declared under a condition are not told
// apart from the others: Package.swift is a program, which plimsoll does not run.
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
	return names, true, true
}

// swiftTargets reads the targets a root Package.swift declares, each in its folder: the path it names, else SwiftPM's
// default, Tests/<name> for a test target, Plugins/<name> for a plugin and Sources/<name> for any other. An import is
// own when it names a target; its folder holds the module's files at any depth.
// A target's name or path built from a variable is not read, and such a module's imports count as outside:
// Package.swift is a program, which plimsoll does not run.
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

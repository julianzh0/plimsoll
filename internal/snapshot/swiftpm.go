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
	// swiftArgument is a named string argument of a call: name: "x" or path: "x".
	swiftArgument = regexp.MustCompile(`\b(?P<label>name|path):[ \t\r\n]*"(?P<value>[^"\n]*)"`)
	// swiftPackageDeclaration is the Package(...) a manifest declares.
	swiftPackageDeclaration = regexp.MustCompile(`\bPackage\(`)
)

// swiftDependencies reads the packages a root Package.swift depends on, by text, each by its identity as SwiftPM
// names it and as Package.resolved records it: the last part of a url or a path without ".git", or a registry id,
// in lower case. manifest is false when the file declares no Package; readable is false when a dependency's place
// is not a string literal.
// NOTE: products a target uses and dependencies declared under a condition are not told apart from the others; a
// manifest of another tools version is read the same way. Upgrade: ask swift package dump-package.
func swiftDependencies(content []byte) (names []string, manifest, readable bool) {
	text := strings.TrimPrefix(string(content), byteOrderMark)
	if !swiftPackageDeclaration.MatchString(text) {
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
// NOTE: a target's name or path built from a variable is not read, and such a module's imports count as outside.
// Upgrade: ask swift package dump-package.
func swiftTargets(content []byte) prefix {
	text := strings.TrimPrefix(string(content), byteOrderMark)
	if !swiftPackageDeclaration.MatchString(text) {
		return prefix{state: PrefixUnreadable}
	}
	folders := map[string]string{} // a target's name to its folder
	for _, at := range swiftTarget.FindAllStringSubmatchIndex(text, -1) {
		call := text[at[1]:closingParen(text, at[1]-1)]
		var name, folder string
		for _, m := range swiftArgument.FindAllStringSubmatch(call, -1) {
			value := m[swiftArgument.SubexpIndex("value")]
			switch {
			case m[swiftArgument.SubexpIndex("label")] == "path" && folder == "":
				folder = path.Clean(value)
			case name == "":
				name = value
			}
		}
		if name == "" {
			continue
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
	return prefix{state: PrefixRead, own: func(_, spec string, _ []string) ([]string, bool) {
		if folder, own := folders[spec]; own {
			return []string{folder}, false
		}
		return nil, false
	}}
}

// closingParen is the offset of the parenthesis that closes the one at open, or the text's end when none does.
func closingParen(text string, open int) int {
	depth := 0
	for i := open; i < len(text); i++ {
		switch text[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(text)
}

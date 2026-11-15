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

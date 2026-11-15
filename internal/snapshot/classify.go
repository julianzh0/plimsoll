package snapshot

import (
	"bytes"
	"path"
	"slices"
	"strings"

	"github.com/juzhxng/plimsoll/internal/conventions"
)

// excluded says whether a path can never be a code file: its suffix is not code or a folder of it is skipped.
func excluded(rules conventions.Rules, filePath string) bool {
	if slices.Contains(rules.NotCode, conventions.Suffix(filePath)) {
		return true
	}
	return inFolders(filePath, rules.SkippedFolders)
}

// inFolders says whether any folder of a path has one of the names.
func inFolders(filePath string, names []string) bool {
	return slices.ContainsFunc(strings.Split(path.Dir(filePath), "/"), func(folder string) bool { return slices.Contains(names, folder) })
}

// generatedOrBinary says whether content is binary or carries the generated-file marker.
func generatedOrBinary(rules conventions.Rules, content []byte) bool {
	if conventions.IsBinary(content) {
		return true
	}
	end := 0
	for range conventions.GeneratedHeaderLines {
		index := bytes.IndexByte(content[end:], '\n')
		if index < 0 {
			end = len(content)
			break
		}
		end += index + 1
	}
	return rules.Generated.Match(content[:end])
}

// testPath says whether a path is test code by its folders or its name.
func testPath(rules conventions.Rules, filePath string) bool {
	if inFolders(filePath, rules.TestFolders) || inFoldersEnding(filePath, rules.TestFolderEndings) || underTestSourceSet(rules, path.Dir(filePath)) ||
		slices.ContainsFunc(rules.TestPaths, func(folder string) bool { _, in := under(filePath, folder); return in }) {
		return true
	}
	name := path.Base(filePath)
	stem := strings.TrimSuffix(name, conventions.Suffix(filePath))
	for _, ending := range rules.TestEndings {
		if strings.HasSuffix(stem, ending) {
			return true
		}
	}
	for _, beginning := range rules.TestBeginnings {
		if strings.HasPrefix(name, beginning) {
			return true
		}
	}
	return false
}

// inFoldersEnding says whether any folder of a path has a name that ends in one of the endings.
func inFoldersEnding(filePath string, endings []string) bool {
	folders := strings.Split(path.Dir(filePath), "/")
	return slices.ContainsFunc(folders, func(folder string) bool {
		return slices.ContainsFunc(endings, func(ending string) bool { return strings.HasSuffix(folder, ending) })
	})
}

// generatedName says whether a file's name marks it as generated.
func generatedName(rules conventions.Rules, filePath string) bool {
	return slices.ContainsFunc(rules.GeneratedNames, func(pattern string) bool {
		matched, _ := path.Match(pattern, path.Base(filePath))
		return matched
	})
}

// testSourceSet says whether the name of a folder directly in a folder src makes it a test source set: a shared test
// folder name, or one of the rules' patterns.
func testSourceSet(rules conventions.Rules, name string) bool {
	return slices.Contains(rules.TestFolders, name) || slices.ContainsFunc(rules.TestSourceSets, func(pattern string) bool {
		matched, _ := path.Match(pattern, name)
		return matched
	})
}

// underTestSourceSet says whether a folder is a test source set or lies under one.
func underTestSourceSet(rules conventions.Rules, folder string) bool {
	parts := strings.Split(folder, "/")
	for i := 1; i < len(parts); i++ {
		if parts[i-1] == "src" && testSourceSet(rules, parts[i]) {
			return true
		}
	}
	return false
}

// otherPath says whether a path's first folder holds other code.
func otherPath(rules conventions.Rules, filePath string) bool {
	first, _, nested := strings.Cut(filePath, "/")
	return nested && slices.Contains(rules.OtherFolders, first)
}

// role classifies a code file by the ordered rules: test code split by content, other code, product.
func role(rules conventions.Rules, filePath string, content []byte) Role {
	isTest := testPath(rules, filePath)
	switch {
	case isTest && rules.TestDefinition.Match(content):
		return Test
	case isTest:
		return TestInfrastructure
	case otherPath(rules, filePath):
		return Other
	default:
		return Product
	}
}

// area classifies a path of the history from the path alone.
func area(rules conventions.Rules, filePath string) Area {
	first, _, nested := strings.Cut(filePath, "/")
	switch {
	case onPaths(rules, filePath, rules.PlanningPaths) || onPaths(rules, filePath, rules.MeasurementPaths):
		return AreaRecords
	case slices.Contains(rules.DocSuffixes, conventions.Suffix(filePath)) || nested && slices.Contains(rules.DocFolders, first):
		return AreaDocs
	case excluded(rules, filePath):
		return AreaOther
	case testPath(rules, filePath):
		return AreaTests
	case otherPath(rules, filePath):
		return AreaOther
	}
	return AreaProduct
}

// onPaths says whether a path lies under a list of paths. An entry ending in "/" is a folder: one folder name
// matches at the root only, since a name like projects/ or evidence/ is also an ordinary folder deeper in the code;
// a chain of two or more folders matches at any depth. Any other entry is a pattern for the file name. A path in a
// skipped folder never matches.
func onPaths(rules conventions.Rules, filePath string, entries []string) bool {
	if inFolders(filePath, rules.SkippedFolders) {
		return false
	}
	for _, entry := range entries {
		if folder, isFolder := strings.CutSuffix(entry, "/"); isFolder {
			if strings.HasPrefix(filePath, folder+"/") || strings.Contains(folder, "/") && strings.Contains(filePath, "/"+folder+"/") {
				return true
			}
		} else if matched, _ := path.Match(entry, path.Base(filePath)); matched {
			return true
		}
	}
	return false
}

// countLines counts newline bytes, plus one when non-empty content does not end with a newline.
func countLines(content []byte) int {
	lines := bytes.Count(content, []byte{'\n'})
	if len(content) > 0 && content[len(content)-1] != '\n' {
		lines++
	}
	return lines
}

// under says whether a path lies under a folder, as a path from the root, and gives the rest of it; "." holds every
// path. Folders are whole names, so `checks` does not hold `checks-old/x`.
func under(filePath, folder string) (rest string, in bool) {
	if folder == "." {
		return filePath, true
	}
	return strings.CutPrefix(filePath, folder+"/")
}

package conventions

import (
	"path"
	"regexp"
	"strings"
)

// TestKind is one kind of test code the repository declares: its name and its usable path patterns.
type TestKind struct {
	Name     string
	Patterns []PathPattern
}

// kindName is a kind's name: a plain name of letters, digits, _ and -, within OwnValueLength.
var kindName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// KindName says whether a name of the tests table is a plain name an answer may print.
func KindName(name string) bool {
	return len(name) <= OwnValueLength && kindName.MatchString(name)
}

// KindOf is the first kind, in declared order, with a pattern that matches a path, or "".
func KindOf(kinds []TestKind, filePath string) string {
	if len(kinds) == 0 {
		return ""
	}
	parts := strings.Split(filePath, "/")
	for _, kind := range kinds {
		for _, pattern := range kind.Patterns {
			if pattern.MatchSegments(parts) {
				return kind.Name
			}
		}
	}
	return ""
}

// anyFolders is the segment of a path pattern that stands for any number of folders, none included.
const anyFolders = "**"

// patternSegment is one segment of a path pattern other than "**": letters, digits, ._+- and the wildcards * and ?
// of path.Match. With this set no other path.Match syntax (classes, escapes) can occur, so a segment never fails to
// compile.
var patternSegment = regexp.MustCompile(`^[A-Za-z0-9._+*?-]+$`)

// PathPattern is a path pattern of a kind of test code: segments from the repository root joined by "/", each
// matched by path.Match against one segment of a file's path, and "**" for any number of folders. A last "**"
// stands for every file below, so it needs at least one segment there.
type PathPattern struct {
	text     string
	segments []string
}

// NewPathPattern brings a value as written to a path pattern, and says whether it has that form: within
// OwnValueLength, no leading or trailing "/", no empty, "." or ".." segment, and "**" only as a whole segment.
func NewPathPattern(value string) (PathPattern, bool) {
	if value == "" || len(value) > OwnValueLength {
		return PathPattern{}, false
	}
	segments := strings.Split(value, "/")
	for _, segment := range segments {
		if segment == anyFolders {
			continue
		}
		if segment == "." || segment == ".." || !patternSegment.MatchString(segment) || strings.Contains(segment, anyFolders) {
			return PathPattern{}, false
		}
	}
	return PathPattern{text: value, segments: segments}, true
}

// Text is the pattern as written.
func (p PathPattern) Text() string { return p.text }

// Match says whether a path from the repository root matches the pattern.
func (p PathPattern) Match(filePath string) bool {
	return p.MatchSegments(strings.Split(filePath, "/"))
}

// MatchSegments says whether a path, split at "/", matches the pattern: for a caller that matches one path against
// many patterns. It walks both lists once, going back only to the last "**" seen, so its work is bounded by the
// product of their lengths.
func (p PathPattern) MatchSegments(parts []string) bool {
	at, part := 0, 0
	resumeAt, resumePart := -1, 0
	for part < len(parts) {
		switch {
		case at < len(p.segments) && p.segments[at] == anyFolders:
			resumeAt, resumePart = at, part
			at++
		case at < len(p.segments) && segmentMatches(p.segments[at], parts[part]):
			at++
			part++
		case resumeAt >= 0:
			resumePart++
			at, part = resumeAt+1, resumePart
		default:
			return false
		}
	}
	return at == len(p.segments)
}

// segmentMatches compares a literal segment directly, and one with a wildcard by path.Match.
func segmentMatches(pattern, segment string) bool {
	if !strings.ContainsAny(pattern, "*?") {
		return pattern == segment
	}
	matched, _ := path.Match(pattern, segment)
	return matched
}

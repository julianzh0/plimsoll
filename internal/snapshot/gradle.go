package snapshot

import (
	"regexp"
	"slices"
	"strings"
)

var (
	gradleBlock   = regexp.MustCompile(`^dependencies[ \t]*\{[ \t]*(?://.*)?$`)
	gradleRuntime = regexp.MustCompile(`^(?:implementation|api|runtimeOnly)\b`)
	gradleEntry   = regexp.MustCompile(`^(?:implementation|api|runtimeOnly)\b[ \t]*\(?[ \t]*(?:(?:platform|enforcedPlatform)[ \t]*\(?[ \t]*)?(?:["'](?P<coordinate>[^"'\n]+)["']|project\([ \t]*(?:path[ \t]*[:=][ \t]*)?["'](?P<project>:[^"'\n]*)["']|(?P<accessor>[A-Za-z_]\w*(?:\.\w+)+))`)
)

// gradleDependencies reads the runtime dependencies of a build.gradle or build.gradle.kts by text: the
// implementation, api and runtimeOnly lines one level below the dependencies block at the first column, as a
// coordinate's group:artifact, a project's path, or a catalogue name as written. manifest is false when the file has
// no dependencies block at the first column; readable is false when a runtime line of the block is written in a
// form the reader does not take.
// NOTE: a catalogue name is kept as written and gradle/libs.versions.toml is not read; dependencies of the modules'
// own build files, of convention plugins (build-logic/, buildSrc/) and of variant configurations are not counted.
// Upgrade: read the catalogue and every tracked build file.
func gradleDependencies(content []byte) (names []string, manifest, readable bool) {
	text := string(content)
	inBlock, indent := false, ""
	names = []string{}
	for line := range strings.Lines(text) {
		line = strings.TrimRight(line, "\r\n")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if line[0] != ' ' && line[0] != '\t' {
			if inBlock {
				inBlock = false
				continue
			}
			if gradleBlock.MatchString(line) {
				inBlock, manifest, indent = true, true, ""
			}
			continue
		}
		if !inBlock {
			continue
		}
		lead := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		if indent == "" {
			indent = lead
		}
		if lead != indent || !gradleRuntime.MatchString(trimmed) {
			continue
		}
		m := gradleEntry.FindStringSubmatch(trimmed)
		if m == nil {
			return nil, true, false
		}
		var name string
		switch {
		case m[gradleEntry.SubexpIndex("coordinate")] != "":
			parts := strings.Split(m[gradleEntry.SubexpIndex("coordinate")], ":")
			if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
				return nil, true, false
			}
			name = parts[0] + ":" + parts[1]
		case m[gradleEntry.SubexpIndex("project")] != "":
			name = m[gradleEntry.SubexpIndex("project")]
		default:
			name = m[gradleEntry.SubexpIndex("accessor")]
		}
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	if !manifest {
		return nil, false, true
	}
	return names, true, true
}

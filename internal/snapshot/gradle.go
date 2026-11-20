package snapshot

import (
	"cmp"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

var (
	gradleBlock = regexp.MustCompile(`^dependencies[ \t]*\{[ \t]*(?://.*)?$`)
	// gradleLine is a dependency line: its configuration and what follows it.
	gradleLine  = regexp.MustCompile(`^([a-z][A-Za-z0-9]*)\b[ \t]*(.*)$`)
	gradleEntry = regexp.MustCompile(`^\(?[ \t]*(?:(?:platform|enforcedPlatform)[ \t]*\(?[ \t]*)?(?:["'](?P<coordinate>[^"'\n]+)["']|project\([ \t]*(?:path[ \t]*[:=][ \t]*)?["'](?P<project>:[^"'\n]*)["']|(?P<accessor>[A-Za-z_]\w*(?:\.\w+)+))`)
)

// gradleRuntime says whether a configuration puts its dependencies on the runtime classpath, as Gradle's Java Library
// plugin documents it: implementation, api and runtimeOnly, and a variant's (debugImplementation, commonMainApi), but
// not one whose variant is a test source set by the shared rules (testImplementation, androidTestApi, intTest...,
// testFixtures...) and no compile-only or tool configuration (compileOnly, kapt, ksp, annotationProcessor).
func gradleRuntime(configuration string) bool {
	if strings.HasSuffix(configuration, "CompileOnly") || strings.HasSuffix(configuration, "CompileOnlyApi") {
		return false
	}
	for _, kind := range []string{"implementation", "api", "runtimeOnly"} {
		if configuration == kind {
			return true
		}
		variant, found := strings.CutSuffix(configuration, strings.ToUpper(kind[:1])+kind[1:])
		if found && variant != "" {
			return !strings.HasPrefix(variant, "test") && !strings.Contains(variant, "Test")
		}
	}
	return false
}

// gradleDependencies reads the runtime dependencies of a build.gradle or build.gradle.kts by text: the lines of a
// runtime configuration one level below the dependencies block at the first column, as a coordinate's
// group:artifact, or a catalogue accessor named by the coordinate its alias stands for in the catalogue (each library
// of a bundle). An accessor the catalogue does not hold stays as written. Modules of the build itself (project(...),
// projects.x) are no dependency. manifest is false when the file has no dependencies block at the first column;
// readable is false when a runtime line of the block is written in a form the reader does not take.
// What convention plugins (build-logic/, buildSrc/) or script logic add is known only to a running Gradle (README,
// "What plimsoll does not read").
func gradleDependencies(content []byte, catalogue map[string][]string) (names []string, manifest, readable bool) {
	inBlock, indent := false, ""
	names = []string{}
	for line := range strings.Lines(string(content)) {
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
		entry := gradleLine.FindStringSubmatch(trimmed)
		if lead != indent || entry == nil || !gradleRuntime(entry[1]) {
			continue
		}
		m := gradleEntry.FindStringSubmatch(entry[2])
		if m == nil {
			return nil, true, false
		}
		var found []string
		switch accessor := m[gradleEntry.SubexpIndex("accessor")]; {
		case m[gradleEntry.SubexpIndex("coordinate")] != "":
			parts := strings.Split(m[gradleEntry.SubexpIndex("coordinate")], ":")
			if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
				return nil, true, false
			}
			found = []string{parts[0] + ":" + parts[1]}
		case m[gradleEntry.SubexpIndex("project")] != "", strings.HasPrefix(accessor, "projects."):
		case catalogue[accessor] != nil:
			found = catalogue[accessor]
		default:
			found = []string{accessor}
		}
		for _, name := range found {
			if !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}
	if !manifest {
		return nil, false, true
	}
	return names, true, true
}

// gradleCatalogue maps each accessor of the nearest tracked catalogue at or above a build file's folder to the
// coordinates (group:name) it stands for: libs.<alias> for a library, its "-", "_" and "." all read as dots, and
// libs.bundles.<name> for each library of a bundle. nil when there is none or it cannot be read.
func gradleCatalogue(at around) map[string][]string {
	for f := range foldersUp(at.folder) {
		if at.content[path.Join(f, conventions.GradleCatalogue)] == nil {
			continue
		}
		document := at.toml(path.Join(f, conventions.GradleCatalogue))
		if document == nil {
			return nil
		}
		accessor := func(prefix, alias string) string {
			return prefix + strings.NewReplacer("-", ".", "_", ".").Replace(alias)
		}
		catalogue := map[string][]string{}
		for alias, entry := range tomlTableAt(document, "libraries") {
			if coordinate := catalogueCoordinate(entry); coordinate != "" {
				catalogue[accessor("libs.", alias)] = []string{coordinate}
			}
		}
		for bundle, members := range tomlTableAt(document, "bundles") {
			aliases, _ := tomlTexts(members)
			for _, alias := range aliases {
				if coordinate := catalogue[accessor("libs.", alias)]; coordinate != nil {
					catalogue[accessor("libs.bundles.", bundle)] = append(catalogue[accessor("libs.bundles.", bundle)], coordinate...)
				}
			}
		}
		return catalogue
	}
	return nil
}

// catalogueCoordinate is a catalogue library's group:name: "group:name:version", module = "group:name", or group and
// name keys.
func catalogueCoordinate(entry any) string {
	coordinate := func(text string) string {
		if parts := strings.Split(text, ":"); len(parts) >= 2 {
			return parts[0] + ":" + parts[1]
		}
		return ""
	}
	if text, isText := entry.(string); isText {
		return coordinate(text)
	}
	table, _ := entry.(map[string]any)
	if module, isText := table["module"].(string); isText {
		return cmp.Or(coordinate(module), module) // "group:name", a version cut; a name without a group as written
	}
	group, _ := table["group"].(string)
	name, _ := table["name"].(string)
	if group == "" || name == "" {
		return ""
	}
	return group + ":" + name
}

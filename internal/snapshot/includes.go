package snapshot

import (
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

var (
	// cmakeComment is a comment of a CMake file, after the strings that may hold a "#".
	cmakeComment = regexp.MustCompile(`"(?:[^"\\]|\\.)*"|#\[\[(?s:.*?)\]\]|#[^\n]*`)
	// cmakeIncludes is the start of a command that names include folders, and whether its first argument is a target.
	cmakeIncludes = regexp.MustCompile(`(?i)\b(?P<target>target_)?include_directories[ \t\r\n]*\(`)
	// cmakeProject is a project command: the folder of its file is the project's source folder.
	cmakeProject = regexp.MustCompile(`(?im)^[ \t]*project[ \t\r\n]*\(`)
	// cmakeArgument is one argument of a command, quoted or bare.
	cmakeArgument = regexp.MustCompile(`"[^"]*"|[^\s"]+`)
	// cmakeFolder is a variable an include folder's path may start with: the folder of the file itself, of its
	// project or of the build's top.
	cmakeFolder = regexp.MustCompile(`^\$\{(?P<of>CMAKE_CURRENT_SOURCE|CMAKE_CURRENT_LIST|PROJECT_SOURCE|CMAKE_SOURCE)_DIR\}`)
)

// cmakeKeyword says whether an argument of include_directories or target_include_directories is a keyword, no folder.
func cmakeKeyword(argument string) bool {
	switch argument {
	case "SYSTEM", "BEFORE", "AFTER", "PUBLIC", "PRIVATE", "INTERFACE":
		return true
	}
	return false
}

// includeFolders finds the folders in which an include is looked for besides the including file's own: every folder
// with a conventional name that holds tracked files, and the folders that the tracked prefix files name by a path.
// An include in quotes is the file beside the including one when that is tracked; else, like an include in angle
// brackets, the tracked files of its path under the include folders: one is the target, several are ambiguous. One
// in quotes that is found nowhere is the project's own, and not found, when a tracked file ends in its path; any
// other is outside.
// NOTE: every file is given every include folder, whatever target a build file names it for. Upgrade: read the
// targets' sources.
func includeFolders(imports conventions.Imports, content map[string][]byte, tracked map[string]bool) prefix {
	holds := map[string]bool{}      // every folder that holds a tracked file, at any depth
	byName := map[string][]string{} // a file name to the tracked paths that carry it
	for file := range tracked {
		byName[path.Base(file)] = append(byName[path.Base(file)], file)
		for folder := path.Dir(file); !holds[folder]; folder = path.Dir(folder) {
			holds[folder] = true
		}
	}
	var folders []string
	for folder := range holds {
		if slices.Contains(imports.Prefix.Folders, path.Base(folder)) {
			folders = append(folders, folder)
		}
	}
	projects := map[string]bool{} // the folders whose prefix file calls project()
	texts := map[string]string{}  // a prefix file's folder to its text, comments blanked
	for file := range tracked {
		if path.Base(file) != imports.Prefix.File || content[file] == nil {
			continue
		}
		text := withoutComments(cmakeComment, string(content[file]))
		texts[path.Dir(file)] = text
		projects[path.Dir(file)] = cmakeProject.MatchString(text)
	}
	for here, text := range texts {
		project, top := projectFolders(projects, here)
		for _, folder := range cmakeIncludeFolders(text, here, project, top) {
			if holds[folder] {
				folders = append(folders, folder)
			}
		}
	}
	slices.Sort(folders)
	folders = slices.Compact(folders)
	isFolder := make(map[string]bool, len(folders))
	for _, folder := range folders {
		isFolder[folder] = true
	}
	return prefix{state: PrefixRead, own: func(from, spec string, _ []string) ([]string, bool) {
		name, angled := strings.CutPrefix(spec, "<")
		name = strings.ReplaceAll(strings.TrimSuffix(name, ">"), `\`, "/") // a Windows separator names the same path
		beside := path.Join(path.Dir(from), name)
		if !angled && tracked[beside] {
			return []string{beside}, false
		}
		var found []string
		written := path.Clean(name)
		if leaves(written) {
			for _, folder := range folders {
				if file := path.Join(folder, name); tracked[file] {
					found = append(found, file)
				}
			}
		} else { // the files of that name whose path ends in it under an include folder
			for _, file := range byName[path.Base(written)] {
				folder, under := strings.CutSuffix(file, "/"+written)
				if under && isFolder[folder] || file == written && isFolder["."] {
					found = append(found, file)
				}
			}
		}
		if len(found) > 0 || angled {
			return found, false
		}
		for strings.HasPrefix(written, "../") {
			written = written[len("../"):]
		}
		if slices.ContainsFunc(byName[path.Base(written)], func(file string) bool { return file == written || strings.HasSuffix(file, "/"+written) }) {
			return []string{beside}, false // beside is not tracked, so the include is not found
		}
		return nil, false
	}}
}

// projectFolders are the folders of the nearest and of the farthest prefix file, at or above a folder, that calls
// project(): the project's own source folder and the top of the build. Both are the repository root when none does.
// NOTE: a project built on its own from a folder below another is taken for part of the one above. Upgrade: none
// by text; the build decides.
func projectFolders(projects map[string]bool, folder string) (nearest, farthest string) {
	nearest, farthest = ".", "."
	found := false
	for ; ; folder = path.Dir(folder) {
		if projects[folder] {
			if !found {
				nearest, found = folder, true
			}
			farthest = folder
		}
		if folder == "." {
			return nearest, farthest
		}
	}
}

// cmakeIncludeFolders are the folders, as cleaned paths from the repository root, that the include commands of one
// CMake file name by a path: relative to the file's folder, or under ${CMAKE_CURRENT_SOURCE_DIR},
// ${CMAKE_CURRENT_LIST_DIR}, ${PROJECT_SOURCE_DIR} or ${CMAKE_SOURCE_DIR}, also inside $<BUILD_INTERFACE:...>.
// An argument that holds any other variable or generator expression, an absolute path and a path that leaves the
// repository name no folder.
// NOTE: only CMakeLists.txt is read, by text: a folder named through any other variable, in a *.cmake module, by a
// function of the project or by another build system is not known. Upgrade: follow set(...) of a path.
func cmakeIncludeFolders(text, here, project, top string) []string {
	var folders []string
	raw := []byte(text)
	for _, at := range cmakeIncludes.FindAllStringSubmatchIndex(text, -1) {
		arguments := cmakeArgument.FindAllString(text[at[1]:closing(raw, at[1]-1)], -1)
		if at[2] >= 0 && len(arguments) > 0 {
			arguments = arguments[1:] // the target
		}
		for _, argument := range arguments {
			argument = strings.Trim(argument, `"`)
			if inner, wrapped := strings.CutPrefix(argument, "$<BUILD_INTERFACE:"); wrapped {
				argument = strings.TrimSuffix(inner, ">")
			}
			base := here
			if m := cmakeFolder.FindStringSubmatch(argument); m != nil {
				switch m[1] {
				case "PROJECT_SOURCE":
					base = project
				case "CMAKE_SOURCE":
					base = top
				}
				argument = "." + argument[len(m[0]):]
			}
			if argument == "" || cmakeKeyword(argument) || strings.Contains(argument, "$") || path.IsAbs(argument) {
				continue
			}
			if folder := path.Join(base, argument); !leaves(folder) {
				folders = append(folders, folder)
			}
		}
	}
	return folders
}

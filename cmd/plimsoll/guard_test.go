package main

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
	"testing"
)

const module = "github.com/julianzh0/plimsoll"

// mcpSDK is the one module plimsoll depends on, imported only by internal/mcpserver.
const mcpSDK = "github.com/modelcontextprotocol/go-sdk"

// wazero runs the tree-sitter grammars, imported only by internal/parse.
const wazero = "github.com/tetratelabs/wazero"

// sourceFile is one Go file of the module and the paths it imports.
type sourceFile struct {
	Path    string
	Imports []string
}

// guardCase is one case of the guard: a file of the module, a go.mod, and the violation the guard names.
type guardCase struct {
	name  string
	file  sourceFile
	goMod string
	want  string // "" means no violation
}

// againstTheImportDirection is what the guard says of an import of the module's own code that its table forbids.
const againstTheImportDirection = "against the import direction"

// guardCases are the guard's cases.
func guardCases() []guardCase {
	const cleanMod = "module " + module + "\n\ngo 1.27.4\n"
	return []guardCase{
		{"a third-party import", sourceFile{"internal/answer/a.go", []string{"github.com/x/y"}}, cleanMod, "outside the standard library"},
		{"a network package", sourceFile{"internal/answer/a.go", []string{"net/http"}}, cleanMod, "a network package"},
		{"a network package in a test", sourceFile{"internal/answer/a_test.go", []string{"net"}}, cleanMod, "a network package"},
		{"a program started outside gitrepo", sourceFile{"internal/answer/a.go", []string{"os/exec"}}, cleanMod, "runs programs outside internal/gitrepo"},
		{"a detector reading the rules", sourceFile{"internal/shape/a.go", []string{module + "/internal/conventions"}}, cleanMod, "against the import direction"},
		{"the links detector reading the rules", sourceFile{"internal/links/a.go", []string{module + "/internal/conventions"}}, cleanMod, "against the import direction"},
		{"the answer importing a detector", sourceFile{"internal/answer/a.go", []string{module + "/internal/shape"}}, cleanMod, "against the import direction"},
		{"the reader importing the answer", sourceFile{"internal/gitrepo/a.go", []string{module + "/internal/answer"}}, cleanMod, "against the import direction"},
		{"a package missing from the table", sourceFile{"internal/extra/a.go", []string{module + "/internal/snapshot"}}, cleanMod, "against the import direction"},
		{"a requirement in go.mod", sourceFile{"cmd/plimsoll/main.go", nil}, cleanMod + "\nrequire github.com/x/y v1.0.0\n", "go.mod requires a dependency"},
		{"a requirement in a require block", sourceFile{"cmd/plimsoll/main.go", nil}, cleanMod + "\nrequire (\n\tgithub.com/x/y v1.0.0\n)\n", "go.mod requires a dependency"},
		{"the MCP SDK and what it needs", sourceFile{"cmd/plimsoll/main.go", nil}, cleanMod + "\nrequire " + mcpSDK + " v1.8.0\n\nrequire (\n\tgithub.com/x/y v1.0.0 // indirect\n)\n", ""},
		{"the MCP SDK in the MCP package", sourceFile{"internal/mcpserver/server.go", []string{mcpSDK + "/mcp"}}, cleanMod, ""},
		{"the MCP SDK outside the MCP package", sourceFile{"internal/answer/a.go", []string{mcpSDK + "/mcp"}}, cleanMod, "outside internal/mcpserver"},
		{"test support running git", sourceFile{"internal/gitrepo/gitrepotest/repo.go", []string{"os/exec"}}, cleanMod, ""},
		{"the reader running git", sourceFile{"internal/gitrepo/git.go", []string{"os/exec", module + "/internal/snapshot"}}, cleanMod, ""},
		{"a test running the binary", sourceFile{"cmd/plimsoll/main_e2e_test.go", []string{"os/exec"}}, cleanMod, ""},
		{"a detector's test reading the rules", sourceFile{"internal/shape/shape_test.go", []string{module + "/internal/conventions"}}, cleanMod, ""},
		{"a detector's own imports", sourceFile{"internal/shape/shape.go", []string{module + "/internal/answer", module + "/internal/snapshot", "strings"}}, cleanMod, ""},
		{"wazero in the parser", sourceFile{"internal/parse/parse.go", []string{wazero, wazero + "/api"}}, cleanMod, ""},
		{"wazero outside the parser", sourceFile{"internal/snapshot/a.go", []string{wazero}}, cleanMod, "outside internal/parse"},
		{"wazero in the parser's test support", sourceFile{"internal/parse/parsetest/parser.go", []string{wazero}}, cleanMod, "outside internal/parse"},
		{"wazero required beside the MCP SDK", sourceFile{"cmd/plimsoll/main.go", nil}, cleanMod + "\nrequire (\n\t" + mcpSDK + " v1.8.0\n\t" + wazero + " v1.12.0\n)\n", ""},
		{"the snapshot using the parser", sourceFile{"internal/snapshot/a.go", []string{module + "/internal/parse"}}, cleanMod, ""},
		{"a detector using the parser", sourceFile{"internal/links/a.go", []string{module + "/internal/parse"}}, cleanMod, "against the import direction"},
	}
}

// Guards the module's promises: no dependency but the MCP SDK and wazero, no network, one place that runs programs, imports point inward.
func TestGuardRulesNameEachViolation(t *testing.T) {
	for _, tc := range guardCases() {
		t.Run(tc.name, func(t *testing.T) {
			got := violations(module, tc.goMod, []sourceFile{tc.file})
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("violations(%v) = %q, want none", tc.file, got)
				}
				return
			}
			if len(got) != 1 || !strings.Contains(got[0], tc.want) {
				t.Fatalf("violations(%v) = %q, want one naming %q", tc.file, got, tc.want)
			}
			if !strings.HasPrefix(tc.want, "go.mod") && !strings.Contains(got[0], tc.file.Path) {
				t.Fatalf("violations(%v) = %q, want it to name %s", tc.file, got, tc.file.Path)
			}
		})
	}
}

func TestModuleKeepsItsGuards(t *testing.T) {
	goMod, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatalf("reading go.mod: %v", err)
	}
	if got := violations(module, string(goMod), moduleSources(t, "../..")); len(got) != 0 {
		t.Fatalf("the module breaks its guards:\n%s", strings.Join(got, "\n"))
	}
}

// allowedImports says, per package folder, which packages of this module its non-test files may import.
func allowedImports() map[string][]string {
	return map[string][]string{
		"cmd/plimsoll":                   nil, // composition: anything
		"internal/mcpserver":             {"internal/answer"},
		"internal/answer":                {"internal/snapshot"},
		"internal/shape":                 {"internal/answer", "internal/snapshot"},
		"internal/process":               {"internal/answer", "internal/snapshot"},
		"internal/settings":              {"internal/answer", "internal/snapshot"},
		"internal/hotspots":              {"internal/answer", "internal/snapshot"},
		"internal/cochange":              {"internal/answer", "internal/snapshot"},
		"internal/links":                 {"internal/answer", "internal/snapshot"},
		"internal/snapshot":              {"internal/conventions", "internal/parse"},
		"internal/snapshot/snapshottest": {"internal/snapshot", "internal/parse/parsetest"},
		"internal/parse":                 {},
		"internal/parse/parsetest":       {"internal/parse"},
		"internal/conventions":           {},
		"internal/gitrepo":               {"internal/snapshot"},
		"internal/gitrepo/gitrepotest":   {},
	}
}

func violations(module, goMod string, files []sourceFile) []string {
	var found []string
	inBlock := false
	for line := range strings.Lines(goMod) {
		entry := strings.TrimSpace(line)
		switch {
		case entry == "require (":
			inBlock = true
			continue
		case inBlock && entry == ")":
			inBlock = false
			continue
		case strings.HasPrefix(entry, "require "):
			entry = strings.TrimSpace(strings.TrimPrefix(entry, "require "))
		case !inBlock:
			continue
		}
		// Only the MCP SDK and wazero are required directly; what they need in turn is marked indirect.
		if entry != "" && !strings.HasPrefix(entry, mcpSDK+" ") && !strings.HasPrefix(entry, wazero+" ") && !strings.HasSuffix(entry, "// indirect") {
			found = append(found, "go.mod requires a dependency: "+entry)
		}
	}
	table := allowedImports()
	for _, file := range files {
		folder := path.Dir(file.Path)
		isTest := strings.HasSuffix(file.Path, "_test.go")
		for _, imported := range file.Imports {
			switch {
			case imported == "net" || strings.HasPrefix(imported, "net/"):
				found = append(found, file.Path+" imports "+imported+", a network package")
			case imported == "os/exec" && !isTest && folder != "internal/gitrepo" && !strings.HasPrefix(folder, "internal/gitrepo/"):
				found = append(found, file.Path+" imports os/exec: it runs programs outside internal/gitrepo")
			case strings.HasPrefix(imported, module+"/"):
				if !isTest && !mayImport(table, folder, strings.TrimPrefix(imported, module+"/")) {
					found = append(found, file.Path+" imports "+imported+", "+againstTheImportDirection)
				}
			case imported == mcpSDK || strings.HasPrefix(imported, mcpSDK+"/"):
				if !isTest && folder != "internal/mcpserver" {
					found = append(found, file.Path+" imports "+imported+", the MCP SDK outside internal/mcpserver")
				}
			case imported == wazero || strings.HasPrefix(imported, wazero+"/"):
				if !isTest && folder != "internal/parse" {
					found = append(found, file.Path+" imports "+imported+", wazero outside internal/parse")
				}
			case strings.Contains(strings.SplitN(imported, "/", 2)[0], "."):
				found = append(found, file.Path+" imports "+imported+", outside the standard library")
			}
		}
	}
	return found
}

func mayImport(table map[string][]string, folder, target string) bool {
	allowed, known := table[folder]
	if !known {
		return false
	}
	if allowed == nil {
		return true
	}
	return slices.Contains(allowed, target)
}

func moduleSources(t *testing.T, root string) []sourceFile {
	t.Helper()
	var files []sourceFile
	for name, content := range moduleFiles(t, root) {
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), name, content, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("reading the imports of %s: %v", name, err)
		}
		file := sourceFile{Path: name}
		for _, spec := range parsed.Imports {
			file.Imports = append(file.Imports, strings.Trim(spec.Path.Value, `"`))
		}
		files = append(files, file)
	}
	return files
}

// moduleFiles are the module's Go files and its go.mod, by path from the root, outside the folders that hold no
// source of it.
func moduleFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	fileSystem := os.DirFS(root)
	err := fs.WalkDir(fileSystem, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "projects", "bin", "testdata", ".superpowers":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") && name != "go.mod" {
			return nil
		}
		content, err := fs.ReadFile(fileSystem, name)
		files[name] = string(content)
		return err
	})
	if err != nil {
		t.Fatalf("reading the module's sources: %v", err)
	}
	return files
}

// TestNoticesNameEveryRedistributedComponent keeps THIRD_PARTY_NOTICES.md complete: every module of go.mod and every
// source of the grammar modules is redistributed in the binary and needs its licence.
func TestNoticesNameEveryRedistributedComponent(t *testing.T) {
	notices, err := os.ReadFile("../../THIRD_PARTY_NOTICES.md")
	if err != nil {
		t.Fatalf("reading the notices: %v", err)
	}
	goMod, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	pins, err := os.ReadFile("../../internal/parse/grammars/pins.txt")
	if err != nil {
		t.Fatal(err)
	}
	var components []string
	inBlock := false
	for line := range strings.Lines(string(goMod)) {
		fields := strings.Fields(line)
		switch {
		case len(fields) == 2 && fields[0] == "require" && fields[1] == "(":
			inBlock = true
		case inBlock && len(fields) > 0 && fields[0] == ")":
			inBlock = false
		case inBlock && len(fields) >= 2:
			components = append(components, fields[0])
		case len(fields) >= 3 && fields[0] == "require":
			components = append(components, fields[1])
		}
	}
	for line := range strings.Lines(string(pins)) {
		fields := strings.Fields(line)
		if len(fields) != 8 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		if strings.HasPrefix(fields[0], "zig-") {
			components = append(components, "zig")
		} else {
			components = append(components, strings.TrimPrefix(fields[1], "https://"))
		}
	}
	for _, component := range slices.Compact(slices.Sorted(slices.Values(components))) {
		heading := "\n## " + component + "\n"
		at := strings.Index(string(notices), heading)
		if at < 0 || !strings.HasPrefix(string(notices)[at+len(heading):], "\nLicence: ") {
			t.Errorf("THIRD_PARTY_NOTICES.md has no section %q followed by a line \"Licence: <identifier>\"", component)
		}
	}
}

// The setup page holds the README's install and connect commands, unchanged and in the same order, so an agent that
// follows the page runs what the README documents. The paste line ("text") is the README's own.
func TestTheSetupPageCarriesTheReadmeCommands(t *testing.T) {
	blocks := func(content []byte, err error, from, to string) []string {
		if err != nil {
			t.Fatal(err)
		}
		text := string(content)
		if from != "" {
			start, end := strings.Index(text, from), strings.Index(text, to)
			if start < 0 || end < start {
				t.Fatalf("the document lacks the part from %q to %q", from, to)
			}
			text = text[start:end]
		}
		var found []string
		for i, part := range strings.Split(text, "```")[1:] {
			if i%2 == 0 && !strings.HasPrefix(part, "text\n") {
				found = append(found, part)
			}
		}
		return found
	}
	readme, err := os.ReadFile("../../README.md")
	page, pageErr := os.ReadFile("../../docs/agent-setup.md")
	readmeBlocks, pageBlocks := blocks(readme, err, "## Install", "## Tools"), blocks(page, pageErr, "## 2. Install and connect", "## 3.")
	if !slices.Equal(pageBlocks, readmeBlocks) {
		t.Errorf("docs/agent-setup.md step 2 blocks:\n%q\nwant the README's install and connect blocks:\n%q", pageBlocks, readmeBlocks)
	}
}

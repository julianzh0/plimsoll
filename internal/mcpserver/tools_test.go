package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/juzhxng/plimsoll/internal/answer"
	"github.com/juzhxng/plimsoll/internal/cochange"
	"github.com/juzhxng/plimsoll/internal/conventions"
	"github.com/juzhxng/plimsoll/internal/hotspots"
	"github.com/juzhxng/plimsoll/internal/links"
	"github.com/juzhxng/plimsoll/internal/process"
	"github.com/juzhxng/plimsoll/internal/settings"
	"github.com/juzhxng/plimsoll/internal/shape"
	"github.com/juzhxng/plimsoll/internal/snapshot"
	"github.com/juzhxng/plimsoll/internal/snapshot/snapshottest"
)

// check validates a decoded JSON value against the keywords the output schema uses: type, required, properties,
// items, enum and a local $ref.
func check(root, schema map[string]any, value any, where string) []string {
	if ref, found := schema["$ref"].(string); found {
		name := strings.TrimPrefix(ref, "#/$defs/")
		definitions, _ := root["$defs"].(map[string]any)
		target, _ := definitions[name].(map[string]any)
		return check(root, target, value, where)
	}
	var problems []string
	if values, found := schema["enum"].([]any); found && !slices.Contains(values, value) {
		problems = append(problems, fmt.Sprintf("%s: %v is not one of %v", where, value, values))
	}
	switch schema["type"] {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return append(problems, where+": not an object")
		}
		required, _ := schema["required"].([]any)
		for _, name := range required {
			if _, present := object[fmt.Sprint(name)]; !present {
				problems = append(problems, fmt.Sprintf("%s: %s is missing", where, name))
			}
		}
		properties, _ := schema["properties"].(map[string]any)
		for name, member := range object {
			if sub, known := properties[name].(map[string]any); known {
				problems = append(problems, check(root, sub, member, where+"."+name)...)
			}
		}
	case "array":
		list, ok := value.([]any)
		if !ok {
			return append(problems, where+": not an array")
		}
		items, _ := schema["items"].(map[string]any)
		for i, item := range list {
			problems = append(problems, check(root, items, item, fmt.Sprintf("%s[%d]", where, i))...)
		}
	case "string":
		if _, ok := value.(string); !ok {
			problems = append(problems, where+": not a string")
		}
	case "integer":
		if number, ok := value.(float64); !ok || number != float64(int64(number)) {
			problems = append(problems, where+": not an integer")
		}
	case "number":
		if _, ok := value.(float64); !ok {
			problems = append(problems, where+": not a number")
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			problems = append(problems, where+": not a boolean")
		}
	}
	return problems
}

func TestSchemaCheckerFindsAMissingMember(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal([]byte(outputSchema), &schema); err != nil {
		t.Fatal(err)
	}
	if problems := check(schema, schema, map[string]any{"question": "other"}, "answer"); len(problems) < 2 {
		t.Errorf("check(an incomplete answer) = %v, want the missing members and the wrong question", problems)
	}
}

// everyDetector is a source in which each detector has a mark: a planning file, a setting without a reader, a
// compose-only variable and a pair of files that changed together.
func everyDetector() snapshot.Source {
	source := snapshottest.Files(map[string]string{"src/a.ts": "x\n"}, map[string]string{
		"src/a.ts": "x\n", "src/b.ts": "y\n", ".env.example": "UNREAD=1\n", "docs/stories/US-1.md": "x\n",
		"compose.yml": "services:\n  app:\n    environment:\n      - ONLY_COMPOSE\n",
		"src/c.ts":    "import \"../tests/x.ts\";\n", "tests/x.ts": "export const x = 1;\n",
		"src/x/a.ts": "import \"../y/b.ts\";\n", "src/y/b.ts": "export const b = 1;\n",
		"src/y/c.ts": "import \"../x/d.ts\";\n", "src/x/d.ts": "export const d = 1;\n",
	})
	for i := range 5 {
		source.Commits = append(source.Commits, snapshottest.Commit(fmt.Sprintf("c%09d", i), "src/a.ts", "src/b.ts"))
	}
	source.Since = []string{"c000000000"}
	return source
}

func TestEveryAnswerMatchesTheOutputSchema(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal([]byte(outputSchema), &schema); err != nil {
		t.Fatalf("the output schema is not JSON: %v", err)
	}
	sources := map[string]snapshot.Source{
		"unchanged": snapshottest.Files(map[string]string{"src/a.ts": "a\n"}, map[string]string{"src/a.ts": "a\n"}),
		"marks": snapshottest.Files(
			map[string]string{"src/new.ts": "x\n", "src/grown.ts": snapshottest.Lines(1600), "src/gone.ts": snapshottest.Lines(1600)},
			map[string]string{"src/new.ts": snapshottest.Lines(1600), "src/grown.ts": snapshottest.Lines(1700)},
		),
		"no product code": snapshottest.Files(nil, map[string]string{"README.md": "x\n"}),
		"every detector":  everyDetector(),
	}
	for name, source := range sources {
		q := answer.Questions{
			Take: func(context.Context, string, string) (snapshot.Snapshot, error) {
				return snapshot.Build(source, conventions.Default(), snapshottest.Readings(t)), nil
			},
			Detectors: []answer.Detector{shape.Detector{}, process.Detector{}, settings.Detector{}, hotspots.Detector{}, cochange.Detector{}, links.Detector{}},
			Version:   "0.1.0",
		}
		status, _ := q.Status(context.Background(), "/repository", "main")
		report, _ := q.Report(context.Background(), "/repository", "main", nil)
		answers := []answer.Answer{status, report}
		kinds := map[string]bool{}
		for _, m := range report.Marks {
			if kinds[m.Subject.Kind] {
				continue
			}
			kinds[m.Subject.Kind] = true
			explained, err := q.Explain(context.Background(), "/repository", "main", m.ID)
			if err != nil {
				t.Fatal(err)
			}
			answers = append(answers, explained)
		}
		if name == "every detector" && (!kinds["files"] || !kinds["setting"] || !kinds["file"] || !kinds["folders"]) {
			t.Errorf("every detector: mark kinds %v, want file, files and setting", kinds)
		}
		for _, a := range answers {
			encoded, err := json.Marshal(a)
			if err != nil {
				t.Fatal(err)
			}
			var decoded any
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatal(err)
			}
			if problems := check(schema, schema, decoded, "answer"); len(problems) > 0 {
				t.Errorf("%s %s does not fit the schema: %v", name, a.Question, problems)
			}
		}
	}
}

func TestDescriptionsAndSchemasNameNoWorkflowMoment(t *testing.T) {
	var tools []*mcp.Tool
	for _, q := range questions() {
		tools = append(tools, toolOf(q))
	}
	encoded, err := json.Marshal(tools)
	if err != nil {
		t.Fatal(err)
	}
	if found := regexp.MustCompile(`(?i)\b(story|slice|planning|closure|after a commit|after every edit)\b`).FindString(string(encoded)); found != "" {
		t.Errorf("the tool list names %q", found)
	}
}

func TestReportTakesAnOptionalListOfFiles(t *testing.T) {
	schemas := map[string]map[string]any{}
	for _, q := range questions() {
		tool := toolOf(q)
		var schema map[string]any
		raw, ok := tool.InputSchema.(json.RawMessage)
		if !ok {
			t.Fatalf("%s's input schema is a %T, want json.RawMessage", tool.Name, tool.InputSchema)
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		schemas[tool.Name] = schema
	}
	files, _ := schemas["report"]["properties"].(map[string]any)["files"].(map[string]any)
	items, _ := files["items"].(map[string]any)
	if files["type"] != "array" || items["type"] != "string" ||
		files["description"] != "Paths of tracked files, relative to the repository's top folder. The report then also gives the files that import each of them." {
		t.Errorf("report's files property = %v", files)
	}
	if required := fmt.Sprint(schemas["report"]["required"]); required != "[repository base]" {
		t.Errorf("report's required = %s, want repository and base", required)
	}
	for _, name := range []string{"status", "explain"} {
		properties, _ := schemas[name]["properties"].(map[string]any)
		if _, has := properties["files"]; has {
			t.Errorf("%s takes files, want only report", name)
		}
	}
	source := snapshottest.Files(nil, map[string]string{"src/a.ts": "import \"./b.ts\";\n", "src/b.ts": "export const b = 1;\n"})
	s := Server{Questions: answer.Questions{
		Take: func(context.Context, string, string) (snapshot.Snapshot, error) {
			return snapshot.Build(source, conventions.Default(), snapshottest.Readings(t)), nil
		},
		Detectors: []answer.Detector{links.Detector{}}, Version: "0.1.0",
	}}
	call := func(tool, arguments string) *mcp.CallToolResult {
		index := slices.IndexFunc(questions(), func(q question) bool { return q.name == tool })
		request := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: tool, Arguments: json.RawMessage(arguments)}}
		result, err := s.handler(questions()[index], make(chan struct{}, concurrentAnswers))(context.Background(), request)
		if err != nil {
			t.Fatalf("call(%s, %s) = %v", tool, arguments, err)
		}
		return result
	}
	text := func(r *mcp.CallToolResult) string {
		content, ok := r.Content[0].(*mcp.TextContent)
		if !ok {
			t.Fatalf("content = %T, want text", r.Content[0])
		}
		return content.Text
	}
	named := call("report", `{"repository":"/repository","base":"main","files":["src/b.ts"]}`)
	if named.IsError || !strings.Contains(text(named), "named file `src/b.ts`") {
		t.Errorf("report with files = %+v, want the named row", named.Content)
	}
	for _, arguments := range []string{`{"repository":"/repository","base":"main","files":"src/a.ts"}`, `{"repository":"/repository","base":"main","files":[1]}`} {
		if r := call("report", arguments); !r.IsError || text(r) != "The argument `files` must be a list of text." {
			t.Errorf("report(%s) = %+v, want the list refusal", arguments, r.Content)
		}
	}
	if r := call("status", `{"repository":"/repository","base":"main","files":[]}`); !r.IsError || text(r) != "Unknown argument `files`." {
		t.Errorf("status with files = %+v, want an unknown argument", r.Content)
	}
}

package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/julianzh0/plimsoll/internal/answer"
)

// The descriptions say what each tool returns and which of the three to use for which need, in plain words, never at
// what moment of a task to ask; the output schema already lists the answer's parts, so they are not repeated here.
const (
	statusDescription = "The short answer about what a change did to a git repository since `base`: the areas it made worse, " +
		"better or could not analyse and the files outside the change that use what it altered (its blast radius), at most five new or grown " +
		"marks, each with its limit and a way to check it, then one line each counting the marks shown only in " +
		"`report`, the items the change touched but did not create, added code without a use, and the standing " +
		"marks. A mark is one measured finding; a standing mark was already over its limit at `base`. Use this " +
		"for the short answer; use `report` for every mark and number, and `explain` for the evidence behind one " +
		"mark."
	reportDescription = "The full answer about what a change did to a git repository since `base`: the same areas and blast radius, every mark, every measured " +
		"number with its value at `base`, the standing marks one by one, those the repository acknowledges in its" +
		" .plimsoll.toml (acknowledging is done in that file, not through a tool), the footprint of the change, " +
		"and what was not analysed. Use this when the five marks of `status` are not enough or you need the " +
		"numbers; use `explain` for the evidence behind one mark. For each repository file named in `files`, the " +
		"answer also lists the files that import it."
	explainDescription = "The evidence behind one mark, computed again from the repository: the files, lines and patterns it rests" +
		" on, the counts, and what was left out. `mark` is the identifier exactly as `status` or `report` printed" +
		" it, with the same `base`. Use this to check one mark; use `status` or `report` to find the marks."
)

const (
	repositoryProperty = `"repository":{"type":"string","description":"Absolute path of the repository: its top folder or any folder inside its working tree."}`
	baseProperty       = `"base":{"type":"string","description":"The revision to compare with: a commit, a tag or a branch. When it is not an ancestor of the head, their merge base is used and the answer says so. Without it, the default base the repository declares in .plimsoll.toml is used."}`
	markProperty       = `"mark":{"type":"string","description":"The identifier of one mark, as status or report printed it, for example shape.file-size:src/store.ts."}`
	filesProperty      = `"files":{"type":"array","items":{"type":"string"},"description":"Paths of tracked files, relative to the repository's top folder. The report then also gives the files that import each of them."}`
)

// outputSchema is the schema of every answer, the same for all three tools.
const outputSchema = `{"type":"object","required":["question","version","base","head","counts","marks","more","reportOnly","resolved","touched","summaries","standing","history","detectors","notAnalysed"],"properties":{"question":{"enum":["status","report","explain"]},"version":{"type":"string"},"base":{"type":"object","required":["asked","commit","mergeBase"],"properties":{"asked":{"type":"string"},"commit":{"type":"string"},"mergeBase":{"type":"boolean"},"declaredIn":{"type":"string"}}},"head":{"type":"object","required":["commit","modified"],"properties":{"commit":{"type":"string"},"modified":{"type":"boolean"}}},"footprint":{"type":"object","required":["commits","files","product","test","docs"],"properties":{"commits":{"type":"integer"},"files":{"type":"integer"},"product":{"$ref":"#/$defs/lines"},"test":{"$ref":"#/$defs/lines"},"docs":{"$ref":"#/$defs/lines"},"testPerProduct":{"type":"number"}}},"counts":{"type":"object","required":["new","grown","resolved"],"properties":{"new":{"type":"integer"},"grown":{"type":"integer"},"resolved":{"type":"integer"}}},"marks":{"type":"array","items":{"$ref":"#/$defs/mark"}},"more":{"type":"integer"},"resolved":{"type":"array","items":{"$ref":"#/$defs/mark"}},"reportOnly":{"type":"array","items":{"$ref":"#/$defs/mark"}},"touched":{"type":"array","items":{"$ref":"#/$defs/mark"}},"summaries":{"type":"array","items":{"type":"string"}},"standing":{"type":"array","items":{"$ref":"#/$defs/mark"}},"acknowledged":{"type":"array","items":{"$ref":"#/$defs/mark"}},"acknowledgedIn":{"type":"string"},"history":{"type":"object","required":["read","usable"],"properties":{"read":{"type":"integer"},"usable":{"type":"integer"}}},"detectors":{"type":"array","items":{"type":"string"}},"notAnalysed":{"type":"array","items":{"type":"object","required":["detector","what"],"properties":{"detector":{"type":"string"},"what":{"type":"string"}}}},"headline":{"type":"object","required":["worse","better","areas"],"properties":{"worse":{"type":"integer"},"better":{"type":"integer"},"areas":{"type":"array","items":{"type":"object","required":["name","state","marks","resolved"],"properties":{"name":{"enum":["size","dependencies","wiring","functions","settings","process"]},"state":{"enum":["pass","fail","not analysed"]},"marks":{"type":"array","items":{"type":"string"}},"resolved":{"type":"array","items":{"type":"string"}}}}}}},"radius":{"type":"object","required":["state","files","byName","asWhole","product","tests","other","inChange","unread","paths","omitted"],"properties":{"state":{"enum":["counted","at least","not analysed"]},"files":{"type":"integer"},"byName":{"type":"integer"},"asWhole":{"type":"integer"},"product":{"type":"integer"},"tests":{"type":"integer"},"other":{"type":"integer"},"inChange":{"type":"integer"},"unread":{"type":"integer"},"paths":{"type":"array","items":{"type":"string"}},"omitted":{"type":"integer"}}},"sections":{"type":"array","items":{"type":"object","required":["detector","statistics","notes"],"properties":{"detector":{"type":"string"},"statistics":{"type":"array","items":{"$ref":"#/$defs/statistic"}},"notes":{"type":"array","items":{"type":"string"}}}}},"evidence":{"type":"object","required":["mark","lines","omitted"],"properties":{"mark":{"type":"string"},"lines":{"type":"array","items":{"type":"string"}},"omitted":{"type":"integer"}}}},"$defs":{"lines":{"type":"object","required":["added","removed"],"properties":{"added":{"type":"integer"},"removed":{"type":"integer"}}},"mark":{"type":"object","required":["id","change","subject","measured","confidence","hint"],"properties":{"id":{"type":"string"},"change":{"enum":["new","grown","resolved","standing","touched"]},"subject":{"type":"object","required":["kind"],"properties":{"kind":{"type":"string"},"paths":{"type":"array","items":{"type":"string"}},"names":{"type":"array","items":{"type":"string"}}}},"measured":{"type":"object","required":["value","limitSource","unit"],"properties":{"value":{"type":"number"},"before":{"type":"number"},"limit":{"type":"number"},"limitSource":{"type":"string"},"total":{"type":"number"},"unit":{"type":"string"}}},"confidence":{"enum":["exact","candidate"]},"hint":{"type":"string"}}},"statistic":{"type":"object","required":["name"],"properties":{"name":{"type":"string"},"subject":{"type":"string"},"value":{"type":"number"},"before":{"type":"number"},"unit":{"type":"string"},"note":{"type":"string"}}}}}`

func inputSchema(properties, required string) json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{` + properties + `},"required":[` + required + `],"additionalProperties":false}`)
}

// optional says whether a call may leave a text argument out; a missing one is "". Without a base, the
// repository's declared default base is used.
func optional(name string) bool { return name == "base" }

// question is one tool: its description, the text arguments it takes, in order, the name of one optional argument
// that is a list of text ("" for none), and how it is asked.
type question struct {
	name, description string
	arguments         []string
	list              string
	ask               func(ctx context.Context, q answer.Questions, values, items []string) (answer.Answer, error)
}

// questions are the three tools, always in this order.
func questions() []question {
	return []question{
		{"status", statusDescription, []string{"repository", "base"}, "", func(ctx context.Context, q answer.Questions, v, _ []string) (answer.Answer, error) {
			return q.Status(ctx, v[0], v[1])
		}},
		{"report", reportDescription, []string{"repository", "base"}, "files", func(ctx context.Context, q answer.Questions, v, files []string) (answer.Answer, error) {
			return q.Report(ctx, v[0], v[1], files)
		}},
		{"explain", explainDescription, []string{"repository", "base", "mark"}, "", func(ctx context.Context, q answer.Questions, v, _ []string) (answer.Answer, error) {
			return q.Explain(ctx, v[0], v[1], v[2])
		}},
	}
}

// toolOf is the tool of one question.
func toolOf(q question) *mcp.Tool {
	properties := map[string]string{"repository": repositoryProperty, "base": baseProperty, "mark": markProperty, "files": filesProperty}
	var members, required []string
	for _, name := range q.arguments {
		members = append(members, properties[name])
		if !optional(name) {
			required = append(required, `"`+name+`"`)
		}
	}
	if q.list != "" {
		members = append(members, properties[q.list])
	}
	closed := false
	return &mcp.Tool{
		Name: q.name, Description: q.description,
		InputSchema:  inputSchema(strings.Join(members, ","), strings.Join(required, ",")),
		OutputSchema: json.RawMessage(outputSchema),
		Annotations:  &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closed},
	}
}

// arguments checks a call's arguments against the names it takes, in this order: unknown ones (the first in
// byte order), missing ones (in the order of names), values that are not text, then the optional list. An optional
// text argument that is left out is "".
func arguments(given map[string]json.RawMessage, names []string, list string) (values, items []string, err error) {
	unknown := slices.Sorted(maps.Keys(given))
	unknown = slices.DeleteFunc(unknown, func(name string) bool { return slices.Contains(names, name) || list != "" && name == list })
	if len(unknown) > 0 {
		return nil, nil, answer.UnknownArgument(unknown[0])
	}
	for _, name := range names {
		if _, found := given[name]; !found && !optional(name) {
			return nil, nil, answer.MissingArgument(name)
		}
	}
	values = make([]string, 0, len(names))
	for _, name := range names {
		raw, found := given[name]
		if !found {
			values = append(values, "")
			continue
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, nil, answer.NotText(name)
		}
		values = append(values, value)
	}
	if raw, found := given[list]; list != "" && found {
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, nil, answer.NotTextList(list)
		}
	}
	return values, items, nil
}

// refusal is a tool result that carries one sentence and no answer.
func refusal(err error) *mcp.CallToolResult {
	sentence := fmt.Sprintf("git could not read the repository: `%s`.", err)
	if refused, ok := errors.AsType[*answer.Refusal](err); ok {
		sentence = refused.Sentence
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: sentence}}, IsError: true}
}

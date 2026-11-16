package main

import (
	"slices"
	"strconv"
	"testing"
)

// The protocol itself is the MCP SDK's; these tests pin what plimsoll decides: its name and version, tools only,
// a tool list that never changes and may be kept for five minutes, and the tools' fixed members.

func TestModernNegotiation(t *testing.T) {
	responses, _ := exchange(t,
		`{"jsonrpc":"2.0","id":"d1","method":"server/discover","params":{`+modern+`}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{`+modern+`}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2027-01-01","io.modelcontextprotocol/clientCapabilities":{}}}}`,
	)
	discover := decode(t, responses[0])["result"]
	if at(t, discover, "capabilities") == nil || len(as[map[string]any](t, at(t, discover, "capabilities"))) != 1 ||
		len(as[map[string]any](t, at(t, discover, "capabilities", "tools"))) != 0 ||
		at(t, discover, "ttlMs") != float64(300000) || at(t, discover, "_meta", "io.modelcontextprotocol/serverInfo", "name") != "plimsoll" ||
		!slices.Contains(as[[]any](t, at(t, discover, "supportedVersions")), any("2026-07-28")) {
		t.Errorf("server/discover = %s, want tools only, a five-minute TTL, plimsoll and 2026-07-28", responses[0])
	}
	list := as[map[string]any](t, decode(t, responses[1])["result"])
	if list["ttlMs"] != float64(300000) {
		t.Errorf("tools/list = %s, want a five-minute TTL", responses[1])
	}
	var names []string
	for _, entry := range as[[]any](t, list["tools"]) {
		tool := as[map[string]any](t, entry)
		names = append(names, as[string](t, tool["name"]))
		annotations, _ := tool["annotations"].(map[string]any)
		if tool["description"] == nil || tool["inputSchema"] == nil || tool["outputSchema"] == nil ||
			annotations["readOnlyHint"] != true || annotations["openWorldHint"] != false {
			t.Errorf("tool %v lacks a member it must have", tool["name"])
		}
	}
	if slices.Sort(names); !slices.Equal(names, []string{"explain", "report", "status"}) {
		t.Errorf("tool names = %v, want explain, report and status", names)
	}
	if code := at(t, decode(t, responses[2]), "error", "code"); code != float64(-32022) {
		t.Errorf("an unknown protocol version = %s, want the unsupported-version error", responses[2])
	}
}

func TestLegacyHandshake(t *testing.T) {
	initialize := func(id int, version string) string {
		return `{"jsonrpc":"2.0","id":` + strconv.Itoa(id) + `,"method":"initialize","params":{"protocolVersion":"` + version + `","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
	}
	responses, _ := exchange(t,
		initialize(1, "2025-11-25"),
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"status","arguments":{}}}`,
	)
	sameJSON(t, responses[0], `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-11-25","capabilities":{"tools":{}},
		"serverInfo":{"name":"plimsoll","version":"`+version+`"}}}`)
	if tools := as[[]any](t, at(t, decode(t, responses[1]), "result", "tools")); len(tools) != 3 {
		t.Errorf("legacy tools/list = %s, want three tools", responses[1])
	}
	if at(t, decode(t, responses[2]), "result", "content", 0, "text") != "The argument `repository` is missing." {
		t.Errorf("legacy tools/call = %s, want the missing-argument refusal", responses[2])
	}
	got, _ := exchange(t, initialize(1, "2025-06-18"))
	if negotiated := at(t, decode(t, got[0]), "result", "protocolVersion"); negotiated != "2025-06-18" {
		t.Errorf("initialize 2025-06-18 answered %v", negotiated)
	}
}

func TestProtocolErrors(t *testing.T) {
	responses, _ := exchange(t,
		`{"jsonrpc":"2.0","id":1,"method":"resources/list","params":{`+modern+`}}`,
		`{"jsonrpc":"2.0","id":2,"method":"server/discover","params":{`+modern+`}}`+"\n"+`{"jsonrpc":"2.0","id":3,"method":"server/discover","params":{`+modern+`}}`,
	)
	if resources, _ := at(t, decode(t, responses[0]), "result", "resources").([]any); len(resources) != 0 {
		t.Errorf("resources/list = %s, want none: plimsoll has tools only", responses[0])
	}
	if len(responses) != 3 || decode(t, responses[1])["id"] != float64(2) || decode(t, responses[2])["id"] != float64(3) {
		t.Errorf("two requests in one write = %q, want ids 2 and 3 answered", responses)
	}
}

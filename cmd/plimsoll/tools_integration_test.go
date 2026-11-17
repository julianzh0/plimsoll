//go:build integration

package main

import (
	"encoding/json"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/gitrepo/gitrepotest"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

// grownRepository is a repository whose src/a.ts grew from 1,480 to 1,620 lines and which gained a dependency.
func grownRepository(t *testing.T) (*gitrepotest.Repo, string) {
	t.Helper()
	repo := gitrepotest.New(t)
	base := repo.Commit(gitrepotest.Commit{Message: "base", Files: map[string]string{
		"src/a.ts": snapshottest.Lines(1480), "package.json": `{"dependencies": {"a": "1"}}`,
	}})
	repo.Commit(gitrepotest.Commit{Message: "grow", Files: map[string]string{
		"src/a.ts": snapshottest.Lines(1620), "package.json": `{"dependencies": {"a": "1", "b": "1"}}`,
	}})
	return repo, base
}

// declaredRepository is a repository on branch feature, cut from main, whose .plimsoll.toml declares the given
// default base; src/a.ts grew on the feature branch from 1,480 to 1,620 lines.
func declaredRepository(t *testing.T, base string) *gitrepotest.Repo {
	t.Helper()
	repo := gitrepotest.New(t)
	repo.Commit(gitrepotest.Commit{Message: "main", Files: map[string]string{
		"src/a.ts": snapshottest.Lines(1480), ".plimsoll.toml": "[defaults]\nbase = \"" + base + "\"\n",
	}})
	repo.Git("switch", "-q", "-c", "feature")
	repo.Commit(gitrepotest.Commit{Message: "grow", Files: map[string]string{"src/a.ts": snapshottest.Lines(1620)}})
	return repo
}

// call is a modern tools/call request.
func call(id int, tool string, arguments map[string]any) string {
	encoded, _ := json.Marshal(arguments)
	return `{"jsonrpc":"2.0","id":` + jsonNumber(id) + `,"method":"tools/call","params":{"name":"` + tool + `","arguments":` + string(encoded) + `,` + modern + `}}`
}

func jsonNumber(n int) string {
	encoded, _ := json.Marshal(n)
	return string(encoded)
}

func result(t *testing.T, line string) map[string]any {
	t.Helper()
	return as[map[string]any](t, decode(t, line)["result"])
}

func text(t *testing.T, r map[string]any) string {
	t.Helper()
	return as[string](t, at(t, r, "content", 0, "text"))
}

func TestIntegrationToolsAnswerForAChangedRepository(t *testing.T) {
	repo, base := grownRepository(t)
	before := gitrepotest.Fingerprint(t, repo.Dir)
	arguments := map[string]any{"repository": repo.Dir, "base": base}
	explainArguments := map[string]any{"repository": repo.Dir, "base": base, "mark": "shape.file-size:src/a.ts"}
	responses, _ := exchange(t, call(1, "status", arguments), call(2, "report", arguments), call(3, "explain", explainArguments))
	status, report, explain := result(t, responses[0]), result(t, responses[1]), result(t, responses[2])

	first, _, _ := strings.Cut(text(t, status), "\n")
	if status["isError"] == true || !regexp.MustCompile("^plimsoll "+regexp.QuoteMeta(version)+" status: base `[0-9a-f]{40}` [0-9a-f]{10}, head [0-9a-f]{10}$").MatchString(first) {
		t.Errorf("status = %s", responses[0])
	}
	structured := as[map[string]any](t, status["structuredContent"])
	if structured["question"] != "status" || !strings.Contains(responses[0], `"id":"shape.file-size:src/a.ts"`) {
		t.Errorf("status structuredContent = %v, want the size mark", structured)
	}
	footprint := "Footprint of the change since the base: 1 commit, 2 files; product +140 -0, test +0 -0, documentation +0 -0; 0.0 test lines added per product line added."
	if at(t, report, "structuredContent", "sections", 0, "detector") != "shape" || !strings.Contains(text(t, report), footprint) {
		t.Errorf("report = %s", responses[1])
	}
	evidence := as[[]any](t, at(t, explain, "structuredContent", "evidence", "lines"))
	if len(evidence) == 0 || evidence[0] != "lines now: 1,620" {
		t.Errorf("explain evidence = %v, want lines now first", evidence)
	}

	declared, _ := exchange(t, call(4, "status", map[string]any{"repository": declaredRepository(t, "main").Dir}))
	declaredFirst, _, _ := strings.Cut(text(t, result(t, declared[0])), "\n")
	if !regexp.MustCompile("^plimsoll "+regexp.QuoteMeta(version)+" status: base `main` \\(declared in `\\.plimsoll\\.toml`\\) [0-9a-f]{10}, head [0-9a-f]{10}$").MatchString(declaredFirst) ||
		!strings.Contains(declared[0], `"declaredIn":".plimsoll.toml"`) || !strings.Contains(declared[0], `"id":"shape.file-size:src/a.ts"`) {
		t.Errorf("status without a base = %s, want the declared base main in the header and the size mark", declared[0])
	}

	// The declared base is the branch the head is on: the question compares the head with itself.
	onMain := gitrepotest.New(t)
	onMain.Commit(gitrepotest.Commit{Message: "main", Files: map[string]string{"src/a.ts": "a\n", ".plimsoll.toml": "[defaults]\nbase = \"main\"\n"}})
	self, _ := exchange(t, call(5, "status", map[string]any{"repository": onMain.Dir}))
	selfText := text(t, result(t, self[0]))
	if !strings.Contains(selfText, "base `main` (declared in `.plimsoll.toml`)") || !strings.Contains(selfText, "0 new, 0 grown, 0 resolved") ||
		!strings.Contains(selfText, "0 commits") {
		t.Errorf("status on the declared base's own branch = %s, want the declared base and 0 commits compared", self[0])
	}

	legacyCall := strings.Replace(call(1, "status", arguments), ","+modern, "", 1)
	legacy, _ := exchange(t, `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{}}}`, legacyCall)
	legacyResult := result(t, legacy[1])
	if legacyResult["resultType"] != nil || text(t, legacyResult) != text(t, status) {
		t.Errorf("legacy status = %s, want the same text without resultType", legacy[1])
	}
	if gitrepotest.Fingerprint(t, repo.Dir) != before {
		t.Errorf("the tools changed the repository")
	}
}

func TestIntegrationRefusals(t *testing.T) {
	repo, base := grownRepository(t)
	unknown, _ := exchange(t, call(1, "summary", map[string]any{"repository": repo.Dir, "base": base}))
	if code := at(t, decode(t, unknown[0]), "error", "code"); code != float64(-32602) {
		t.Errorf("an unknown tool = %s, want the invalid-params error", unknown[0])
	}
	cases := []struct {
		name      string
		tool      string
		arguments map[string]any
		want      string
	}{
		{"an unknown base", "status", map[string]any{"repository": repo.Dir, "base": "nope"}, "Unknown base `nope`: git finds no such commit in this repository."},
		{"no repository", "status", map[string]any{"repository": t.TempDir(), "base": "HEAD"}, "`repository` does not name a folder inside a git working tree."},
		{"an unknown mark", "explain", map[string]any{"repository": repo.Dir, "base": base, "mark": "shape.file-size:nope.ts"},
			"Unknown mark `shape.file-size:nope.ts`: nothing with this identifier exists against this base now. `report` lists the current identifiers."},
		{"a missing base", "status", map[string]any{"repository": repo.Dir},
			"The question names no base, and no tracked `.plimsoll.toml` declares a default base."},
		{"a declared base git does not know", "status", map[string]any{"repository": declaredRepository(t, "nope").Dir},
			"Unknown base `nope`, the default base `.plimsoll.toml` declares: git finds no branch, remote branch or tag of this name in this repository."},
		{"a declared base without its form", "report", map[string]any{"repository": declaredRepository(t, "zzmarker~1").Dir},
			"The question names no base, and the default base of `.plimsoll.toml` cannot be used: `base` is not the name of a branch, a remote branch or a tag."},
		{"an unknown argument", "status", map[string]any{"repository": repo.Dir, "base": base, "bse": "x"}, "Unknown argument `bse`."},
		{"a number for the repository", "status", map[string]any{"repository": 7, "base": base}, "The argument `repository` must be text."},
		{"a relative path", "status", map[string]any{"repository": "relative/path", "base": base}, "`repository` must be an absolute path."},
		{"a text for files", "report", map[string]any{"repository": repo.Dir, "base": base, "files": "src/a.ts"}, "The argument `files` must be a list of text."},
	}
	for i, tc := range cases {
		responses, _ := exchange(t, call(i+2, tc.tool, tc.arguments))
		r := result(t, responses[0])
		content := as[[]any](t, r["content"])
		if _, structured := r["structuredContent"]; r["isError"] != true || len(content) != 1 || text(t, r) != tc.want || structured {
			t.Errorf("%s = %s, want isError, one text %q and no structuredContent", tc.name, responses[0], tc.want)
		}
	}
}

func TestIntegrationRepositoryInALanguageWithoutAnEntry(t *testing.T) {
	repo := gitrepotest.New(t, gitrepotest.Commit{Message: "base", Files: map[string]string{"bin/a.sh": "echo a\n", "bin/b.sh": "echo b\necho c\n"}})
	responses, _ := exchange(t, call(1, "status", map[string]any{"repository": repo.Dir, "base": "HEAD"}))
	found := false
	for _, entry := range as[[]any](t, at(t, result(t, responses[0]), "structuredContent", "notAnalysed")) {
		item := as[map[string]any](t, entry)
		found = found || item["detector"] == "repository" && item["what"] == "not verified for `.sh` (3 lines in 2 files)"
	}
	if !found {
		t.Errorf("status of a shell repository = %s, want the not-verified entry", responses[0])
	}
}

func TestIntegrationAnswersAreRepeatableAndLeaveNoTrace(t *testing.T) {
	repo, base := grownRepository(t)
	before := gitrepotest.Fingerprint(t, repo.Dir)
	arguments := map[string]any{"repository": repo.Dir, "base": base}
	responses, stderr := exchange(t,
		call(1, "status", arguments), call(2, "report", arguments),
		call(3, "explain", map[string]any{"repository": repo.Dir, "base": base, "mark": "shape.file-size:src/a.ts"}),
		call(4, "status", arguments),
		call(5, "report", map[string]any{"repository": repo.Dir, "base": base, "files": []string{"src/a.ts", "README.md"}}),
		call(6, "report", map[string]any{"repository": repo.Dir, "base": base, "files": []string{"README.md", "src/a.ts"}}),
	)
	// The same answer: the same result, whatever the request's id.
	sameAnswer := func(a, b string) bool { return reflect.DeepEqual(decode(t, a)["result"], decode(t, b)["result"]) }
	if !sameAnswer(responses[4], responses[5]) {
		t.Errorf("report with the files in two orders differs:\n%s\n%s", responses[4], responses[5])
	}
	if !sameAnswer(responses[0], responses[3]) {
		t.Errorf("the same status twice differs:\n%s\n%s", responses[0], responses[3])
	}
	alone, _ := exchange(t, call(1, "status", arguments))
	if !sameAnswer(alone[0], responses[0]) {
		t.Errorf("a second process answers differently:\n%s\n%s", alone[0], responses[0])
	}
	if gitrepotest.Fingerprint(t, repo.Dir) != before {
		t.Errorf("the calls changed the repository")
	}
	if stderr != "" {
		t.Errorf("standard error = %q, want nothing", stderr)
	}
	declared := declaredRepository(t, "main")
	withoutBase := map[string]any{"repository": declared.Dir}
	twice, _ := exchange(t, call(1, "status", withoutBase), call(2, "status", withoutBase))
	if !sameAnswer(twice[0], twice[1]) {
		t.Errorf("the same status without a base twice differs:\n%s\n%s", twice[0], twice[1])
	}
	unchanged, _ := exchange(t, call(1, "status", map[string]any{"repository": repo.Dir, "base": "HEAD"}))
	if words := regexp.MustCompile(`(?i)\b(clean|healthy)\b`).FindString(text(t, result(t, unchanged[0]))); words != "" {
		t.Errorf("the status of an unchanged repository says %q", words)
	}
}

func TestIntegrationEveryDetectorAnswersForOneRepository(t *testing.T) {
	repo := gitrepotest.New(t)
	repo.Commit(gitrepotest.Commit{Message: "start", Files: map[string]string{
		"src/config.ts": "export const port = process.env.PORT;\n", ".env.example": "PORT=1\n",
		"docker-compose.yml":   "services:\n  app:\n    environment:\n      - PORT=1\n      - REDIS_URL=${REDIS_URL}\n",
		"src/store.ts":         snapshottest.Lines(1400),
		"tests/fakes/store.ts": snapshottest.Lines(10),
	}})
	var base string
	for i := 1; i <= 3; i++ {
		base = repo.Commit(gitrepotest.Commit{Message: "grow " + jsonNumber(i), Files: map[string]string{
			"src/store.ts": snapshottest.Lines(1400 + i), "tests/fakes/store.ts": snapshottest.Lines(10 + i),
		}})
	}
	repo.Commit(gitrepotest.Commit{Message: "past the limit", Files: map[string]string{
		"src/store.ts": snapshottest.Lines(1620), "tests/fakes/store.ts": snapshottest.Lines(20),
		".env.example": "PORT=1\nSESSION_GRACE_MS=2\n", "docs/review-packet-1.md": "notes\n",
		"src/wire.ts": "import \"../tests/fakes/store.ts\";\n",
	}})
	before := gitrepotest.Fingerprint(t, repo.Dir)
	arguments := map[string]any{"repository": repo.Dir, "base": base}
	explained := []string{
		"process.planning-file:docs/review-packet-1.md", "settings.no-reader:SESSION_GRACE_MS", "co-change.pair:src/store.ts+tests/fakes/store.ts",
		"links.test-import:src/wire.ts",
	}
	requests := []string{call(1, "report", arguments)}
	named := map[string]any{"repository": repo.Dir, "base": base, "files": []string{"src/store.ts"}}
	for i, mark := range explained {
		requests = append(requests, call(i+2, "explain", map[string]any{"repository": repo.Dir, "base": base, "mark": mark}))
	}
	requests = append(requests, call(len(requests)+1, "report", named))
	responses, _ := exchange(t, requests...)
	report := as[map[string]any](t, result(t, responses[0])["structuredContent"])
	namedRow := false
	for _, section := range as[[]any](t, at(t, result(t, responses[len(responses)-1]), "structuredContent", "sections")) {
		s := as[map[string]any](t, section)
		for _, row := range as[[]any](t, s["statistics"]) {
			namedRow = namedRow || s["detector"] == "links" && as[map[string]any](t, row)["name"] == "named file"
		}
	}
	if !namedRow {
		t.Errorf("report with files = %s, want a named file row in the links section", responses[len(responses)-1])
	}

	want := []any{"shape", "process", "settings", "hotspots", "co-change", "links"}
	var sections []any
	for _, section := range as[[]any](t, report["sections"]) {
		sections = append(sections, as[map[string]any](t, section)["detector"])
	}
	if !slices.Equal(as[[]any](t, report["detectors"]), want) || !slices.Equal(sections, want) {
		t.Errorf("detectors = %v, sections = %v, want %v", report["detectors"], sections, want)
	}
	hints := map[string]any{}
	for _, list := range []string{"marks", "reportOnly", "standing"} {
		for _, entry := range as[[]any](t, report[list]) {
			m := as[map[string]any](t, entry)
			hints[list+" "+as[string](t, m["id"])] = m["hint"]
		}
	}
	for _, id := range []string{"reportOnly shape.file-size:src/store.ts", "marks process.planning-file:docs/review-packet-1.md",
		"marks settings.no-reader:SESSION_GRACE_MS", "reportOnly co-change.pair:src/store.ts+tests/fakes/store.ts", "standing settings.compose-only:REDIS_URL",
		"reportOnly links.test-import:src/wire.ts"} {
		if _, found := hints[id]; !found {
			t.Errorf("report holds no %s; it holds %v", id, slices.Sorted(maps.Keys(hints)))
		}
	}
	if hint := hints["reportOnly shape.file-size:src/store.ts"]; hint != "count the file's lines at the base and now." {
		t.Errorf("size hint = %q", hint)
	}
	for i, mark := range explained {
		r := result(t, responses[i+1])
		if r["isError"] == true || len(as[[]any](t, at(t, r, "structuredContent", "evidence", "lines"))) == 0 {
			t.Errorf("explain %s = %s, want evidence", mark, responses[i+1])
		}
	}
	if gitrepotest.Fingerprint(t, repo.Dir) != before {
		t.Errorf("the tools changed the repository")
	}
}

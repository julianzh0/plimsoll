package settings

import (
	"maps"
	"slices"
	"testing"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

// detect runs the detector and checks what every mark must carry.
func detect(t *testing.T, source snapshot.Source) (snapshot.Snapshot, answer.Findings) {
	t.Helper()
	s := snapshot.Build(source, conventions.Default(), snapshottest.Readings(t))
	f := Detector{}.Detect(s)
	for _, m := range f.Marks {
		if m.ID == "" || m.Change == "" || m.Subject.Kind != "setting" || len(m.Subject.Names) != 1 || m.Measured.Unit == "" ||
			m.Measured.LimitSource == "" || m.Hint == "" || m.Confidence != answer.Candidate {
			t.Errorf("mark %+v lacks a field it must carry", m)
		}
	}
	return s, f
}

func marksOf(f answer.Findings, rule string) map[string]answer.Mark {
	found := map[string]answer.Mark{}
	for _, m := range f.Marks {
		if answer.RuleOf(m.ID) == rule {
			found[m.Subject.Names[0]] = m
		}
	}
	return found
}

func TestDefinedTwiceCountsLinesThatAreNotCommentedOut(t *testing.T) {
	cases := []struct {
		base, now string
		want      answer.Change
	}{
		{"XX=1\n", "XX=1\nXX=2\n", answer.New}, {"XX=1\n", "# XX=1\nXX=2\n", ""},
		{"XX=1\nXX=2\n", "XX=1\nXX=2\nXX=3\n", answer.Grown}, {"XX=1\nXX=2\n", "XX=1\nXX=2\n", answer.Standing},
	}
	for _, tc := range cases {
		code := "XX\n"
		_, f := detect(t, snapshottest.Files(map[string]string{".env.example": tc.base, "src/a.ts": code}, map[string]string{".env.example": tc.now, "src/a.ts": code}))
		m, found := marksOf(f, ruleDefinedTwice)["XX"]
		if tc.want == "" && found || tc.want != "" && m.Change != tc.want {
			t.Errorf("defined-twice from %q to %q = %+v, want %q", tc.base, tc.now, m, tc.want)
		}
	}
	_, f := detect(t, snapshottest.Files(map[string]string{".env.example": "XX=1\n", "src/a.ts": "XX\n"}, map[string]string{".env.example": "XX=1\nXX=2\n", "src/a.ts": "XX\n"}))
	if m := marksOf(f, ruleDefinedTwice)["XX"]; m.Measured.Value != 2 || *m.Measured.Before != 1 {
		t.Errorf("defined-twice new: measured %+v, want 2 now and 1 before", m.Measured)
	}
}

func TestComposeOnlyMarks(t *testing.T) {
	compose := "services:\n  api:\n    environment:\n      - PORT=1\n      - REDIS_URL=${REDIS_URL}\n      - NODE_OPTIONS=--max-old-space-size=1024\n"
	code := "PORT\nREDIS_URL\n"
	_, f := detect(t, snapshottest.Files(nil, map[string]string{".env.example": "PORT=1\n", "docker-compose.yml": compose, "src/a.ts": code}))
	if marks := marksOf(f, ruleComposeOnly); len(marks) != 1 || marks["REDIS_URL"].Change != answer.New {
		t.Errorf("compose-only marks = %v, want REDIS_URL alone: NODE_OPTIONS has a fixed value", slices.Collect(maps.Keys(marks)))
	}
	if hint := marksOf(f, ruleComposeOnly)["REDIS_URL"].Hint; hint != "set under `environment` in `docker-compose.yml`; `.env.example` has no line for it." {
		t.Errorf("new compose-only hint = %q", hint)
	}
	_, resolved := detect(t, snapshottest.Files(map[string]string{".env.example": "PORT=1\n", "docker-compose.yml": compose, "src/a.ts": code},
		map[string]string{".env.example": "PORT=1\n", "docker-compose.yml": "services:\n  api:\n    environment:\n      - PORT=1\n", "src/a.ts": code}))
	if m := marksOf(resolved, ruleComposeOnly)["REDIS_URL"]; m.Change != answer.Resolved ||
		m.Hint != "set under `environment` in `docker-compose.yml` at the base; `.env.example` has no line for it." {
		t.Errorf("resolved compose-only mark = %+v", m)
	}
	_, commented := detect(t, snapshottest.Files(nil, map[string]string{".env.example": "PORT=1\n# REDIS_URL=\n", "docker-compose.yml": compose, "src/a.ts": code}))
	if len(marksOf(commented, ruleComposeOnly)) != 0 {
		t.Errorf("a commented-out setting still counts as named: marks %v", commented.Marks)
	}
	_, unread := detect(t, snapshottest.Files(nil, map[string]string{".env.example": "PORT=1\n", "docker-compose.yml": compose + "    env_file: .env\n  db:\n    environment: {A: 1}\n", "src/a.ts": code}))
	if !slices.Contains(unread.NotAnalysed, "`docker-compose.yml`: 2 lines of `environment` could not be read") {
		t.Errorf("unread compose lines: not analysed %q", unread.NotAnalysed)
	}
	_, none := detect(t, snapshottest.Files(nil, map[string]string{".env.example": "PORT=1\n", "src/a.ts": code}))
	if slices.ContainsFunc(none.Statistics, func(row answer.Statistic) bool { return row.Name == "compose variables without a setting" }) || !slices.Contains(none.NotAnalysed, "no compose file tracked, so no compose variable was compared") {
		t.Errorf("no compose file: %+v", none)
	}
}

func TestBrokenSettingsFileStopsTheAudit(t *testing.T) {
	for _, side := range []string{"now", "base"} {
		source := snapshottest.Files(map[string]string{".env.example": "XX=1\n"}, map[string]string{".env.example": "XX=1\n"})
		entries := source.Now
		if side == "base" {
			entries = source.Before
		}
		entries[0].Content, entries[0].Binary = nil, true
		source.Changes = []snapshot.Change{{Path: ".env.example"}}
		_, f := detect(t, source)
		if len(f.Marks) != 0 || len(f.Statistics) != 0 || !slices.Contains(f.NotAnalysed, "`.env.example` could not be read, so no settings were compared") {
			t.Errorf("unreadable at %s: %+v", side, f)
		}
	}
}

func TestNoSettingsFileStopsTheAudit(t *testing.T) {
	_, f := detect(t, snapshottest.Files(nil, map[string]string{"src/a.ts": "export const a = 1;\n"}))
	if len(f.Marks) != 0 || len(f.Statistics) != 0 || len(f.NotAnalysed) != 0 {
		t.Errorf("no .env.example: %+v", f)
	}
}

func TestDefinedTwiceRunsOnAReadSettingsFileOnly(t *testing.T) {
	read := snapshottest.Files(map[string]string{".env.example": "A=1\n"}, map[string]string{".env.example": "A=2\n"})
	if _, f := detect(t, read); !slices.Equal(f.RanRules, []string{"settings.defined-twice"}) {
		t.Errorf("a read .env.example: ran %q", f.RanRules)
	}
	unread := snapshottest.Files(map[string]string{".env.example": "A=1\n"}, map[string]string{".env.example": "A=1\n"})
	unread.Now[0].Content, unread.Now[0].Binary = nil, true
	if _, f := detect(t, unread); len(f.RanRules) != 0 {
		t.Errorf("an unread .env.example: ran %q, want none", f.RanRules)
	}
	if _, f := detect(t, snapshottest.Files(nil, map[string]string{"a.txt": "a\n"})); len(f.RanRules) != 0 {
		t.Errorf("no .env.example: ran %q, want none", f.RanRules)
	}
}

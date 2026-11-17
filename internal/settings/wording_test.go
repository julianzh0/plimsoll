package settings

import (
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

func setting(rule, name string, change answer.Change, value float64, before, total *float64) answer.Mark {
	return answer.Mark{ID: answer.ID("settings", rule, name), Change: change, Subject: answer.Subject{Kind: "setting", Names: []string{name}},
		Measured: answer.Measured{Value: value, Before: before, Total: total}}
}

func TestSettingsWordingStandingAndEvidence(t *testing.T) {
	cases := map[string]answer.Mark{
		"`XX` is defined on 2 lines of `.env.example`; it was on 1 at the base.":                            setting(ruleDefinedTwice, "XX", answer.New, 2, new(1.0), nil),
		"`XX` is defined on 2 lines of `.env.example`; `.env.example` did not exist at the base.":           setting(ruleDefinedTwice, "XX", answer.New, 2, nil, nil),
		"`XX` is defined on 1 line of `.env.example`, no longer on more than one; it was on 2 at the base.": setting(ruleDefinedTwice, "XX", answer.Resolved, 1, new(2.0), nil),
		"`REDIS_URL` is set in the `environment` of 2 compose files; `.env.example` has no line for it.":    setting(ruleComposeOnly, "REDIS_URL", answer.Standing, 2, nil, nil),
		"`REDIS_URL` no longer stands as a compose variable without a setting.":                             setting(ruleComposeOnly, "REDIS_URL", answer.Resolved, 0, new(1.0), nil),
	}
	for want, m := range cases {
		if got := (Detector{}).Wording(m); got != want {
			t.Errorf("Wording(%s %s) =\n%q\nwant\n%q", m.ID, m.Change, got, want)
		}
	}
	var items []answer.Mark
	for range 8 {
	}
	items = append(items, setting(ruleDefinedTwice, "B", answer.Standing, 2, nil, nil))
	for range 13 {
		items = append(items, setting(ruleComposeOnly, "C", answer.Standing, 1, nil, nil))
	}
	want := "1 name defined on more than one line; 13 compose variables without a setting"
	if got := (Detector{}).Standing(items); got != want {
		t.Errorf("Standing = %q, want %q", got, want)
	}
	s, f := detect(t, snapshottest.Files(nil, map[string]string{
		".env.example": "SECRET_TOKEN=hunter2\nSECRET_TOKEN=x\n", "docker-compose.yml": "a:\n  environment:\n    - OTHER=swordfish\n",
	}))
	for _, m := range f.Marks {
		lines := append(Detector{}.Explain(s, m), Detector{}.Wording(m), m.Hint)
		for _, line := range lines {
			if strings.Contains(line, "hunter2") || strings.Contains(line, "swordfish") {
				t.Errorf("%s holds a value: %q", m.ID, line)
			}
		}
	}
}

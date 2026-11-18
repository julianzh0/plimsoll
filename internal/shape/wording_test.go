package shape

import (
	"testing"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/conventions"
)

const (
	ratioLimitSource = "plimsoll's rule: test code at most twice the product code"
	sizeLimitSource  = "plimsoll's default, taken from Pylint and SonarQube"
)

func ratio(change answer.Change, now float64, before *float64) answer.Mark {
	return answer.Mark{ID: answer.ID(name, "test-ratio", "repository"), Change: change,
		Measured: answer.Measured{Value: now, Before: before, Limit: new(float64(2)), LimitSource: ratioLimitSource}}
}

func size(change answer.Change, now float64, before *float64, limit float64) answer.Mark {
	return answer.Mark{ID: answer.ID(name, "file-size", "src/a.ts"), Change: change, Subject: answer.Subject{Kind: "file", Paths: []string{"src/a.ts"}},
		Measured: answer.Measured{Value: now, Before: before, Limit: new(limit), LimitSource: sizeLimitSource}}
}

func addition(rule, path string, names []string, now float64, before *float64) answer.Mark {
	return answer.Mark{ID: answer.ID(name, rule, path), Change: answer.Grown, Subject: answer.Subject{Kind: "file", Paths: []string{path}, Names: names},
		Measured: answer.Measured{Value: now, Before: before, LimitSource: additionSource}}
}

func TestWordingOfEachRuleAndChange(t *testing.T) {
	cases := []struct {
		mark answer.Mark
		want string
	}{
		{ratio(answer.New, 2.1, new(1.9)), "The test ratio is 2.1, over the limit of 2.0 (plimsoll's rule: test code at most twice the product code); it was 1.9 at the base."},
		{ratio(answer.New, 2.1, nil), "The test ratio is 2.1, over the limit of 2.0 (plimsoll's rule: test code at most twice the product code); there was no product code at the base."},
		{ratio(answer.Grown, 3.8, new(3.7)), "The test ratio rose from 3.7 to 3.8; the limit is 2.0 (plimsoll's rule: test code at most twice the product code)."},
		{ratio(answer.Standing, 3.7, new(3.7)), "The test ratio is 3.7, over the limit of 2.0 (plimsoll's rule: test code at most twice the product code); it was 3.7 at the base."},
		{ratio(answer.Resolved, 1.9, new(2.3)), "The test ratio is 1.9, no longer over the limit of 2.0 (plimsoll's rule: test code at most twice the product code); it was 2.3 at the base."},
		{size(answer.New, 1620, new(float64(1480)), 1500), "`src/a.ts` crossed the limit of 1,500 lines in this change: 1,480 → 1,620 (+140). Limit: plimsoll's default, taken from Pylint and SonarQube."},
		{size(answer.New, 1600, nil, 1500), "`src/a.ts` is new in this change with 1,600 lines, over the limit of 1,500. Limit: plimsoll's default, taken from Pylint and SonarQube."},
		{size(answer.Grown, 3640, new(float64(3515)), 1500), "`src/a.ts` grew in this change: 3,515 → 3,640 (+125) lines, already over the limit of 1,500 at the base. Limit: plimsoll's default, taken from Pylint and SonarQube."},
		{size(answer.Standing, 1550, new(float64(1600)), 1500), "`src/a.ts` has 1,550 lines, over the limit of 1,500 (plimsoll's default, taken from Pylint and SonarQube); it had 1,600 at the base."},
		{size(answer.Resolved, 1410, new(float64(1620)), 1500), "`src/a.ts` has 1,410 lines, no longer over the limit of 1,500 (plimsoll's default, taken from Pylint and SonarQube); it had 1,620 at the base."},
		{size(answer.Resolved, 0, new(float64(1620)), 1500), "`src/a.ts` is no longer tracked; it had 1,620 lines at the base, over the limit of 1,500 (plimsoll's default, taken from Pylint and SonarQube)."},
		{addition("dependencies", "package.json", []string{"ioredis"}, 6, new(float64(5))), "`package.json` names 1 runtime dependency that was not there at the base: `ioredis` (6 now, 5 at the base)."},
		{addition("dependencies", "package.json", []string{"a", "b"}, 7, new(float64(5))), "`package.json` names 2 runtime dependencies that were not there at the base: `a`, `b` (7 now, 5 at the base)."},
		{addition("dependencies", "package.json", []string{"a", "b"}, 2, nil), "`package.json` names 2 runtime dependencies that were not there at the base: `a`, `b` (2 now; the file did not exist at the base)."},
		{addition("settings", ".env.example", []string{"ALPHA", "BETA"}, 93, new(float64(91))), "`.env.example` names 2 settings that were not there at the base: `ALPHA`, `BETA` (93 now, 91 at the base)."},
		{addition("settings", ".env.example", []string{"ALPHA"}, 92, new(float64(91))), "`.env.example` names 1 setting that was not there at the base: `ALPHA` (92 now, 91 at the base)."},
		{complexity(answer.New, 44, nil), "`src/sync.ts` `syncAll` is new in this change with a cognitive complexity of 44, over the limit of 25. Limit: plimsoll's rule, read blind on agent pull requests; counted as SonarQube's cognitive complexity."},
		{complexity(answer.New, 42, new(float64(17))), "`src/sync.ts` `syncAll` crossed the limit of 25 in this change: cognitive complexity 17 → 42 (+25). Limit: plimsoll's rule, read blind on agent pull requests; counted as SonarQube's cognitive complexity."},
		{nesting(answer.New, 5, nil), "`src/sync.ts` `syncAll` is new in this change and nests 5 levels deep, at or over the limit of 4 (CodeScene's threshold for deep nesting)."},
		{nesting(answer.New, 4, new(float64(2))), "`src/sync.ts` `syncAll` got deeper in this change: nesting 2 → 4 levels, reaching the limit of 4 (CodeScene's threshold for deep nesting)."},
		{nesting(answer.Grown, 6, new(float64(4))), "`src/sync.ts` `syncAll` got deeper in this change: nesting 4 → 6 levels, already at the limit of 4 at the base (CodeScene's threshold for deep nesting)."},
	}
	for _, tc := range cases {
		if got := (Detector{}).Wording(tc.mark); got != tc.want {
			t.Errorf("Wording(%s %s) =\n%s\nwant\n%s", tc.mark.ID, tc.mark.Change, got, tc.want)
		}
	}
}

func TestStandingNamesTheRatioAndCountsFilesPerLimit(t *testing.T) {
	cases := []struct {
		items []answer.Mark
		want  string
	}{
		{[]answer.Mark{ratio(answer.Standing, 3.7, new(3.7)), size(answer.Standing, 1600, new(float64(1600)), 1500), size(answer.Standing, 1700, new(float64(1700)), 1500)},
			"test ratio 3.7 (limit 2.0); 2 files over 1,500 lines"},
		{[]answer.Mark{size(answer.Standing, 1600, new(float64(1600)), 1500), size(answer.Standing, 900, new(float64(900)), 800)},
			"1 file over 800 lines; 1 file over 1,500 lines"},
		{[]answer.Mark{size(answer.Grown, 1700, new(float64(1600)), 1500), addition("dependencies", "package.json", []string{"a"}, 3, new(float64(2))),
			addition("dependencies", "web/package.json", []string{"b"}, 3, new(float64(2))), addition("settings", ".env.example", []string{"X", "Y"}, 4, new(float64(2)))},
			"1 file over 1,500 lines grew, `src/a.ts` the most (+100); runtime dependencies added in 2 manifests (`a`, `b`); 2 settings added"},
		{[]answer.Mark{size(answer.New, 1620, new(float64(1480)), 1500), size(answer.Grown, 1700, new(float64(1600)), 1500)},
			"1 file crossed 1,500 lines, `src/a.ts` at 1,620; 1 file over 1,500 lines grew, `src/a.ts` the most (+100)"},
		{[]answer.Mark{size(answer.New, 1620, new(float64(1480)), 1500), size(answer.New, 1700, nil, 1500)},
			"2 files crossed 1,500 lines, the largest `src/a.ts` at 1,700"},
		{[]answer.Mark{nesting(answer.New, 4, nil), nesting(answer.New, 5, nil)}, "2 functions nested deeper are listed, `src/sync.ts` `syncAll` the deepest of them (5 levels)"},
		{[]answer.Mark{nesting(answer.New, 4, nil)}, "1 function nested deeper is listed, `src/sync.ts` `syncAll` (4 levels)"},
	}
	for _, tc := range cases {
		if got := (Detector{}).Standing(tc.items); got != tc.want {
			t.Errorf("Standing = %q, want %q", got, tc.want)
		}
	}
}

// complexity is a complexity mark of src/sync.ts syncAll with the default limit.
func complexity(change answer.Change, now float64, before *float64) answer.Mark {
	limit := conventions.Default().Limits.FunctionComplexity
	return answer.Mark{ID: answer.ID(name, ruleComplexity, "src/sync.ts:syncAll"), Change: change, Confidence: answer.Candidate,
		Subject:  answer.Subject{Kind: "function", Paths: []string{"src/sync.ts"}, Names: []string{"syncAll"}},
		Measured: answer.Measured{Value: now, Before: before, Limit: &limit.Value, LimitSource: limit.Source, Unit: "cognitive complexity"}}
}

// nesting is a nesting item of src/sync.ts syncAll with the default limit.
func nesting(change answer.Change, now float64, before *float64) answer.Mark {
	limit := conventions.Default().Limits.FunctionNesting
	return answer.Mark{ID: answer.ID(name, ruleNesting, "src/sync.ts:syncAll"), Change: change, Confidence: answer.Candidate,
		Subject:  answer.Subject{Kind: "function", Paths: []string{"src/sync.ts"}, Names: []string{"syncAll"}},
		Measured: answer.Measured{Value: now, Before: before, Limit: &limit.Value, LimitSource: limit.Source, Unit: "levels"}}
}

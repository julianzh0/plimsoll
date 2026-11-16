package cochange

import (
	"slices"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/answer"
)

func TestPairWordingStandingAndEvidence(t *testing.T) {
	value, before, total, limit := 6.0, 5.0, 7.0, 5.0
	m := answer.Mark{ID: "co-change.pair:" + a + "+" + b, Subject: answer.Subject{Kind: "files", Paths: []string{a, b}},
		Measured: answer.Measured{Value: value, Before: &before, Total: &total, Limit: &limit}}
	want := "`src/a.ts` and `tests/fakes/b.ts` changed together in 6 of the 7 commits that touched either; 5 at the base."
	if got := (Detector{}).Wording(m); got != want || strings.Contains(got, "coupling") {
		t.Errorf("Wording = %q, want %q", got, want)
	}
	items := make([]answer.Mark, 35)
	for i := range items {
		items[i] = m
	}
	if got := (Detector{}).Standing(items); got != "35 pairs of files that changed together in at least 5 commits" {
		t.Errorf("Standing(35) = %q", got)
	}
	if got := (Detector{}).Standing(items[:1]); got != "1 pair of files that changed together in at least 5 commits" {
		t.Errorf("Standing(1) = %q", got)
	}
	s := sample{files: files(), base: times(5, a, b), since: times(1, a, b)}.build(t)
	grown := Detector{}.Detect(s).Marks[0]
	got := Detector{}.Explain(s, grown)
	wantLines := []string{
		"shared usable commits: 6; at the base: 5",
		"usable commits that touched either file: 6",
		"`src/a.ts`: product code; `tests/fakes/b.ts`: test infrastructure",
		"commit 5000000001, since the base",
		"commit b000000005", "commit b000000004", "commit b000000003", "commit b000000002", "commit b000000001",
	}
	if !slices.Equal(got, wantLines) {
		t.Errorf("Explain = %q, want %q", got, wantLines)
	}
}

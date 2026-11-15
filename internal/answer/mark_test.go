package answer

import (
	"slices"
	"testing"
)

func TestCompareDerivesTheChange(t *testing.T) {
	cases := []struct {
		name      string
		base, now Reading
		step      float64
		want      Change
		found     bool
	}{
		{"crossed the limit", Reading{false, 1480}, Reading{true, 1620}, 1, New, true},
		{"new and over the limit", Reading{false, 0}, Reading{true, 1600}, 1, New, true},
		{"grew by one step", Reading{true, 1600}, Reading{true, 1601}, 1, Grown, true},
		{"unchanged", Reading{true, 1600}, Reading{true, 1600}, 1, Standing, true},
		{"shrunk, still over", Reading{true, 1600}, Reading{true, 1550}, 1, Standing, true},
		{"back under the limit", Reading{true, 1600}, Reading{false, 1400}, 1, Resolved, true},
		{"deleted", Reading{true, 1600}, Reading{false, 0}, 1, Resolved, true},
		{"under the limit on both sides", Reading{false, 1400}, Reading{false, 1450}, 1, "", false},
		{"a ratio rising by its step", Reading{true, 3.7}, Reading{true, 3.8}, 0.1, Grown, true},
		{"a ratio unchanged", Reading{true, 2.1}, Reading{true, 2.1}, 0.1, Standing, true},
	}
	for _, tc := range cases {
		got, found := Compare(tc.base, tc.now, tc.step)
		if got != tc.want || found != tc.found {
			t.Errorf("Compare(%s) = %q, %v, want %q, %v", tc.name, got, found, tc.want, tc.found)
		}
	}
}

func TestIDJoinsDetectorRuleAndSubject(t *testing.T) {
	id := ID("shape", "file-size", "src/a+b:c.ts")
	if id != "shape.file-size:src/a+b:c.ts" || DetectorOf(id) != "shape" || RuleOf(id) != "file-size" {
		t.Errorf("ID = %q with detector %q and rule %q, want shape.file-size:src/a+b:c.ts, shape, file-size", id, DetectorOf(id), RuleOf(id))
	}
}

func mark(id string, change Change, confidence Confidence, value float64, limit *float64) Mark {
	return Mark{ID: id, Change: change, Confidence: confidence, Measured: Measured{Value: value, Limit: limit}}
}

func TestOrderPutsNewExactAndFarthestFirstWithTwoPerDetector(t *testing.T) {
	limit := func(v float64) *float64 { return &v }
	marks := []Mark{
		mark("shape.file-size:a.ts", New, Exact, 3000, limit(1500)),
		mark("shape.file-size:b.ts", New, Exact, 1600, limit(1500)),
		mark("shape.file-size:c.ts", New, Exact, 2000, limit(1500)),
		mark("process.planning-file:docs/x.md", New, Exact, 1, nil),
		mark("settings.no-reader:X", New, Candidate, 0, nil),
		mark("shape.test-ratio:repository", Grown, Exact, 3.8, limit(2.0)),
		mark("shape.dependencies:package.json", Grown, Exact, 6, nil),
		mark("co-change.pair:a+b", Grown, Exact, 6, limit(5)),
	}
	want := []string{
		"shape.file-size:a.ts", "shape.file-size:c.ts", "process.planning-file:docs/x.md", "settings.no-reader:X",
		"co-change.pair:a+b", "shape.file-size:b.ts", "shape.test-ratio:repository", "shape.dependencies:package.json",
	}
	reversed := slices.Clone(marks)
	slices.Reverse(reversed)
	for _, input := range [][]Mark{marks, reversed, append(slices.Clone(marks[5:]), marks[:5]...)} {
		var got []string
		for _, m := range Order(input, nil) {
			got = append(got, m.ID)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("Order = %v, want %v", got, want)
		}
	}
	backed := map[string]bool{"shape.test-ratio:repository": true}
	wantBacked := []string{
		"shape.test-ratio:repository", "shape.file-size:a.ts", "process.planning-file:docs/x.md", "settings.no-reader:X",
		"co-change.pair:a+b", "shape.file-size:c.ts", "shape.file-size:b.ts", "shape.dependencies:package.json",
	}
	var got []string
	for _, m := range Order(marks, backed) {
		got = append(got, m.ID)
	}
	if !slices.Equal(got, wantBacked) {
		t.Errorf("Order with a backed grown mark = %v, want it first and the rest in order %v", got, wantBacked)
	}
}

func TestCountAndDecimalFormatNumbers(t *testing.T) {
	for value, want := range map[float64]string{0: "0", 999: "999", 3515: "3,515", 1234567: "1,234,567", -30: "-30"} {
		if got := Count(value); got != want {
			t.Errorf("Count(%v) = %q, want %q", value, got, want)
		}
	}
	for value, want := range map[float64]string{3.7: "3.7", 2: "2.0", 0.8: "0.8"} {
		if got := Decimal(value); got != want {
			t.Errorf("Decimal(%v) = %q, want %q", value, got, want)
		}
	}
}

package snapshot_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

var (
	columns = snapshottest.GoColumns
	files   = snapshottest.GoFiles
	cases   = snapshottest.GoCases
	goFile  = snapshottest.GoFile
)

// repeats lists the pairs the rule found: "<path> <name> <- <counterpart path> <counterpart name> <status>".
func repeats(t *testing.T, base, now map[string]string) ([]string, snapshot.FunctionRepeats) {
	t.Helper()
	found := snapshot.Build(snapshottest.Files(base, now), conventions.Default(), snapshottest.Readings(t)).Repeats
	var lines []string
	for _, r := range found.Listed {
		lines = append(lines, fmt.Sprintf("%s %s <- %s %s %s", r.Path, r.Name, r.Counterpart.Path, r.Counterpart.Name, r.Counterpart.Status))
	}
	return lines, found
}

func TestBuildComparesNoRepeatsWithoutTheCodeItNeeds(t *testing.T) {
	base := map[string]string{"pkg/a.go": goFile(columns("Original"))}
	now := map[string]string{"pkg/a.go": goFile(columns("Original")), "pkg/b.go": goFile(columns("Copied"))}
	for _, tt := range []struct {
		unavailable []string
		listed      int
	}{
		{[]string{"pkg/old.go"}, 0},   // a changed code file whose base the clone lacks: nothing is compared
		{[]string{"data/big.txt"}, 1}, // no code: the rule runs
	} {
		source := snapshottest.Files(base, now)
		source.Unavailable = tt.unavailable
		got := snapshot.Build(source, conventions.Default(), snapshottest.Readings(t)).Repeats
		if len(got.Listed) != tt.listed || (tt.listed == 0) != (len(got.Unavailable) > 0) {
			t.Errorf("Build(unavailable %q).Repeats = %d listed, unavailable %q; want %d listed", tt.unavailable, len(got.Listed), got.Unavailable, tt.listed)
		}
	}
}

func TestBuildFindsAFunctionThatRepeatsAnother(t *testing.T) {
	original := map[string]string{"pkg/a.go": goFile(columns("Original"))}
	with := func(extra map[string]string) map[string]string {
		m := map[string]string{"pkg/a.go": goFile(columns("Original"))}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	tests := []struct {
		name      string
		base, now map[string]string
		want      []string
	}{
		{"a copy in a new file", original, with(map[string]string{"pkg/b.go": goFile(columns("Copied"))}),
			[]string{"pkg/b.go Copied <- pkg/a.go Original unchanged"}},
		{"a function moved and renamed", map[string]string{"pkg/a.go": goFile(columns("Original"), files("ToMove"))},
			with(map[string]string{"pkg/c.go": goFile(files("Moved"))}), nil},
		{"a copy in test support", original, with(map[string]string{"pkg/testing/b.go": goFile(columns("Copied"))}), nil},
		{"two like functions added together", nil, map[string]string{"pkg/x.go": goFile(columns("First")), "pkg/y.go": goFile(columns("Second"))},
			[]string{"pkg/y.go Second <- pkg/x.go First new"}},
		{"a counterpart that was there wins", original,
			with(map[string]string{"pkg/b.go": goFile(columns("NewOne")), "pkg/c.go": goFile(columns("NewTwo"))}),
			[]string{"pkg/b.go NewOne <- pkg/a.go Original unchanged", "pkg/c.go NewTwo <- pkg/a.go Original unchanged"}},
		{"a function the change rewrote", map[string]string{"pkg/a.go": goFile(columns("Original")), "pkg/b.go": goFile(files("Rewritten"))},
			with(map[string]string{"pkg/b.go": goFile(strings.Replace(columns("Original"), "Original", "Rewritten", 1))}), nil},
		{"a long switch beside a short one", map[string]string{"pkg/s.go": goFile(cases("isHook", 8))},
			map[string]string{"pkg/s.go": goFile(cases("isHook", 8)), "pkg/e.go": goFile(cases("isEntity", 60))}, nil},
	}
	for _, tt := range tests {
		got, _ := repeats(t, tt.base, tt.now)
		if strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
			t.Errorf("Build(%s).Repeats = %q, want %q", tt.name, got, tt.want)
		}
	}
}

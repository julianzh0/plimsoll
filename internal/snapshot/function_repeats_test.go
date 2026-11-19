package snapshot_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

// columns is a Go function of about 490 tokens that reads four columns of rows into numbers.
func columns(name string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "func %s(rows [][]string, column int, limit int) (map[string]float64, error) {\n", name)
	for i := 1; i <= 4; i++ {
		fmt.Fprintf(&b, `	values%d := make([]float64, 0, len(rows))
	for n, row := range rows {
		if column >= len(row) || n > limit {
			return nil, fmt.Errorf("row %%d has %%d columns, want %%d", n, len(row), column+%d)
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(row[column]), 64)
		if err != nil {
			return nil, fmt.Errorf("row %%d: %%w", n, err)
		}
		values%d = append(values%d, v*%d)
	}
`, i, i, i, i, i)
	}
	b.WriteString("\treturn map[string]float64{\"a\": values1[0], \"b\": values2[0], \"c\": values3[0], \"d\": values4[0]}, nil\n}\n")
	return b.String()
}

// files is a Go function of about 230 tokens of another shape: it reads files and keeps their trimmed lines.
func files(name string) string {
	return fmt.Sprintf(`func %s(paths []string, keep func(string) bool) ([]string, error) {
	var kept []string
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %%s: %%w", path, err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if keep != nil && !keep(line) {
				continue
			}
			switch {
			case len(kept) > 1000:
				return kept, errors.New("too many lines")
			case strings.Contains(line, "\t"):
				kept = append(kept, strings.ReplaceAll(line, "\t", " "))
			default:
				kept = append(kept, line)
			}
		}
	}
	sort.Strings(kept)
	return kept, nil
}
`, name)
}

// cases is a Go function that is a switch over n names.
func cases(name string, n int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "func %s(name string) bool {\n\tswitch name {\n", name)
	for i := range n {
		fmt.Fprintf(&b, "\tcase \"n%d\":\n\t\treturn true\n", i)
	}
	b.WriteString("\t}\n\treturn false\n}\n")
	return b.String()
}

func goFile(functions ...string) string {
	return "package p\n\n" + strings.Join(functions, "\n")
}

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

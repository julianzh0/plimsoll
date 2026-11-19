package snapshottest

import (
	"fmt"
	"strings"
)

// GoColumns is a Go function of about 490 tokens that reads four columns of rows into numbers.
func GoColumns(name string) string {
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

// GoFile is a Go file of package p that holds the functions.
func GoFile(functions ...string) string {
	return "package p\n\n" + strings.Join(functions, "\n")
}

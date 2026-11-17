package snapshottest

import (
	"fmt"
	"strings"
)

// Function is a TypeScript function of the name whose cognitive complexity is depth*(depth+1)/2 + flat and whose
// deepest nesting is depth: depth ifs inside each other on its lines 2 to depth+1, their closing braces, then flat ifs
// of one line each ("  if (a === 101) { a += 1; }", counting up), "  return a;" and "}". It has 2*depth + flat + 3
// lines and ends with a newline.
func Function(name string, depth, flat int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "function %s(a: number): number {\n", name)
	for level := range depth {
		fmt.Fprintf(&b, "%sif (a === %d) {\n", strings.Repeat("  ", level+1), level+1)
	}
	for level := depth; level > 0; level-- {
		fmt.Fprintf(&b, "%s}\n", strings.Repeat("  ", level))
	}
	for i := range flat {
		fmt.Fprintf(&b, "  if (a === %d) { a += 1; }\n", 101+i)
	}
	b.WriteString("  return a;\n}\n")
	return b.String()
}

package snapshot

import (
	"fmt"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/parse/parsetest"
)

func TestARunPastedTwiceIsOneRepeat(t *testing.T) {
	var block strings.Builder
	for i := range 30 {
		fmt.Fprintf(&block, "  const v%d = load(a, \"n%d\", %d);\n", i, i, i)
	}
	content := []byte("function f(a: number) {\n" + block.String() + block.String() + "  return a;\n}\n")
	rules := conventions.Default()
	language, _ := rules.LanguageOf("a.ts")
	tree, err := parsetest.Parser(t).Nodes("typescript", content)
	if err != nil {
		t.Fatal(err)
	}
	fs := functionsOf(*language.Functions, language.Tokens, rules.FunctionRules, tree, content)
	if len(fs) != 1 || len(fs[0].repeats) != 1 || fs[0].repeats[0].copies[0].startLine != 2 || fs[0].repeats[0].copies[1].startLine != 32 {
		t.Errorf("functionsOf(30 statements written twice) = %+v, want one repeat at lines 2 and 32", fs)
	}
}

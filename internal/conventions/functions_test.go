package conventions

import (
	"maps"
	"slices"
	"testing"

	"github.com/julianzh0/plimsoll/internal/parse/parsetest"
)

func TestEveryKindAFunctionTableNamesIsInItsGrammar(t *testing.T) {
	parser := parsetest.Parser(t)
	for _, l := range Default().Languages {
		// every language plimsoll reads has its functions measured: no entry is left out
		if len(l.Functions.Declared) == 0 {
			t.Errorf("%s: Functions.Declared = none, want the kinds that declare a function", l.Name)
			continue
		}
		if l.Tokens.Pattern == nil || l.Tokens.Pattern.SubexpIndex("ws") < 0 || l.Tokens.Pattern.SubexpIndex("com") < 0 || l.Tokens.Pattern.SubexpIndex("lit") < 0 ||
			l.Tokens.Pattern.SubexpIndex("id") < 0 || l.Tokens.Pattern.SubexpIndex("p") < 0 {
			t.Errorf("%s: Tokens = %v, want a pattern with the groups ws, com, lit, id and p", l.Name, l.Tokens)
		}
		known := map[string]bool{}
		for _, grammar := range append([]string{l.Grammar}, slices.Collect(maps.Values(l.GrammarBySuffix))...) {
			tree, err := parser.Nodes(grammar, nil)
			if err != nil {
				t.Fatalf("Nodes(%s) error = %v", grammar, err)
			}
			for _, kind := range tree.Kinds {
				known[kind] = true
			}
		}
		k := l.Functions
		for _, kind := range slices.Concat(k.Declared, k.Bodies, k.Closures, k.NamedClosures, k.NamedBy, k.Ifs, k.ElseClauses, k.FlatBranches,
			k.Nesting, k.Logic, k.Parentheses, k.Markup, k.DepthIfs, k.Depth, k.Blocks, k.SwitchBlocks, k.Declarators, k.Names, k.MisreadIn) {
			if !known[kind] {
				t.Errorf("%s: its function table names the kind %q, which none of its grammars has", l.Name, kind)
			}
		}
	}
}

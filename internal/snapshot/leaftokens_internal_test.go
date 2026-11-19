package snapshot

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/parse"
)

// TestLeafTokensFollowTheGrammarKinds checks the token kinds of every grammar on a small file: names, keywords and
// operators by their text, a literal (a whole string with its interpolations, a number) as one token, comments dropped.
// Each token of the want is I:<name>, K:<keyword or operator> or L.
func TestLeafTokensFollowTheGrammarKinds(t *testing.T) {
	raw, err := os.ReadFile("testdata/leaf_tokens.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct{ Grammar, Code, Want string }
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	parser, err := parse.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(parser.Close)
	for _, c := range cases {
		tree, err := parser.Nodes(c.Grammar, []byte(c.Code))
		if err != nil {
			t.Fatalf("Nodes(%s) = %v", c.Grammar, err)
		}
		kinds, known := conventions.TokenKindsOf(c.Grammar)
		if !known {
			t.Fatalf("TokenKindsOf(%s) = none, want the grammar's kinds", c.Grammar)
		}
		var got []string
		readable := eachLeafToken(leafKindsOf(tree, kinds), tree, shapeOf(tree.Nodes), 0, len(c.Code), func(class leafClass, start, end int) {
			switch class {
			case leafLiteral:
				got = append(got, "L")
			case leafName:
				got = append(got, "I:"+c.Code[start:end])
			default:
				got = append(got, "K:"+c.Code[start:end])
			}
		})
		if !readable || strings.Join(got, " ") != c.Want {
			t.Errorf("eachLeafToken(%s %q) = %v %q, want true %q", c.Grammar, c.Code, readable, strings.Join(got, " "), c.Want)
		}
	}
}

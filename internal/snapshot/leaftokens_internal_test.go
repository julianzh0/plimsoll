package snapshot

import (
	"slices"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/parse"
)

// TestRunsOfTokensWithNamesAndLiteralsSwappedDiffer checks that a run's hash tells where a name stands and where a
// literal stands, in the view without names: a name and a literal in swapped places are no shared run.
func TestRunsOfTokensWithNamesAndLiteralsSwappedDiffer(t *testing.T) {
	name := tokenID(leafName, []byte("x"))
	call, comma := tokenID(leafKeyword, []byte("(")), tokenID(leafKeyword, []byte(","))
	a := []uint64{name, call, literalToken, comma, name, comma, literalToken, comma, name, call}
	b := []uint64{literalToken, call, name, comma, name, comma, literalToken, comma, name, call}
	if ga, gb := gramsOf(a, 10, withoutNames), gramsOf(b, 10, withoutNames); ga[0] == gb[0] {
		t.Errorf("gramsOf(%v) = gramsOf(%v) = %x, want different runs", a, b, ga[0])
	}
}

// TestLeafTokensFollowTheGrammarKinds checks the token kinds of every grammar on a small file: names, keywords and
// operators by their text, a literal (a whole string with its interpolations, a number) as one token, comments dropped.
// Each token of the want is I:<name>, K:<keyword or operator> or L.
func TestLeafTokensFollowTheGrammarKinds(t *testing.T) {
	cases := []struct{ grammar, code, want string }{
		{"go", "package p\nfunc f(a int) string { /* c */ x := \"a\\n\" + `raw`; if a > 0x1F { return x }; return 'r' }\n",
			"K:package I:p K:func I:f K:( I:a I:int K:) I:string K:{ I:x K::= L K:+ L K:; K:if I:a K:> L K:{ K:return I:x K:} K:; K:return L K:}"},
		{"python", "def f(a):\n    # c\n    s = f\"x{a + 1}y\" \"z\"\n    return s if a else None\n",
			"K:def I:f K:( I:a K:) K:: I:s K:= L K:return I:s K:if I:a K:else K:None"},
		{"typescript", "function f<T>(a: T): string { const s = `x${a}y`; return /re/g.test(s) ? \"a\" : 'b'; }\n",
			"K:function I:f K:< I:T K:> K:( I:a K:: I:T K:) K:: K:string K:{ K:const I:s K:= L K:; K:return L K:. I:test K:( I:s K:) K:? L K:: L K:; K:}"},
		{"tsx", "function F({a}: P) { return <div className=\"c\">{a.b}</div>; }\n",
			"K:function I:F K:( K:{ I:a K:} K:: I:P K:) K:{ K:return K:< I:div I:className K:= L K:> K:{ I:a K:. I:b K:} K:</ I:div K:> K:; K:}"},
		{"tsx", "const x = <T,>(a: T) => a as unknown;\n",
			"K:const I:x K:= K:< I:T K:, K:> K:( I:a K:: I:T K:) K:=> I:a K:as K:unknown K:;"},
		{"java", "class C { int f(int a) { String s = \"x\"; char c = 'y'; return a + 1 + 0x2; } }\n",
			"K:class I:C K:{ K:int I:f K:( K:int I:a K:) K:{ I:String I:s K:= L K:; K:char I:c K:= L K:; K:return I:a K:+ L K:+ L K:; K:} K:}"},
		{"kotlin", "fun f(a: Int): String { val s = \"x${a}y$a\"; return s + 'c' + 1L }\n",
			"K:fun I:f K:( I:a K:: I:Int K:) K:: I:String K:{ K:val I:s K:= L K:return I:s K:+ L K:+ L K:}"},
		{"csharp", "class C { string F(int a) { var s = $\"x{a}y\"; var v = @\"q\"; return s + 'c' + 1.5; } }\n",
			"K:class I:C K:{ K:string I:F K:( K:int I:a K:) K:{ K:var I:s K:= L K:; K:var I:v K:= L K:; K:return I:s K:+ L K:+ L K:; K:} K:}"},
		{"rust", "fn f(a: u32) -> String { let s = format!(\"x{}\", a); let r = r#\"raw\"#; s + 'c'.to_string().as_str() + r }\n",
			"K:fn I:f K:( I:a K:: K:u32 K:) K:-> I:String K:{ K:let I:s K:= I:format K:! K:( L K:, I:a K:) K:; K:let I:r K:= L K:; I:s K:+ L K:. I:to_string K:( K:) K:. I:as_str K:( K:) K:+ I:r K:}"},
		{"swift", "func f(a: Int) -> String { let s = \"x\\(a)y\"; return s + \"\\(1)\" }\n",
			"K:func I:f K:( I:a K:: I:Int K:) K:-> I:String K:{ K:let I:s K:= L K:return I:s K:+ L K:}"},
		{"dart", "String f(int a) { final s = 'x${a}y'; return s + \"z\" + 1.toString(); }\n",
			"I:String I:f K:( I:int I:a K:) K:{ K:final I:s K:= L K:; K:return I:s K:+ L K:+ L K:. I:toString K:( K:) K:; K:}"},
		{"c", "#include <stdio.h>\nint f(int a) { char *s = \"x\" \"y\"; return a + 'c' + 1; }\n",
			"K:#include L K:int I:f K:( K:int I:a K:) K:{ K:char K:* I:s K:= L K:; K:return I:a K:+ L K:+ L K:; K:}"},
		{"cpp", "int f(int a) { auto s = std::string(\"x\"); return a + 1u; }\n",
			"K:int I:f K:( K:int I:a K:) K:{ K:auto I:s K:= I:std K::: I:string K:( L K:) K:; K:return I:a K:+ L K:; K:}"},
	}
	parser, err := parse.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(parser.Close)
	for _, c := range cases {
		tree, err := parser.Nodes(c.grammar, []byte(c.code))
		if err != nil {
			t.Fatalf("Nodes(%s) = %v", c.grammar, err)
		}
		kinds, known := conventions.TokenKindsOf(c.grammar)
		if !known {
			t.Fatalf("TokenKindsOf(%s) = none, want the grammar's kinds", c.grammar)
		}
		var got []string
		readable := !slices.ContainsFunc(tree.Nodes, func(n parse.Node) bool { return n.Error })
		eachLeafToken(leafKindsOf(tree, kinds), shapeOf(tree.Nodes), 0, len(tree.Nodes), func(class leafClass, start, end int) {
			switch class {
			case leafLiteral:
				got = append(got, "L")
			case leafName:
				got = append(got, "I:"+c.code[start:end])
			case leafKeyword:
				got = append(got, "K:"+c.code[start:end])
			}
		})
		if !readable || strings.Join(got, " ") != c.want {
			t.Errorf("eachLeafToken(%s %q) = %v %q, want true %q", c.grammar, c.code, readable, strings.Join(got, " "), c.want)
		}
	}
}

package snapshot

import (
	"slices"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// kinds are the tokens of a text as "text" for code, "=text" for a literal and "@text" for an identifier.
func kinds(t *testing.T, language, text string) []string {
	t.Helper()
	entry, found := conventions.Default().LanguageOf(language)
	if !found {
		t.Fatalf("no language %q", language)
	}
	var out []string
	for tok := range eachToken(entry.Tokens, []byte(text), 1) {
		s := text[tok.start:tok.end]
		switch {
		case tok.identifier:
			s = "@" + s
		case tok.word:
			s = "=" + s
		}
		out = append(out, s)
	}
	return out
}

func TestStringsAreLiteralPiecesAndTheirHolesCode(t *testing.T) {
	for _, tc := range []struct {
		language, text string
		want           []string
	}{
		{"a.cpp", `R"sql(a "b" )sql" x`, []string{`=R"sql(a "b" )sql"`, "@x"}},
		{"a.cpp", `u8R"(a)" x`, []string{`=u8R"(a)"`, "@x"}},
		{"a.cpp", `R"this delimiter is far too long(x)this delimiter is far too long"`, []string{"@R", `="this delimiter is far too long(x)this delimiter is far too long"`}},
		{"a.rs", `r####"a "### b"#### x`, []string{`=r####"a "### b"####`, "@x"}},
		{"a.rs", `br#"a"# cr"b" r#match`, []string{`=br#"a"#`, `=cr"b"`, "@r", "#", "@match"}},
		{"a.rs", `b"bytes"`, []string{`=b"bytes"`}},
		{"a.swift", `#"Hello \#(user.name)"#`, []string{`=#"Hello \#(`, "@user", ".", "@name", `=)"#`}},
		{"a.swift", `"a \(f(x)) b"`, []string{`="a \(`, "@f", "(", "@x", ")", `=) b"`}},
		{"a.swift", `#"no \(hole)"#`, []string{`=#"no \(hole)"#`}},
		{"a.cs", `$"a {x} {{b}}"`, []string{`=$"a {`, "@x", `=} {{b}}"`}},
		{"a.cs", `$$"""{{{x}}}"""`, []string{`=$$"""{{{`, "@x", `=}}}"""`}},
		{"a.cs", `@"a\" x`, []string{`=@"a\"`, "@x"}},
		{"a.cs", `$@"a""{y}"`, []string{`=$@"a""{`, "@y", `=}"`}},
		{"a.py", `f"{left != right}"`, []string{`=f"{`, "@left", "!", "=", "@right", `=}"`}},
		{"a.py", `f"{x!r:>{width}}"`, []string{`=f"{`, "@x", `=!r:>{`, "@width", `=}}"`}},
		{"a.py", `rf'{a}' b'{c}'`, []string{`=rf'{`, "@a", `=}'`, `=b'{c}'`}},
		{"a.ts", "`a ${b + `c ${d}`} e`", []string{"=`a ${", "@b", "+", "=`c ${", "@d", "=}`", "=} e`"}},
		{"a.ts", "`${ {k: 1}.k }`", []string{"=`${", "{", "@k", ":", "1", "}", ".", "@k", "=}`"}},
		{"a.kt", `"a $name ${x.y}" """raw\n$z"""`, []string{`="a $`, "@name", `= ${`, "@x", ".", "@y", `=}"`, `="""raw\n$`, "@z", `="""`}},
		{"a.dart", `'a $b' r'$c' "${d}"`, []string{`='a $`, "@b", `='`, `=r'$c'`, `="${`, "@d", `=}"`}},
		{"a.py", `f"{x // 2}"`, []string{`=f"{`, "@x", "/", "/", "2", `=}"`}},
		{"a.ts", "`a ${ /* } */ b }`", []string{"=`a ${", "@b", "=}`"}},
		{"a.py", `rf"\{{{x}\}}" rf"^\{{(\w+)\}}$"`, []string{`=rf"\{{{`, "@x", `=}\}}"`, `=rf"^\{{(\w+)\}}$"`}},
		{"a.kt", `"a ñ$größe"`, []string{`="a ñ$`, "@größe", `="`}},
		{"a.swift", "###x", []string{"#", "#", "#", "@x"}},
		// still open at a line end or the end of the text: what was handed out stays, the rest is one literal piece
		{"a.py", `f"""a {b} c`, []string{`=f"""a {`, "@b", `=} c`}},
		{"a.ts", "`a ${b", []string{"=`a ${", "@b"}},
		{"a.swift", `#"a`, []string{`=#"a`}},
		{"a.py", "f'a\nb'", []string{`=f'a`, "@b", "'"}},
		{"a.kt", "\"a $name\nx", []string{`="a $`, "@name", "@x"}},
		{"a.py", "f'{x:\ny = 1", []string{`=f'{`, "@x", "=:", "@y", "=", "1"}},
		{"a.py", "f'{x:{y:\nz", []string{`=f'{`, "@x", "=:{", "@y", "=:", "@z"}}, // the line end closes both format texts
	} {
		if got := kinds(t, tc.language, tc.text); !slices.Equal(got, tc.want) {
			t.Errorf("%s tokens(%q) = %q, want %q", tc.language, tc.text, got, tc.want)
		}
	}
}

// Nesting has no limit but the text: 100,000 templates inside each other are read in one pass.
func TestDeeplyNestedHolesAreRead(t *testing.T) {
	const depth = 100000
	text := strings.Repeat("`${", depth) + "x" + strings.Repeat("}`", depth)
	got := kinds(t, "a.ts", text)
	if len(got) != 2*depth+1 || got[depth] != "@x" {
		t.Errorf("tokens of %d nested templates: %d tokens, middle %q; want %d with x in the middle", depth, len(got), got[min(depth, len(got)-1)], 2*depth+1)
	}
}

func FuzzEachToken(f *testing.F) {
	for _, seed := range []string{"`${`${a}`}`", `f"{x!r:{y}}"`, "f'{x:{y:\nz", `#"\#(a)"#`, `$$"""{{b}}"""`, `R"d(x)d"`, `r#"x"#`} {
		f.Add(seed)
	}
	var entries []conventions.Language
	for _, name := range []string{"a.ts", "a.py", "a.swift", "a.cs", "a.cpp", "a.rs", "a.kt", "a.dart"} {
		entry, _ := conventions.Default().LanguageOf(name)
		entries = append(entries, entry)
	}
	f.Fuzz(func(t *testing.T, text string) {
		for _, entry := range entries {
			name := entry.Name
			last := 0
			for tok := range eachToken(entry.Tokens, []byte(text), 1) {
				if tok.start < last || tok.end <= tok.start || tok.end > len(text) {
					t.Fatalf("%s %q: token [%d,%d) after %d, want in order, without overlap, inside the text", name, text, tok.start, tok.end, last)
				}
				last = tok.end
			}
		}
	})
}

// Many R" that open no raw string, in a file of few literals, are read in one pass: each part of the file is searched
// for literals once.
func TestRawStringOpenersAreReadInOnePass(t *testing.T) {
	cpp, _ := conventions.Default().LanguageOf("a.cpp")
	content := []byte(strings.Repeat("x = R\"\n", 50000) + "int y = 1; // \"a\"\n")
	if got := withoutRawStrings(cpp.Literals, content); string(got) != string(content) {
		t.Errorf("withoutRawStrings blanked text that holds no raw string")
	}
}

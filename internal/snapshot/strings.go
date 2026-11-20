package snapshot

import (
	"bytes"
	"unicode"
	"unicode/utf8"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// stringForm is how one string the scanner reads goes on after its opening delimiter.
type stringForm struct {
	closer      string // the closing delimiter
	escape      string // a backslash, with a raw string's hashes, before a character that is then text; "" for none
	holeOpen    string // what opens a hole read as code; "" for none
	holeClose   string // what closes it, at the hole's own bracket level
	doubled     bool   // a doubled { or } is text (C#, Python)
	quoteTwice  bool   // "" is a quote (C# verbatim)
	singleLine  bool   // a line end ends the string
	dollarName  bool   // $name is one identifier (Kotlin, Dart)
	pythonHoles bool   // a hole's conversion and format text, at its own level, are text with holes of their own
}

// stringAt is the string, of the forms a language's scanner reads, that starts at a token's start, and the length of
// its opening delimiter. Other strings are no match here: the token pattern reads them. Its first byte is checked
// before anything else, so code that starts no string costs one comparison.
func stringAt(syntax conventions.StringSyntax, text []byte, at int) (stringForm, int, bool) {
	c := text[at]
	switch syntax {
	case conventions.CppRawStrings:
		if c == 'R' || c == 'u' || c == 'U' || c == 'L' {
			return cppRawAt(text, at)
		}
	case conventions.RustRawStrings:
		if c == 'r' || c == 'b' || c == 'c' {
			return rustRawAt(text, at)
		}
	case conventions.SwiftStrings:
		// a run of # is tried at its first byte only: where the first starts no string, no later one does
		if c == '"' || c == '#' && (at == 0 || text[at-1] != '#') {
			return swiftStringAt(text, at)
		}
	case conventions.CSharpStrings:
		if c == '$' || c == '@' || c == '"' {
			return csharpStringAt(text, at)
		}
	case conventions.PythonStrings:
		if bytes.IndexByte([]byte("rRbBuUfF"), c) >= 0 {
			return pythonStringAt(text, at)
		}
	case conventions.TemplateStrings:
		if c == '`' {
			return stringForm{closer: "`", escape: `\`, holeOpen: "${", holeClose: "}"}, 1, true
		}
	case conventions.KotlinStrings:
		if c == '"' {
			return quotedAt(text, at, stringForm{holeOpen: "${", holeClose: "}", dollarName: true}, false)
		}
	case conventions.DartStrings:
		if c == 'r' && at+1 < len(text) && (text[at+1] == '\'' || text[at+1] == '"') { // raw: no holes, no escapes
			raw, length, _ := quotedAt(text, at+1, stringForm{}, false)
			raw.escape = ""
			return raw, length + 1, true
		}
		if c == '\'' || c == '"' {
			return quotedAt(text, at, stringForm{holeOpen: "${", holeClose: "}", dollarName: true}, true)
		}
	case conventions.PlainStrings:
	}
	return stringForm{}, 0, false
}

// quotedAt is a string in the quote at, or in three of them: a single one ends at a line end and reads escapes; three
// end with three and read escapes when escapesInTriple is set.
func quotedAt(text []byte, at int, form stringForm, escapesInTriple bool) (stringForm, int, bool) {
	quote := text[at]
	if run(text, at, quote) >= 3 {
		form.closer = string([]byte{quote, quote, quote})
		if escapesInTriple {
			form.escape = `\`
		}
		return form, 3, true
	}
	form.closer, form.escape, form.singleLine = string(quote), `\`, true
	return form, 1, true
}

// cppRawAt is a C++ raw string, R"d(…)d" with an optional u8, u, U or L before it: d is at most 16 characters, none of
// them a space, a parenthesis, a backslash or a control character; otherwise no raw string starts.
func cppRawAt(text []byte, at int) (stringForm, int, bool) {
	i := at
	for _, prefix := range []string{"u8", "u", "U", "L"} {
		if bytes.HasPrefix(text[i:], []byte(prefix+`R"`)) {
			i += len(prefix)
			break
		}
	}
	if !bytes.HasPrefix(text[i:], []byte(`R"`)) {
		return stringForm{}, 0, false
	}
	i += 2
	start := i
	for i < len(text) && i-start <= 16 && text[i] != '(' {
		if c := text[i]; c <= ' ' || c == ')' || c == '\\' || c == 0x7f {
			return stringForm{}, 0, false
		}
		i++
	}
	if i >= len(text) || text[i] != '(' || i-start > 16 {
		return stringForm{}, 0, false
	}
	return stringForm{closer: ")" + string(text[start:i]) + `"`}, i + 1 - at, true
}

// rustRawAt is a Rust raw string, r"…" or r#"…"# with any number of #, with an optional b or c before it. r#name with
// no quote is a raw identifier.
func rustRawAt(text []byte, at int) (stringForm, int, bool) {
	i := at
	if i < len(text) && (text[i] == 'b' || text[i] == 'c') {
		i++
	}
	if i >= len(text) || text[i] != 'r' {
		return stringForm{}, 0, false
	}
	hashes := run(text, i+1, '#')
	if i+1+hashes >= len(text) || text[i+1+hashes] != '"' {
		return stringForm{}, 0, false
	}
	return stringForm{closer: `"` + string(bytes.Repeat([]byte("#"), hashes))}, i + 2 + hashes - at, true
}

// swiftStringAt is a Swift string, single-line or in three quotes, with any number of # around it: its escapes and its
// holes, \(…), take as many # after the backslash.
func swiftStringAt(text []byte, at int) (stringForm, int, bool) {
	hashes := run(text, at, '#')
	marks := string(bytes.Repeat([]byte("#"), hashes))
	form := stringForm{escape: `\` + marks, holeOpen: `\` + marks + "(", holeClose: ")"}
	i := at + hashes
	switch {
	case bytes.HasPrefix(text[i:], []byte(`"""`)):
		form.closer = `"""` + marks
		return form, hashes + 3, true
	case i < len(text) && text[i] == '"':
		form.closer, form.singleLine = `"`+marks, true
		return form, hashes + 1, true
	}
	return stringForm{}, 0, false
}

// csharpStringAt is a C# string the token pattern cannot read: raw, in three or more quotes with any number of $
// (holes take as many braces), verbatim (@"…", $@"…", @$"…", where "" is a quote), or interpolated ($"…").
func csharpStringAt(text []byte, at int) (stringForm, int, bool) {
	dollars := run(text, at, '$')
	i := at + dollars
	verbatim := i < len(text) && text[i] == '@'
	if verbatim {
		i++
		if dollars == 0 {
			dollars = run(text, i, '$')
			i += dollars
		}
	}
	quotes := run(text, i, '"')
	if quotes >= 3 && !verbatim {
		form := stringForm{closer: string(bytes.Repeat([]byte(`"`), quotes))}
		if dollars > 0 {
			form.holeOpen, form.holeClose = string(bytes.Repeat([]byte("{"), dollars)), string(bytes.Repeat([]byte("}"), dollars))
		}
		return form, i + quotes - at, true
	}
	if quotes == 0 || dollars == 0 && !verbatim {
		return stringForm{}, 0, false
	}
	form := stringForm{closer: `"`, quoteTwice: verbatim}
	if !verbatim {
		form.escape, form.singleLine = `\`, true
	}
	if dollars > 0 {
		form.holeOpen, form.holeClose, form.doubled = "{", "}", true
	}
	return form, i + 1 - at, true
}

// pythonStringAt is a Python f-string: a prefix of one or two letters of r, b, u and f, with an f, before one or three
// quotes.
func pythonStringAt(text []byte, at int) (stringForm, int, bool) {
	i, formatted := at, false
	for i < len(text) && i-at < 2 && bytes.IndexByte([]byte("rRbBuUfF"), text[i]) >= 0 {
		formatted = formatted || text[i] == 'f' || text[i] == 'F'
		i++
	}
	if !formatted || i >= len(text) || (text[i] != '"' && text[i] != '\'') {
		return stringForm{}, 0, false
	}
	form, length, _ := quotedAt(text, i, stringForm{holeOpen: "{", holeClose: "}", doubled: true, pythonHoles: true}, true)
	return form, i - at + length, true
}

// run is the number of bytes c from at on.
func run(text []byte, at int, c byte) int {
	n := 0
	for at+n < len(text) && text[at+n] == c {
		n++
	}
	return n
}

// identifierEnd is the end of a $name: letters, digits and underscores after its first letter or underscore, as the
// token pattern reads a name; at itself when none starts there.
func identifierEnd(text []byte, at int) int {
	i := at
	for i < len(text) {
		r, size := utf8.DecodeRune(text[i:])
		if r != '_' && !unicode.IsLetter(r) && (i == at || !unicode.IsNumber(r)) {
			break
		}
		i += size
	}
	return i
}

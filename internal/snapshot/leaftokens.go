package snapshot

import (
	"strings"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/parse"
)

// leafClass is what a token of the function-repeat rule is.
type leafClass int

const (
	leafKeyword leafClass = iota // a keyword or an operator, known by its text
	leafName                     // a name (identifier), known by its text, or not at all in the view without names
	leafLiteral                  // a literal with all its inside; every literal is the same token
)

// leafKinds are one grammar's token kinds resolved against one tree's kind names.
type leafKinds struct{ names, literals kindSet }

func leafKindsOf(tree parse.Tree, kinds conventions.TokenKinds) leafKinds {
	return leafKinds{names: kindsOf(tree, kinds.Names), literals: kindsOf(tree, kinds.Literals)}
}

// eachLeafToken calls token for each token inside the bytes [start, stop) of a parsed file, in text order: a node whose
// kind name holds "comment" is dropped with all inside it; a named node of a literal kind is one literal token; any
// other leaf of non-zero width is a name when it is a named node of a name kind, and otherwise a keyword or operator.
// It returns false when an error node lies inside the bytes: the code there cannot be read.
func eachLeafToken(kinds leafKinds, tree parse.Tree, s shape, start, stop int, token func(class leafClass, start, end int)) bool {
	readable := true
	for i := 0; i < len(s.nodes); i++ {
		n := s.nodes[i]
		if int(n.Start) >= stop {
			break
		}
		if int(n.End) <= start {
			i = s.endOf(i) - 1
			continue
		}
		if int(n.Start) < start || int(n.End) > stop { // a node around the bytes: go inside it
			continue
		}
		if n.Error {
			readable = false
		}
		switch {
		case strings.Contains(tree.Kinds[n.Kind], "comment"):
			i = s.endOf(i) - 1
		case n.Named && kinds.literals.has(n):
			token(leafLiteral, int(n.Start), int(n.End))
			i = s.endOf(i) - 1
		case s.endOf(i) == i+1 && n.End > n.Start:
			class := leafKeyword
			if n.Named && kinds.names.has(n) {
				class = leafName
			}
			token(class, int(n.Start), int(n.End))
		}
	}
	return readable
}

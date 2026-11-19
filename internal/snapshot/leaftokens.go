package snapshot

import (
	"hash/fnv"
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

// eachLeafToken calls token for each token of the nodes [from, to) of a parsed file, in text order: a node whose kind
// name holds "comment" is dropped with all inside it; a named node of a literal kind is one literal token; any other
// leaf of non-zero width is a name when it is a named node of a name kind, and otherwise a keyword or operator. It
// returns false when an error node lies among them: the code there cannot be read.
func eachLeafToken(kinds leafKinds, tree parse.Tree, s shape, from, to int, token func(class leafClass, start, end int)) bool {
	readable := true
	for i := from; i < to; i++ {
		n := s.nodes[i]
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

// A token of the function-repeat rule is a number: a hash of its text, the top bit set for a name; every literal is
// one number. The same text gives the same number in any process, so kept readings answer as fresh ones.
const (
	nameBit       uint64 = 1 << 63
	literalToken  uint64 = 1
	anyNameToken  uint64 = nameBit | 1 // every name, in the view without names
	keywordOffset uint64 = 2           // keeps a keyword's number clear of literalToken
)

func tokenID(class leafClass, text []byte) uint64 {
	if class == leafLiteral {
		return literalToken
	}
	h := fnv.New64a()
	_, _ = h.Write(text) // a hash's Write never fails
	if class == leafName {
		return h.Sum64() | nameBit
	}
	return max(h.Sum64()&^nameBit, keywordOffset)
}

// withoutNames is a token in the view without names: every name the same.
func withoutNames(id uint64) uint64 {
	if id&nameBit != 0 {
		return anyNameToken
	}
	return id
}

// functionTokens are the tokens of the nodes [from, to): one function.
func functionTokens(kinds leafKinds, tree parse.Tree, s shape, from, to int, content []byte) []uint64 {
	var ids []uint64
	eachLeafToken(kinds, tree, s, from, to, func(class leafClass, start, end int) {
		ids = append(ids, tokenID(class, content[start:end]))
	})
	return ids
}

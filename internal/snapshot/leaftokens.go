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

// leafKinds are one grammar's token kinds resolved against one tree's kind names; comments are the kinds whose name
// holds "comment".
type leafKinds struct{ names, literals, comments kindSet }

func leafKindsOf(tree parse.Tree, kinds conventions.TokenKinds) leafKinds {
	comments := make(kindSet, len(tree.Kinds))
	for i, kind := range tree.Kinds {
		comments[i] = strings.Contains(kind, "comment")
	}
	return leafKinds{names: kindsOf(tree, kinds.Names), literals: kindsOf(tree, kinds.Literals), comments: comments}
}

// eachLeafToken calls token for each token of the nodes [from, to) of a parsed file, in text order: a node whose kind
// name holds "comment" is dropped with all inside it; a named node of a literal kind is one literal token; any other
// leaf of non-zero width is a name when it is a named node of a name kind, and otherwise a keyword or operator.
func eachLeafToken(kinds leafKinds, s shape, from, to int, token func(class leafClass, start, end int)) {
	for i := from; i < to; i++ {
		n := s.nodes[i]
		switch {
		case kinds.comments.has(n):
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
}

// A token of the function-repeat rule is a number: a hash of its text, the top bit set for a name; every literal is
// one number. The same text gives the same number in any process, so kept readings answer as fresh ones.
const (
	nameBit       uint64 = 1 << 63
	literalToken  uint64 = 1
	anyNameToken  uint64 = nameBit | 1 // every name, in the view without names
	keywordOffset uint64 = 2           // keeps a keyword's number clear of literalToken
	fnvOffset            = 14695981039346656037
	fnvPrime             = 1099511628211
)

func tokenID(class leafClass, text []byte) uint64 {
	if class == leafLiteral {
		return literalToken
	}
	h := uint64(fnvOffset) // FNV-1a, inline: no hasher to allocate per token
	for _, b := range text {
		h = (h ^ uint64(b)) * fnvPrime
	}
	if class == leafName {
		return h | nameBit
	}
	return max(h&^nameBit, keywordOffset)
}

// withoutNames is a token in the view without names: every name the same.
func withoutNames(id uint64) uint64 {
	if id&nameBit != 0 {
		return anyNameToken
	}
	return id
}

// functionTokens are the tokens of the nodes [from, to), one function, and the byte each starts at.
func functionTokens(kinds leafKinds, s shape, from, to int, content []byte) ([]uint64, []int) {
	var ids []uint64
	var starts []int
	eachLeafToken(kinds, s, from, to, func(class leafClass, start, end int) {
		ids = append(ids, tokenID(class, content[start:end]))
		starts = append(starts, start)
	})
	return ids, starts
}

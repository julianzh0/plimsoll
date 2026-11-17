package parse

import (
	"encoding/binary"
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

// Node is one node of a file's syntax tree.
type Node struct {
	Kind       uint16 // index into Tree.Kinds; one index per name
	Named      bool   // false for a token such as "else" or "&&"
	Error      bool   // an ERROR node, or a token the grammar had to assume (a missing closing brace)
	Name       bool   // the node is the name field of its parent
	Depth      uint32 // 0 for the root
	Start, End uint32 // byte offsets into the content
}

// Tree is the nodes of one file in text order, a parent before its children, and the grammar's kind names. Kinds is
// shared by every tree of the grammar: read it, never change it.
type Tree struct {
	Kinds []string
	Nodes []Node
}

// nodeRecord is the size of one node as the shim writes it: kind and flags as little-endian uint16, then depth, start
// byte and end byte as uint32.
const nodeRecord = 16

// The flags of a node record.
const (
	flagNamed = 1
	flagError = 2
	flagName  = 4
)

// The kind tree-sitter gives an ERROR node, outside the grammar's symbol table, and the name it gets as the last kind.
const (
	errorSymbol = 0xFFFF
	errorKind   = "ERROR"
)

// Nodes parses content with the named grammar and returns its tree. Any failure (an unknown grammar, a file the grammar
// cannot read within its memory cap) is an error; the parser stays usable.
func (p *Parser) Nodes(grammar string, content []byte) (Tree, error) {
	return use(p, grammar, func(in *instance) (Tree, error) {
		kinds, err := p.kindNames(grammar, in)
		if err != nil {
			return Tree{}, err
		}
		nodes, err := in.walk(content, kinds)
		return Tree{Kinds: kinds.names, Nodes: nodes}, err
	})
}

// kindTable is a grammar's kind names, with errorKind last, and for each symbol the index of the first kind of its name:
// a grammar may give several symbols one name.
type kindTable struct {
	names []string
	index []uint16
}

// walk parses content, copies the shim's node records and releases them in the module.
func (in *instance) walk(content []byte, kinds kindTable) ([]Node, error) {
	at, err := in.put(content)
	if err != nil {
		return nil, err
	}
	defer in.free(at)
	count, err := in.call("shim_walk", uint64(at), uint64(len(content)))
	if err != nil {
		return nil, err
	}
	defer func() { _, _ = in.call("shim_walk_release") }()
	records, err := in.call("shim_nodes")
	if err != nil {
		return nil, err
	}
	raw, ok := in.mod.Memory().Read(api.DecodeU32(records), api.DecodeU32(count)*nodeRecord)
	if !ok {
		return nil, errMemory
	}
	le := binary.LittleEndian
	nodes := make([]Node, 0, len(raw)/nodeRecord)
	for b := raw; len(b) >= nodeRecord; b = b[nodeRecord:] {
		symbol, flags := le.Uint16(b), le.Uint16(b[2:])
		start, end := le.Uint32(b[8:]), le.Uint32(b[12:])
		var kind uint16
		switch {
		case symbol == errorSymbol:
			kind = uint16(len(kinds.names) - 1)
		case int(symbol) < len(kinds.index):
			kind = kinds.index[symbol]
		default:
			return nil, errRecord
		}
		if start > end || int(end) > len(content) {
			return nil, errRecord
		}
		nodes = append(nodes, Node{Kind: kind, Named: flags&flagNamed != 0, Error: flags&flagError != 0,
			Name: flags&flagName != 0, Depth: le.Uint32(b[4:]), Start: start, End: end})
	}
	return nodes, nil
}

// kindNames returns the grammar's kind table, read from the instance once per parser.
func (p *Parser) kindNames(grammar string, in *instance) (kindTable, error) {
	if table, found := p.kinds[grammar]; found {
		return table, nil
	}
	count, err := in.call("shim_kind_count")
	if err != nil {
		return kindTable{}, err
	}
	n := api.DecodeU32(count)
	if n >= errorSymbol {
		return kindTable{}, fmt.Errorf("%d kinds do not fit a node record", n)
	}
	length, err := in.put([]byte{0, 0, 0, 0})
	if err != nil {
		return kindTable{}, err
	}
	defer in.free(length)
	names := make([]string, 0, n+1)
	for id := range n {
		name, err := in.lengthString("shim_kind_name", length, uint64(id))
		if err != nil {
			return kindTable{}, err
		}
		names = append(names, name)
	}
	p.kinds[grammar] = newKindTable(append(names, errorKind))
	return p.kinds[grammar], nil
}

// newKindTable gives every symbol the index of the first kind of its name.
func newKindTable(names []string) kindTable {
	first := map[string]uint16{}
	index := make([]uint16, len(names)-1)
	for id, name := range names[:len(names)-1] {
		if _, seen := first[name]; !seen {
			first[name] = uint16(id)
		}
		index[id] = first[name]
	}
	return kindTable{names: names, index: index}
}

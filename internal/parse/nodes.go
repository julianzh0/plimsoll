package parse

import (
	"encoding/binary"
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

// Node is one node of a file's syntax tree.
type Node struct {
	Kind       uint16 // index into Tree.Kinds
	Named      bool   // false for a token such as "else" or "&&"
	Error      bool   // an ERROR node: the grammar could not fit the text here
	Name       bool   // the node is the name field of its parent
	Depth      uint32 // 0 for the root
	Start, End uint32 // byte offsets into the content
}

// Tree is the nodes of one file in text order, a parent before its children, and the grammar's kind names.
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

// errorKind is the last kind name: tree-sitter's error symbol lies outside the grammar's symbol table.
const errorKind = "ERROR"

// Nodes parses content with the named grammar and returns its tree. Any failure (an unknown grammar, a file the grammar
// cannot read within its memory cap) is an error; the parser stays usable.
func (p *Parser) Nodes(grammar string, content []byte) (Tree, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return Tree{}, errClosed
	}
	in, err := p.instance(grammar)
	if err != nil {
		return Tree{}, err
	}
	tree, err := in.walk(content)
	if err != nil || in.mod.Memory().Size() > recyclePages*pageBytes {
		in.close()
		delete(p.instances, grammar)
	}
	return tree, err
}

// walk parses content and decodes the shim's node records.
func (in *instance) walk(content []byte) (Tree, error) {
	kinds, err := in.kindNames()
	if err != nil {
		return Tree{}, err
	}
	at, err := in.put(content)
	if err != nil {
		return Tree{}, err
	}
	defer in.free(at)
	count, err := in.call("shim_walk", uint64(at), uint64(len(content)))
	if err != nil {
		return Tree{}, err
	}
	records, err := in.call("shim_nodes")
	if err != nil {
		return Tree{}, err
	}
	raw, ok := in.mod.Memory().Read(api.DecodeU32(records), api.DecodeU32(count)*nodeRecord)
	if !ok {
		return Tree{}, errMemory
	}
	le := binary.LittleEndian
	nodes := make([]Node, 0, len(raw)/nodeRecord)
	for b := raw; len(b) >= nodeRecord; b = b[nodeRecord:] {
		kind, flags := le.Uint16(b), le.Uint16(b[2:])
		start, end := le.Uint32(b[8:]), le.Uint32(b[12:])
		if start > end || int(end) > len(content) {
			return Tree{}, errRecord
		}
		if int(kind) >= len(kinds)-1 {
			kind = uint16(len(kinds) - 1)
		}
		nodes = append(nodes, Node{Kind: kind, Named: flags&flagNamed != 0, Error: flags&flagError != 0,
			Name: flags&flagName != 0, Depth: le.Uint32(b[4:]), Start: start, End: end})
	}
	return Tree{Kinds: kinds, Nodes: nodes}, nil
}

// kindNames reads the grammar's kind names once per instance, with errorKind last.
func (in *instance) kindNames() ([]string, error) {
	if in.kinds != nil {
		return in.kinds, nil
	}
	count, err := in.call("shim_kind_count")
	if err != nil {
		return nil, err
	}
	length, err := in.put([]byte{0, 0, 0, 0})
	if err != nil {
		return nil, err
	}
	defer in.free(length)
	n := api.DecodeU32(count)
	if n >= 0xFFFF {
		return nil, fmt.Errorf("%d kinds do not fit a node record", n)
	}
	kinds := make([]string, 0, n+1)
	for id := range n {
		name, err := in.lengthString("shim_kind_name", length, uint64(id))
		if err != nil {
			return nil, err
		}
		kinds = append(kinds, name)
	}
	in.kinds = append(kinds, errorKind)
	return in.kinds, nil
}

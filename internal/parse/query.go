package parse

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"regexp"

	"github.com/tetratelabs/wazero/api"
)

// captureRecord is the size of one capture as the shim writes it: match, pattern, capture, start byte, end byte, start
// row, end row, each a little-endian uint32. A record whose pattern is topLevel is a top-level node of the file.
const (
	captureRecord = 28
	topLevel      = 0xFFFFFFFF
)

// predicate is one predicate of a pattern: an operator, the capture it tests and the text it compares with.
type predicate struct {
	op      string
	capture string
	text    string
	re      *regexp.Regexp
}

// query is a query compiled in one instance.
type query struct {
	ptr        uint64
	names      []string
	predicates [][]predicate // by pattern
}

// instance is one running grammar module and the queries compiled in it.
type instance struct {
	ctx     context.Context
	mod     api.Module
	queries map[string]*query
}

func newInstance(ctx context.Context, mod api.Module) *instance {
	return &instance{ctx: ctx, mod: mod, queries: map[string]*query{}}
}

func (in *instance) close(ctx context.Context) { _ = in.mod.Close(ctx) }

var errMemory = errors.New("the content does not fit the grammar's memory")

// call runs an exported function of the shim and returns its one result, or 0 for none.
func (in *instance) call(name string, args ...uint64) (uint64, error) {
	results, err := in.mod.ExportedFunction(name).Call(in.ctx, args...)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	if len(results) == 0 {
		return 0, nil
	}
	return results[0], nil
}

// put copies b and a closing zero into the module's memory and returns where.
func (in *instance) put(b []byte) (uint32, error) {
	if len(b) >= memoryCapPages*pageBytes {
		return 0, errMemory
	}
	r, err := in.call("shim_alloc", uint64(len(b)+1))
	if err != nil {
		return 0, err
	}
	at := api.DecodeU32(r)
	if at == 0 || !in.mod.Memory().Write(at, append(append([]byte{}, b...), 0)) {
		return 0, errMemory
	}
	return at, nil
}

func (in *instance) free(at uint32) { _, _ = in.call("shim_free", uint64(at)) }

func (in *instance) text(at, length uint32) (string, error) {
	b, ok := in.mod.Memory().Read(at, length)
	if !ok {
		return "", errMemory
	}
	return string(b), nil
}

// read runs the query on content.
func (in *instance) read(source string, content []byte) (Result, error) {
	q, err := in.compile(source)
	if err != nil {
		return Result{}, err
	}
	at, err := in.put(content)
	if err != nil {
		return Result{}, err
	}
	defer in.free(at)
	count, err := in.call("shim_query_run", q.ptr, uint64(at), uint64(len(content)))
	if err != nil {
		return Result{}, err
	}
	records, err := in.call("shim_query_captures")
	if err != nil {
		return Result{}, err
	}
	raw, ok := in.mod.Memory().Read(api.DecodeU32(records), api.DecodeU32(count)*captureRecord)
	if !ok {
		return Result{}, errMemory
	}
	return decode(q, raw, content), nil
}

// decode turns capture records into a result: the captures of each match whose predicates hold, and the top-level
// nodes.
func decode(q *query, raw, content []byte) Result {
	var result Result
	type match struct {
		pattern  uint32
		captures []Capture
	}
	var order []uint32
	matches := map[uint32]*match{}
	le := binary.LittleEndian
	for at := 0; at+captureRecord <= len(raw); at += captureRecord {
		b := raw[at:]
		start, end := int(le.Uint32(b[12:])), int(le.Uint32(b[16:]))
		pattern := le.Uint32(b[4:])
		if pattern == topLevel {
			result.Top = append(result.Top, Span{Start: start, End: end})
			continue
		}
		c := Capture{Match: le.Uint32(b), Name: q.names[le.Uint32(b[8:])], Start: start, End: end,
			Line: int(le.Uint32(b[20:])) + 1, Last: int(le.Uint32(b[24:])) + 1}
		m, seen := matches[c.Match]
		if !seen {
			m = &match{pattern: pattern}
			matches[c.Match] = m
			order = append(order, c.Match)
		}
		m.captures = append(m.captures, c)
	}
	for _, id := range order {
		m := matches[id]
		if holds(q.predicates[m.pattern], m.captures, content) {
			stripPrefixes(q.predicates[m.pattern], m.captures, content)
			result.Captures = append(result.Captures, m.captures...)
		}
	}
	return result
}

// compile compiles a query in this instance once and keeps it.
func (in *instance) compile(source string) (*query, error) {
	if q, found := in.queries[source]; found {
		return q, nil
	}
	src, err := in.put([]byte(source))
	if err != nil {
		return nil, err
	}
	defer in.free(src)
	ptr, err := in.call("shim_query_new", uint64(src), uint64(len(source)))
	if err != nil {
		return nil, err
	}
	if ptr == 0 {
		offset, _ := in.call("shim_query_error_offset")
		return nil, fmt.Errorf("the query does not compile at byte %d", offset)
	}
	q := &query{ptr: ptr}
	length, err := in.put([]byte{0, 0, 0, 0})
	if err != nil {
		return nil, err
	}
	defer in.free(length)
	captures, err := in.call("shim_query_capture_count", ptr)
	if err != nil {
		return nil, err
	}
	for id := range captures {
		name, err := in.lengthString("shim_query_capture_name", length, ptr, id)
		if err != nil {
			return nil, err
		}
		q.names = append(q.names, name)
	}
	patterns, err := in.call("shim_query_pattern_count", ptr)
	if err != nil {
		return nil, err
	}
	for pattern := range patterns {
		preds, err := in.predicates(q, length, pattern)
		if err != nil {
			return nil, err
		}
		q.predicates = append(q.predicates, preds)
	}
	in.queries[source] = q
	return q, nil
}

// lengthString calls a shim function that returns a string's address and writes its length at length.
func (in *instance) lengthString(name string, length uint32, args ...uint64) (string, error) {
	at, err := in.call(name, append(args, uint64(length))...)
	if err != nil {
		return "", err
	}
	n, ok := in.mod.Memory().ReadUint32Le(length)
	if !ok {
		return "", errMemory
	}
	return in.text(api.DecodeU32(at), n)
}

// predicates reads the predicates of one pattern: steps of (type, value), type 0 ending a predicate, 1 naming a
// capture, 2 a string. Only the forms plimsoll evaluates are accepted.
func (in *instance) predicates(q *query, length uint32, pattern uint64) ([]predicate, error) {
	steps, err := in.call("shim_query_predicates", q.ptr, pattern, uint64(length))
	if err != nil {
		return nil, err
	}
	n, ok := in.mod.Memory().ReadUint32Le(length)
	if !ok {
		return nil, errMemory
	}
	var found []predicate
	var args []string
	for i := range n {
		kind, ok1 := in.mod.Memory().ReadUint32Le(api.DecodeU32(steps) + 8*i)
		value, ok2 := in.mod.Memory().ReadUint32Le(api.DecodeU32(steps) + 8*i + 4)
		if !ok1 || !ok2 {
			return nil, errMemory
		}
		switch kind {
		case 0:
			p, err := predicateOf(args)
			if err != nil {
				return nil, err
			}
			found = append(found, p)
			args = nil
		case 1:
			args = append(args, "@"+q.names[value])
		case 2:
			s, err := in.lengthString("shim_query_string", length, q.ptr, uint64(value))
			if err != nil {
				return nil, err
			}
			args = append(args, s)
		}
	}
	return found, nil
}

// predicateOf checks one predicate's shape: an operator plimsoll knows, a capture, a text.
func predicateOf(args []string) (predicate, error) {
	if len(args) != 3 || len(args[1]) < 2 || args[1][0] != '@' {
		return predicate{}, fmt.Errorf("a predicate of an unknown shape: %q", args)
	}
	p := predicate{op: args[0], capture: args[1][1:], text: args[2]}
	switch p.op {
	case "eq?", "not-eq?", "strip-prefix!":
	case "match?", "not-match?":
		re, err := regexp.Compile(p.text)
		if err != nil {
			return predicate{}, fmt.Errorf("the pattern of %s: %w", p.op, err)
		}
		p.re = re
	default:
		return predicate{}, fmt.Errorf("an unknown predicate: %s", p.op)
	}
	return p, nil
}

// holds says whether every test predicate of a match holds for its captures.
func holds(preds []predicate, captures []Capture, content []byte) bool {
	for _, p := range preds {
		for _, c := range captures {
			if c.Name != p.capture {
				continue
			}
			text := content[c.Start:c.End]
			switch p.op {
			case "eq?":
				if !bytes.Equal(text, []byte(p.text)) {
					return false
				}
			case "not-eq?":
				if bytes.Equal(text, []byte(p.text)) {
					return false
				}
			case "match?":
				if !p.re.Match(text) {
					return false
				}
			case "not-match?":
				if p.re.Match(text) {
					return false
				}
			}
		}
	}
	return true
}

// stripPrefixes applies #strip-prefix! @capture "text": a capture that starts with the text starts after it.
func stripPrefixes(preds []predicate, captures []Capture, content []byte) {
	for _, p := range preds {
		if p.op != "strip-prefix!" {
			continue
		}
		for i, c := range captures {
			if c.Name == p.capture && bytes.HasPrefix(content[c.Start:c.End], []byte(p.text)) {
				captures[i].Start += len(p.text)
			}
		}
	}
}

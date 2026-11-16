// Package parse runs tree-sitter grammars inside the process and answers queries on a file's content: every capture
// of every match whose predicates hold, and the file's top-level nodes. Each grammar is tree-sitter's C library and
// the grammar's parser compiled to one WebAssembly module, embedded in the binary and run by wazero. A module is given
// nothing it could reach outside its memory, and its memory is capped.
//
// The package knows no language, path or rule: callers name a grammar and give a query.
package parse

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
)

// MemoryCapMiB is the memory one grammar may use to read one file. The largest file each grammar read in the sixteen
// field repositories needed at most 46 MiB of module memory (Python, 678 KB), so 128 MiB leaves 2.8 times that. A file
// that needs more is not read: the call returns an error and the grammar starts afresh on its next file.
const MemoryCapMiB = 128

const (
	pageBytes      = 65536 // a WebAssembly page
	pagesPerMiB    = 16
	memoryCapPages = MemoryCapMiB * pagesPerMiB
	// recyclePages is the memory a grammar may keep between files: half the cap. Memory of a WebAssembly module never
	// shrinks, so a grammar that grew past it for one large file is closed and made again on its next file. It lies
	// above the 18 to 33 MiB a grammar holds after the largest files of the field repositories other than that one
	// Python file, so those keep their instance.
	recyclePages = memoryCapPages / 2
)

// Capture is one captured node of a match.
type Capture struct {
	Name       string
	Start, End int // byte offsets
	Line       int // the line it starts on, from 1
}

// Span is a stretch of the content, in byte offsets.
type Span struct{ Start, End int }

// Result is a query's answer on one file.
type Result struct {
	Matches [][]Capture // the captures of each match whose predicates hold, in match order
	Top     []Span      // the file's top-level named nodes
}

// Parser runs the embedded grammars. It belongs to whoever created it; Close frees every module's memory and leaves any
// other parser working. Read may be called from any number of goroutines.
//
// NOTE: one parse runs at a time for the whole parser, so cold reading uses one core (6 to 10 MB of source a second).
// Upgrade: more lanes, each costing one MemoryCapMiB of memory.
type Parser struct {
	mu        sync.Mutex
	ctx       context.Context
	runtime   wazero.Runtime
	compiled  map[string]wazero.CompiledModule
	instances map[string]*instance
	closed    bool
	// unmapped is set when the system refused an instance's memory (see start).
	unmapped bool
}

// ErrUnavailable marks a failure of the moment, not of the file: the parser is closed, or the system refused a
// grammar's memory. The same content may read on a later call.
var ErrUnavailable = errors.New("the parser is not available")

var errClosed = fmt.Errorf("closed: %w", ErrUnavailable)

// New returns a parser. It compiles nothing yet: a grammar is compiled on its first use.
func New() (*Parser, error) {
	p := &Parser{compiled: map[string]wazero.CompiledModule{}, instances: map[string]*instance{}}
	p.ctx = experimental.WithMemoryAllocator(context.Background(), experimental.MemoryAllocatorFunc(p.mapMemory))
	p.runtime = wazero.NewRuntimeWithConfig(p.ctx, wazero.NewRuntimeConfig().WithMemoryLimitPages(memoryCapPages))
	if err := failingHost(p.ctx, p.runtime); err != nil {
		_ = p.runtime.Close(p.ctx)
		return nil, fmt.Errorf("starting the parser: %w", err)
	}
	return p, nil
}

// Close ends the parser and frees the memory of all its grammars. A Read after Close returns an error.
func (p *Parser) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	_ = p.runtime.Close(p.ctx)
}

// Read parses content with the named grammar and runs the query on it. Any failure (an unknown grammar, a query that
// does not compile, a file the grammar cannot read within its memory cap) is an error; the parser stays usable.
func (p *Parser) Read(grammar, query string, content []byte) (Result, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return Result{}, errClosed
	}
	in, err := p.instance(grammar)
	if err != nil {
		return Result{}, err
	}
	result, err := in.read(query, content)
	if err != nil || in.mod.Memory().Size() > recyclePages*pageBytes {
		in.close()
		delete(p.instances, grammar)
	}
	return result, err
}

// instance returns the grammar's running module, compiling and starting it when it has none.
func (p *Parser) instance(grammar string) (*instance, error) {
	if in, found := p.instances[grammar]; found {
		return in, nil
	}
	compiled, found := p.compiled[grammar]
	if !found {
		bytes, err := moduleBytes(grammar)
		if err != nil {
			return nil, err
		}
		if compiled, err = p.runtime.CompileModule(p.ctx, bytes); err != nil {
			return nil, fmt.Errorf("compiling the %s grammar: %w", grammar, err)
		}
		p.compiled[grammar] = compiled
	}
	mod, err := p.start(compiled)
	if err != nil {
		return nil, fmt.Errorf("starting the %s grammar: %w", grammar, err)
	}
	in := newInstance(p.ctx, mod)
	p.instances[grammar] = in
	return in, nil
}

// start instantiates a compiled grammar. When the system refuses its memory mapping, wazero's allocator hook has no
// way to say so and wazero panics inside the instantiation; that panic, and only that one, becomes the error of this
// one grammar, and the process goes on (the file is unread, never fatal).
func (p *Parser) start(compiled wazero.CompiledModule) (mod api.Module, err error) {
	p.unmapped = false
	defer func() {
		if r := recover(); r != nil {
			if !p.unmapped {
				panic(r)
			}
			mod, err = nil, fmt.Errorf("the system refused the grammar's memory (%v): %w", r, ErrUnavailable)
		}
	}()
	return p.runtime.InstantiateModule(p.ctx, compiled, wazero.NewModuleConfig().WithName("").WithStartFunctions("_initialize"))
}

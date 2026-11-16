package parse

import (
	"syscall"

	"github.com/tetratelabs/wazero/experimental"
)

// mapped is the memory of one grammar instance: the whole cap mapped once outside the Go heap. A page is resident only
// once it is touched, and Free returns everything to the system when the instance closes. Memory reserved in the Go
// heap instead stayed resident and doubled the collector's target for every kept instance (measured: four kept Kotlin
// instances, 518 MB of live heap against 6 MB here).
//
// NOTE: experimental.MemoryAllocator is an API wazero may change in a minor release; wazero is pinned and bumped by
// hand. Way back: drop this allocator and let wazero grow memory by copying.
type mapped struct{ bytes []byte }

func mapMemory(_, _ uint64) experimental.LinearMemory {
	bytes, err := syscall.Mmap(-1, 0, memoryCapPages*pageBytes, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_ANON|syscall.MAP_PRIVATE)
	if err != nil {
		return &mapped{}
	}
	return &mapped{bytes: bytes}
}

// Reallocate gives the first size bytes of the mapping, or nil past the cap, which wazero reports as a failed growth.
func (m *mapped) Reallocate(size uint64) []byte {
	if size > uint64(len(m.bytes)) {
		return nil
	}
	return m.bytes[:size]
}

func (m *mapped) Free() {
	if m.bytes != nil {
		_ = syscall.Munmap(m.bytes)
		m.bytes = nil
	}
}

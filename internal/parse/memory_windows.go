//go:build windows

package parse

import (
	"syscall"
	"unsafe"
)

// The kernel32 calls and flags that reserve and release memory outside the Go heap.
var (
	kernel32     = syscall.NewLazyDLL("kernel32.dll")
	virtualAlloc = kernel32.NewProc("VirtualAlloc")
	virtualFree  = kernel32.NewProc("VirtualFree")
)

const (
	memCommit     = 0x1000
	memReserve    = 0x2000
	memRelease    = 0x8000
	pageReadWrite = 0x04
)

// reserve commits size bytes outside the Go heap; Windows makes a committed page resident only once it is touched.
func reserve(size int) ([]byte, error) {
	address, _, err := virtualAlloc.Call(0, uintptr(size), memReserve|memCommit, pageReadWrite)
	if address == 0 {
		return nil, err
	}
	// The address is memory the system gave outside the Go heap, read as a pointer without a uintptr conversion.
	start := *(*unsafe.Pointer)(unsafe.Pointer(&address))
	return unsafe.Slice((*byte)(start), size), nil
}

// release returns memory made by reserve to the system.
func release(bytes []byte) {
	_, _, _ = virtualFree.Call(uintptr(unsafe.Pointer(&bytes[0])), 0, memRelease)
}

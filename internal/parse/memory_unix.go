//go:build unix

package parse

import "syscall"

// reserve maps size bytes of anonymous memory outside the Go heap; a page is resident only once it is touched.
func reserve(size int) ([]byte, error) {
	return syscall.Mmap(-1, 0, size, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_ANON|syscall.MAP_PRIVATE)
}

// release returns a mapping made by reserve to the system.
func release(bytes []byte) { _ = syscall.Munmap(bytes) }

package memcore

import "unsafe"

// MemoryMoveNoHeapPointers performs a fast memory move between two regions that
// are known NOT to contain Go GC-managed pointers. The behavior is identical
// to memmove in C: overlapping regions are handled safely.
//
// This function is implemented per-platform in memmove_platform.go.
//
//go:inline
//go:nosplit
func MemoryMoveNoHeapPointers(dst, src unsafe.Pointer, n uintptr) {
	platformMemoryMoveNoHeapPointers(dst, src, n)
}

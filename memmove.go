package memcore

import "unsafe"

const (
	manualMoveThreshold uintptr = 32
)

// MemoryMoveNoHeapPointers moves memory between two regions known to
// contain no GC pointers. Automatically selects optimal path.
func MemoryMoveNoHeapPointers(dst, src unsafe.Pointer, n uintptr) {
	if n == 0 || dst == src {
		return
	}

	switch {
	case n <= manualMoveThreshold:
		// Naive overlap-safe copy
		d := uintptr(dst)
		s := uintptr(src)
		if s < d && s+n > d {
			// Overlapping regions, copy backward
			for i := n; i > 0; i-- {
				*(*byte)(unsafe.Pointer(d + i - 1)) =
					*(*byte)(unsafe.Pointer(s + i - 1))
			}
		} else {
			// Non-overlapping or forward-safe
			for i := uintptr(0); i < n; i++ {
				*(*byte)(unsafe.Pointer(d + i)) =
					*(*byte)(unsafe.Pointer(s + i))
			}
		}

	default:
		memmoveInternal(dst, src, n)
	}
}

//go:linkname memmoveInternal runtime.memmove
//go:nosplit
func memmoveInternal(dst, src unsafe.Pointer, n uintptr)

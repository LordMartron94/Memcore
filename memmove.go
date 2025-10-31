package memcore

import "unsafe"

// MemoryMoveNoHeapPointers copies n bytes between two regions known to
// contain no Go heap pointers. It automatically selects the optimal path
// for size and overlap safety.
//
// For n <= 8, it uses direct register moves (1–2 instructions).
// For larger sizes, it calls the runtime's highly optimized memmove.
func MemoryMoveNoHeapPointers(dst, src unsafe.Pointer, n uintptr) {
	if n == 0 || dst == src {
		return
	}

	// Handle tiny copies inline — minimal branch and loop overhead.
	switch n {
	case 1:
		*(*uint8)(dst) = *(*uint8)(src)
		return
	case 2:
		*(*uint16)(dst) = *(*uint16)(src)
		return
	case 4:
		*(*uint32)(dst) = *(*uint32)(src)
		return
	case 8:
		*(*uint64)(dst) = *(*uint64)(src)
		return
	}

	// For anything larger, rely on memmove’s tuned vectorized copy.
	memmoveInternal(dst, src, n)
}

//go:linkname memmoveInternal runtime.memmove
//go:nosplit
func memmoveInternal(dst, src unsafe.Pointer, n uintptr)

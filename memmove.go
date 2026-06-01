package memcore

import "unsafe"

/*
MemoryMoveNoHeapPointers copies n bytes from src to dst without GC write barriers.

[Context]
Safe only when both regions contain no Go heap pointers. Handles overlap via runtime memmove for n > 8.

[Parameters]
dst, src - Region bases; identical pointers are a no-op.
n - Byte count; zero is a no-op.

[Complexity]
Time: O(n). Space: O(1).

[Side Effects]
Mutates dst for n > 0 when dst != src.
*/
func MemoryMoveNoHeapPointers(dst, src unsafe.Pointer, n uintptr) {
	if n == 0 || dst == src {
		return
	}

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

	memmoveInternal(dst, src, n)
}

//go:linkname memmoveInternal runtime.memmove
//go:nosplit
func memmoveInternal(dst, src unsafe.Pointer, n uintptr)

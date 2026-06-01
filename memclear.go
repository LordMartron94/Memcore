package memcore

import "unsafe"

const (
	manualThreshold uintptr = 32 // up to 32 bytes → manual loop
)

/*
MemoryClearNoHeapPointers zeroes n bytes at ptr without involving the GC write barrier.

[Context]
Use only on manual mmap regions or other memory known to contain no Go heap pointers.

[Parameters]
ptr - Start of the region; nil is a no-op.
n - Number of bytes to clear; zero is a no-op.

[Complexity]
Time: O(n). Space: O(1).

[Side Effects]
Mutates memory at [ptr, ptr+n). Selects a hand loop for n <= 32 else runtime.memclrNoHeapPointers.
*/
func MemoryClearNoHeapPointers(ptr unsafe.Pointer, n uintptr) {
	switch {
	case n == 0 || ptr == nil:
		return

	case n <= manualThreshold:
		p := uintptr(ptr)
		for i := uintptr(0); i < n; i++ {
			*(*byte)(unsafe.Pointer(p + i)) = 0
		}

	default:
		memclrNoHeapPointers(ptr, n)
	}
}

//go:linkname memclrNoHeapPointers runtime.memclrNoHeapPointers
//go:nosplit
func memclrNoHeapPointers(ptr unsafe.Pointer, n uintptr)

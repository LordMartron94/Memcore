package memcore

import "unsafe"

// Tuned empirically — adjust later based on benchmarks.
const (
	manualThreshold uintptr = 32 // up to 32 bytes → manual loop
)

// MemoryClearNoHeapPointers clears memory without GC awareness.
// It dynamically selects the optimal method based on size.
func MemoryClearNoHeapPointers(ptr unsafe.Pointer, n uintptr) {
	switch {
	case n == 0 || ptr == nil:
		return

	case n <= manualThreshold:
		// Hand-rolled tight loop for very small clears
		p := uintptr(ptr)
		for i := uintptr(0); i < n; i++ {
			*(*byte)(unsafe.Pointer(p + i)) = 0
		}

	default:
		// Use Go's highly optimized REP STOSQ variant
		memclrNoHeapPointers(ptr, n)
	}
}

//go:linkname memclrNoHeapPointers runtime.memclrNoHeapPointers
//go:nosplit
func memclrNoHeapPointers(ptr unsafe.Pointer, n uintptr)

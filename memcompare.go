package memcore

import "unsafe"

// MemoryCompareNoHeapPointers compares two memory regions byte by byte.
//
// Returns true if both regions are identical for n bytes.
// Safe only for memory known to contain no Go heap pointers.
//
// For very small blocks, it performs direct register compares.
// For mid-size ranges, it uses a manual tight loop.
// For larger sizes, it defers to the runtime’s highly optimized memcmp.
//
//go:nosplit
func MemoryCompareNoHeapPointers(a, b unsafe.Pointer, n uintptr) bool {
	if n == 0 || a == b {
		return true
	}

	// Ultra-small: direct scalar compares (register level)
	switch n {
	case 1:
		return *(*uint8)(a) == *(*uint8)(b)
	case 2:
		return *(*uint16)(a) == *(*uint16)(b)
	case 4:
		return *(*uint32)(a) == *(*uint32)(b)
	case 8:
		return *(*uint64)(a) == *(*uint64)(b)
	}

	// Small/medium: tight manual loop (branchless for < manualThreshold)
	if n <= manualThreshold {
		ap := uintptr(a)
		bp := uintptr(b)
		for i := uintptr(0); i < n; i++ {
			if *(*byte)(unsafe.Pointer(ap + i)) != *(*byte)(unsafe.Pointer(bp + i)) {
				return false
			}
		}
		return true
	}

	// Large: delegate to runtime’s vectorized SSE2/AVX path
	return memequalInternal(a, b, n)
}

//go:linkname memequalInternal runtime.memequal
//go:nosplit
func memequalInternal(a, b unsafe.Pointer, size uintptr) bool

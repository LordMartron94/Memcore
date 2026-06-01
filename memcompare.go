package memcore

import "unsafe"

/*
MemoryCompareNoHeapPointers reports whether two regions of n bytes are byte-identical.

[Context]
Safe only when both regions contain no Go heap pointers. Uses scalar compares for tiny sizes,
a manual loop up to manualThreshold, then runtime.memequal.

[Parameters]
a, b - Region bases; identical pointers compare equal for any n.
n - Byte count; zero compares equal.

[Returns]
true when all n bytes match.

[Complexity]
Time: O(n). Space: O(1).

[Side Effects]
Pure function; read-only on both regions.
*/
//go:nosplit
func MemoryCompareNoHeapPointers(a, b unsafe.Pointer, n uintptr) bool {
	if n == 0 || a == b {
		return true
	}

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

	return memequalInternal(a, b, n)
}

//go:linkname memequalInternal runtime.memequal
//go:nosplit
func memequalInternal(a, b unsafe.Pointer, size uintptr) bool

package memcore

import "unsafe"

const zeroChunk64 uint64 = 0

// SizeOf returns the size of any item in an int64-type.
// Note: This is safe for most operating systems, but on certain exotic architectures,
// the conversion can result in mismatches.
func SizeOf[T any]() uint64 {
	return (uint64)(unsafe.Sizeof(new(*T)))
}

// AlignOf returns the alignment of an arbitary type.
func AlignOf[T any]() uint64 {
	return (uint64)(unsafe.Alignof(new(*T)))
}

// MemoryClearNoHeapPointers uses the go runtime internal memclrNoHeapPointers to set a given amount of memory to 0.
func MemoryClearNoHeapPointers(pointer unsafe.Pointer, length uintptr) {
	memclrNoHeapPointers(pointer, length)
}

//go:linkname memclrNoHeapPointers runtime.memclrNoHeapPointers
//go:inline
func memclrNoHeapPointers(ptr unsafe.Pointer, n uintptr)

// IsMemoryClear checks if a region of memory starting from ptr for length is all 0.
//
//go:nosplit
//go:inline
func IsMemoryClear(ptr unsafe.Pointer, length uintptr) bool {
	var nChunks uintptr = 0

	if length >= 8 {
		nChunks = length / 8

		for i := uintptr(0); i < nChunks; i++ {
			addr := unsafe.Add(ptr, i*8)
			if *(*uint64)(addr) != zeroChunk64 {
				return false
			}
		}
	}

	// Check remainder bytes
	startIdx := nChunks * 8 // compute here, not in advance for performance.
	for i := uintptr(0); i < length-startIdx; i++ {
		addr := unsafe.Add(ptr, startIdx+i)
		if *(*byte)(addr) != 0 {
			return false
		}
	}

	return true
}

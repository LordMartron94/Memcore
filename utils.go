package memcore

import (
	"reflect"
	"unsafe"
)

// SizeOf returns the size of any item in an int64-type.
// Note: This is safe for most operating systems, but on certain exotic architectures,
// the conversion can result in mismatches.
//
//go:inline
//go:nosplit
func SizeOf[T any]() uint64 {
	var zero T
	return (uint64)(unsafe.Sizeof(zero))
}

// AlignOf returns the alignment of an arbitary type.
//
//go:inline
//go:nosplit
func AlignOf[T any]() uint64 {
	var zero T
	return (uint64)(unsafe.Alignof(zero))
}

// TypeOf wraps reflect.TypeFor[T]
//
//go:inline
//go:nosplit
func TypeOf[T any]() reflect.Type {
	return reflect.TypeFor[T]()
}

// AlignUp rounds `n` up to the nearest multiple of `alignment`.
//
// It uses efficient bit-masking to avoid division or modulo operations.
// The alignment value must be a power of two.
//
// For example:
//
//	AlignUp(13, 8)  => 16
//	AlignUp(32, 8)  => 32
//
// This function is typically used for aligning memory offsets, sizes,
// or indices to machine-friendly boundaries.
//
//go:inline
//go:nosplit
func AlignUp(n, alignment uint64) uint64 {
	mask := alignment - 1
	return (n + mask) &^ mask
}

// NextPowerOfTwo returns the smallest power of two greater than or equal to `x`.
//
// If `x` is already a power of two, it is returned unchanged.
// For example:
//
//	NextPowerOfTwo(0)  => 1
//	NextPowerOfTwo(1)  => 1
//	NextPowerOfTwo(5)  => 8
//	NextPowerOfTwo(64) => 64
//
// This function uses a branchless bit-twiddling method that runs in constant time
// and avoids loops or floating-point operations.
//
// ⚠️ Range:
// - For inputs in [0, 2⁶³], the result is always valid.
// - For inputs in (2⁶³, 2⁶⁴−1], the function overflows to 0 due to bit shifting limits.
//
//go:inline
//go:nosplit
func NextPowerOfTwo(x uint64) uint64 {
	if x == 0 {
		return 1
	}
	x--
	x |= x >> 1
	x |= x >> 2
	x |= x >> 4
	x |= x >> 8
	x |= x >> 16
	x |= x >> 32
	return x + 1
}

// IsAligned reports whether the given pointer is aligned to `alignment` bytes.
//
// The alignment must be a power of two.
// This function performs a pure address check and does not inspect memory.
//
// Typical use cases:
// - SIMD kernel precondition checks
// - Asserting allocator guarantees
// - Selecting aligned vs unaligned fast paths
//
//go:inline
//go:nosplit
func IsAligned(ptr unsafe.Pointer, alignment uint64) bool {
	if alignment&(alignment-1) != 0 {
		panic("alignment must be power of two")
	}

	return uintptr(ptr)&uintptr(alignment-1) == 0
}

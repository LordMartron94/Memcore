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

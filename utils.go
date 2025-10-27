package memcore

import "unsafe"

// SizeOf returns the size of any item in an int64-type.
// Note: This is safe for most operating systems, but on certain exotic architectures,
// the conversion can result in mismatches.
func SizeOf[T any]() uint64 {
	var zero T
	return (uint64)(unsafe.Sizeof(zero))
}

// AlignOf returns the alignment of an arbitary type.
func AlignOf[T any]() uint64 {
	var zero T
	return (uint64)(unsafe.Alignof(zero))
}

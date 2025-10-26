package primitives

import "memcore"

// ContainerRequiredBytes computes how many bytes are required to store `capacity`
// elements of type T inside an Array, Stack, or FixedList, considering alignment.
//
// Example:
// bytes := primitives.ContainerRequiredBytes
// addr := allocator.Malloc(bytes, memcore.AlignOf[int64]())
func ContainerRequiredBytes[T any](capacity uint64) uint64 {
	size := memcore.SizeOf[T]()
	align := memcore.AlignOf[T]()
	total := size * capacity
	return alignIdxUp(total, align)
}

//go:inline
func alignIdxUp(idx uint64, alignment uint64) uint64 {
	mask := alignment - 1
	return (idx + mask) &^ mask
}

// Note: alignIdxUp is also in memforge, but duplicated here for inlining benefits.

// Package primitives provides low-level data structures.
package primitives

import (
	"fmt"
	"memcore"
	"unsafe"
)

// Array is a custom array implementation build on top of the custom allocators.
// It contains an unsafe Pointer internally and therefore can NOT be stored in custom allocated memory.
type Array[T any] struct {
	ptr      unsafe.Pointer
	capacity uint64

	itemSize uint64
}

// ArrayCreateAt creates an instance of an array for type T at a specific memory address.
// Ensure the address is properly aligned and has the right size.
//
// ⚠️ capacity is in elements, not bytes.
// When using memory returned from a byte allocator,
// convert using: capacity = bytes / memcore.SizeOf[T]()
//
// ⚠️ Important: Do NOT allocate this struct itself inside manually-managed memory.
// The struct contains Go pointers and must remain
// visible to the Go garbage collector.
//
// You may, however, point its internal data (the `ptr` field) to memory that was
// manually allocated (e.g. via mmap or a custom allocator). In other words:
//
//	✅ Safe:   header on Go heap, data in manual memory
//	❌ Unsafe: header and data both in manual memory
func ArrayCreateAt[T any](addr unsafe.Pointer, capacity uint64) *Array[T] {
	return &Array[T]{
		ptr:      addr,
		capacity: capacity,
		itemSize: memcore.SizeOf[T](),
	}
}

// ArrayCapacityGet returns the total amount of elements that can be stored.
func ArrayCapacityGet[T any](array *Array[T]) uint64 {
	return array.capacity
}

// ArrayItemGetAt returns T at idx within the array.
// It returns an error if the idx is invalid.
func ArrayItemGetAt[T any](array *Array[T], idx uint64) (T, error) {
	if error := arrayGuaranteeIdxValidity(array, idx); error != nil {
		var zero T
		return zero, error
	}

	return *(*T)(arrayGetPtrAtIdx(array, idx)), nil
}

// ArrayItemGetAtUnsafe returns T at idx within the array.
// It does no bounds checks.
//
//go:inline
func ArrayItemGetAtUnsafe[T any](array *Array[T], idx uint64) T {
	return *(*T)(arrayGetPtrAtIdx(array, idx))
}

// ArrayItemPtrGetAt returns a pointer to T at idx within the array.
// It returns an error if the idx is invalid.
//
// Using this pointer after deletion or overwriting this idx is undefined behaviour.
// Use at your own discretion!
func ArrayItemPtrGetAt[T any](array *Array[T], idx uint64) (*T, error) {
	if error := arrayGuaranteeIdxValidity(array, idx); error != nil {
		return nil, error
	}

	return (*T)(arrayGetPtrAtIdx(array, idx)), nil
}

// ArrayItemPtrGetAtUnsafe returns a pointer to T at idx within the array.
// It does no bounds checks.
//
// Using this pointer after deletion or overwriting this idx is undefined behaviour.
// Use at your own discretion!
//
//go:inline
func ArrayItemPtrGetAtUnsafe[T any](array *Array[T], idx uint64) *T {
	return (*T)(arrayGetPtrAtIdx(array, idx))
}

// ArraySetAt sets idx of array to value T.
// It returns an error if the idx is invalid.
func ArraySetAt[T any](array *Array[T], idx uint64, value T) error {
	if error := arrayGuaranteeIdxValidity(array, idx); error != nil {
		return error
	}

	currentPtr := arrayGetPtrAtIdx(array, idx)
	*(*T)(currentPtr) = value

	return nil
}

// ArraySetAtUnsafe sets idx of array to value T.
// It does no bounds checks.
func ArraySetAtUnsafe[T any](array *Array[T], idx uint64, value T) {
	currentPtr := arrayGetPtrAtIdx(array, idx)
	*(*T)(currentPtr) = value
}

// ArrayDeleteAt resets memory to 0 at a given index, using pointers to this
// index gotten earlier is undefined behaviour.
// It returns an error if the index is invalid.
func ArrayDeleteAt[T any](array *Array[T], idx uint64) error {
	if error := arrayGuaranteeIdxValidity(array, idx); error != nil {
		return error
	}

	currentPtr := arrayGetPtrAtIdx(array, idx)
	memcore.MemoryClearNoHeapPointers(currentPtr, uintptr(array.itemSize))
	return nil
}

// ArrayDeleteAtUnsafe resets memory to 0 at a given index, using pointers to this
// index gotten earlier is undefined behaviour.
// It does no bounds checks.
func ArrayDeleteAtUnsafe[T any](array *Array[T], idx uint64) {
	currentPtr := arrayGetPtrAtIdx(array, idx)
	memcore.MemoryClearNoHeapPointers(currentPtr, uintptr(array.itemSize))
}

// ArrayClear resets the entire array's memory to 0, allowing it to be reused.
// Do NOT use pointers to previous array items after this as that is undefined behaviour.
func ArrayClear[T any](array *Array[T]) {
	memcore.MemoryClearNoHeapPointers(array.ptr, uintptr(array.capacity)*uintptr(array.itemSize))
}

// ArrayIsIdxValid checks whether the given index is valid.
//
//go:inline
func ArrayIsIdxValid[T any](array *Array[T], idx uint64) bool {
	return idx < array.capacity
}

// -------------------------- PRIVATE HELPERS

//go:inline
func arrayGetPtrAtIdx[T any](array *Array[T], idx uint64) unsafe.Pointer {
	return unsafe.Add(array.ptr, idx*array.itemSize)
}

//go:inline
func arrayGuaranteeIdxValidity[T any](array *Array[T], idx uint64) error {
	idxValid := idx < array.capacity

	if !idxValid {
		return fmt.Errorf("invalid index: %v, must be between 0 and %v (exclusive)", idx, array.capacity)
	}

	return nil
}

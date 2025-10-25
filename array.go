// Package memcore provides low-level core infrastructure for memory management.
package memcore

import (
	"fmt"
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
// Do NOT store this inside custom allocated memory.
func ArrayCreateAt[T any](addr unsafe.Pointer, capacity uint64) *Array[T] {
	return &Array[T]{
		ptr:      addr,
		capacity: capacity,
		itemSize: SizeOf[T](),
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
// It rdoes no bounds checks.
func ArraySetAtUnsafe[T any](array *Array[T], idx uint64, value T) {
	currentPtr := arrayGetPtrAtIdx(array, idx)
	*(*T)(currentPtr) = value
}

// ArrayInsertAt inserts a value T at idx for array, pushing values after it an idx up.
// In other words, it shifts all existing items from idx through capacity-2 one slot to the right.
// It returns an error if subsequent items cannot be moved.
//
// Note: This function treats an all-zero memory region as empty.
// If you intentionally stored a valid zero-value T at the last index,
// it will still be treated as empty and overwritten.
// This can lead to subtle bugs if zero values are semantically meaningful in your use case.
func ArrayInsertAt[T any](array *Array[T], idx uint64, value T) error {
	if error := arrayGuaranteeIdxValidity(array, idx); error != nil {
		return error
	}

	addr := arrayGetPtrAtIdx(array, array.capacity-1)
	if !IsMemoryClear(addr, uintptr(array.itemSize)) {
		return fmt.Errorf("no space left in array, cannot insert")
	}

	for i := array.capacity - 2; i >= idx; i-- {
		addr := arrayGetPtrAtIdx(array, i)
		v := *(*T)(addr)
		ArraySetAtUnsafe(array, i+1, v)
	}

	ArraySetAtUnsafe(array, idx, value)

	return nil
}

// ArrayInsertAtUnsafe inserts a value T at idx for array, pushing values after it an idx up.
// In other words, it shifts all existing items from idx through capacity-2 one slot to the right.
//
// It does no validation checks at all.
//
// Precondition: The caller must ensure there is at least one free slot at the end of the array,
// and that idx is within valid bounds. Violating these conditions results in undefined behavior.
func ArrayInsertAtUnsafe[T any](array *Array[T], idx uint64, value T) {
	for i := array.capacity - 2; i >= idx; i-- {
		addr := arrayGetPtrAtIdx(array, i)
		v := *(*T)(addr)
		ArraySetAtUnsafe(array, i+1, v)
	}

	ArraySetAtUnsafe(array, idx, value)
}

// ArrayDeleteAt resets memory to 0 at a given index, using pointers to this
// index gotten earlier is undefined behaviour.
// It returns an error if the index is invalid.
func ArrayDeleteAt[T any](array *Array[T], idx uint64) error {
	if error := arrayGuaranteeIdxValidity(array, idx); error != nil {
		return error
	}

	currentPtr := arrayGetPtrAtIdx(array, idx)
	MemoryClearNoHeapPointers(currentPtr, uintptr(array.itemSize))
	return nil
}

// ArrayDeleteAtUnsafe resets memory to 0 at a given index, using pointers to this
// index gotten earlier is undefined behaviour.
// It does no bounds checks.
func ArrayDeleteAtUnsafe[T any](array *Array[T], idx uint64) {
	currentPtr := arrayGetPtrAtIdx(array, idx)
	MemoryClearNoHeapPointers(currentPtr, uintptr(array.itemSize))
}

// ArrayDeleteAndShiftAt deletes the element at idx and shifts subsequent
// elements left by one slot. The last element is cleared.
// Returns an error if idx is invalid.
func ArrayDeleteAndShiftAt[T any](array *Array[T], idx uint64) error {
	if err := arrayGuaranteeIdxValidity(array, idx); err != nil {
		return err
	}

	for i := idx; i < array.capacity-1; i++ {
		nextVal := ArrayItemGetAtUnsafe(array, i+1)
		ArraySetAtUnsafe(array, i, nextVal)
	}

	lastPtr := arrayGetPtrAtIdx(array, array.capacity-1)
	MemoryClearNoHeapPointers(lastPtr, uintptr(array.itemSize))

	return nil
}

// ArrayDeleteAndShiftAtUnsafe deletes at idx and shifts subsequent elements left.
// It does no validation checks at all.
//
// Precondition: idx must be valid, otherwise undefined behaviour.
func ArrayDeleteAndShiftAtUnsafe[T any](array *Array[T], idx uint64) {
	for i := idx; i < array.capacity-1; i++ {
		nextVal := ArrayItemGetAtUnsafe(array, i+1)
		ArraySetAtUnsafe(array, i, nextVal)
	}
	lastPtr := arrayGetPtrAtIdx(array, array.capacity-1)
	MemoryClearNoHeapPointers(lastPtr, uintptr(array.itemSize))
}

// ArrayClear resets the entire array's memory to 0, allowing it to be reused.
// Do NOT use pointers to previous array items after this as that is undefined behaviour.
func ArrayClear[T any](array *Array[T]) {
	MemoryClearNoHeapPointers(array.ptr, uintptr(array.capacity)*uintptr(array.itemSize))
}

// ArrayBinarySearch performs a binary search O(log n) to find a value matching
// the predicate in the array.
// This requires that the array is sorted, otherwise bugs will exist.
//
// It will return the index of the predicate in the array.
// It returns an error if the predicate cannot be found.
//
// The predicate function must return:
// <br>- negative when item is below predicate
// <br>- 0 when item is equal to predicate
// <br>- positive when the item is above predicate
func ArrayBinarySearch[T any](array *Array[T], predicate func(item T) int8) (uint64, error) {
	var lowerBound uint64 = 0
	var upperBound uint64 = array.capacity - 1

	for lowerBound <= upperBound {
		currentIdx := (lowerBound + upperBound) / 2 // Center
		v := ArrayItemGetAtUnsafe(array, currentIdx)

		result := predicate(v)
		if result < 0 {
			lowerBound = currentIdx + 1
		} else if result > 0 {
			upperBound = currentIdx - 1
		} else {
			return currentIdx, nil
		}
	}

	return 0, fmt.Errorf("predicate cannot be found in the array")
}

// ArrayBinarySearchInterval uses a binary search O(log n) to find the position
// where predicate can be inserted to keep order.
//
// It returns the index of the previous item and the index of the next item
// when inserted in between them.
//
// The predicate function must return:
// <br>- negative when item is below predicate
// <br>- 0 or positive when the item is above predicate
func ArrayBinarySearchInterval[T any](array *Array[T], predicate func(item T) int8) (uint64, uint64) {
	if array.capacity == 0 {
		return ^uint64(0), ^uint64(0)
	}

	var lowerBound uint64 = 0
	var upperBound uint64 = array.capacity

	for lowerBound < upperBound {
		currentIdx := (lowerBound + upperBound) / 2 // Center
		v := ArrayItemGetAtUnsafe(array, currentIdx)

		result := predicate(v)
		if result < 0 {
			lowerBound = currentIdx + 1
			continue
		}
		if result >= 0 {
			upperBound = currentIdx
			continue
		}
	}

	if lowerBound == 0 {
		return ^uint64(0), 0 // no previous, insert at beginning
	}

	if lowerBound == array.capacity {
		return array.capacity - 1, array.capacity
	}

	return lowerBound - 1, lowerBound
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

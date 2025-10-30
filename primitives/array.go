// Package primitives provides low-level data structures.
package primitives

import (
	"fmt"
	"memcore"
	"unsafe"
)

const arrayMemmoveThreshold uint64 = 128

func setByMove[T any](array *Array[T], idx uint64, value T) {
	dstPtr := arrayGetPtrAtIdx(array, idx)
	srcPtr := unsafe.Pointer(&value)
	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, uintptr(memcore.SizeOf[T]()))
}

func setByAssign[T any](array *Array[T], idx uint64, value T) {
	currentPtr := arrayGetPtrAtIdx(array, idx)
	*(*T)(currentPtr) = value
}

// Array is a custom array implementation build on top of the custom allocators.
// It contains an unsafe Pointer internally and therefore can NOT be stored in custom allocated memory.
type Array[T any] struct {
	ptr      unsafe.Pointer
	capacity uint64

	setFn func(idx uint64, value T)

	itemSize        uint64
	itemSizeUintPtr uintptr
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
	itemSize := memcore.SizeOf[T]()

	array := &Array[T]{
		ptr:             addr,
		capacity:        capacity,
		itemSize:        itemSize,
		itemSizeUintPtr: uintptr(itemSize),
	}

	if itemSize > arrayMemmoveThreshold {
		array.setFn = func(i uint64, v T) { setByMove(array, i, v) }
	} else {
		array.setFn = func(i uint64, v T) { setByAssign(array, i, v) }
	}

	return array
}

// ArrayCapacityGet returns the total amount of elements that can be stored.
//
//go:nosplit
//go:inline
func ArrayCapacityGet[T any](array *Array[T]) uint64 {
	return array.capacity
}

// ArrayItemGetAt returns T at idx within the array.
// It returns an error if the idx is invalid.
//
//go:nosplit
//go:inline
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
//
//go:nosplit
//go:inline
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
//
//go:nosplit
//go:inline
func ArraySetAt[T any](array *Array[T], idx uint64, value T) error {
	if error := arrayGuaranteeIdxValidity(array, idx); error != nil {
		return error
	}

	array.setFn(idx, value)

	return nil
}

// ArraySetAtUnsafe sets idx of array to value T.
// It does no bounds checks.
//
//go:nosplit
//go:inline
func ArraySetAtUnsafe[T any](array *Array[T], idx uint64, value T) {
	array.setFn(idx, value)
}

// ArrayReplaceInternal replaces srcIdx with the value at destIdx efficiently.
//
//go:inline
func ArrayReplaceInternal[T any](array *Array[T], srcIdx, destIdx uint64) error {
	if error := arrayGuaranteeIdxValidity(array, srcIdx); error != nil {
		return error
	}

	if error := arrayGuaranteeIdxValidity(array, destIdx); error != nil {
		return error
	}

	srcPtr := arrayGetPtrAtIdx(array, srcIdx)
	dstPtr := arrayGetPtrAtIdx(array, destIdx)

	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, array.itemSizeUintPtr)

	return nil
}

// ArrayReplaceInternalUnsafe replaces srcIdx with the value at destIdx efficiently.
//
// It does no bounds checks.
//
//go:inline
func ArrayReplaceInternalUnsafe[T any](array *Array[T], srcIdx, destIdx uint64) {
	srcPtr := arrayGetPtrAtIdx(array, srcIdx)
	dstPtr := arrayGetPtrAtIdx(array, destIdx)

	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, array.itemSizeUintPtr)
}

// ArrayShiftRight shifts a contiguous range of elements in the array
// `count` positions to the right, preserving their order.
//
// Parameters:
//   - from:  starting index of the range to shift (inclusive)
//   - to:    ending index of the range to shift (inclusive)
//   - count: number of positions to shift the range to the right
//
// The data between [from, to] is moved rightward by `count` slots, such that the
// element originally at `to` ends up at index `to + count`. Any elements in the
// destination range [from+count, to+count] will be overwritten.
//
// Example:
//
//	Before: [A, B, C, D, E, F, G]
//	Call:   ArrayShiftRightUnsafe(arr, from=2, to=4, count=1)
//	Moves:  C→D, D→E, E→F
//	After:  [A, B, _, C, D, E, G]
//
//go:nosplit
//go:inline
func ArrayShiftRight[T any](array *Array[T], from, to, count uint64) error {
	if from >= array.capacity || to >= array.capacity {
		return fmt.Errorf("invalid range: from=%d to=%d capacity=%d", from, to, array.capacity)
	}
	if count == 0 || from >= to {
		return nil
	}

	ArrayShiftRightUnsafe(array, from, to, count)
	return nil
}

// ArrayShiftRightUnsafe shifts a contiguous range of elements in the array
// `count` positions to the right, preserving their order. It performs no bounds
// checks, so callers must ensure valid indices.
//
// Parameters:
//   - from:  starting index of the range to shift (inclusive)
//   - to:    ending index of the range to shift (inclusive)
//   - count: number of positions to shift the range to the right
//
// The data between [from, to] is moved rightward by `count` slots, such that the
// element originally at `to` ends up at index `to + count`. Any elements in the
// destination range [from+count, to+count] will be overwritten.
//
// Example:
//
//	Before: [A, B, C, D, E, F, G]
//	Call:   ArrayShiftRightUnsafe(arr, from=2, to=4, count=1)
//	Moves:  C→D, D→E, E→F
//	After:  [A, B, _, C, D, E, G]
//
//go:nosplit
//go:inline
func ArrayShiftRightUnsafe[T any](array *Array[T], from, to, count uint64) {
	elemSize := array.itemSizeUintPtr
	srcPtr := arrayGetPtrAtIdx(array, from)
	dstPtr := arrayGetPtrAtIdx(array, from+count)

	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, uintptr((to-from+1)*uint64(elemSize)))
}

// ArrayShiftLeft shifts a contiguous range of elements in the array
// `count` positions to the left, preserving their order.
//
// Parameters:
//   - from:  starting index of the range to shift (inclusive)
//   - to:    ending index of the range to shift (inclusive)
//   - count: number of positions to shift the range to the left
//
// The data between [from, to] is moved leftward by `count` slots, such that the
// element originally at `from` ends up at index `from - count`. Any elements in
// the destination range [from-count, to-count] will be overwritten.
//
// Example:
//
//	Before: [A, B, C, D, E, F, G]
//	Call:   ArrayShiftLeft(arr, from=3, to=5, count=1)
//	Moves:  D→C, E→D, F→E
//	After:  [A, B, C, D, E, _, G]
//
// Performs bounds checks and returns an error if the range exceeds capacity.
//
//go:nosplit
//go:inline
func ArrayShiftLeft[T any](array *Array[T], from, to, count uint64) error {
	if from >= array.capacity || to >= array.capacity {
		return fmt.Errorf("invalid range: from=%d to=%d capacity=%d", from, to, array.capacity)
	}
	if count == 0 || from >= to {
		return nil
	}

	ArrayShiftLeftUnsafe(array, from, to, count)
	return nil
}

// ArrayShiftLeftUnsafe shifts a contiguous range of elements in the array
// `count` positions to the left, preserving their order. It performs no bounds
// checks, so callers must ensure valid indices.
//
// Parameters:
//   - from:  starting index of the range to shift (inclusive)
//   - to:    ending index of the range to shift (inclusive)
//   - count: number of positions to shift the range to the left
//
// The data between [from, to] is moved leftward by `count` slots, such that the
// element originally at `from` ends up at index `from - count`. Any elements in
// the destination range [from-count, to-count] will be overwritten.
//
// Example:
//
//	Before: [A, B, C, D, E, F, G]
//	Call:   ArrayShiftLeft(arr, from=3, to=5, count=1)
//	Moves:  D→C, E→D, F→E
//	After:  [A, B, C, D, E, _, G]
//
//go:nosplit
//go:inline
func ArrayShiftLeftUnsafe[T any](array *Array[T], from, to, count uint64) {
	elemSize := array.itemSizeUintPtr
	srcPtr := arrayGetPtrAtIdx(array, from+count)
	dstPtr := arrayGetPtrAtIdx(array, from)

	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, uintptr((to-from+1)*uint64(elemSize)))
}

// ArrayDeleteAt resets memory to 0 at a given index, using pointers to this
// index gotten earlier is undefined behaviour.
// It returns an error if the index is invalid.
//
//go:nosplit
//go:inline
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
//
//go:nosplit
//go:inline
func ArrayDeleteAtUnsafe[T any](array *Array[T], idx uint64) {
	currentPtr := arrayGetPtrAtIdx(array, idx)
	memcore.MemoryClearNoHeapPointers(currentPtr, uintptr(array.itemSize))
}

// ArrayClear resets the entire array's memory to 0, allowing it to be reused.
// Do NOT use pointers to previous array items after this as that is undefined behaviour.
//
//go:nosplit
//go:inline
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

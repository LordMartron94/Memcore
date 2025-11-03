// Package primitives provides low-level data structures.
package primitives

import (
	"fmt"
	"memcore"
	"unsafe"
)

const arrayMemmoveThreshold uint64 = 128

func setByMove[T any](instance *Array[T], baseAddr unsafe.Pointer, idx uint64, value T) {
	dstPtr := arrayGetPtrAtIdx(instance, baseAddr, idx)
	srcPtr := unsafe.Pointer(&value)
	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, uintptr(memcore.SizeOf[T]()))
}

func setByAssign[T any](instance *Array[T], baseAddr unsafe.Pointer, idx uint64, value T) {
	currentPtr := arrayGetPtrAtIdx(instance, baseAddr, idx)
	*(*T)(currentPtr) = value
}

func ArrayRequiredBytesGet[T any](capacity uint64) uint64 {
	headerSize := memcore.SizeOf[Array[T]]()
	itemSize := memcore.SizeOf[T]()
	return headerSize + itemSize*capacity
}

func ArrayRequiredAlignmentGet[T any]() uint64 {
	return max(memcore.AlignOf[T](), memcore.AlignOf[Array[T]]())
}

// Array is a custom array implementation built on top of the custom allocators.
type Array[T any] struct {
	dataAddrOffset uintptr
	capacity       uint64

	// setFn func(idx uint64, value T)

	itemSize        uint64
	itemSizeUintPtr uintptr
}

// ArrayInitializeAt initializes an instance of an array for type T at a specific memory address.
// Ensure the address is properly aligned and has the right size.
//
// ⚠️ capacity is in elements, not bytes.
func ArrayInitializeAt[T any](arrayAddr memcore.Pointer, capacity uint64) {
	headerSize := memcore.SizeOf[Array[T]]()

	itemSize := memcore.SizeOf[T]()
	arrayPtr := memcore.MemcorePointerDereferenceObjectUnsafe[Array[T]](arrayAddr)
	*arrayPtr = Array[T]{
		dataAddrOffset:  uintptr(headerSize),
		capacity:        capacity,
		itemSize:        itemSize,
		itemSizeUintPtr: uintptr(itemSize),
	}

	// if itemSize > arrayMemmoveThreshold {
	// 	array.setFn = func(i uint64, v T) { setByMove(array, i, v) }
	// } else {
	// 	array.setFn = func(i uint64, v T) { setByAssign(array, i, v) }
	// }
}

// ArraySnapshotCreate creates a deep copy of an array at a new memory location
// defined by the destination pointer (which points to the start of the new array header).
// It copies both the header and the data that follow it, maintaining the same relative layout.
func ArraySnapshotCreate[T any](dest memcore.Pointer, instance memcore.Pointer) memcore.Pointer {
	arrayPtr := memcore.MemcorePointerDereferenceObjectUnsafe[Array[T]](instance)
	totalSize := ArrayRequiredBytesGet[T](arrayPtr.capacity)

	srcAddr := memcore.MemcorePointerDereferenceRaw(instance)
	dstAddr := memcore.MemcorePointerDereferenceRaw(dest)

	memcore.MemoryMoveNoHeapPointers(dstAddr, srcAddr, uintptr(totalSize))

	return dest
}

// ArraySnapshotRestore replaces the entire memory block of one array
// (header + data) with that of another array of the same type and capacity.
// Both arrays must live in manual memory managed by memcore.
func ArraySnapshotRestore[T any](dest, src memcore.Pointer) error {
	dstHeader := memcore.MemcorePointerDereferenceObjectUnsafe[Array[T]](dest)
	srcHeader := memcore.MemcorePointerDereferenceObjectUnsafe[Array[T]](src)

	if dstHeader.capacity != srcHeader.capacity {
		return fmt.Errorf("cannot restore snapshot: unequal capacities (dest=%v, src=%v)", dstHeader.capacity, srcHeader.capacity)
	}

	if dest == src {
		return nil
	}

	totalBytes := ArrayRequiredBytesGet[T](dstHeader.capacity)

	dstAddr := memcore.MemcorePointerDereferenceRaw(dest)
	srcAddr := memcore.MemcorePointerDereferenceRaw(src)

	memcore.MemoryMoveNoHeapPointers(dstAddr, srcAddr, uintptr(totalBytes))

	return nil
}

// ArrayCapacityGet returns the total amount of elements that can be stored.
//
//go:nosplit
//go:inline
func ArrayCapacityGet[T any](array memcore.Pointer) uint64 {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[Array[T]](array)
	return instance.capacity
}

// ArrayItemGetAt returns T at idx within the array.
// It returns an error if the idx is invalid.
//
//go:nosplit
//go:inline
func ArrayItemGetAt[T any](array memcore.Pointer, idx uint64) (T, error) {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[Array[T]](array)
	baseAddr := memcore.MemcorePointerDereferenceRaw(array)

	if error := arrayGuaranteeIdxValidity(instance, idx); error != nil {
		var zero T
		return zero, error
	}

	return *(*T)(arrayGetPtrAtIdx(instance, baseAddr, idx)), nil
}

// ArrayItemGetAtUnsafe returns T at idx within the array.
// It does no bounds checks.
//
//go:inline
func ArrayItemGetAtUnsafe[T any](array memcore.Pointer, idx uint64) T {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[Array[T]](array)
	baseAddr := memcore.MemcorePointerDereferenceRaw(array)
	return *(*T)(arrayGetPtrAtIdx(instance, baseAddr, idx))
}

// ArrayItemPtrGetAt returns a pointer to T at idx within the array.
// It returns an error if the idx is invalid.
//
// Using this pointer after deletion or overwriting this idx is undefined behaviour.
// Use at your own discretion!
//
//go:nosplit
//go:inline
func ArrayItemPtrGetAt[T any](array memcore.Pointer, idx uint64) (*T, error) {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[Array[T]](array)
	baseAddr := memcore.MemcorePointerDereferenceRaw(array)
	if error := arrayGuaranteeIdxValidity(instance, idx); error != nil {
		return nil, error
	}

	return (*T)(arrayGetPtrAtIdx(instance, baseAddr, idx)), nil
}

// ArrayItemPtrGetAtUnsafe returns a pointer to T at idx within the array.
// It does no bounds checks.
//
// Using this pointer after deletion or overwriting this idx is undefined behaviour.
// Use at your own discretion!
//
//go:inline
func ArrayItemPtrGetAtUnsafe[T any](array memcore.Pointer, idx uint64) *T {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[Array[T]](array)
	baseAddr := memcore.MemcorePointerDereferenceRaw(array)
	return (*T)(arrayGetPtrAtIdx(instance, baseAddr, idx))
}

// ArraySetAt sets idx of array to value T.
// It returns an error if the idx is invalid.
//
//go:nosplit
//go:inline
func ArraySetAt[T any](array memcore.Pointer, idx uint64, value T) error {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[Array[T]](array)
	if error := arrayGuaranteeIdxValidity(instance, idx); error != nil {
		return error
	}

	// array.setFn(idx, value)
	baseAddr := memcore.MemcorePointerDereferenceRaw(array)
	setByAssign(instance, baseAddr, idx, value)

	return nil
}

// ArraySetAtUnsafe sets idx of array to value T.
// It does no bounds checks.
//
//go:nosplit
//go:inline
func ArraySetAtUnsafe[T any](array memcore.Pointer, idx uint64, value T) {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[Array[T]](array)
	baseAddr := memcore.MemcorePointerDereferenceRaw(array)
	setByAssign(instance, baseAddr, idx, value)
}

// ArrayReplaceInternal replaces srcIdx with the value at destIdx efficiently.
//
//go:inline
func ArrayReplaceInternal[T any](array memcore.Pointer, srcIdx, destIdx uint64) error {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[Array[T]](array)
	if error := arrayGuaranteeIdxValidity(instance, srcIdx); error != nil {
		return error
	}

	if error := arrayGuaranteeIdxValidity(instance, destIdx); error != nil {
		return error
	}

	baseAddr := memcore.MemcorePointerDereferenceRaw(array)

	srcPtr := arrayGetPtrAtIdx(instance, baseAddr, srcIdx)
	dstPtr := arrayGetPtrAtIdx(instance, baseAddr, destIdx)

	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, instance.itemSizeUintPtr)

	return nil
}

// ArrayReplaceInternalUnsafe replaces srcIdx with the value at destIdx efficiently.
//
// It does no bounds checks.
//
//go:inline
func ArrayReplaceInternalUnsafe[T any](array memcore.Pointer, srcIdx, destIdx uint64) {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[Array[T]](array)
	baseAddr := memcore.MemcorePointerDereferenceRaw(array)
	srcPtr := arrayGetPtrAtIdx(instance, baseAddr, srcIdx)
	dstPtr := arrayGetPtrAtIdx(instance, baseAddr, destIdx)

	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, instance.itemSizeUintPtr)
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
func ArrayShiftRight[T any](array memcore.Pointer, from, to, count uint64) error {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[Array[T]](array)
	if from >= instance.capacity || to >= instance.capacity {
		return fmt.Errorf("invalid range: from=%d to=%d capacity=%d", from, to, instance.capacity)
	}
	if count == 0 || from >= to {
		return nil
	}

	ArrayShiftRightUnsafe[T](array, from, to, count)
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
func ArrayShiftRightUnsafe[T any](array memcore.Pointer, from, to, count uint64) {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[Array[T]](array)
	baseAddr := memcore.MemcorePointerDereferenceRaw(array)
	elemSize := instance.itemSizeUintPtr
	srcPtr := arrayGetPtrAtIdx(instance, baseAddr, from)
	dstPtr := arrayGetPtrAtIdx(instance, baseAddr, from+count)

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
func ArrayShiftLeft[T any](array memcore.Pointer, from, to, count uint64) error {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[Array[T]](array)
	if from >= instance.capacity || to >= instance.capacity {
		return fmt.Errorf("invalid range: from=%d to=%d capacity=%d", from, to, instance.capacity)
	}
	if count == 0 || from >= to {
		return nil
	}

	ArrayShiftLeftUnsafe[T](array, from, to, count)
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
func ArrayShiftLeftUnsafe[T any](array memcore.Pointer, from, to, count uint64) {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[Array[T]](array)
	baseAddr := memcore.MemcorePointerDereferenceRaw(array)

	elemSize := instance.itemSizeUintPtr
	srcPtr := arrayGetPtrAtIdx(instance, baseAddr, from+count)
	dstPtr := arrayGetPtrAtIdx(instance, baseAddr, from)

	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, uintptr((to-from+1)*uint64(elemSize)))
}

// ArrayDeleteAt resets memory to 0 at a given index, using pointers to this
// index gotten earlier is undefined behaviour.
// It returns an error if the index is invalid.
//
//go:nosplit
//go:inline
func ArrayDeleteAt[T any](array memcore.Pointer, idx uint64) error {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[Array[T]](array)
	if error := arrayGuaranteeIdxValidity(instance, idx); error != nil {
		return error
	}

	baseAddr := memcore.MemcorePointerDereferenceRaw(array)
	currentPtr := arrayGetPtrAtIdx(instance, baseAddr, idx)
	memcore.MemoryClearNoHeapPointers(currentPtr, uintptr(instance.itemSize))
	return nil
}

// ArrayDeleteAtUnsafe resets memory to 0 at a given index, using pointers to this
// index gotten earlier is undefined behaviour.
// It does no bounds checks.
//
//go:nosplit
//go:inline
func ArrayDeleteAtUnsafe[T any](array memcore.Pointer, idx uint64) {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[Array[T]](array)
	baseAddr := memcore.MemcorePointerDereferenceRaw(array)
	currentPtr := arrayGetPtrAtIdx(instance, baseAddr, idx)
	memcore.MemoryClearNoHeapPointers(currentPtr, uintptr(instance.itemSize))
}

// ArrayClear resets the entire array's memory to 0, allowing it to be reused.
// Do NOT use pointers to previous array items after this as that is undefined behaviour.
//
//go:nosplit
//go:inline
func ArrayClear[T any](array memcore.Pointer) {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[Array[T]](array)
	baseAddr := memcore.MemcorePointerDereferenceRaw(array)
	memcore.MemoryClearNoHeapPointers(arrayComputeDataAddr(instance, baseAddr), uintptr(instance.capacity)*uintptr(instance.itemSize))
}

// ArrayIsIdxValid checks whether the given index is valid.
//
//go:inline
func ArrayIsIdxValid[T any](array memcore.Pointer, idx uint64) bool {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[Array[T]](array)
	return idx < instance.capacity
}

// -------------------------- PRIVATE HELPERS

//go:inline
func arrayGetPtrAtIdx[T any](instance *Array[T], baseAddr unsafe.Pointer, idx uint64) unsafe.Pointer {
	return unsafe.Add(arrayComputeDataAddr(instance, baseAddr), idx*instance.itemSize)
}

//go:inline
func arrayGuaranteeIdxValidity[T any](instance *Array[T], idx uint64) error {
	idxValid := idx < instance.capacity

	if !idxValid {
		return fmt.Errorf("invalid index: %v, must be between 0 and %v (exclusive)", idx, instance.capacity)
	}

	return nil
}

//go:inline
func arrayComputeDataAddr[T any](instance *Array[T], baseAddr unsafe.Pointer) unsafe.Pointer {
	return unsafe.Add(baseAddr, instance.dataAddrOffset)
}

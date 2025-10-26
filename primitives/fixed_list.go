package primitives

import (
	"fmt"
	"memcore"
	"unsafe"
)

// FixedOrderedList is a list of fixed size.
// It uses an array under the hood but enforces sequential semantics.
type FixedOrderedList[T any] struct {
	array  *Array[T]
	length uint64
}

// FixedOrderedListCreateAt creates an instance of a fixed list for type T at a specific memory address.
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
func FixedOrderedListCreateAt[T any](addr unsafe.Pointer, capacity uint64) *FixedOrderedList[T] {
	array := ArrayCreateAt[T](addr, capacity)

	return &FixedOrderedList[T]{
		array:  array,
		length: 0,
	}
}

// FixedOrderedListItemGetAt returns T at idx within the list.
// It returns an error if the idx is invalid.
//
//go:inline
func FixedOrderedListItemGetAt[T any](fixedList *FixedOrderedList[T], idx uint64) (T, error) {
	if err := fixedListGuaranteeIdxReadValidity(fixedList, idx); err != nil {
		var zero T
		return zero, err
	}

	item := ArrayItemGetAtUnsafe(fixedList.array, idx) // unsafe because length is
	// always smaller than or equal to array capacity, so we don't also need internal bounds check.
	return item, nil
}

// FixedOrderedListItemGetAtUnsafe returns T at idx within the list.
// It does no bounds checks.
//
//go:inline
func FixedOrderedListItemGetAtUnsafe[T any](fixedList *FixedOrderedList[T], idx uint64) T {
	return ArrayItemGetAtUnsafe(fixedList.array, idx)
}

// FixedOrderedListInsert adds an item into the fixed list.
// It returns an error if the bounds are invalid.
func FixedOrderedListInsert[T any](fixedList *FixedOrderedList[T], item T) error {
	if err := ArraySetAt(fixedList.array, fixedList.length, item); err != nil {
		return err
	}

	fixedList.length++

	return nil
}

// FixedOrderedListInsertUnsafe adds an item into the fixed list.
// It does not do bounds checks.
func FixedOrderedListInsertUnsafe[T any](fixedList *FixedOrderedList[T], item T) {
	ArraySetAtUnsafe(fixedList.array, fixedList.length, item)

	fixedList.length++
}

// FixedOrderedListSetAt sets idx of list to value T.
// It returns an error if the idx is invalid.
func FixedOrderedListSetAt[T any](fixedList *FixedOrderedList[T], idx uint64, value T) error {
	if error := fixedListGuaranteeIdxInsertionValidity(fixedList, idx); error != nil {
		return error
	}

	currentPtr := fixedListGetPtrAtIdx(fixedList, idx)
	*(*T)(currentPtr) = value

	return nil
}

// FixedOrderedListSetAtUnsafe sets idx of list to value T.
// It does no bounds checks.
func FixedOrderedListSetAtUnsafe[T any](fixedList *FixedOrderedList[T], idx uint64, value T) {
	currentPtr := fixedListGetPtrAtIdx(fixedList, idx)
	*(*T)(currentPtr) = value
}

// FixedOrderedListInsertAt inserts a value T into the list at position idx,
// shifting all elements from idx through length-1 one slot to the right
// to make space for the new element.
//
// This operation runs in O(n) time since all elements after idx are moved.
// It returns an error if:
//   - idx is outside the valid range [0, length]
//   - or the list has reached its fixed capacity.
//
// Special cases:
//   - If the list is empty and idx == 0, the value is inserted directly without shifting.
//   - Inserting at idx == length appends the value to the end of the list.
func FixedOrderedListInsertAt[T any](fixedList *FixedOrderedList[T], idx uint64, value T) error {
	if err := fixedListGuaranteeIdxInsertionValidity(fixedList, idx); err != nil {
		return err
	}

	// Empty list fast path
	if fixedList.length == 0 && idx == 0 {
		ArraySetAtUnsafe(fixedList.array, 0, value)
		fixedList.length = 1
		return nil
	}

	// Ensure capacity before shifting
	if fixedList.length >= fixedList.array.capacity {
		return fmt.Errorf("no space left in fixed list, capacity reached")
	}

	// Shift elements right to open slot at idx
	for i := fixedList.length; i > idx; i-- {
		ArraySetAtUnsafe(fixedList.array, i, ArrayItemGetAtUnsafe(fixedList.array, i-1))
	}

	ArraySetAtUnsafe(fixedList.array, idx, value)
	fixedList.length++
	return nil
}

// FixedOrderedListInsertAtUnsafe inserts a value T at position idx without
// performing any bounds or capacity checks. It shifts all elements from idx
// through length-1 one slot to the right and overwrites the new slot with `value`.
//
// This variant is intended for hot-path scenarios where correctness is guaranteed
// by the caller. If idx >= length or the list is full, behavior is undefined.
//
// Precondition:
//   - The caller must ensure there is at least one free slot remaining.
//   - The caller must ensure idx <= length.
func FixedOrderedListInsertAtUnsafe[T any](fixedList *FixedOrderedList[T], idx uint64, value T) {
	for i := fixedList.length; i > idx; i-- {
		ArraySetAtUnsafe(fixedList.array, i, ArrayItemGetAtUnsafe(fixedList.array, i-1))
	}
	ArraySetAtUnsafe(fixedList.array, idx, value)
	fixedList.length++
}

// FixedOrderedListDelete deletes the element at idx and shifts subsequent
// elements left by one slot. The last element is cleared.
// Returns an error if idx is invalid.
func FixedOrderedListDelete[T any](fixedList *FixedOrderedList[T], idx uint64) error {
	if err := fixedListGuaranteeIdxReadValidity(fixedList, idx); err != nil {
		return err
	}

	for i := idx; i < fixedList.length-1; i++ {
		nextVal := ArrayItemGetAtUnsafe(fixedList.array, i+1)
		ArraySetAtUnsafe(fixedList.array, i, nextVal)
	}

	lastPtr := fixedListGetPtrAtIdx(fixedList, fixedList.length-1)
	memcore.MemoryClearNoHeapPointers(lastPtr, uintptr(fixedList.array.itemSize))

	fixedList.length--

	return nil
}

// FixedOrderedListDeleteUnsafe deletes at idx and shifts subsequent elements left.
// It does no validation checks at all.
//
// Precondition: idx must be valid, otherwise undefined behaviour.
func FixedOrderedListDeleteUnsafe[T any](fixedList *FixedOrderedList[T], idx uint64) {
	for i := idx; i < fixedList.length-1; i++ {
		nextVal := ArrayItemGetAtUnsafe(fixedList.array, i+1)
		ArraySetAtUnsafe(fixedList.array, i, nextVal)
	}
	lastPtr := fixedListGetPtrAtIdx(fixedList, fixedList.length-1)
	memcore.MemoryClearNoHeapPointers(lastPtr, uintptr(fixedList.array.itemSize))

	fixedList.length--
}

// FixedOrderedListBinarySearch performs a binary search O(log n) to find a value matching
// the predicate in the fixed list.
// This requires that the list is sorted, otherwise bugs will exist.
//
// It will return the index of the predicate in the list.
// It returns an error if the predicate cannot be found.
//
// The predicate function must return:
// <br>- negative when item is below predicate
// <br>- 0 when item is equal to predicate
// <br>- positive when the item is above predicate
func FixedOrderedListBinarySearch[T any](fixedList *FixedOrderedList[T], predicate func(item T) int8) (uint64, error) {
	if fixedList.length == 0 {
		return 0, fmt.Errorf("empty list")
	}

	var lowerBound uint64 = 0
	var upperBound uint64 = fixedList.length - 1

	for lowerBound <= upperBound {
		currentIdx := (lowerBound + upperBound) / 2 // Center
		v := ArrayItemGetAtUnsafe(fixedList.array, currentIdx)

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

// FixedOrderedListBinarySearchInterval uses a binary search O(log n) to find the position
// where predicate can be inserted to keep order.
//
// It returns the index of the previous item and the index of the next item
// when inserted in between them.
//
// The predicate function must return:
// <br>- negative when item is below predicate
// <br>- 0 or positive when the item is above predicate
func FixedOrderedListBinarySearchInterval[T any](fixedList *FixedOrderedList[T], predicate func(item T) int8) (uint64, uint64) {
	if fixedList.length == 0 {
		return ^uint64(0), 0
	}

	var lowerBound uint64 = 0
	var upperBound uint64 = fixedList.length

	for lowerBound < upperBound {
		currentIdx := (lowerBound + upperBound) / 2 // Center
		v := ArrayItemGetAtUnsafe(fixedList.array, currentIdx)

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

	if lowerBound == fixedList.length {
		return fixedList.length - 1, fixedList.length
	}

	return lowerBound - 1, lowerBound
}

// FixedOrderedListBinarySearchInsertionPoint is a convencience wrapper around FixedListBinarySearchInterval,
// it only returns the new insertion point, rather than previous and next indices.
//
// The predicate function must return:
// <br>- negative when item is below predicate
// <br>- 0 or positive when the item is above predicate
func FixedOrderedListBinarySearchInsertionPoint[T any](fixedList *FixedOrderedList[T], predicate func(item T) int8) uint64 {
	if fixedList.length == 0 {
		return 0
	}
	_, nextIdx := FixedOrderedListBinarySearchInterval(fixedList, predicate)
	if nextIdx == ^uint64(0) {
		return 0
	}
	return nextIdx
}

// FixedOrderedListClear resets the list to allow for reuse.
// Using pointers to previous items in the array is undefined behaviour.
// It does not zero the underlying memory as that is not necessary due to insertion semantics.
func FixedOrderedListClear[T any](fixedList *FixedOrderedList[T]) {
	fixedList.length = 0
}

// FixedOrderedListClearAndZero resets the list to allow for reuse.
// Using pointers to previous items in the array is undefined behaviour.
// It does zero the underlying memory, use only for sensitive information.
func FixedOrderedListClearAndZero[T any](fixedList *FixedOrderedList[T]) {
	ArrayClear(fixedList.array)
	fixedList.length = 0
}

func FixedOrderedListLengthGet[T any](l *FixedOrderedList[T]) uint64   { return l.length }
func FixedOrderedListCapacityGet[T any](l *FixedOrderedList[T]) uint64 { return l.array.capacity }

// FixedOrderedListIsIdxValid checks whether the given index is valid.
//
//go:inline
func FixedOrderedListIsIdxValid[T any](l *FixedOrderedList[T], idx uint64) bool {
	return idx < l.length
}

func UnsafeRawPtr[T any](l *FixedOrderedList[T]) unsafe.Pointer {
	return unsafe.Pointer(fixedListGetPtrAtIdx(l, 0))
}

// ----------------------------------------------- PRIVATE HELPERS

//go:inline
func fixedListGuaranteeIdxInsertionValidity[T any](fixedList *FixedOrderedList[T], idx uint64) error {
	if idx >= fixedList.array.capacity {
		return fmt.Errorf("index %v out of bounds: capacity %v", idx, fixedList.array.capacity)
	}
	if idx > fixedList.length {
		return fmt.Errorf("invalid index: %v, must be between 0 and %v (inclusive)", idx, fixedList.length)
	}

	return nil
}

//go:inline
func fixedListGuaranteeIdxReadValidity[T any](fixedList *FixedOrderedList[T], idx uint64) error {
	if idx >= fixedList.length {
		return fmt.Errorf("invalid index: %v, must be between 0 and %v (exclusive)", idx, fixedList.length)
	}

	return nil
}

//go:inline
func fixedListGetPtrAtIdx[T any](fixedList *FixedOrderedList[T], idx uint64) unsafe.Pointer {
	return unsafe.Add(fixedList.array.ptr, idx*fixedList.array.itemSize)
}

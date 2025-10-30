package primitives

import (
	"fmt"
	"memcore"
	"unsafe"
)

// FixedOrderedList is a list of fixed size.
// It uses an array under the hood but enforces sequential semantics.
type FixedOrderedList[T any] struct {
	dataArray *Array[T]
	indices   *Array[uint64]
	freeList  *Stack[uint64]

	length   uint64
	capacity uint64
}

// FixedOrderedListRequiredBytes computes the necessary amount of bytes for the fixed list.
func FixedOrderedListRequiredBytes[T any](capacity uint64) uint64 {
	sizeData := alignIdxUp(
		capacity*memcore.SizeOf[T](),
		memcore.AlignOf[T](),
	)
	sizeIndices := alignIdxUp(
		capacity*memcore.SizeOf[uint64](),
		memcore.AlignOf[uint64](),
	)
	sizeFreelist := alignIdxUp(
		capacity*memcore.SizeOf[uint64](),
		memcore.AlignOf[uint64](),
	)
	return sizeData + sizeIndices + sizeFreelist
}

// FixedOrderedListCreateAt creates an instance of a fixed list for type T at a specific memory address.
// Ensure the address is properly aligned and has the right size (call FixedOrderedListRequiredBytes to compute).
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
	sizeIndices := alignIdxUp(capacity*memcore.SizeOf[uint64](), memcore.AlignOf[uint64]())
	sizeFreelist := alignIdxUp(capacity*memcore.SizeOf[uint64](), memcore.AlignOf[uint64]())

	// --- derive subaddresses
	indicesAddr := addr
	freeListAddr := unsafe.Add(indicesAddr, sizeIndices)
	dataAddr := unsafe.Add(freeListAddr, sizeFreelist)

	// --- build subcontainers
	indices := ArrayCreateAt[uint64](indicesAddr, capacity)
	freeList := StackCreateAt[uint64](freeListAddr, capacity)
	data := ArrayCreateAt[T](dataAddr, capacity)

	// --- fill freelist with all available slots
	for i := uint64(0); i < capacity; i++ {
		StackPushUnsafe(freeList, i)
	}

	return &FixedOrderedList[T]{
		dataArray: data,
		indices:   indices,
		freeList:  freeList,
		length:    0,
		capacity:  capacity,
	}
}

// FixedOrderedListItemGetAt returns T at idx within the list.
// It returns an error if the idx is invalid.
//
//go:inline
//go:nosplit
func FixedOrderedListItemGetAt[T any](fixedList *FixedOrderedList[T], idx uint64) (T, error) {
	if err := fixedListGuaranteeIdxReadValidity(fixedList, idx); err != nil {
		var zero T
		return zero, err
	}

	item := fixedListGetElement(fixedList, idx)
	return item, nil
}

// FixedOrderedListItemGetAtUnsafe returns T at idx within the list.
// It does no bounds checks.
//
//go:inline
//go:nosplit
func FixedOrderedListItemGetAtUnsafe[T any](fixedList *FixedOrderedList[T], idx uint64) T {
	return fixedListGetElement(fixedList, idx)
}

// FixedOrderedListItemPtrGetAt returns a pointer to  T at idx within the list.
// It returns an error if the idx is invalid.
//
// Using this pointer after deletion or overwriting this idx is undefined behaviour.
// Use at your own discretion!
//
//go:inline
//go:nosplit
func FixedOrderedListItemPtrGetAt[T any](fixedList *FixedOrderedList[T], idx uint64) (*T, error) {
	if err := fixedListGuaranteeIdxReadValidity(fixedList, idx); err != nil {
		return nil, err
	}

	item := fixedListGetElementPtr(fixedList, idx)
	return item, nil
}

// FixedOrderedListItemPtrGetAtUnsafe returns a pointer to T at idx within the list.
// It does no bounds checks.
//
// Using this pointer after deletion or overwriting this idx is undefined behaviour.
// Use at your own discretion!
//
//go:inline
//go:nosplit
func FixedOrderedListItemPtrGetAtUnsafe[T any](fixedList *FixedOrderedList[T], idx uint64) *T {
	return fixedListGetElementPtr(fixedList, idx)
}

// FixedOrderedListAppend adds an item into the fixed list.
// It returns an error if the bounds are invalid.
//
//go:nosplit
//go:inline
func FixedOrderedListAppend[T any](fixedList *FixedOrderedList[T], item T) error {
	slot := StackPopUnsafe(fixedList.freeList)
	ArraySetAtUnsafe(fixedList.indices, fixedList.length, slot)

	if err := ArraySetAt(fixedList.dataArray, slot, item); err != nil {
		return err
	}

	fixedList.length++

	return nil
}

// FixedOrderedListAppendUnsafe adds an item into the fixed list.
// It does not do bounds checks.
//
//go:nosplit
//go:inline
func FixedOrderedListAppendUnsafe[T any](fixedList *FixedOrderedList[T], item T) {
	slot := StackPopUnsafe(fixedList.freeList)
	ArraySetAtUnsafe(fixedList.indices, fixedList.length, slot)
	ArraySetAtUnsafe(fixedList.dataArray, slot, item)

	fixedList.length++
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
	if fixedList.length >= fixedList.dataArray.capacity {
		return fmt.Errorf("no space left in fixed list, capacity reached")
	}

	slot := StackPopUnsafe(fixedList.freeList)
	ArraySetAtUnsafe(fixedList.dataArray, slot, value)
	fixedListInsertIndex(fixedList, idx, slot)

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
//
//go:nosplit
func FixedOrderedListInsertAtUnsafe[T any](fixedList *FixedOrderedList[T], idx uint64, value T) {
	slot := StackPopUnsafe(fixedList.freeList)
	ArraySetAtUnsafe(fixedList.dataArray, slot, value)
	fixedListInsertIndex(fixedList, idx, slot)

	fixedList.length++
}

// FixedOrderedListDelete deletes the element at idx and shifts subsequent
// elements left by one slot. The last element is cleared.
// Returns an error if idx is invalid.
//
//go:nosplit
func FixedOrderedListDelete[T any](fixedList *FixedOrderedList[T], idx uint64) error {
	if err := fixedListGuaranteeIdxReadValidity(fixedList, idx); err != nil {
		return err
	}

	slot := ArrayItemGetAtUnsafe(fixedList.indices, idx)
	fixedListRemoveIndex(fixedList, idx)
	StackPushUnsafe(fixedList.freeList, slot)

	fixedList.length--
	return nil
}

// FixedOrderedListDeleteUnsafe deletes at idx and shifts subsequent elements left.
// It does no validation checks at all.
//
// Precondition: idx must be valid, otherwise undefined behaviour.
//
//go:nosplit
func FixedOrderedListDeleteUnsafe[T any](fixedList *FixedOrderedList[T], idx uint64) {
	slot := ArrayItemGetAtUnsafe(fixedList.indices, idx)
	fixedListRemoveIndex(fixedList, idx)
	StackPushUnsafe(fixedList.freeList, slot)

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
// <br>- 127 to skip
//
//go:nosplit
//go:inline
func FixedOrderedListBinarySearch[T any](l *FixedOrderedList[T], predicate func(item T) int8) (uint64, error) {
	n := l.length
	if n == 0 {
		return 0, fmt.Errorf("empty list")
	}

	lo := uint64(0)
	hi := n

	for lo < hi {
		mid := (lo + hi) >> 1
		item := fixedListGetElement(l, mid)

		cmp := predicate(item)
		if cmp < 0 {
			lo = mid + 1
		} else if cmp > 0 {
			hi = mid
		} else {
			return mid, nil
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
//
//go:nosplit
//go:inline
func FixedOrderedListBinarySearchInterval[T any](l *FixedOrderedList[T], predicate func(item T) int8) (uint64, uint64) {
	n := l.length
	if n == 0 {
		return ^uint64(0), 0
	}

	lo := uint64(0)
	hi := n

	for lo < hi {
		mid := (lo + hi) >> 1
		item := fixedListGetElement(l, mid)

		if predicate(item) < 0 {
			lo = mid + 1
		} else {
			hi = mid
		}
	}

	switch lo {
	case 0:
		return ^uint64(0), 0
	case n:
		return n - 1, n
	default:
		return lo - 1, lo
	}
}

// FixedOrderedListBinarySearchInsertionPoint is a convencience wrapper around FixedListBinarySearchInterval,
// it only returns the new insertion point, rather than previous and next indices.
//
// The predicate function must return:
// <br>- negative when item is below predicate
// <br>- 0 or positive when the item is above predicate
//
//go:nosplit
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
//
//go:inline
func FixedOrderedListClear[T any](fixedList *FixedOrderedList[T]) {
	fixedList.length = 0
	StackClear(fixedList.freeList)

	for i := uint64(0); i < fixedList.capacity; i++ {
		StackPushUnsafe(fixedList.freeList, i)
	}
}

// FixedOrderedListClearAndZero resets the list to allow for reuse.
// Using pointers to previous items in the array is undefined behaviour.
// It does zero the underlying memory, use only for sensitive information.
//
//go:nosplit
//go:inline
func FixedOrderedListClearAndZero[T any](fixedList *FixedOrderedList[T]) {
	ArrayClear(fixedList.dataArray)
	StackClearAndZero(fixedList.freeList)

	for i := uint64(0); i < fixedList.capacity; i++ {
		StackPushUnsafe(fixedList.freeList, i)
	}

	fixedList.length = 0
}

func FixedOrderedListLengthGet[T any](l *FixedOrderedList[T]) uint64   { return l.length }
func FixedOrderedListCapacityGet[T any](l *FixedOrderedList[T]) uint64 { return l.capacity }

// FixedOrderedListIsIdxValid checks whether the given index is valid.
//
//go:inline
func FixedOrderedListIsIdxValid[T any](l *FixedOrderedList[T], idx uint64) bool {
	return idx < l.length
}

// ----------------------------------------------- PRIVATE HELPERS

//go:inline
func fixedListInsertIndex[T any](fixedList *FixedOrderedList[T], logicalIdx uint64, slot uint64) {
	n := fixedList.length
	for i := n; i > logicalIdx; i-- {
		prev := ArrayItemGetAtUnsafe(fixedList.indices, i-1)
		ArraySetAtUnsafe(fixedList.indices, i, prev)
	}

	ArraySetAtUnsafe(fixedList.indices, logicalIdx, slot)
}

//go:inline
func fixedListRemoveIndex[T any](fixedList *FixedOrderedList[T], logicalIdx uint64) {
	n := fixedList.length
	for i := logicalIdx; i+1 < n; i++ {
		next := ArrayItemGetAtUnsafe(fixedList.indices, i+1)
		ArraySetAtUnsafe(fixedList.indices, i, next)
	}
}

//go:inline
func fixedListGetPhysicalIdx[T any](fixedList *FixedOrderedList[T], logicalIdx uint64) uint64 {
	return ArrayItemGetAtUnsafe(fixedList.indices, logicalIdx)
}

//go:inline
func fixedListGetElement[T any](fixedList *FixedOrderedList[T], logicalIdx uint64) T {
	physicalIdx := fixedListGetPhysicalIdx(fixedList, logicalIdx)
	return ArrayItemGetAtUnsafe(fixedList.dataArray, physicalIdx)
}

//go:inline
func fixedListGetElementPtr[T any](fixedList *FixedOrderedList[T], logicalIdx uint64) *T {
	physicalIdx := fixedListGetPhysicalIdx(fixedList, logicalIdx)
	return ArrayItemPtrGetAtUnsafe(fixedList.dataArray, physicalIdx)
}

//go:inline
func fixedListGuaranteeIdxInsertionValidity[T any](fixedList *FixedOrderedList[T], idx uint64) error {
	if idx >= fixedList.capacity {
		return fmt.Errorf("index %v out of bounds: capacity %v", idx, fixedList.capacity)
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

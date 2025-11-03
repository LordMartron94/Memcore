package primitives

import (
	"fmt"
	"memcore"
)

//
// ────────────────────────────────────────────────────────────────────────────────
//   STRUCTURE OVERVIEW
// ────────────────────────────────────────────────────────────────────────────────
//
// FixedOrderedList[T] is a manually managed, fixed-capacity sequential list
// built on top of Array[T] and Stack[uint64] primitives.
//
// Each element is stored in a contiguous Array[T], but logical ordering is
// maintained via an Array[uint64] index table. A freelist Stack[uint64] holds
// all unused slots, allowing O(1) slot reuse without heap allocations.
//
// All internal substructures (arrays, stacks, snapshots) live inside the same
// memcore namespace. Consequently, the list is fully relocatable and can be
// snapshotted or restored purely through pointer-offset reconstruction.
//
// The struct itself does not contain Go pointers and is therefore safe to
// allocate in manually managed memory, provided all subpointers are registered
// in memcore before initialization.
//

// FixedOrderedList is a relocatable, manually-managed sequential container.
type FixedOrderedList[T any] struct {
	dataArray        memcore.Pointer // Array[T]
	indices          memcore.Pointer // Array[uint64]
	freeList         memcore.Pointer // Stack[uint64]
	freeListSnapshot memcore.Pointer // Stack[uint64]

	length   uint64 // number of logically occupied slots
	capacity uint64 // total available slots
}

//
// ────────────────────────────────────────────────────────────────────────────────
//   SIZE & ALIGNMENT
// ────────────────────────────────────────────────────────────────────────────────
//

// FixedOrderedListRequiredBytes returns the total number of bytes required to
// store a FixedOrderedList[T] with a given element capacity.
//
// The calculation includes:
//   - One Array[T] data store
//   - One Array[uint64] index table
//   - Two Stack[uint64] freelists (active + snapshot)
//   - Header
//
// Use this function to determine how much manual memory must be allocated
// prior to calling FixedOrderedListInitializeAt.
func FixedOrderedListRequiredBytes[T any](capacity uint64) uint64 {
	sizeData := ArrayRequiredBytesGet[T](capacity)
	sizeIndices := ArrayRequiredBytesGet[uint64](capacity)
	sizeFreelist := StackRequiredBytesGet[uint64](capacity)
	return sizeData + sizeIndices + sizeFreelist + sizeFreelist + memcore.SizeOf[FixedOrderedList[T]]()
}

// FixedOrderedListRequiredAlignment returns the maximum alignment constraint
// among all internal components (data array, indices, stacks).
func FixedOrderedListRequiredAlignment[T any]() uint64 {
	return max(
		ArrayRequiredAlignmentGet[T](),
		ArrayRequiredAlignmentGet[uint64](),
		StackRequiredAlignmentGet[uint64](),
		memcore.AlignOf[FixedOrderedList[T]](),
	)
}

//
// ────────────────────────────────────────────────────────────────────────────────
//   INITIALIZATION / DESTRUCTION
// ────────────────────────────────────────────────────────────────────────────────
//

// FixedOrderedListInitializeAt constructs a FixedOrderedList[T] header and
// initializes its subcomponents in a pre-allocated, manually managed region.
//
// Preconditions:
//   - The memory region referenced by listAddr must belong to a registered
//     memcore namespace.
//   - The region must have at least FixedOrderedListRequiredBytes[T](capacity)
//     bytes available and satisfy FixedOrderedListRequiredAlignment[T].
//
// The function creates and registers four internal pointers within the same
// namespace (indices, freeList, freeListSnapshot, and dataArray), initializes
// all of them, fills the freelist, and snapshots it for future resets.
//
// Capacity is expressed in number of elements, not bytes.
func FixedOrderedListInitializeAt[T any](listAddr memcore.Pointer, capacity uint64) {
	addressSpace := memcore.PointerAddressSpace(listAddr)
	baseOffset := memcore.PointerOffset(listAddr)

	// Compute offsets for internal components relative to list header
	sizeIndices := ArrayRequiredBytesGet[uint64](capacity)
	sizeFreelist := StackRequiredBytesGet[uint64](capacity)
	sizeFreelistSnapshot := sizeFreelist

	offsetIndices := uintptr(memcore.SizeOf[FixedOrderedList[T]]())
	offsetFreelist := offsetIndices + uintptr(sizeIndices)
	offsetFreelistSnapshot := offsetFreelist + uintptr(sizeFreelist)
	offsetData := offsetFreelistSnapshot + uintptr(sizeFreelistSnapshot)

	// Create subpointers within the same namespace
	indicesPtr := memcore.MemcorePointerCreate(addressSpace, baseOffset + offsetIndices, memcore.TypeOf[Array[uint64]]())
	freeListPtr := memcore.MemcorePointerCreate(addressSpace, baseOffset + offsetFreelist, memcore.TypeOf[Stack[uint64]]())
	freeListSnapshotPtr := memcore.MemcorePointerCreate(addressSpace, baseOffset + offsetFreelistSnapshot, memcore.TypeOf[Stack[uint64]]())
	dataPtr := memcore.MemcorePointerCreate(addressSpace, baseOffset + offsetData, memcore.TypeOf[Array[T]]())

	memcore.MemcorePointerRegister(indicesPtr)
	memcore.MemcorePointerUpdateType(indicesPtr, memcore.TypeOf[Array[uint64]]())

	memcore.MemcorePointerRegister(freeListPtr)
	memcore.MemcorePointerUpdateType(freeListPtr, memcore.TypeOf[Stack[uint64]]())

	memcore.MemcorePointerRegister(freeListSnapshotPtr)
	memcore.MemcorePointerUpdateType(freeListSnapshotPtr, memcore.TypeOf[Stack[uint64]]())

	memcore.MemcorePointerRegister(dataPtr)
	memcore.MemcorePointerUpdateType(dataPtr, memcore.TypeOf[Array[T]]())

	// Initialize substructures
	ArrayInitializeAt[uint64](indicesPtr, capacity)
	StackInitializeAt[uint64](freeListPtr, capacity)
	ArrayInitializeAt[T](dataPtr, capacity)

	// Populate freelist with all slots [0, capacity)
	for i := uint64(0); i < capacity; i++ {
		StackPushUnsafe(freeListPtr, i)
	}

	// Snapshot freelist for O(1) resets
	StackSnapshotCreate[uint64](freeListSnapshotPtr, freeListPtr)

	// Write header fields
	list := memcore.MemcorePointerDereferenceObjectUnsafe[FixedOrderedList[T]](listAddr)
	*list = FixedOrderedList[T]{
		dataArray:        dataPtr,
		indices:          indicesPtr,
		freeList:         freeListPtr,
		freeListSnapshot: freeListSnapshotPtr,
		length:           0,
		capacity:         capacity,
	}
}

// FixedOrderedListDestroy unregisters all internal pointers belonging to
// the FixedOrderedList. It must be called before destroying the parent
// allocator or namespace.
func FixedOrderedListDestroy[T any](listAddr memcore.Pointer) {
	list := memcore.MemcorePointerDereferenceObjectUnsafe[FixedOrderedList[T]](listAddr)
	memcore.MemcorePointerUnregister(list.dataArray)
	memcore.MemcorePointerUnregister(list.indices)
	memcore.MemcorePointerUnregister(list.freeList)
	memcore.MemcorePointerUnregister(list.freeListSnapshot)
}

//
// ────────────────────────────────────────────────────────────────────────────────
//   ELEMENT ACCESSORS
// ────────────────────────────────────────────────────────────────────────────────
//

// FixedOrderedListItemGetAt retrieves the element at logical index `idx`.
// Returns an error if `idx` is out of range.
func FixedOrderedListItemGetAt[T any](list memcore.Pointer, idx uint64) (T, error) {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[FixedOrderedList[T]](list)
	if err := fixedListGuaranteeIdxReadValidity(instance, idx); err != nil {
		var zero T
		return zero, err
	}
	return fixedListGetElement(instance, idx), nil
}

// FixedOrderedListItemGetAtUnsafe retrieves an element without performing
// any bounds checks.
func FixedOrderedListItemGetAtUnsafe[T any](list memcore.Pointer, idx uint64) T {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[FixedOrderedList[T]](list)
	return fixedListGetElement(instance, idx)
}

// FixedOrderedListItemPtrGetAt returns a pointer to the element at `idx`,
// validating that the index is within range.
func FixedOrderedListItemPtrGetAt[T any](list memcore.Pointer, idx uint64) (*T, error) {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[FixedOrderedList[T]](list)
	if err := fixedListGuaranteeIdxReadValidity(instance, idx); err != nil {
		return nil, err
	}
	return fixedListGetElementPtr(instance, idx), nil
}

// FixedOrderedListItemPtrGetAtUnsafe returns a pointer to the element
// at `idx` without any safety checks.
func FixedOrderedListItemPtrGetAtUnsafe[T any](list memcore.Pointer, idx uint64) *T {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[FixedOrderedList[T]](list)
	return fixedListGetElementPtr(instance, idx)
}

//
// ────────────────────────────────────────────────────────────────────────────────
//   INSERTION & DELETION
// ────────────────────────────────────────────────────────────────────────────────
//

// FixedOrderedListAppend appends a new element to the end of the list.
// Returns an error if the list is already full.
func FixedOrderedListAppend[T any](list memcore.Pointer, value T) error {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[FixedOrderedList[T]](list)
	if instance.length >= instance.capacity {
		return fmt.Errorf("fixed list full: capacity %d", instance.capacity)
	}
	slot := StackPopUnsafe[uint64](instance.freeList)
	ArraySetAtUnsafe(instance.indices, instance.length, slot)
	ArraySetAtUnsafe(instance.dataArray, slot, value)
	instance.length++
	return nil
}

// FixedOrderedListAppendUnsafe appends without capacity validation.
func FixedOrderedListAppendUnsafe[T any](list memcore.Pointer, value T) {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[FixedOrderedList[T]](list)
	slot := StackPopUnsafe[uint64](instance.freeList)
	ArraySetAtUnsafe(instance.indices, instance.length, slot)
	ArraySetAtUnsafe(instance.dataArray, slot, value)
	instance.length++
}

// FixedOrderedListInsertAt inserts a value at a given logical index, shifting
// subsequent elements to the right. Runs in O(n) time.
func FixedOrderedListInsertAt[T any](list memcore.Pointer, idx uint64, value T) error {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[FixedOrderedList[T]](list)
	if err := fixedListGuaranteeIdxInsertionValidity(instance, idx); err != nil {
		return err
	}
	if instance.length >= instance.capacity {
		return fmt.Errorf("no space left in fixed list")
	}
	slot := StackPopUnsafe[uint64](instance.freeList)
	ArraySetAtUnsafe(instance.dataArray, slot, value)
	fixedListInsertIndex(instance, idx, slot)
	instance.length++
	return nil
}

// FixedOrderedListInsertAtUnsafe inserts without validation.
func FixedOrderedListInsertAtUnsafe[T any](list memcore.Pointer, idx uint64, value T) {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[FixedOrderedList[T]](list)
	slot := StackPopUnsafe[uint64](instance.freeList)
	ArraySetAtUnsafe(instance.dataArray, slot, value)
	fixedListInsertIndex(instance, idx, slot)
	instance.length++
}

// FixedOrderedListDelete removes an element at logical index `idx` and
// shifts remaining elements left by one slot. Returns an error if `idx`
// is invalid.
func FixedOrderedListDelete[T any](list memcore.Pointer, idx uint64) error {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[FixedOrderedList[T]](list)
	if err := fixedListGuaranteeIdxReadValidity(instance, idx); err != nil {
		return err
	}
	slot := ArrayItemGetAtUnsafe[uint64](instance.indices, idx)
	fixedListRemoveIndex(instance, idx)
	StackPushUnsafe(instance.freeList, slot)
	instance.length--
	return nil
}

// FixedOrderedListDeleteUnsafe deletes without validation.
func FixedOrderedListDeleteUnsafe[T any](list memcore.Pointer, idx uint64) {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[FixedOrderedList[T]](list)
	slot := ArrayItemGetAtUnsafe[uint64](instance.indices, idx)
	fixedListRemoveIndex(instance, idx)
	StackPushUnsafe(instance.freeList, slot)
	instance.length--
}

//
// ────────────────────────────────────────────────────────────────────────────────
//   SEARCH UTILITIES
// ────────────────────────────────────────────────────────────────────────────────
//

// FixedOrderedListBinarySearch performs a binary search over the list using
// the provided comparator predicate. The list must be sorted according to
// the same predicate logic.
//
// The predicate must return:
//   - negative when item < target
//   - 0 when item == target
//   - positive when item > target
//
// Returns the index of the matching element or an error if not found.
func FixedOrderedListBinarySearch[T any](list memcore.Pointer, predicate func(item T) int8) (uint64, error) {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[FixedOrderedList[T]](list)
	n := instance.length
	if n == 0 {
		return 0, fmt.Errorf("empty list")
	}

	lo := uint64(0)
	hi := n
	for lo < hi {
		mid := (lo + hi) >> 1
		item := fixedListGetElement(instance, mid)
		cmp := predicate(item)
		if cmp < 0 {
			lo = mid + 1
		} else if cmp > 0 {
			hi = mid
		} else {
			return mid, nil
		}
	}
	return 0, fmt.Errorf("predicate not found")
}

// FixedOrderedListBinarySearchInterval locates the interval between two
// neighboring elements where a new value could be inserted to preserve order.
//
// Returns:
//   - (prevIdx, nextIdx): indices of the neighboring elements
//   - (^uint64(0), 0) if the list is empty
//
// The predicate rules are identical to BinarySearch but do not require equality.
func FixedOrderedListBinarySearchInterval[T any](list memcore.Pointer, predicate func(item T) int8) (uint64, uint64) {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[FixedOrderedList[T]](list)
	n := instance.length
	if n == 0 {
		return ^uint64(0), 0
	}

	lo := uint64(0)
	hi := n
	for lo < hi {
		mid := (lo + hi) >> 1
		item := fixedListGetElement(instance, mid)
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

// FixedOrderedListBinarySearchInsertionPoint is a convenience wrapper
// around BinarySearchInterval that returns only the new insertion index.
func FixedOrderedListBinarySearchInsertionPoint[T any](list memcore.Pointer, predicate func(item T) int8) uint64 {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[FixedOrderedList[T]](list)
	if instance.length == 0 {
		return 0
	}
	_, next := FixedOrderedListBinarySearchInterval(list, predicate)
	if next == ^uint64(0) {
		return 0
	}
	return next
}

//
// ────────────────────────────────────────────────────────────────────────────────
//   CLEAR / RESET
// ────────────────────────────────────────────────────────────────────────────────
//

// FixedOrderedListClear resets logical length to zero and restores the
// freelist from its snapshot, allowing instant reuse of all slots.
// The underlying memory contents remain untouched.
func FixedOrderedListClear[T any](list memcore.Pointer) {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[FixedOrderedList[T]](list)
	StackSnapshotRestore[uint64](instance.freeList, instance.freeListSnapshot)
	instance.length = 0
}

// FixedOrderedListClearAndZero behaves like Clear but additionally wipes
// all data array memory using zeroing semantics (useful for sensitive data).
func FixedOrderedListClearAndZero[T any](list memcore.Pointer) {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[FixedOrderedList[T]](list)
	ArrayClear[T](instance.dataArray)
	StackSnapshotRestore[uint64](instance.freeList, instance.freeListSnapshot)
	instance.length = 0
}

//
// ────────────────────────────────────────────────────────────────────────────────
//   LENGTH / CAPACITY UTILITIES
// ────────────────────────────────────────────────────────────────────────────────
//

func FixedOrderedListLengthGet[T any](list memcore.Pointer) uint64 {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[FixedOrderedList[T]](list)
	return instance.length
}

func FixedOrderedListCapacityGet[T any](list memcore.Pointer) uint64 {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[FixedOrderedList[T]](list)
	return instance.capacity
}

func FixedOrderedListIsIdxValid[T any](list memcore.Pointer, idx uint64) bool {
	instance := memcore.MemcorePointerDereferenceObjectUnsafe[FixedOrderedList[T]](list)
	return idx < instance.length
}

//
// ────────────────────────────────────────────────────────────────────────────────
//   PRIVATE INTERNALS
// ────────────────────────────────────────────────────────────────────────────────
//

func fixedListInsertIndex[T any](list *FixedOrderedList[T], logicalIdx, slot uint64) {
	n := list.length
	for i := n; i > logicalIdx; i-- {
		prev := ArrayItemGetAtUnsafe[uint64](list.indices, i-1)
		ArraySetAtUnsafe(list.indices, i, prev)
	}
	ArraySetAtUnsafe(list.indices, logicalIdx, slot)
}

func fixedListRemoveIndex[T any](list *FixedOrderedList[T], logicalIdx uint64) {
	n := list.length
	for i := logicalIdx; i+1 < n; i++ {
		next := ArrayItemGetAtUnsafe[uint64](list.indices, i+1)
		ArraySetAtUnsafe(list.indices, i, next)
	}
}

func fixedListGetPhysicalIdx[T any](list *FixedOrderedList[T], logicalIdx uint64) uint64 {
	return ArrayItemGetAtUnsafe[uint64](list.indices, logicalIdx)
}

func fixedListGetElement[T any](list *FixedOrderedList[T], logicalIdx uint64) T {
	physicalIdx := fixedListGetPhysicalIdx(list, logicalIdx)
	return ArrayItemGetAtUnsafe[T](list.dataArray, physicalIdx)
}

func fixedListGetElementPtr[T any](list *FixedOrderedList[T], logicalIdx uint64) *T {
	physicalIdx := fixedListGetPhysicalIdx(list, logicalIdx)
	return ArrayItemPtrGetAtUnsafe[T](list.dataArray, physicalIdx)
}

func fixedListGuaranteeIdxInsertionValidity[T any](list *FixedOrderedList[T], idx uint64) error {
	if idx >= list.capacity {
		return fmt.Errorf("index %v out of bounds (capacity %v)", idx, list.capacity)
	}
	if idx > list.length {
		return fmt.Errorf("invalid insertion index %v (0 ≤ idx ≤ %v)", idx, list.length)
	}
	return nil
}

func fixedListGuaranteeIdxReadValidity[T any](list *FixedOrderedList[T], idx uint64) error {
	if idx >= list.length {
		return fmt.Errorf("invalid read index %v (0 ≤ idx < %v)", idx, list.length)
	}
	return nil
}

package primitives

import (
	"fmt"
	"memcore"
	"unsafe"
)

const chunkSizeElements uint64 = 64
const chunkBitShiftAmount uint64 = 6 // X >> 6 = X / 64
const chunkBitAndAmount uint64 = 63  // X & 63 = X % 64

type chunkHeader struct {
	minGlobalIdx   uint64
	maxGlobalIdx   uint64
	remainingSlots uint8
}

// SegmentedFixedOrderedList is a segmented list of fixed size.
// It uses fixed ordered lists under the hood but chunks them.
type SegmentedFixedOrderedList[T any] struct {
	chunks       []*FixedOrderedList[T]
	chunkHeaders *Array[chunkHeader]

	globalCapacityElements uint64
}

// SegmentedFixedOrderedListCapacityBytesGet computes the necessary amount of bytes for the segmented fixed list.
func SegmentedFixedOrderedListCapacityBytesGet[T any](capacityElements uint64) uint64 {
	chunkAmount := capacityElements >> chunkBitShiftAmount
	chunkAmount = max(chunkAmount, 1)

	chunkArraySize := segmentedFixedOrderedListChunkBytes[T]() * chunkAmount
	chunkHeaderArraySize := segmentedFixedOrderedListChunkHeaderArrayBytes(chunkAmount)

	return chunkArraySize + chunkHeaderArraySize
}

// SegmentedFixedOrderedListCreateAt creates an instance of a segmented fixed list for type T at a specific memory address.
// Ensure the address is properly aligned and has the right size (call SegmentedFixedOrderedListCapacityBytesGet to compute).
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
func SegmentedFixedOrderedListCreateAt[T any](addr unsafe.Pointer, capacityElements uint64) *SegmentedFixedOrderedList[T] {
	chunkAmount := capacityElements >> chunkBitShiftAmount
	chunkAmount = max(chunkAmount, 1)
	chunkHeaderArrayBytes := segmentedFixedOrderedListChunkHeaderArrayBytes(chunkAmount)

	chunkHeadersAddr := addr
	chunkStartAddr := unsafe.Add(chunkHeadersAddr, chunkHeaderArrayBytes)
	chunkBytes := segmentedFixedOrderedListChunkBytes[T]()

	chunks := make([]*FixedOrderedList[T], chunkAmount)
	chunkHeaders := ArrayCreateAt[chunkHeader](chunkHeadersAddr, chunkAmount)
	for i := uint64(0); i < chunkAmount; i++ {
		ArraySetAtUnsafe(chunkHeaders, i, chunkHeader{
			minGlobalIdx:   i * chunkSizeElements,
			maxGlobalIdx:   (i+1)*chunkSizeElements - 1,
			remainingSlots: uint8(chunkSizeElements),
		})

		chunkAddr := unsafe.Add(chunkStartAddr, (i * chunkBytes))
		chunks[i] = FixedOrderedListCreateAt[T](chunkAddr, chunkSizeElements)
	}

	return &SegmentedFixedOrderedList[T]{
		chunks:                 chunks,
		chunkHeaders:           chunkHeaders,
		globalCapacityElements: capacityElements,
	}
}

// SegmentedFixedOrderedListClear clears the segmented fixed ordered list for re-use.
// Using pointers to previous items in the array is undefined behaviour.
// It does not clear memory.
func SegmentedFixedOrderedListClear[T any](segmentedList *SegmentedFixedOrderedList[T]) {
	chunkHeaderAmount := ArrayCapacityGet(segmentedList.chunkHeaders)
	for i := uint64(0); i < chunkHeaderAmount; i++ {
		chunk := segmentedList.chunks[i]
		FixedOrderedListClear(chunk)

		hdr := ArrayItemPtrGetAtUnsafe(segmentedList.chunkHeaders, i)
		hdr.remainingSlots = uint8(chunkSizeElements)
	}
}

// SegmentedFixedOrderedListClearAndZero clears the segmented fixed ordered list for re-use.
// Using pointers to previous items in the array is undefined behaviour.
// It clears memory.
func SegmentedFixedOrderedListClearAndZero[T any](segmentedList *SegmentedFixedOrderedList[T]) {
	chunkHeaderAmount := ArrayCapacityGet(segmentedList.chunkHeaders)
	for i := uint64(0); i < chunkHeaderAmount; i++ {
		chunk := segmentedList.chunks[i]
		FixedOrderedListClearAndZero(chunk)

		hdr := ArrayItemPtrGetAtUnsafe(segmentedList.chunkHeaders, i)
		hdr.remainingSlots = uint8(chunkSizeElements)
	}
}

// SegmentedFixedOrderedListAppend adds an item into the segmented fixed list.
// It returns an error if the bounds are invalid.
//
//go:nosplit
//go:inline
func SegmentedFixedOrderedListAppend[T any](seg *SegmentedFixedOrderedList[T], value T) error {
	chunkAmount := ArrayCapacityGet(seg.chunkHeaders)
	for i := uint64(0); i < chunkAmount; i++ {
		h := ArrayItemPtrGetAtUnsafe(seg.chunkHeaders, i)
		if h.remainingSlots > 0 {
			FixedOrderedListAppendUnsafe(seg.chunks[i], value)
			h.remainingSlots--
			return nil
		}
	}
	return fmt.Errorf("append: no free chunk available")
}

// SegmentedFixedOrderedListInsert inserts a value into the segmented list at the given global index.
//
// Conceptually, this function determines which fixed-size chunk corresponds to the given
// global index (using bit shifting and masking), then inserts the element into that chunk.
//
// If the target chunk still has available space, the value is inserted directly,
// shifting elements locally within that 64-element segment (O(chunk_size) time).
//
// If the target chunk is full, the last element of that chunk is evicted ("spilled")
// and propagated to the next chunk on the right. The same process recursively continues
// until an available slot is found or the final chunk is reached.
//
// This ensures the entire segmented list maintains global sequential order while
// keeping insertion and shifting operations locally bounded.
//
// Time complexity:
//   - O(chunk_size) in the common case (single-chunk shift)
//   - O(k * chunk_size) in rare worst case when spill cascades through k chunks
//
// Errors:
//   - Returns an error if the index is out of bounds
//   - Returns an error if all chunks to the right are full (no space available)
//
// Invariants:
//   - Each chunk may hold at most `chunkSizeElements` items.
//   - Spill propagation strictly moves rightward.
//   - The total number of free slots across all chunks remains constant.
func SegmentedFixedOrderedListInsert[T any](segmentedList *SegmentedFixedOrderedList[T], idx uint64, value T) error {
	chunkIdx := idx >> chunkBitShiftAmount

	chunkAmount := ArrayCapacityGet(segmentedList.chunkHeaders)
	if chunkIdx >= chunkAmount {
		return fmt.Errorf("cannot insert: idx %v out of bounds", idx)
	}

	chunk := segmentedList.chunks[chunkIdx]

	chunkLocalIdx := idx & chunkBitAndAmount
	chunkHeader := ArrayItemPtrGetAtUnsafe(segmentedList.chunkHeaders, chunkIdx)

	if chunkHeader.remainingSlots > 0 {
		FixedOrderedListInsertAtUnsafe(chunk, chunkLocalIdx, value)
		chunkHeader.remainingSlots--
		// Unsafe because the invariants guarantee that the chunkLocalIdx is between 0 and chunk capacity.
		return nil
	} else {
		incomingElement := FixedOrderedListItemGetAtUnsafe(chunk, chunkSizeElements-1)
		FixedOrderedListDeleteUnsafe(chunk, chunkSizeElements-1)
		FixedOrderedListInsertAtUnsafe(chunk, chunkLocalIdx, value)

		return segmentedFixedOrderedListHandleCascade(segmentedList, chunkIdx, chunkHeader, chunkAmount, incomingElement)
	}
}

// SegmentedFixedOrderedListInsertUnsafe inserts a value into the segmented fixed list
// at the given global index without performing any bounds or capacity checks.
//
// The caller must ensure that:
//   - idx is within the global capacity of the list,
//   - there exists at least one available slot to the right (no total overflow),
//   - and the chunk metadata (remainingSlots) is consistent.
//
// Behavior:
//   - The global index is decomposed into a chunk index and local offset via bit shifts.
//   - If the target chunk has free space, insertion is performed directly.
//   - If the chunk is full, the last element is evicted and recursively cascaded to the next chunk.
//
// Complexity:
//   - O(chunk_size) in typical case.
//   - O(k * chunk_size) in rare cascading cases where k chunks are full.
//
// ⚠️ Undefined behaviour occurs if invariants are violated.
func SegmentedFixedOrderedListInsertUnsafe[T any](segmentedList *SegmentedFixedOrderedList[T], idx uint64, value T) {
	// Derive chunk and local index
	chunkIdx := idx >> chunkBitShiftAmount
	chunkLocalIdx := idx & chunkBitAndAmount

	chunk := segmentedList.chunks[chunkIdx]
	chunkHeader := ArrayItemPtrGetAtUnsafe(segmentedList.chunkHeaders, chunkIdx)

	// Fast path: local space available
	if chunkHeader.remainingSlots > 0 {
		FixedOrderedListInsertAtUnsafe(chunk, chunkLocalIdx, value)
		chunkHeader.remainingSlots--
		return
	}

	// Slow path: cascade rightward
	incoming := FixedOrderedListItemGetAtUnsafe(chunk, chunkSizeElements-1)
	FixedOrderedListDeleteUnsafe(chunk, chunkSizeElements-1)
	FixedOrderedListInsertAtUnsafe(chunk, chunkLocalIdx, value)

	segmentedFixedOrderedListHandleCascadeUnsafe(
		segmentedList,
		chunkIdx,
		chunkHeader,
		incoming,
	)
}

// SegmentedFixedOrderedListDelete deletes the element at the given global index.
//
// Conceptually, this function determines which fixed-size chunk corresponds to the given
// global index (via bit shifting), removes that element, and then cascades elements leftward
// to fill the resulting gap if necessary.
//
// If the deletion leaves a chunk partially empty, the first element from the next chunk
// (to the right) is pulled leftward to maintain contiguous global order.
// This process may recursively continue until all affected chunks remain balanced.
//
// This ensures global sequential order and local compactness.
//
// Time complexity:
//   - O(chunk_size) in the common case (local shift only)
//   - O(k * chunk_size) in rare worst case where k chunks cascade leftward
//
// Errors:
//   - Returns an error if idx is out of range.
//   - Returns an error if the list is already empty.
//
// Invariants:
//   - Spill propagation strictly moves leftward.
//   - Each chunk maintains continuous order.
//   - Total number of elements remains consistent across the structure.
func SegmentedFixedOrderedListDelete[T any](segmentedList *SegmentedFixedOrderedList[T], idx uint64) error {
	chunkAmount := ArrayCapacityGet(segmentedList.chunkHeaders)
	if chunkAmount == 0 {
		return fmt.Errorf("cannot delete from empty segmented list")
	}

	chunkIdx := idx >> chunkBitShiftAmount
	if chunkIdx >= chunkAmount {
		return fmt.Errorf("delete: global index %v out of bounds", idx)
	}

	chunkLocalIdx := idx & chunkBitAndAmount
	chunk := segmentedList.chunks[chunkIdx]
	chunkHeader := ArrayItemPtrGetAtUnsafe(segmentedList.chunkHeaders, chunkIdx)

	// Safety: check that this chunk actually contains an element at that index
	if chunkHeader.remainingSlots >= uint8(chunkSizeElements) {
		return fmt.Errorf("delete: chunk %v is already empty", chunkIdx)
	}
	if chunkLocalIdx >= chunkSizeElements-uint64(chunkHeader.remainingSlots) {
		return fmt.Errorf("delete: index %v exceeds logical length of chunk %v", chunkLocalIdx, chunkIdx)
	}

	// Perform local deletion
	FixedOrderedListDeleteUnsafe(chunk, chunkLocalIdx)
	chunkHeader.remainingSlots++

	// Cascade left if needed (fill the hole)
	if chunkIdx+1 < chunkAmount {
		if err := segmentedFixedOrderedListHandleLeftCascade(
			segmentedList, chunkIdx, chunkHeader, chunkAmount,
		); err != nil {
			return err
		}
	}

	return nil
}

// SegmentedFixedOrderedListDeleteUnsafe deletes the element at the given global index
// without performing any bounds or validity checks.
//
// The caller must ensure that:
//   - idx < total length (no underflow),
//   - at least one element exists at idx,
//   - and chunk headers are consistent.
//
// Behavior:
//   - The global index is decomposed into a chunk index and local offset.
//   - The element is removed from the corresponding chunk.
//   - If the chunk becomes underfull, the first element from the next chunk is pulled left.
//   - The cascade continues recursively until the rightmost affected chunk becomes balanced.
//
// Complexity:
//   - O(chunk_size) in the common case (local shift)
//   - O(k * chunk_size) in rare cascades where multiple right chunks are emptied.
//
// ⚠️ Undefined behaviour occurs if invariants are violated.
func SegmentedFixedOrderedListDeleteUnsafe[T any](segmentedList *SegmentedFixedOrderedList[T], idx uint64) {
	chunkIdx := idx >> chunkBitShiftAmount
	chunkLocalIdx := idx & chunkBitAndAmount

	chunk := segmentedList.chunks[chunkIdx]
	chunkHeader := ArrayItemPtrGetAtUnsafe(segmentedList.chunkHeaders, chunkIdx)

	FixedOrderedListDeleteUnsafe(chunk, chunkLocalIdx)
	chunkHeader.remainingSlots++

	// If this chunk is now partially empty, we can optionally balance
	segmentedFixedOrderedListHandleLeftCascadeUnsafe(segmentedList, chunkIdx, chunkHeader)
}

func SegmentedFixedOrderedListSetAt[T any](seg *SegmentedFixedOrderedList[T], idx uint64, value T) error {
	chunkIdx := idx >> chunkBitShiftAmount
	if chunkIdx >= ArrayCapacityGet(seg.chunkHeaders) {
		return fmt.Errorf("set: idx %v out of bounds", idx)
	}
	localIdx := idx & chunkBitAndAmount
	return FixedOrderedListSetAt(seg.chunks[chunkIdx], localIdx, value)
}

func SegmentedFixedOrderedListSetAtUnsafe[T any](seg *SegmentedFixedOrderedList[T], idx uint64, value T) {
	chunkIdx := idx >> chunkBitShiftAmount
	localIdx := idx & chunkBitAndAmount
	FixedOrderedListSetAtUnsafe(seg.chunks[chunkIdx], localIdx, value)
}

func SegmentedFixedOrderedListLengthGet[T any](seg *SegmentedFixedOrderedList[T]) uint64 {
	var total uint64
	chunks := ArrayCapacityGet(seg.chunkHeaders)
	for i := uint64(0); i < chunks; i++ {
		total += chunkSizeElements - uint64(ArrayItemGetAtUnsafe(seg.chunkHeaders, i).remainingSlots)
	}
	return total
}

func SegmentedFixedOrderedListCapacityGet[T any](seg *SegmentedFixedOrderedList[T]) uint64 {
	return seg.globalCapacityElements
}

// SegmentedFixedOrderedListItemGetAt returns the value at a given global index.
func SegmentedFixedOrderedListItemGetAt[T any](seg *SegmentedFixedOrderedList[T], idx uint64) (T, error) {
	chunkIdx := idx >> chunkBitShiftAmount
	if chunkIdx >= ArrayCapacityGet(seg.chunkHeaders) {
		var zero T
		return zero, fmt.Errorf("get: idx %v out of bounds", idx)
	}
	localIdx := idx & chunkBitAndAmount
	chunk := seg.chunks[chunkIdx]
	return FixedOrderedListItemGetAt(chunk, localIdx)
}

// SegmentedFixedOrderedListItemGetAtUnsafe returns the value at a given global index (no checks).
func SegmentedFixedOrderedListItemGetAtUnsafe[T any](seg *SegmentedFixedOrderedList[T], idx uint64) T {
	chunkIdx := idx >> chunkBitShiftAmount
	localIdx := idx & chunkBitAndAmount
	return FixedOrderedListItemGetAtUnsafe(seg.chunks[chunkIdx], localIdx)
}

// SegmentedFixedOrderedListItemPtrGetAt returns a pointer to the value at a given global index.
func SegmentedFixedOrderedListItemPtrGetAt[T any](seg *SegmentedFixedOrderedList[T], idx uint64) (*T, error) {
	chunkIdx := idx >> chunkBitShiftAmount
	if chunkIdx >= ArrayCapacityGet(seg.chunkHeaders) {
		return nil, fmt.Errorf("ptr-get: idx %v out of bounds", idx)
	}
	localIdx := idx & chunkBitAndAmount
	chunk := seg.chunks[chunkIdx]
	return FixedOrderedListItemPtrGetAt(chunk, localIdx)
}

// SegmentedFixedOrderedListItemPtrGetAtUnsafe returns a pointer to the value at a given global index (no checks).
func SegmentedFixedOrderedListItemPtrGetAtUnsafe[T any](seg *SegmentedFixedOrderedList[T], idx uint64) *T {
	chunkIdx := idx >> chunkBitShiftAmount
	localIdx := idx & chunkBitAndAmount
	return FixedOrderedListItemPtrGetAtUnsafe(seg.chunks[chunkIdx], localIdx)
}

// SegmentedFixedOrderedListBinarySearch performs a binary search O(log n)
// to find a value matching the predicate across all chunks.
//
// Semantics match FixedOrderedListBinarySearch exactly:
//   - predicate(item) < 0 → item is below predicate (go right)
//   - predicate(item) > 0 → item is above predicate (go left)
//   - predicate(item) == 0 → match
//
//go:nosplit
//go:inline
func SegmentedFixedOrderedListBinarySearch[T any](seg *SegmentedFixedOrderedList[T], predicate func(item T) int8) (uint64, error) {
	chunkAmount := ArrayCapacityGet(seg.chunkHeaders)
	if chunkAmount == 0 {
		return 0, fmt.Errorf("empty segmented list")
	}

	loChunk := uint64(0)
	hiChunk := chunkAmount

	for loChunk < hiChunk {
		midChunk := (loChunk + hiChunk) >> 1
		chunk := seg.chunks[midChunk]
		chunkLen := FixedOrderedListLengthGet(chunk)
		if chunkLen == 0 {
			return 0, fmt.Errorf("empty chunk encountered")
		}

		firstItem, _ := FixedOrderedListItemGetAt(chunk, 0)
		lastItem, _ := FixedOrderedListItemGetAt(chunk, chunkLen-1)

		firstCmp := predicate(firstItem)
		lastCmp := predicate(lastItem)

		if lastCmp < 0 {
			loChunk = midChunk + 1
		} else if firstCmp > 0 {
			hiChunk = midChunk
		} else {
			localIdx, err := FixedOrderedListBinarySearch(chunk, predicate)
			if err != nil {
				return 0, err
			}
			return midChunk*chunkSizeElements + localIdx, nil
		}
	}

	return 0, fmt.Errorf("predicate cannot be found in segmented list")
}

// SegmentedFixedOrderedListBinarySearchInterval uses a binary search O(log n)
// to find the position where predicate can be inserted to keep global order.
//
// It returns the global indices of the previous and next items
// between which the new item should be inserted.
//
// The predicate function must return:
//   - negative when item is below predicate
//   - 0 or positive when item is above predicate
//
//go:nosplit
//go:inline
func SegmentedFixedOrderedListBinarySearchInterval[T any](seg *SegmentedFixedOrderedList[T], predicate func(item T) int8) (uint64, uint64) {
	chunkAmount := ArrayCapacityGet(seg.chunkHeaders)
	if chunkAmount == 0 {
		return ^uint64(0), 0
	}

	loChunk := uint64(0)
	hiChunk := chunkAmount

	for loChunk < hiChunk {
		midChunk := (loChunk + hiChunk) >> 1
		chunk := seg.chunks[midChunk]
		chunkLen := FixedOrderedListLengthGet(chunk)

		if chunkLen == 0 {
			hiChunk = midChunk
			continue
		}

		firstItem, _ := FixedOrderedListItemGetAt(chunk, 0)
		lastItem, _ := FixedOrderedListItemGetAt(chunk, chunkLen-1)

		firstCmp := predicate(firstItem)
		lastCmp := predicate(lastItem)

		if lastCmp < 0 {
			loChunk = midChunk + 1
			continue
		}
		if firstCmp > 0 {
			hiChunk = midChunk
			continue
		}

		// target lies within this chunk’s range → defer to intra-chunk interval
		prev, next := FixedOrderedListBinarySearchInterval(chunk, predicate)
		if prev == ^uint64(0) {
			if midChunk == 0 {
				return ^uint64(0), next
			}
			prevLen := FixedOrderedListLengthGet(seg.chunks[midChunk-1])
			return (midChunk-1)*chunkSizeElements + (prevLen - 1),
				midChunk*chunkSizeElements + next
		}
		return midChunk*chunkSizeElements + prev, midChunk*chunkSizeElements + next
	}

	switch loChunk {
	case 0:
		return ^uint64(0), 0
	case chunkAmount:
		lastChunk := seg.chunks[chunkAmount-1]
		lastLen := FixedOrderedListLengthGet(lastChunk)
		return chunkAmount*chunkSizeElements - (chunkSizeElements - lastLen) - 1,
			chunkAmount * chunkSizeElements
	default:
		prevChunk := seg.chunks[loChunk-1]
		prevLen := FixedOrderedListLengthGet(prevChunk)
		return (loChunk-1)*chunkSizeElements + (prevLen - 1),
			loChunk * chunkSizeElements
	}
}

// SegmentedFixedOrderedListBinarySearchInsertionPoint is a convenience wrapper around
// SegmentedFixedOrderedListBinarySearchInterval, returning only the new global insertion point.
//
// The predicate function must return:
//   - negative when item is below predicate
//   - 0 or positive when the item is above predicate
//
//go:nosplit
func SegmentedFixedOrderedListBinarySearchInsertionPoint[T any](seg *SegmentedFixedOrderedList[T], predicate func(item T) int8) uint64 {
	chunkAmount := ArrayCapacityGet(seg.chunkHeaders)
	if chunkAmount == 0 {
		return 0
	}
	_, nextIdx := SegmentedFixedOrderedListBinarySearchInterval(seg, predicate)
	if nextIdx == ^uint64(0) {
		return 0
	}
	return nextIdx
}

// -------------------------------------------------- Private helpers

//go:inline
func segmentedFixedOrderedListHandleCascade[T any](
	segmentedList *SegmentedFixedOrderedList[T],
	targetChunkIdx uint64, targetChunkHeader *chunkHeader,
	chunkAmount uint64, incomingElement T,
) error {
	nextChunkIdx := targetChunkIdx + 1
	if nextChunkIdx >= chunkAmount {
		return fmt.Errorf("cannot insert, no space to the right of insertion")
	}

	nextChunk := segmentedList.chunks[nextChunkIdx]
	nextChunkHeader := ArrayItemPtrGetAtUnsafe(segmentedList.chunkHeaders, nextChunkIdx)

	// Case 1: next chunk has space → insert and stop cascade
	if nextChunkHeader.remainingSlots > 0 {
		FixedOrderedListInsertAtUnsafe(nextChunk, 0, incomingElement)
		nextChunkHeader.remainingSlots--
		targetChunkHeader.remainingSlots++
		return nil
	}

	// Case 2: next chunk full → spill further right
	spill := FixedOrderedListItemGetAtUnsafe(nextChunk, chunkSizeElements-1)
	FixedOrderedListDeleteUnsafe(nextChunk, chunkSizeElements-1)
	FixedOrderedListInsertAtUnsafe(nextChunk, 0, incomingElement)

	targetChunkHeader.remainingSlots++

	return segmentedFixedOrderedListHandleCascade(
		segmentedList,
		nextChunkIdx,
		nextChunkHeader,
		chunkAmount,
		spill,
	)
}

//go:inline
func segmentedFixedOrderedListHandleCascadeUnsafe[T any](
	segmentedList *SegmentedFixedOrderedList[T],
	targetChunkIdx uint64,
	targetChunkHeader *chunkHeader,
	incomingElement T,
) {
	nextIdx := targetChunkIdx + 1
	nextChunk := segmentedList.chunks[nextIdx]
	nextHeader := ArrayItemPtrGetAtUnsafe(segmentedList.chunkHeaders, nextIdx)

	// Case 1: next chunk has space
	if nextHeader.remainingSlots > 0 {
		FixedOrderedListInsertAtUnsafe(nextChunk, 0, incomingElement)
		nextHeader.remainingSlots--
		targetChunkHeader.remainingSlots++
		return
	}

	// Case 2: next chunk full → recursive spill
	spill := FixedOrderedListItemGetAtUnsafe(nextChunk, chunkSizeElements-1)
	FixedOrderedListDeleteUnsafe(nextChunk, chunkSizeElements-1)
	FixedOrderedListInsertAtUnsafe(nextChunk, 0, incomingElement)

	targetChunkHeader.remainingSlots++

	segmentedFixedOrderedListHandleCascadeUnsafe(
		segmentedList,
		nextIdx,
		nextHeader,
		spill,
	)
}

//go:inline
func segmentedFixedOrderedListHandleLeftCascade[T any](
	segmentedList *SegmentedFixedOrderedList[T],
	targetChunkIdx uint64,
	targetChunkHeader *chunkHeader,
	chunkAmount uint64,
) error {
	nextChunkIdx := targetChunkIdx + 1
	if nextChunkIdx >= chunkAmount {
		return nil
	}

	nextChunk := segmentedList.chunks[nextChunkIdx]
	nextHeader := ArrayItemPtrGetAtUnsafe(segmentedList.chunkHeaders, nextChunkIdx)

	if nextHeader.remainingSlots < uint8(chunkSizeElements) {
		elem := FixedOrderedListItemGetAtUnsafe(nextChunk, 0)
		FixedOrderedListDeleteUnsafe(nextChunk, 0)
		FixedOrderedListInsertAtUnsafe(
			segmentedList.chunks[targetChunkIdx],
			FixedOrderedListLengthGet(segmentedList.chunks[targetChunkIdx]),
			elem,
		)

		targetChunkHeader.remainingSlots--
		nextHeader.remainingSlots++

		if nextHeader.remainingSlots == uint8(chunkSizeElements) && nextChunkIdx+1 < chunkAmount {
			return segmentedFixedOrderedListHandleLeftCascade(
				segmentedList, nextChunkIdx, nextHeader, chunkAmount,
			)
		}
	}

	return nil
}

//go:inline
func segmentedFixedOrderedListHandleLeftCascadeUnsafe[T any](
	segmentedList *SegmentedFixedOrderedList[T],
	targetChunkIdx uint64,
	targetChunkHeader *chunkHeader,
) {
	nextChunkIdx := targetChunkIdx + 1
	if nextChunkIdx >= ArrayCapacityGet(segmentedList.chunkHeaders) {
		return
	}

	nextChunk := segmentedList.chunks[nextChunkIdx]
	nextHeader := ArrayItemPtrGetAtUnsafe(segmentedList.chunkHeaders, nextChunkIdx)

	if nextHeader.remainingSlots < uint8(chunkSizeElements) {
		elem := FixedOrderedListItemGetAtUnsafe(nextChunk, 0)
		FixedOrderedListDeleteUnsafe(nextChunk, 0)
		FixedOrderedListInsertAtUnsafe(segmentedList.chunks[targetChunkIdx],
			FixedOrderedListLengthGet(segmentedList.chunks[targetChunkIdx]), elem)

		targetChunkHeader.remainingSlots--
		nextHeader.remainingSlots++

		if nextHeader.remainingSlots == uint8(chunkSizeElements) {
			segmentedFixedOrderedListHandleLeftCascadeUnsafe(segmentedList, nextChunkIdx, nextHeader)
		}
	}
}

//go:inline
func segmentedFixedOrderedListChunkBytes[T any]() uint64 {
	return FixedOrderedListRequiredBytes[T](chunkSizeElements)
}

//go:inline
func segmentedFixedOrderedListChunkHeaderArrayBytes(chunkAmount uint64) uint64 {
	chunkHeadersSize := alignIdxUp(
		chunkAmount*memcore.SizeOf[chunkHeader](),
		memcore.AlignOf[chunkHeader](),
	)

	return chunkHeadersSize
}

package primitives

import (
	"fmt"
	commontesting "foundation/testing"
	"testing"
	"unsafe"
)

// ------------------------------------------------------------
// SegmentedFixedOrderedList Test Suite
// ------------------------------------------------------------

func TestSegmentedFixedOrderedList(t *testing.T) {
	t.Run("create_and_clear", testSegmentedCreateAndClear)
	t.Run("append_and_get", testSegmentedAppendAndGet)
	t.Run("insert_with_cascade", testSegmentedInsertWithCascade)
	t.Run("delete_with_left_cascade", testSegmentedDeleteWithCascade)
	t.Run("binary_search_across_chunks", testSegmentedBinarySearch)
	t.Run("binary_search_interval", testSegmentedBinarySearchInterval)
	t.Run("clear_and_zero_resets", testSegmentedClearAndZero)
	t.Run("capacity_and_length_consistency", testSegmentedCapacityLength)
}

// ------------------------------------------------------------
// Individual Test Cases
// ------------------------------------------------------------

func testSegmentedCreateAndClear(t *testing.T) {
	const totalElements = 128
	requiredBytes := SegmentedFixedOrderedListCapacityBytesGet[int](totalElements)
	mem := make([]byte, requiredBytes)
	list := SegmentedFixedOrderedListCreateAt[int](unsafe.Pointer(&mem[0]), totalElements)

	commontesting.Assert(list != nil, "list is nil after creation", "list created", t)
	commontesting.Assert(SegmentedFixedOrderedListCapacityGet(list) == totalElements, "capacity mismatch", "capacity correct", t)

	SegmentedFixedOrderedListClear(list)
	commontesting.Assert(SegmentedFixedOrderedListLengthGet(list) == 0, "length not zero after clear", "cleared successfully", t)
}

func testSegmentedAppendAndGet(t *testing.T) {
	const totalElements = 128
	requiredBytes := SegmentedFixedOrderedListCapacityBytesGet[int](totalElements)
	mem := make([]byte, requiredBytes)
	list := SegmentedFixedOrderedListCreateAt[int](unsafe.Pointer(&mem[0]), totalElements)

	for i := 0; i < 100; i++ {
		err := SegmentedFixedOrderedListAppend(list, i)
		commontesting.Assert(err == nil, "append failed", "append succeeded", t)
	}

	for i := 0; i < 100; i++ {
		val, err := SegmentedFixedOrderedListItemGetAt(list, uint64(i))
		commontesting.Assert(err == nil && val == i, "get mismatch", "values retrieved correctly", t)
	}

	length := SegmentedFixedOrderedListLengthGet(list)
	commontesting.Assert(length == 100, "length mismatch after appends", "length correct", t)
}

func testSegmentedInsertWithCascade(t *testing.T) {
	const totalElements = 128
	requiredBytes := SegmentedFixedOrderedListCapacityBytesGet[int](totalElements)
	mem := make([]byte, requiredBytes)
	list := SegmentedFixedOrderedListCreateAt[int](unsafe.Pointer(&mem[0]), totalElements)

	// Fill to just before full so cascade has space to propagate
	fillCount := int(chunkSizeElements*2) - 1
	for i := 0; i < fillCount; i++ {
		err := SegmentedFixedOrderedListAppend(list, i)
		commontesting.Assert(err == nil, "append failed", "append succeeded", t)
	}

	// Insert in middle of first chunk → should cascade right into second
	err := SegmentedFixedOrderedListInsert(list, 32, 999)
	commontesting.Assert(err == nil, "insert cascade failed", "cascade succeeded", t)

	// Verify inserted value
	val, _ := SegmentedFixedOrderedListItemGetAt(list, 32)
	commontesting.Assert(val == 999, "inserted value not found", "value inserted correctly", t)

	// Verify rightward cascade preserved order
	nextVal, _ := SegmentedFixedOrderedListItemGetAt(list, 33)
	commontesting.Assert(nextVal == 32, "order disrupted after cascade", "order preserved", t)
}

func testSegmentedDeleteWithCascade(t *testing.T) {
	const totalElements = 128
	requiredBytes := SegmentedFixedOrderedListCapacityBytesGet[int](totalElements)
	mem := make([]byte, requiredBytes)
	list := SegmentedFixedOrderedListCreateAt[int](unsafe.Pointer(&mem[0]), totalElements)

	for i := 0; i < int(chunkSizeElements*2); i++ {
		_ = SegmentedFixedOrderedListAppend(list, i)
	}

	// Delete near chunk boundary → should trigger left cascade
	err := SegmentedFixedOrderedListDelete(list, 63)
	commontesting.Assert(err == nil, "delete with cascade failed", "delete cascade succeeded", t)

	val, _ := SegmentedFixedOrderedListItemGetAt(list, 63)
	commontesting.Assert(val == 64, "cascade did not pull left element correctly", "cascade fill correct", t)
}

func testSegmentedBinarySearch(t *testing.T) {
	const totalElements = 128
	requiredBytes := SegmentedFixedOrderedListCapacityBytesGet[int](totalElements)
	mem := make([]byte, requiredBytes)
	list := SegmentedFixedOrderedListCreateAt[int](unsafe.Pointer(&mem[0]), totalElements)

	for i := 0; i < 128; i++ {
		_ = SegmentedFixedOrderedListAppend(list, i)
	}

	idx, err := SegmentedFixedOrderedListBinarySearch(list, func(v int) int8 {
		switch {
		case v < 42:
			return -1
		case v > 42:
			return 1
		default:
			return 0
		}
	})
	commontesting.Assert(err == nil && idx == 42, "binary search failed", "binary search correct", t)
}

func testSegmentedBinarySearchInterval(t *testing.T) {
	const totalElements = 128
	requiredBytes := SegmentedFixedOrderedListCapacityBytesGet[int](totalElements)
	mem := make([]byte, requiredBytes)
	list := SegmentedFixedOrderedListCreateAt[int](unsafe.Pointer(&mem[0]), totalElements)

	// Even numbers: 0, 2, 4, ..., 126
	for i := 0; i < 64; i++ {
		_ = SegmentedFixedOrderedListAppend(list, i*2)
	}

	// Strict predicate:  -1 when below target, +1 when above, 0 when equal
	prev, next := SegmentedFixedOrderedListBinarySearchInterval(list, func(v int) int8 {
		switch {
		case v < 31:
			return -1
		case v > 31:
			return 1
		default:
			return 0
		}
	})

	// 31 should fit between 30 (idx=15) and 32 (idx=16)
	msg := fmt.Sprintf("interval incorrect, got: prev=%v, next=%v, expected prev=15,next=16", prev, next)
	commontesting.Assert(prev == 15 && next == 16, msg, "interval correct", t)
}

func testSegmentedClearAndZero(t *testing.T) {
	const totalElements = 128
	requiredBytes := SegmentedFixedOrderedListCapacityBytesGet[int](totalElements)
	mem := make([]byte, requiredBytes)
	list := SegmentedFixedOrderedListCreateAt[int](unsafe.Pointer(&mem[0]), totalElements)

	for i := 0; i < 64; i++ {
		_ = SegmentedFixedOrderedListAppend(list, i)
	}
	SegmentedFixedOrderedListClearAndZero(list)

	length := SegmentedFixedOrderedListLengthGet(list)
	commontesting.Assert(length == 0, "length not reset after clear and zero", "cleared and zeroed successfully", t)
}

func testSegmentedCapacityLength(t *testing.T) {
	const totalElements = 256
	requiredBytes := SegmentedFixedOrderedListCapacityBytesGet[int](totalElements)
	mem := make([]byte, requiredBytes)
	list := SegmentedFixedOrderedListCreateAt[int](unsafe.Pointer(&mem[0]), totalElements)

	for i := 0; i < 128; i++ {
		_ = SegmentedFixedOrderedListAppend(list, i)
	}

	capacity := SegmentedFixedOrderedListCapacityGet(list)
	length := SegmentedFixedOrderedListLengthGet(list)
	commontesting.Assert(capacity == totalElements, "capacity mismatch", "capacity consistent", t)
	commontesting.Assert(length == 128, "length mismatch", "length consistent", t)
}

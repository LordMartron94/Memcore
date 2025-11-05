package memcore

import (
	"fmt"
	"unsafe"
)

var (
	functionIDCounter uint32 = 0

	regionRegistry []memoryRegion = make([]memoryRegion, 0)
	regionFreeList []uint32       = make([]uint32, 0)

	functionRegistry map[uint32]interface{} = make(map[uint32]interface{})
)

// MemcoreMarkManagementStateReset resets the state to preserve memory.
func MemcoreMarkManagementStateReset(resetFunctions bool) {
	if resetFunctions {
		functionIDCounter = 0
		functionRegistry = make(map[uint32]interface{})
	}

	regionRegistry = make([]memoryRegion, 0)
	regionFreeList = make([]uint32, 0)
}

type FunctionID = uint32

// memoryRegion provides information necessary to interact with memory regions.
type memoryRegion struct {
	base      uintptr
	sizeBytes uint64
	active    bool
}

// MarkRaw provides the basic information necessary to interact with raw memory.
// It is the fastest, but also the most unsafe.
type MarkRaw struct {
	regionID uint32
	offset   uintptr
}

// ---------------------------------------- REGIONS

// MemcoreRegionRegister registers a memory region and returns the ID.
//
//go:nosplit
//go:inline
func MemcoreRegionRegister(baseAddr uintptr, sizeBytes uint64) uint32 {
	var id uint32
	if len(regionFreeList) > 0 {
		id = regionFreeList[len(regionFreeList)-1]
		regionFreeList = regionFreeList[:len(regionFreeList)-1]
		regionRegistry[id] = memoryRegion{baseAddr, sizeBytes, true}
	} else {
		id = uint32(len(regionRegistry))
		regionRegistry = append(regionRegistry, memoryRegion{baseAddr, sizeBytes, true})
	}
	return id
}

// MemcoreRegionUnregister removes a memory region.
//
//go:nosplit
//go:inline
func MemcoreRegionUnregister(regionID uint32) {
	r := &regionRegistry[regionID]
	r.base = 0
	r.sizeBytes = 0
	r.active = false
	regionFreeList = append(regionFreeList, regionID)
}

// MemcoreRegionBaseUpdate updates the base address of a region.
//
//go:nosplit
//go:inline
func MemcoreRegionBaseUpdate(regionID uint32, newBase uintptr) {
	regionRegistry[regionID].base = newBase
}

// ---------------------------------------- MARKS

// MemcoreMarkCreate creates a marker to memory.
//
//go:nosplit
//go:inline
func MemcoreMarkCreate(regionID uint32, offset uintptr) MarkRaw {
	mark := MarkRaw{
		regionID: regionID,
		offset:   offset,
	}
	return mark
}

// MemcoreMarkOffsetFrom creates a new mark relative to another mark.
//
//go:nosplit
//go:inline
func MemcoreMarkOffsetFrom(base MarkRaw, offset uintptr) MarkRaw {
	return MarkRaw{
		regionID: base.regionID,
		offset:   base.offset + offset,
	}
}

// MemcoreMarkSubtractBaseOffset removes a base offset from the mark's offset.
// This is useful to compute an internal offset relative to a base.
//
//go:nosplit
//go:inline
func MemcoreMarkSubtractBaseOffset(mark MarkRaw, baseOffset uintptr) uintptr {
	return mark.offset - baseOffset
}

// MemcoreMarkAlignedOffsetFrom creates a new mark relative to another mark,
// rounding the resulting offset up to the specified alignment.
//
// This is useful when placing objects or structures that require specific
// alignment boundaries (e.g. 8, 16, or 64 bytes) within the same memory region.
//
// The alignment must be a power of two.
//
// Example:
//
//	base := MemcoreMarkCreate(regionID, 0)
//	aligned := MemcoreMarkAlignedOffsetFrom(base, 13, 8)
//	// aligned.offset == 16
//
//go:nosplit
//go:inline
func MemcoreMarkAlignedOffsetFrom(base MarkRaw, offset uintptr, alignment uint64) MarkRaw {
	return MarkRaw{
		regionID: base.regionID,
		offset:   uintptr(AlignUp(uint64(base.offset+offset), alignment)),
	}
}

// MemcoreMarkDereference returns a pointer to the memory marked.
//
//go:nosplit
//go:inline
func MemcoreMarkDereference(mark MarkRaw) unsafe.Pointer {
	r := regionRegistry[mark.regionID]
	if !r.active {
		panic("memcore: invalid region ID dereference")
	}
	return unsafe.Pointer(r.base + mark.offset)
}

// MemcoreMarkDereferenceObject returns the memory marked interpretedd as object T.
//
//go:nosplit
//go:inline
func MemcoreMarkDereferenceObject[T any](mark MarkRaw) *T {
	addr := MemcoreMarkDereference(mark)
	return (*T)(addr)
}

// MemcoreMarkIsValid reports whether the mark references a valid region.
//
//go:nosplit
//go:inline
func MemcoreMarkIsValid(m MarkRaw) bool {
	return int(m.regionID) < len(regionRegistry) && regionRegistry[m.regionID].active
}

// MemcoreMarkOffsetIs checks whether an offset is the same as the target.
//
//go:nosplit
//go:inline
func MemcoreMarkOffsetIs(m MarkRaw, offset uintptr) bool {
	return m.offset == offset
}

// MemcoreMarkBelongsToRegion returns true if both marks refer to the same region.
//
//go:nosplit
//go:inline
func MemcoreMarkBelongsToRegion(m MarkRaw, other MarkRaw) bool {
	return m.regionID == other.regionID
}

// ---------------------------------------- FUNCTIONS

// MemcoreFunctionRegister registers a function to the registry and returns its ID.
//
//go:nosplit
//go:inline
func MemcoreFunctionRegister(function interface{}) FunctionID {
	id := functionIDCounter

	functionRegistry[id] = function

	functionIDCounter++
	return id
}

// MemcoreFunctionRegisterTyped registers a function T to the registry and returns its ID.
//
//go:nosplit
//go:inline
func MemcoreFunctionRegisterTyped[T any](function T) FunctionID {
	id := functionIDCounter

	functionRegistry[id] = function

	functionIDCounter++
	return id
}

// MemcoreFunctionRetrieve retrieves a function from the registry by its ID.
//
//go:nosplit
//go:inline
func MemcoreFunctionRetrieve(functionID uint32) interface{} {
	return functionRegistry[functionID]
}

// MemcoreFunctionRetrieveTyped retrieves a function from the registry interpreted as T.
//
//go:nosplit
//go:inline
func MemcoreFunctionRetrieveTyped[T any](id uint32) T {
	fn := MemcoreFunctionRetrieve(id)
	v, ok := fn.(T)
	if !ok {
		panic(fmt.Errorf("function ID %d: type mismatch: stored %T, requested %T", id, fn, *new(T)))
	}
	return v
}

package memcore

import (
	"fmt"
	"reflect"
	"unsafe"
)

var (
	// Initialize with one empty/inactive region so the first real region gets ID 1
	regionRegistry []memoryRegion = []memoryRegion{{base: 0, sizeBytes: 0, active: false}}
	regionFreeList []uint32       = make([]uint32, 0)
	regionBases    []uintptr      = []uintptr{0}

	functionRegistry  []interface{} = make([]interface{}, 0)
	functionFreeList  []uint32      = make([]uint32, 0)
	functionIDCounter uint32        = 0

	objectRegistry  []MarkRaw = make([]MarkRaw, 0)
	objectFreeList  []uint32  = make([]uint32, 0)
	objectActive    []bool    = make([]bool, 0)
	objectIDCounter uint32    = 0
)

// MemcoreMarkManagementStateReset resets the state to preserve memory.
func MemcoreMarkManagementStateReset(resetFunctions bool) {
	if resetFunctions {
		MemcoreFunctionRegistryClear()
	}

	regionRegistry = []memoryRegion{{base: 0, sizeBytes: 0, active: false}}
	regionFreeList = make([]uint32, 0)
	regionBases = []uintptr{0}

	MemcoreObjectRegistryClear()
}

type FunctionID = uint32
type ObjectID = uint32

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
		regionBases[id] = baseAddr
	} else {
		id = uint32(len(regionRegistry))
		regionRegistry = append(regionRegistry, memoryRegion{baseAddr, sizeBytes, true})
		regionBases = append(regionBases, baseAddr)
	}
	return id
}

// MemcoreRegionUnregister removes a memory region.
//
//go:nosplit
//go:inline
func MemcoreRegionUnregister(regionID uint32) {
	if int(regionID) >= len(regionRegistry) {
		return
	}
	r := &regionRegistry[regionID]
	r.base = 0
	r.sizeBytes = 0
	r.active = false

	if int(regionID) < len(regionBases) {
		regionBases[regionID] = 0
	}

	regionFreeList = append(regionFreeList, regionID)
}

// MemcoreRegionBaseUpdate updates the base address of a region.
//
//go:nosplit
//go:inline
func MemcoreRegionBaseUpdate(regionID uint32, newBase uintptr) {
	regionRegistry[regionID].base = newBase
	regionBases[regionID] = newBase
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
// It also returns the padding that occured because of the alignment.
//
//go:nosplit
//go:inline
func MemcoreMarkAlignedOffsetFrom(base MarkRaw, offset uintptr, alignment uint64) (MarkRaw, uint64) {
	idx := AlignUp(uint64(base.offset+offset), alignment)
	padding := idx - (uint64(base.offset + offset))

	return MarkRaw{
		regionID: base.regionID,
		offset:   uintptr(idx),
	}, padding
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

// MemcoreMarkDereferenceUnsafe returns a pointer to the memory marked.
// This variant is pure pointer arithmetic and does not validate whether the
// region or mark is valid.
//
//go:nosplit
//go:inline
func MemcoreMarkDereferenceUnsafe(mark MarkRaw) unsafe.Pointer {
	return unsafe.Pointer(regionBases[mark.regionID] + mark.offset)
}

// MemcoreMarkDereferenceObject returns the memory marked interpreted as object T.
//
//go:nosplit
//go:inline
func MemcoreMarkDereferenceObject[T any](mark MarkRaw) *T {
	addr := MemcoreMarkDereference(mark)
	return (*T)(addr)
}

// MemcoreMarkDereferenceWithType returns a typed pointer via reflect.Type.
//
//go:nosplit
//go:inline
func MemcoreMarkDereferenceWithType(mark MarkRaw, t reflect.Type) unsafe.Pointer {
	addr := MemcoreMarkDereference(mark)
	val := reflect.NewAt(t, addr)
	return unsafe.Pointer(val.Pointer())
}

// MemcoreMarkDereferenceWithTypeUnsafe returns a typed pointer via reflect.Type.
// This variant is pure pointer arithmetic and does not validate whether the
// region or mark is valid.
//
//go:nosplit
//go:inline
func MemcoreMarkDereferenceWithTypeUnsafe(mark MarkRaw, t reflect.Type) unsafe.Pointer {
	addr := MemcoreMarkDereferenceUnsafe(mark)
	val := reflect.NewAt(t, addr)
	return unsafe.Pointer(val.Pointer())
}

// MemcoreMarkDereferenceObjectUnsafe returns the memory marked interpreted as object T.
// This variant is pure pointer arithmetic and does not validate whether the
// region or mark is valid.
//
//go:nosplit
//go:inline
func MemcoreMarkDereferenceObjectUnsafe[T any](mark MarkRaw) *T {
	addr := MemcoreMarkDereferenceUnsafe(mark)
	return (*T)(addr)
}

// MemcoreMarkDereferenceObjectAlt returns the memory marked interpreted as object T as well as the raw pointer.
//
//go:nosplit
//go:inline
func MemcoreMarkDereferenceObjectAlt[T any](mark MarkRaw) (unsafe.Pointer, *T) {
	addr := MemcoreMarkDereference(mark)
	return addr, (*T)(addr)
}

// MemcoreMarkDereferenceObjectAltUnsafe returns the memory marked interpreted as object T as well as the raw pointer.
// This variant is pure pointer arithmetic and does not validate whether the
// region or mark is valid.
//
//go:nosplit
//go:inline
func MemcoreMarkDereferenceObjectAltUnsafe[T any](mark MarkRaw) (unsafe.Pointer, *T) {
	addr := MemcoreMarkDereferenceUnsafe(mark)
	return addr, (*T)(addr)
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

// MemcoreFunctionRegister registers a function and returns its ID.
//
//go:nosplit
//go:inline
func MemcoreFunctionRegister(fn interface{}) FunctionID {
	var id uint32
	if n := len(functionFreeList); n > 0 {
		id = functionFreeList[n-1]
		functionFreeList = functionFreeList[:n-1]
		functionRegistry[id] = fn
	} else {
		id = uint32(len(functionRegistry))
		functionRegistry = append(functionRegistry, fn)
	}
	return id
}

// MemcoreFunctionRegisterTyped registers a strongly typed function and returns its ID.
//
//go:nosplit
//go:inline
func MemcoreFunctionRegisterTyped[T any](function T) FunctionID {
	return MemcoreFunctionRegister(function)
}

// MemcoreFunctionRebind updates an existing function entry.
//
//go:nosplit
//go:inline
func MemcoreFunctionRebind(id FunctionID, fn interface{}) {
	if int(id) >= len(functionRegistry) {
		panic("memcore: invalid FunctionID in rebind")
	}
	functionRegistry[id] = fn
}

// MemcoreFunctionUnregister removes a function entry.
//
//go:nosplit
//go:inline
func MemcoreFunctionUnregister(id FunctionID) {
	if int(id) >= len(functionRegistry) {
		return
	}
	functionRegistry[id] = nil
	functionFreeList = append(functionFreeList, id)
}

// MemcoreFunctionRetrieve retrieves a raw function by ID.
//
//go:nosplit
//go:inline
func MemcoreFunctionRetrieve(id FunctionID) interface{} {
	if int(id) >= len(functionRegistry) {
		return nil
	}
	return functionRegistry[id]
}

// MemcoreFunctionRetrieveTyped retrieves a function and casts it to type T.
//
//go:nosplit
//go:inline
func MemcoreFunctionRetrieveTyped[T any](id FunctionID) T {
	fn := functionRegistry[id]
	if fn == nil {
		panic(fmt.Errorf("function ID %d inactive", id))
	}
	v, ok := fn.(T)
	if !ok {
		panic(fmt.Errorf("function ID %d type mismatch", id))
	}
	return v
}

// MemcoreFunctionRegistryClear resets all registered functions.
func MemcoreFunctionRegistryClear() {
	functionRegistry = make([]interface{}, 0)
	functionFreeList = make([]uint32, 0)
	functionIDCounter = 0
}

// ---------------------------------------- OBJECTS

// MemcoreObjectRegister assigns a new ObjectID and links it to a MarkRaw.
//
//go:nosplit
//go:inline
func MemcoreObjectRegister(mark MarkRaw) ObjectID {
	var id uint32
	if n := len(objectFreeList); n > 0 {
		id = objectFreeList[n-1]
		objectFreeList = objectFreeList[:n-1]
		objectRegistry[id] = mark
		objectActive[id] = true
	} else {
		id = uint32(len(objectRegistry))
		objectRegistry = append(objectRegistry, mark)
		objectActive = append(objectActive, true)
	}
	objectIDCounter++
	return id
}

// MemcoreObjectRebind updates an existing ObjectID → MarkRaw mapping.
//
//go:nosplit
//go:inline
func MemcoreObjectRebind(id ObjectID, mark MarkRaw) {
	if int(id) >= len(objectRegistry) {
		panic("memcore: invalid ObjectID in rebind")
	}
	objectRegistry[id] = mark
}

// MemcoreObjectUnregister removes an object mapping entirely.
//
//go:nosplit
//go:inline
func MemcoreObjectUnregister(id ObjectID) {
	if int(id) >= len(objectRegistry) {
		return
	}
	objectRegistry[id] = MarkRaw{}
	objectActive[id] = false
	objectFreeList = append(objectFreeList, id)
}

// MemcoreObjectResolve retrieves the MarkRaw associated with an ObjectID.
//
//go:nosplit
//go:inline
func MemcoreObjectResolve(id ObjectID) (MarkRaw, bool) {
	if int(id) >= len(objectRegistry) || !objectActive[id] {
		return MarkRaw{}, false
	}
	m := objectRegistry[id]
	if !regionRegistry[m.regionID].active {
		objectActive[id] = false
		objectFreeList = append(objectFreeList, id)
		return MarkRaw{}, false
	}
	return m, true
}

// MemcoreObjectRegistryClear resets all object mappings.
func MemcoreObjectRegistryClear() {
	objectRegistry = make([]MarkRaw, 0)
	objectFreeList = make([]uint32, 0)
	objectIDCounter = 0
}

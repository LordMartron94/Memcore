package memcore

import (
	"fmt"
	"reflect"
	"unsafe"
)

type funcKey struct {
	ptr uintptr
	typ reflect.Type
}

var (
	// Initialize with one empty/inactive region so the first real region gets ID 1
	regionRegistry []memoryRegion = []memoryRegion{{base: 0, sizeBytes: 0, active: false}}
	regionFreeList []uint32       = make([]uint32, 0)
	regionBases    []uintptr      = []uintptr{0}

	functionRegistry []interface{} = make([]interface{}, 0)
	functionFreeList []uint32      = make([]uint32, 0)
	functionKeyMap                 = make(map[funcKey]FunctionID)

	objectRegistry  []MarkRaw = make([]MarkRaw, 0)
	objectFreeList  []uint32  = make([]uint32, 0)
	objectActive    []bool    = make([]bool, 0)
	objectIDCounter uint32    = 0
)

/*
MemcoreMarkManagementStateReset clears region, object, and optionally function registries.

[Parameters]
resetFunctions - When true, also clears the function registry via MemcoreFunctionRegistryClear.

[Side Effects]
Resets global mark management state; existing region IDs, object IDs, and marks become invalid.
*/
func MemcoreMarkManagementStateReset(resetFunctions bool) {
	if resetFunctions {
		MemcoreFunctionRegistryClear()
	}

	regionRegistry = []memoryRegion{{base: 0, sizeBytes: 0, active: false}}
	regionFreeList = make([]uint32, 0)
	regionBases = []uintptr{0}

	MemcoreObjectRegistryClear()
}

/*
FunctionID identifies a registered callback stored in the memcore function registry.
*/
type FunctionID = uint32

/*
ObjectID identifies a MarkRaw registered in the memcore object registry.
*/
type ObjectID = uint32

type memoryRegion struct {
	base      uintptr
	sizeBytes uint64
	active    bool
}

/*
MarkRaw is an opaque (regionID, offset) handle to manual memory outside the Go heap.

[Context]
Preferred representation for memforge and other manual allocators: dereference via
MemcoreMarkDereference rather than retaining raw uintptrs across growth or remap.

[Invariants]
Marks are only valid while their region is registered and active. Unsafe variants skip checks.
*/
type MarkRaw struct {
	regionID uint32
	offset   uintptr
}

// ---------------------------------------- REGIONS

/*
MemcoreRegionRegister records a contiguous manual memory span and returns its region ID.

[Parameters]
baseAddr - Base address of the mapping or arena.
sizeBytes - Extent of the region in bytes.

[Returns]
A region ID used when creating marks with MemcoreMarkCreate.

[Complexity]
Time: O(1) amortized. Space: O(1) per registration.
*/
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

/*
MemcoreRegionUnregister deactivates a region and recycles its ID.

[Parameters]
regionID - Previously returned by MemcoreRegionRegister.

[Side Effects]
Marks in this region become invalid for MemcoreMarkDereference; no-op if regionID is out of range.
*/
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

/*
MemcoreRegionBaseUpdate changes the base address of a registered region after remap or relocation.

[Context]
Call when mremap or similar moves the mapping but marks keep the same region ID and offsets.
*/
//go:nosplit
//go:inline
func MemcoreRegionBaseUpdate(regionID uint32, newBase uintptr) {
	regionRegistry[regionID].base = newBase
	regionBases[regionID] = newBase
}

/*
MemcoreAddressBelongsToActiveRegion reports whether address lies inside any active region span.

[Returns]
True when address is within [region.base, region.base+region.sizeBytes) for an active region.
*/
func MemcoreAddressBelongsToActiveRegion(address uintptr) bool {
	if address == 0 {
		return false
	}
	for i := 1; i < len(regionRegistry); i++ {
		region := regionRegistry[i]
		if !region.active || region.base == 0 || region.sizeBytes == 0 {
			continue
		}
		regionEnd := region.base + uintptr(region.sizeBytes)
		if address >= region.base && address < regionEnd {
			return true
		}
	}
	return false
}

// ---------------------------------------- MARKS

/*
MemcoreMarkCreate builds a mark at offset within regionID.

[Returns]
A MarkRaw with no validation that regionID is active or offset is in bounds.
*/
//go:nosplit
//go:inline
func MemcoreMarkCreate(regionID uint32, offset uintptr) MarkRaw {
	mark := MarkRaw{
		regionID: regionID,
		offset:   offset,
	}
	return mark
}

/*
MemcoreMarkOffsetFrom returns a mark at base.offset plus offset in the same region.
*/
//go:nosplit
//go:inline
func MemcoreMarkOffsetFrom(base MarkRaw, offset uintptr) MarkRaw {
	return MarkRaw{
		regionID: base.regionID,
		offset:   base.offset + offset,
	}
}

/*
MemcoreMarkSubtractBaseOffset returns mark.offset minus baseOffset for arena-relative indices.
*/
//go:nosplit
//go:inline
func MemcoreMarkSubtractBaseOffset(mark MarkRaw, baseOffset uintptr) uintptr {
	return mark.offset - baseOffset
}

/*
MemcoreMarkAlignedOffsetFrom advances base by offset then rounds up to alignment.

[Parameters]
alignment - Must be a power of two.

[Returns]
The aligned mark and padding bytes inserted before the aligned offset.

[Example]
	base := MemcoreMarkCreate(regionID, 0)
	aligned, padding := MemcoreMarkAlignedOffsetFrom(base, 13, 8)
	// aligned.offset == 16, padding == 3
*/
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

/*
MemcoreMarkDereference resolves a mark to an unsafe.Pointer in the registered region.

[Errors]
Panics if the region is inactive or regionID is invalid.

[Side Effects]
Pure aside from the panic path; does not mutate memory.
*/
//go:nosplit
//go:inline
func MemcoreMarkDereference(mark MarkRaw) unsafe.Pointer {
	r := regionRegistry[mark.regionID]
	if !r.active {
		panic("memcore: invalid region ID dereference")
	}
	return unsafe.Pointer(r.base + mark.offset)
}

/*
MemcoreMarkDereferenceUnsafe resolves a mark using cached region bases without active checks.

[Invariants]
Invalid regionID or offset produces undefined behavior rather than a guaranteed panic.
*/
//go:nosplit
//go:inline
func MemcoreMarkDereferenceUnsafe(mark MarkRaw) unsafe.Pointer {
	return unsafe.Pointer(regionBases[mark.regionID] + mark.offset)
}

/*
MemcoreMarkDereferenceObject returns *T at the mark via MemcoreMarkDereference.
*/
//go:nosplit
//go:inline
func MemcoreMarkDereferenceObject[T any](mark MarkRaw) *T {
	addr := MemcoreMarkDereference(mark)
	return (*T)(addr)
}

/*
MemcoreMarkDereferenceWithType returns a reflect-typed pointer at the mark.
*/
//go:nosplit
//go:inline
func MemcoreMarkDereferenceWithType(mark MarkRaw, t reflect.Type) unsafe.Pointer {
	addr := MemcoreMarkDereference(mark)
	val := reflect.NewAt(t, addr)
	return unsafe.Pointer(val.Pointer())
}

/*
MemcoreMarkDereferenceWithTypeUnsafe is the unchecked variant of MemcoreMarkDereferenceWithType.
*/
//go:nosplit
//go:inline
func MemcoreMarkDereferenceWithTypeUnsafe(mark MarkRaw, t reflect.Type) unsafe.Pointer {
	addr := MemcoreMarkDereferenceUnsafe(mark)
	val := reflect.NewAt(t, addr)
	return unsafe.Pointer(val.Pointer())
}

/*
MemcoreMarkDereferenceObjectUnsafe returns *T via MemcoreMarkDereferenceUnsafe.
*/
//go:nosplit
//go:inline
func MemcoreMarkDereferenceObjectUnsafe[T any](mark MarkRaw) *T {
	addr := MemcoreMarkDereferenceUnsafe(mark)
	return (*T)(addr)
}

/*
MemcoreMarkDereferenceObjectAlt returns both the raw pointer and *T at the mark.
*/
//go:nosplit
//go:inline
func MemcoreMarkDereferenceObjectAlt[T any](mark MarkRaw) (unsafe.Pointer, *T) {
	addr := MemcoreMarkDereference(mark)
	return addr, (*T)(addr)
}

/*
MemcoreMarkDereferenceObjectAltUnsafe is the unchecked variant of MemcoreMarkDereferenceObjectAlt.
*/
//go:nosplit
//go:inline
func MemcoreMarkDereferenceObjectAltUnsafe[T any](mark MarkRaw) (unsafe.Pointer, *T) {
	addr := MemcoreMarkDereferenceUnsafe(mark)
	return addr, (*T)(addr)
}

/*
MemcoreMarkIsValid reports whether mark.regionID refers to an active region.
*/
//go:nosplit
//go:inline
func MemcoreMarkIsValid(m MarkRaw) bool {
	return int(m.regionID) < len(regionRegistry) && regionRegistry[m.regionID].active
}

/*
MemcoreMarkOffsetIs reports whether m.offset equals offset.
*/
//go:nosplit
//go:inline
func MemcoreMarkOffsetIs(m MarkRaw, offset uintptr) bool {
	return m.offset == offset
}

/*
MemcoreMarkBelongsToRegion reports whether m and other share the same regionID.
*/
//go:nosplit
//go:inline
func MemcoreMarkBelongsToRegion(m MarkRaw, other MarkRaw) bool {
	return m.regionID == other.regionID
}

// ---------------------------------------- FUNCTIONS

/*
MemcoreFunctionRegister stores fn in the global function table and returns its ID.

[Side Effects]
Indexes fn by reflect pointer and type for later MemcoreFunctionGetID lookups.
*/
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

	ptr := reflect.ValueOf(fn).Pointer()
	typ := reflect.TypeOf(fn)
	functionKeyMap[funcKey{ptr: ptr, typ: typ}] = id
	return id
}

/*
MemcoreFunctionRegisterTyped registers a typed function and returns its FunctionID.
*/
//go:nosplit
//go:inline
func MemcoreFunctionRegisterTyped[T any](function T) FunctionID {
	return MemcoreFunctionRegister(function)
}

/*
MemcoreFunctionGetID looks up a previously registered function by identity.

[Returns]
The FunctionID and true if found; 0 and false if not registered or slot was cleared.
*/
//go:nosplit
//go:inline
func MemcoreFunctionGetID(fn interface{}) (FunctionID, bool) {
	ptr := reflect.ValueOf(fn).Pointer()
	typ := reflect.TypeOf(fn)

	id, ok := functionKeyMap[funcKey{ptr: ptr, typ: typ}]
	if !ok {
		return 0, false
	}
	if int(id) >= len(functionRegistry) || functionRegistry[id] == nil {
		delete(functionKeyMap, funcKey{ptr: ptr, typ: typ})
		return 0, false
	}
	return id, true
}

/*
MemcoreFunctionRegisterOrGet returns an existing ID for fn or registers it.
*/
//go:nosplit
//go:inline
func MemcoreFunctionRegisterOrGet(fn interface{}) FunctionID {
	if id, ok := MemcoreFunctionGetID(fn); ok {
		return id
	}
	return MemcoreFunctionRegister(fn)
}

/*
MemcoreFunctionRebind replaces the function stored at id and updates the identity map.

[Errors]
Panics if id is out of range.
*/
//go:nosplit
//go:inline
func MemcoreFunctionRebind(id FunctionID, fn interface{}) {
	if int(id) >= len(functionRegistry) {
		panic("memcore: invalid FunctionID in rebind")
	}

	oldFn := functionRegistry[id]
	if oldFn != nil {
		oldPtr := reflect.ValueOf(oldFn).Pointer()
		oldTyp := reflect.TypeOf(oldFn)
		delete(functionKeyMap, funcKey{ptr: oldPtr, typ: oldTyp})
	}

	functionRegistry[id] = fn
	ptr := reflect.ValueOf(fn).Pointer()
	typ := reflect.TypeOf(fn)
	functionKeyMap[funcKey{ptr: ptr, typ: typ}] = id
}

/*
MemcoreFunctionUnregister clears slot id and recycles it; no-op if id is out of range.
*/
//go:nosplit
//go:inline
func MemcoreFunctionUnregister(id FunctionID) {
	if int(id) >= len(functionRegistry) {
		return
	}

	fn := functionRegistry[id]
	if fn != nil {
		ptr := reflect.ValueOf(fn).Pointer()
		typ := reflect.TypeOf(fn)
		delete(functionKeyMap, funcKey{ptr: ptr, typ: typ})
	}

	functionRegistry[id] = nil
	functionFreeList = append(functionFreeList, id)
}

/*
MemcoreFunctionRetrieve returns the raw function at id or nil if out of range.
*/
//go:nosplit
//go:inline
func MemcoreFunctionRetrieve(id FunctionID) interface{} {
	if int(id) >= len(functionRegistry) {
		return nil
	}
	return functionRegistry[id]
}

/*
MemcoreFunctionRetrieveTyped returns the function at id cast to T.

[Errors]
Panics if the slot is inactive or the stored value is not assignable to T.
*/
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

/*
MemcoreFunctionRegistryClear drops all registered functions and the identity map.
*/
func MemcoreFunctionRegistryClear() {
	functionRegistry = make([]interface{}, 0)
	functionFreeList = make([]uint32, 0)
	functionKeyMap = make(map[funcKey]FunctionID)
}

// ---------------------------------------- OBJECTS

/*
MemcoreObjectRegister assigns a new ObjectID to mark.

[Returns]
A recycled or newly allocated ObjectID.
*/
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

/*
MemcoreObjectRebind updates the mark stored at id.

[Errors]
Panics if id is out of range.
*/
//go:nosplit
//go:inline
func MemcoreObjectRebind(id ObjectID, mark MarkRaw) {
	if int(id) >= len(objectRegistry) {
		panic("memcore: invalid ObjectID in rebind")
	}
	objectRegistry[id] = mark
}

/*
MemcoreObjectUnregister clears object id and recycles its slot.
*/
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

/*
MemcoreObjectResolve returns the mark for id when the object and region are still active.

[Returns]
The mark and true on success; zero mark and false if id is unknown or region was unregistered.
*/
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

/*
MemcoreObjectRegistryClear drops all object mappings.
*/
func MemcoreObjectRegistryClear() {
	objectRegistry = make([]MarkRaw, 0)
	objectFreeList = make([]uint32, 0)
	objectIDCounter = 0
}

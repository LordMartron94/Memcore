package memcore

import (
	"fmt"
	"reflect"
	"sync/atomic"
	"unsafe"
)

type funcKey struct {
	ptr uintptr
	typ reflect.Type
}

var (
	// Initialize with one empty/inactive region so the first real region gets ID 1
	regionRegistry []memoryRegion = []memoryRegion{{base: 0, sizeBytes: 0, active: false, opaque: false}}
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

	regionRegistry = []memoryRegion{{base: 0, sizeBytes: 0, active: false, opaque: false}}
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
	base             uintptr
	sizeBytes        uint64
	active           bool
	opaque           bool
	parentRegionID   uint32
	parentByteOffset uintptr
	activeChildCount int32
}

func memoryRegionRootInit(base uintptr, sizeBytes uint64, opaque bool) memoryRegion {
	return memoryRegion{
		base:      base,
		sizeBytes: sizeBytes,
		active:    true,
		opaque:    opaque,
	}
}

func memoryRegionSubRegionInit(
	parentRegionID uint32,
	parentByteOffset uintptr,
	capacityBytes uint64,
	opaque bool,
) memoryRegion {
	return memoryRegion{
		sizeBytes:        capacityBytes,
		active:           true,
		opaque:           opaque,
		parentRegionID:   parentRegionID,
		parentByteOffset: parentByteOffset,
	}
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
		regionRegistry[id] = memoryRegionRootInit(baseAddr, sizeBytes, false)
		regionBases[id] = baseAddr
	} else {
		id = uint32(len(regionRegistry))
		regionRegistry = append(regionRegistry, memoryRegionRootInit(baseAddr, sizeBytes, false))
		regionBases = append(regionBases, baseAddr)
	}
	return id
}

/*
MemcoreRegionUnregister deactivates a region and recycles its ID.

Sub-regions decrement the parent activeChildCount. Root regions panic when activeChildCount is non-zero.
*/
//go:nosplit
//go:inline
func MemcoreRegionUnregister(regionID uint32) {
	if int(regionID) >= len(regionRegistry) {
		return
	}

	r := &regionRegistry[regionID]
	if !r.active {
		return
	}

	if r.parentRegionID != 0 {
		memcoreRegionActiveChildCountDecrement(r.parentRegionID)
	} else if atomic.LoadInt32(&r.activeChildCount) > 0 {
		panic("memcore: cannot unregister root region with active sub-regions")
	}

	r.base = 0
	r.sizeBytes = 0
	r.active = false
	r.opaque = false
	r.parentRegionID = 0
	r.parentByteOffset = 0
	r.activeChildCount = 0

	if int(regionID) < len(regionBases) {
		regionBases[regionID] = 0
	}

	regionFreeList = append(regionFreeList, regionID)
}

func memcoreRegionActiveChildCountIncrement(parentRegionID uint32) {
	if int(parentRegionID) >= len(regionRegistry) {
		panic("memcore: invalid parent region ID for sub-region registration")
	}

	parent := &regionRegistry[parentRegionID]
	if !parent.active {
		panic("memcore: inactive parent region for sub-region registration")
	}

	atomic.AddInt32(&parent.activeChildCount, 1)
}

func memcoreRegionActiveChildCountDecrement(parentRegionID uint32) {
	if int(parentRegionID) >= len(regionRegistry) {
		return
	}

	parent := &regionRegistry[parentRegionID]
	if !parent.active {
		return
	}

	atomic.AddInt32(&parent.activeChildCount, -1)
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
MemcoreRegionRegisterOpaque records a logical region with no CPU mapping.

[Context]
Use for GPU device memory and other backends where offsets are tracked in memcore but bytes are
not dereferenceable from the host. Marks in opaque regions are valid handles for bind offsets only.

[Returns]
A region ID with logical size sizeBytes and base address zero.
*/
//go:nosplit
//go:inline
func MemcoreRegionRegisterOpaque(sizeBytes uint64) uint32 {
	var id uint32
	if len(regionFreeList) > 0 {
		id = regionFreeList[len(regionFreeList)-1]
		regionFreeList = regionFreeList[:len(regionFreeList)-1]
		regionRegistry[id] = memoryRegionRootInit(0, sizeBytes, true)
		regionBases[id] = 0
	} else {
		id = uint32(len(regionRegistry))
		regionRegistry = append(regionRegistry, memoryRegionRootInit(0, sizeBytes, true))
		regionBases = append(regionBases, 0)
	}
	return id
}

/*
MemcoreRegionRegisterSubRegion creates an alias window into parentRegionID starting at parentOffset.

The sub-region inherits the parent opaque flag and increments parent activeChildCount.
*/
//go:nosplit
func MemcoreRegionRegisterSubRegion(parentRegionID uint32, parentOffset uintptr, capacityBytes uint64) uint32 {
	if capacityBytes == 0 {
		panic("memcore: sub-region capacity must be greater than zero")
	}

	if int(parentRegionID) >= len(regionRegistry) {
		panic("memcore: invalid parent region ID for sub-region registration")
	}

	parent := regionRegistry[parentRegionID]
	if !parent.active {
		panic("memcore: inactive parent region for sub-region registration")
	}

	windowEnd := uint64(parentOffset) + capacityBytes
	if windowEnd < uint64(parentOffset) || windowEnd > parent.sizeBytes {
		panic("memcore: sub-region window exceeds parent region extent")
	}

	memcoreRegionActiveChildCountIncrement(parentRegionID)

	var id uint32
	if len(regionFreeList) > 0 {
		id = regionFreeList[len(regionFreeList)-1]
		regionFreeList = regionFreeList[:len(regionFreeList)-1]
		regionRegistry[id] = memoryRegionSubRegionInit(parentRegionID, parentOffset, capacityBytes, parent.opaque)
		regionBases[id] = 0
	} else {
		id = uint32(len(regionRegistry))
		regionRegistry = append(regionRegistry, memoryRegionSubRegionInit(parentRegionID, parentOffset, capacityBytes, parent.opaque))
		regionBases = append(regionBases, 0)
	}

	return id
}

/*
MemcoreRegionRegisterSubRegionFromMark registers a sub-region at parentMark with capacityBytes.
*/
//go:nosplit
//go:inline
func MemcoreRegionRegisterSubRegionFromMark(parentMark MarkRaw, capacityBytes uint64) uint32 {
	return MemcoreRegionRegisterSubRegion(
		MemcoreMarkRegionIDGet(parentMark),
		MemcoreMarkOffsetGet(parentMark),
		capacityBytes,
	)
}

/*
MemcoreRegionIsSubRegion reports whether regionID refers to an active alias window.
*/
//go:nosplit
//go:inline
func MemcoreRegionIsSubRegion(regionID uint32) bool {
	if int(regionID) >= len(regionRegistry) {
		return false
	}

	region := regionRegistry[regionID]
	return region.active && region.parentRegionID != 0
}

/*
MemcoreMarkResolveToRoot walks sub-region parent chains and returns the absolute root mark.
*/
//go:nosplit
func MemcoreMarkResolveToRoot(mark MarkRaw) MarkRaw {
	regionID := mark.regionID
	offset := mark.offset

	for {
		if int(regionID) >= len(regionRegistry) {
			panic("memcore: invalid region ID during root resolution")
		}

		region := regionRegistry[regionID]
		if !region.active {
			panic("memcore: inactive region during root resolution")
		}

		if region.parentRegionID == 0 {
			return MemcoreMarkCreate(regionID, offset)
		}

		offset += region.parentByteOffset
		regionID = region.parentRegionID
	}
}

/*
MemcoreRegionIsOpaque reports whether regionID refers to an active opaque region.
*/
//go:nosplit
//go:inline
func MemcoreRegionIsOpaque(regionID uint32) bool {
	if int(regionID) >= len(regionRegistry) {
		return false
	}
	region := regionRegistry[regionID]
	return region.active && region.opaque
}

/*
MemcoreMarkRegionIsOpaque reports whether mark.regionID refers to an active opaque region.
*/
//go:nosplit
//go:inline
func MemcoreMarkRegionIsOpaque(mark MarkRaw) bool {
	return MemcoreRegionIsOpaque(mark.regionID)
}

/*
MemcoreMarkRegionIDGet returns the region ID embedded in mark.
*/
//go:nosplit
//go:inline
func MemcoreMarkRegionIDGet(mark MarkRaw) uint32 {
	return mark.regionID
}

/*
MemcoreMarkOffsetGet returns the byte offset embedded in mark.
*/
//go:nosplit
//go:inline
func MemcoreMarkOffsetGet(mark MarkRaw) uintptr {
	return mark.offset
}

/*
MemcoreMarkOffsetInBounds reports whether mark.offset plus sizeBytes lies within the region extent.

[Returns]
False when the region is inactive, sizeBytes overflows, or the range exceeds region.sizeBytes.
*/
//go:nosplit
//go:inline
func MemcoreMarkOffsetInBounds(mark MarkRaw, sizeBytes uint64) bool {
	if int(mark.regionID) >= len(regionRegistry) {
		return false
	}
	region := regionRegistry[mark.regionID]
	if !region.active || region.sizeBytes == 0 {
		return false
	}
	start := uint64(mark.offset)
	end := start + sizeBytes
	return end >= start && end <= region.sizeBytes
}

//go:nosplit
//go:inline
func memcoreMarkDereferenceGuard(region memoryRegion) {
	if region.opaque {
		panic("memcore: cannot dereference mark in opaque region")
	}
}

/*
MemcoreRegionSizeGet returns the registered byte extent of an active region.

[Errors]
Panics if regionID is invalid or inactive.
*/
//go:nosplit
//go:inline
func MemcoreRegionSizeGet(regionID uint32) uint64 {
	if int(regionID) >= len(regionRegistry) {
		panic("memcore: invalid region ID size query")
	}
	region := regionRegistry[regionID]
	if !region.active {
		panic("memcore: inactive region size query")
	}
	return region.sizeBytes
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
		if !region.active || region.opaque || region.base == 0 || region.sizeBytes == 0 {
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
	resolved := MemcoreMarkResolveToRoot(mark)
	r := regionRegistry[resolved.regionID]
	if !r.active {
		panic("memcore: invalid region ID dereference")
	}
	memcoreMarkDereferenceGuard(r)
	return unsafe.Pointer(r.base + resolved.offset)
}

/*
MemcoreMarkDereferenceUnsafe resolves a mark using cached region bases without active checks.

[Invariants]
Invalid regionID or offset produces undefined behavior rather than a guaranteed panic.
*/
//go:nosplit
//go:inline
func MemcoreMarkDereferenceUnsafe(mark MarkRaw) unsafe.Pointer {
	resolved := MemcoreMarkResolveToRoot(mark)
	if int(resolved.regionID) < len(regionRegistry) {
		memcoreMarkDereferenceGuard(regionRegistry[resolved.regionID])
	}
	return unsafe.Pointer(regionBases[resolved.regionID] + resolved.offset)
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

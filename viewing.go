package memcore

import "unsafe"

/*
MemcoreViewFn exposes a zero-copy (pointer, length) view of a MarkRaw-backed object.
*/
type MemcoreViewFn func(object MarkRaw) (unsafe.Pointer, uint64)

var (
	viewRegistry []MemcoreViewFn = make([]MemcoreViewFn, 0)

	typeIDRegistry        = make(map[uintptr]uint32)
	typeIDCounter  uint32 = 0
)

/*
MemcoreViewRegister installs fn as the raw view getter for type T.

[Side Effects]
Grows viewRegistry as needed; overwrites any prior registration for T.
*/
//go:nosplit
//go:inline
func MemcoreViewRegister[T any](fn MemcoreViewFn) {
	typeID := typeIDOf[T]()

	for len(viewRegistry) <= int(typeID) {
		viewRegistry = append(viewRegistry, nil)
	}
	viewRegistry[typeID] = fn
}

/*
MemcoreViewGet returns the registered view getter for T, or nil if none.
*/
//go:nosplit
//go:inline
func MemcoreViewGet[T any]() MemcoreViewFn {
	typeID := typeIDOf[T]()
	if int(typeID) >= len(viewRegistry) {
		return nil
	}
	return viewRegistry[typeID]
}

/*
MemcoreView invokes the registered getter for T on mark v without allocating.

[Errors]
Panics when no view was registered for T.

[Returns]
Pointer and byte length suitable for read-only iteration.
*/
//go:nosplit
//go:inline
func MemcoreView[T any](v MarkRaw) (unsafe.Pointer, uint64) {
	viewFn := MemcoreViewGet[T]()
	if viewFn == nil {
		panic("memcore: no view registered for type")
	}
	return viewFn(v)
}

//go:nosplit
//go:inline
func typeIDOf[T any]() uint32 {
	var ptr uintptr = uintptr(unsafe.Pointer((*T)(nil)))
	id, ok := typeIDRegistry[ptr]
	if ok {
		return id
	}
	newID := typeIDCounter
	typeIDCounter++
	typeIDRegistry[ptr] = newID
	return newID
}

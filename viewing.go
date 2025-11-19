package memcore

import "unsafe"

// MemcoreViewFn returns the pointer and length of a raw object for read-only access.
type MemcoreViewFn func(object MarkRaw) (unsafe.Pointer, uint64)

// Internal registry for these raw views.
var (
	viewRegistry []MemcoreViewFn = make([]MemcoreViewFn, 0)

	typeIDRegistry        = make(map[uintptr]uint32)
	typeIDCounter  uint32 = 0
)

// MemcoreViewRegister registers a zero-copy raw-view getter for a type.
//
//go:nosplit
//go:inline
func MemcoreViewRegister[T any](fn MemcoreViewFn) {
	typeID := typeIDOf[T]()

	for len(viewRegistry) <= int(typeID) {
		viewRegistry = append(viewRegistry, nil)
	}
	viewRegistry[typeID] = fn
}

// MemcoreViewGet retrieves the registered view getter for a type.
//
//go:nosplit
//go:inline
func MemcoreViewGet[T any]() MemcoreViewFn {
	typeID := typeIDOf[T]()
	if int(typeID) >= len(viewRegistry) {
		return nil
	}
	return viewRegistry[typeID]
}

// MemcoreView returns a zero-copy pointer+length pair for a MarkRaw object.
// No allocation occurs.
//
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

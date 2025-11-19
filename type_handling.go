package memcore

import "reflect"

type MemcoreType uint32 // MemcoreType is a manual memory-safe way to interact with types.

// Type returns the real reflect.Type
func (m MemcoreType) Type() reflect.Type {
	return typeRegistry[m]
}

var typeRegistry map[MemcoreType]reflect.Type
var reverseTypeRegistry map[reflect.Type]MemcoreType

// MemcoreTypeRetrieve retrieves the MemcoreType for a given Type.
// If it does not exist yet, it will create it.
func MemcoreTypeRetrieve[T any]() MemcoreType {
	itemType := reflect.TypeFor[T]()
	v, ok := reverseTypeRegistry[itemType]
	if ok {
		return v
	} else {
		id := MemcoreType(len(typeRegistry))
		typeRegistry[id] = itemType
		reverseTypeRegistry[itemType] = id
		return id
	}
}

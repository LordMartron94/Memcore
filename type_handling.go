package memcore

import "reflect"

/*
MemcoreType is a compact ID for a reflect.Type stored in manual memory registries.
*/
type MemcoreType uint32

/*
Type resolves the MemcoreType to its registered reflect.Type.
*/
func (m MemcoreType) Type() reflect.Type {
	return typeRegistry[m]
}

var typeRegistry map[MemcoreType]reflect.Type = make(map[MemcoreType]reflect.Type)
var reverseTypeRegistry map[reflect.Type]MemcoreType = make(map[reflect.Type]MemcoreType)

/*
MemcoreTypeRetrieve returns the stable MemcoreType for T, creating a registry entry on first use.

[Side Effects]
Mutates typeRegistry and reverseTypeRegistry when T is new.

[Complexity]
Time: O(1) amortized per distinct type. Space: O(1) per registered type.
*/
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

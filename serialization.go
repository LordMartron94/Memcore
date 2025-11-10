package memcore

import (
	"fmt"
	"reflect"
	"sync"
)

var serializerRegistry = sync.Map{}

// MemcoreSerializerRegister registers a custom deterministic serializer function
// for a given type T.
//
// The registered function receives a pointer to an instance of T and must return
// a slice of bytes that *deterministically* represents the semantic content of T.
// Two values of T that are considered equal (according to their equality comparer)
// must produce identical byte slices.
//
// This mechanism allows hashing and comparison systems to remain
// type-agnostic while preserving semantic equality across memory regions,
// allocators, or sessions.
//
// Example:
//
//	memcore.MemcoreSerializerRegister[memstruct.String](func(v *memstruct.String) []byte {
//	    base := unsafe.Add(unsafe.Pointer(v), v.DataAddrOffset)
//	    return unsafe.Slice((*byte)(base), v.Length)
//	})
//
// After registration, MemcoreSerializeDeterministic will automatically use
// this serializer whenever it encounters a *memstruct.String* value.
func MemcoreSerializerRegister[T any](fn func(*T) []byte) {
	serializerRegistry.Store(reflect.TypeOf((*T)(nil)).Elem(), any(fn))
}

// MemcoreRegisteredSerializerGet retrieves the deterministic serializer function
// previously registered for type T.
//
// If no serializer is registered, it returns nil. Callers should typically use
// MemcoreSerializeDeterministic instead, which transparently falls back to a
// raw byte-level representation when no custom serializer exists.
func MemcoreRegisteredSerializerGet[T any]() func(*T) []byte {
	if v, ok := serializerRegistry.Load(reflect.TypeOf((*T)(nil)).Elem()); ok {
		return v.(func(*T) []byte)
	}
	return nil
}

func MemcoreSerialize[T any](v *T) ([]byte, error) {
	serializer := MemcoreRegisteredSerializerGet[T]()
	if serializer == nil {
		return nil, fmt.Errorf("could not find serializer for type")
	}

	return serializer(v), nil
}

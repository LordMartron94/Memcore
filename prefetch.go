package memcore

import "unsafe"

/*
PrefetchRead hints the CPU to load the cache line containing ptr into L1.

[Context]
Advisory only; no-op on unsupported architectures via prefetch_fallback.go.

[Side Effects]
None beyond possible cache effects; does not access memory beyond the prefetch instruction semantics.
*/
func PrefetchRead(ptr unsafe.Pointer) {
	platformPrefetchReadAsm(ptr)
}

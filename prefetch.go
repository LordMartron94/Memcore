package memcore

import "unsafe"

// PrefetchRead prefetches the given memory address into L1 cache.
// It’s advisory only; ignored on unsupported CPUs.
func PrefetchRead(ptr unsafe.Pointer) {
	platformPrefetchReadAsm(ptr)
}

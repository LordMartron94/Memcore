//go:build unix

package memcore

/*
#include <string.h>
*/
import "C"
import "unsafe"

// MemoryMoveNoHeapPointers calls libc memmove, which is implemented in highly
// optimized assembly (AVX2 / NEON) on all major platforms.
func platformMemoryMoveNoHeapPointers(dst, src unsafe.Pointer, n uintptr) {
	if n == 0 || dst == src {
		return
	}
	C.memmove(dst, src, C.size_t(n))
}

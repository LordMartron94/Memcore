//go:build windows

package memcore

import (
	"syscall"
	"unsafe"
)

var rtlMoveMemory = syscall.NewLazyDLL("kernel32.dll").NewProc("RtlMoveMemory")

func platformMemoryMoveNoHeapPointers(dst, src unsafe.Pointer, n uintptr) {
	if n == 0 || dst == src {
		return
	}
	rtlMoveMemory.Call(uintptr(dst), uintptr(src), n)
}

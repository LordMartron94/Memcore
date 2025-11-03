// Package memcore provides low-level core infrastructure for memory management.
package memcore

import (
	"sync"
	"unsafe"
)

var mmapRegistry sync.Map

// MemoryMap represents a mapped memory region.
type MemoryMap []byte

func MemmapRequest(byteAmount int, protection MemoryProtectionFlag, flags MemoryMapFlag) (MemoryMap, error) {
	m, err := platformMemmapRequest(byteAmount, protection, flags)
	if err == nil && len(m) > 0 {
		mmapRegistry.Store(uintptr(unsafe.Pointer(&m[0])), byteAmount)
	}
	return m, err
}

func MemmapUnmap(memmap MemoryMap) error {
	err := platformMemmapUnmap(memmap)
	if err == nil && len(memmap) > 0 {
		mmapRegistry.Delete(uintptr(unsafe.Pointer(&memmap[0])))
	}
	return err
}

func MemmapProtect(memmap MemoryMap, protection MemoryProtectionFlag) error {
	return platformMemmapProtect(memmap, protection)
}

func MemmapLock(memmap MemoryMap) error {
	return platformMemmapLock(memmap)
}

func MemmapUnlock(memmap MemoryMap) error {
	return platformMemmapUnlock(memmap)
}

func MemmapLockAll(flags MemoryLockAllFlag) error {
	return platformMemmapLockAll(flags)
}

func MemmapUnlockAll() error {
	return platformMemmapUnlockAll()
}

func MemmapAdvise(memmap MemoryMap, advice MemoryAdviceFlag) error {
	return platformMemmapAdvise(memmap, advice)
}

func MemmapSync(memmap MemoryMap, syncFlags MemorySyncFlag) error {
	return platformMemmapSync(memmap, syncFlags)
}

func MemmapRemap(memmap MemoryMap, newSize int, flags MemoryRemapFlag) (MemoryMap, error) {
	m, err := platformMemmapRemap(memmap, newSize, flags)
	if err == nil && len(m) > 0 && len(memmap) > 0 {
		mmapRegistry.Delete(uintptr(unsafe.Pointer(&memmap[0])))
		mmapRegistry.Store(uintptr(unsafe.Pointer(&m[0])), newSize)
	}
	return m, err
}

func MemmapRemapAt(addr unsafe.Pointer, oldSize, newSize int, flags MemoryRemapFlag) (MemoryMap, error) {
	m, err := platformMemmapRemapAt(addr, oldSize, newSize, flags)
	if err == nil && len(m) > 0 {
		mmapRegistry.Delete(uintptr(addr))
		mmapRegistry.Store(uintptr(unsafe.Pointer(&m[0])), newSize)
	}
	return m, err
}

func MemmapRequestAt(addr unsafe.Pointer, byteAmount int, protection MemoryProtectionFlag, flags MemoryMapFlag) (unsafe.Pointer, error) {
	mmap, err := platformMemmapRequestAt(addr, byteAmount, protection, flags)
	if err == nil {
		mmapRegistry.Store(uintptr(addr), byteAmount)
	}
	return mmap, err
}

func MemmapUnmapAt(addr unsafe.Pointer, byteAmount int) error {
	err := platformMemmapUnmapAt(addr, byteAmount)
	if err == nil {
		mmapRegistry.Delete(uintptr(addr))
	}
	return err
}

// MemmapUnmapAllRegions forcibly unmaps every region tracked in mmapRegistry.
// Used to guarantee no leaks remain after panic or abrupt benchmark termination.
func MemmapUnmapAllRegions() {
	mmapRegistry.Range(func(k, v any) bool {
		size := v.(int)
		_ = MemmapUnmapAt(unsafe.Pointer(k.(uintptr)), size)
		mmapRegistry.Delete(k)
		return true
	})
}

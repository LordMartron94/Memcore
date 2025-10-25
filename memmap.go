// Package memcore provides low-level core infrastructure for memory management.
package memcore

import "unsafe"

// MemoryMap represents a mapped memory region.
type MemoryMap []byte

func MemmapRequest(byteAmount int, protection MemoryProtectionFlag, flags MemoryMapFlag) (MemoryMap, error) {
	return platformMemmapRequest(byteAmount, protection, flags)
}

func MemmapUnmap(memmap MemoryMap) error {
	return platformMemmapUnmap(memmap)
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
	return platformMemmapRemap(memmap, newSize, flags)
}

func MemmapRequestAt(addr unsafe.Pointer, byteAmount int, protection MemoryProtectionFlag, flags MemoryMapFlag) (unsafe.Pointer, error) {
	return platformMemmapRequestAt(addr, byteAmount, protection, flags)
}

func MemmapUnmapAt(addr unsafe.Pointer, byteAmount int) error {
	return platformMemmapUnmapAt(addr, byteAmount)
}

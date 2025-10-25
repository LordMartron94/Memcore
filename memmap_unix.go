//go:build unix

package memcore

import (
	"fmt"
	"golang.org/x/sys/unix"
	"unsafe"
)

func platformMemmapRequest(byteAmount int, protection MemoryProtectionFlag, flags MemoryMapFlag) (MemoryMap, error) {
	mmap, err := unix.Mmap(-1, 0, byteAmount, int(protection), int(flags))
	if err != nil {
		return nil, fmt.Errorf("requesting memory map failed: %w", err)
	}
	return MemoryMap(mmap), nil
}

func platformMemmapUnmap(memmap MemoryMap) error {
	if err := unix.Munmap(memmap); err != nil {
		return fmt.Errorf("unmapping memory failed: %w", err)
	}
	return nil
}

func platformMemmapProtect(memmap MemoryMap, protection MemoryProtectionFlag) error {
	if err := unix.Mprotect(memmap, int(protection)); err != nil {
		return fmt.Errorf("mprotect failed: %w", err)
	}
	return nil
}

func platformMemmapLock(memmap MemoryMap) error {
	if err := unix.Mlock(memmap); err != nil {
		return fmt.Errorf("mlock failed: %w", err)
	}
	return nil
}

func platformMemmapUnlock(memmap MemoryMap) error {
	if err := unix.Munlock(memmap); err != nil {
		return fmt.Errorf("munlock failed: %w", err)
	}
	return nil
}

func platformMemmapLockAll(flags MemoryLockAllFlag) error {
	if err := unix.Mlockall(int(flags)); err != nil {
		return fmt.Errorf("mlockall failed: %w", err)
	}
	return nil
}

func platformMemmapUnlockAll() error {
	if err := unix.Munlockall(); err != nil {
		return fmt.Errorf("munlockall failed: %w", err)
	}
	return nil
}

func platformMemmapAdvise(memmap MemoryMap, advice MemoryAdviceFlag) error {
	if err := unix.Madvise(memmap, int(advice)); err != nil {
		return fmt.Errorf("madvise failed: %w", err)
	}
	return nil
}

func platformMemmapSync(memmap MemoryMap, syncFlags MemorySyncFlag) error {
	if err := unix.Msync(memmap, int(syncFlags)); err != nil {
		return fmt.Errorf("msync failed: %w", err)
	}
	return nil
}

func platformMemmapRemap(memmap MemoryMap, newSize int, flags MemoryRemapFlag) (MemoryMap, error) {
	newMap, err := unix.Mremap(memmap, newSize, int(flags))
	if err != nil {
		return nil, fmt.Errorf("mremap failed: %w", err)
	}
	return MemoryMap(newMap), nil
}

func platformMemmapRequestAt(addr unsafe.Pointer, byteAmount int, protection MemoryProtectionFlag, flags MemoryMapFlag) (unsafe.Pointer, error) {
	p, err := unix.MmapPtr(-1, 0, addr, uintptr(byteAmount), int(protection), int(flags)|int(MAP_FIXED))
	if err != nil {
		return nil, fmt.Errorf("mmap at fixed address failed: %w", err)
	}
	return p, nil
}

func platformMemmapUnmapAt(addr unsafe.Pointer, byteAmount int) error {
	if err := unix.MunmapPtr(addr, uintptr(byteAmount)); err != nil {
		return fmt.Errorf("munmap at fixed address failed: %w", err)
	}
	return nil
}

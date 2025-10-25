// Package memcore provides low-level core infrastructure for memory management.
package memcore

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

// MemoryMap represents a memory region mapped into the process' address space.
// It is a direct alias of a byte slice returned by unix.Mmap and should always
// be unmapped via memmapUnmap when no longer in use.
type MemoryMap []byte

// MemmapRequest creates a new anonymous or file-backed mapping of byteAmount bytes
// with the specified protection and flags. For anonymous mappings, fd = -1 and offset = 0.
func MemmapRequest(byteAmount int, protection MemoryProtectionFlag, flags MemoryMapFlag) (MemoryMap, error) {
	mmap, err := unix.Mmap(-1, 0, byteAmount, int(protection), int(flags))
	if err != nil {
		return nil, fmt.Errorf("requesting memory map failed: %w", err)
	}
	return MemoryMap(mmap), nil
}

// MemmapUnmap unmaps a previously created mapping, returning it to the OS.
// After unmapping, the memory region must not be accessed.
func MemmapUnmap(memmap MemoryMap) error {
	if err := unix.Munmap(memmap); err != nil {
		return fmt.Errorf("unmapping memory failed: %w", err)
	}
	return nil
}

// MemmapProtect changes the protection flags (PROT_*) of the given memory range.
// Equivalent to mprotect(2). The length is derived from the slice length.
func MemmapProtect(memmap MemoryMap, protection MemoryProtectionFlag) error {
	if err := unix.Mprotect(memmap, int(protection)); err != nil {
		return fmt.Errorf("mprotect failed: %w", err)
	}
	return nil
}

// MemmapLock pins the specified memory region in RAM using mlock(2).
// Locked memory will not be swapped out. Requires CAP_IPC_LOCK or appropriate limits.
func MemmapLock(memmap MemoryMap) error {
	if err := unix.Mlock(memmap); err != nil {
		return fmt.Errorf("mlock failed: %w", err)
	}
	return nil
}

// MemmapUnlock unlocks a region previously locked with memmapLock, allowing it to be swapped.
func MemmapUnlock(memmap MemoryMap) error {
	if err := unix.Munlock(memmap); err != nil {
		return fmt.Errorf("munlock failed: %w", err)
	}
	return nil
}

// MemmapLockAll locks all current and future mappings of the calling process (mlockall(2)).
func MemmapLockAll(flags MemoryLockAllFlag) error {
	if err := unix.Mlockall(int(flags)); err != nil {
		return fmt.Errorf("mlockall failed: %w", err)
	}
	return nil
}

// MemmapUnlockAll unlocks all regions previously locked by memmapLockAll (munlockall(2)).
func MemmapUnlockAll() error {
	if err := unix.Munlockall(); err != nil {
		return fmt.Errorf("munlockall failed: %w", err)
	}
	return nil
}

// MemmapAdvise gives advice to the kernel about how the specified memory will be used.
// This can improve paging behavior. Wraps madvise(2).
func MemmapAdvise(memmap MemoryMap, advice MemoryAdviceFlag) error {
	if err := unix.Madvise(memmap, int(advice)); err != nil {
		return fmt.Errorf("madvise failed: %w", err)
	}
	return nil
}

// MemmapSync flushes changes made to a file-backed mapping back to disk (msync(2)).
// Useful for durability when modifying memory-mapped files.
func MemmapSync(memmap MemoryMap, syncFlags MemorySyncFlag) error {
	if err := unix.Msync(memmap, int(syncFlags)); err != nil {
		return fmt.Errorf("msync failed: %w", err)
	}
	return nil
}

// MemmapRemap changes the size (and optionally location) of an existing mapping.
// Wraps mremap(2), which is Linux-specific. Returns a new MemoryMap object.
//
// If the mapping is successfully resized or moved, the returned MemoryMap
// should be used from that point on. The original slice must not be used
// after remapping.
func MemmapRemap(memmap MemoryMap, newSize int, flags MemoryRemapFlag) (MemoryMap, error) {
	newMap, err := unix.Mremap(memmap, newSize, int(flags))
	if err != nil {
		return nil, fmt.Errorf("mremap failed: %w", err)
	}
	return MemoryMap(newMap), nil
}

// MemmapRequestAt explicitly maps a region at a fixed address.
// This uses MmapPtr internally and returns an unsafe pointer to the region.
// Only use this if you control address space layout (e.g. custom arena allocators).
func MemmapRequestAt(addr unsafe.Pointer, byteAmount int, protection MemoryProtectionFlag, flags MemoryMapFlag) (unsafe.Pointer, error) {
	p, err := unix.MmapPtr(-1, 0, addr, uintptr(byteAmount), int(protection), int(flags)|int(MAP_FIXED))
	if err != nil {
		return nil, fmt.Errorf("mmap at fixed address failed: %w", err)
	}
	return p, nil
}

// MemmapUnmapAt unmaps a region previously mapped at a fixed address.
// This uses MunmapPtr instead of the slice-based Munmap.
func MemmapUnmapAt(addr unsafe.Pointer, byteAmount int) error {
	if err := unix.MunmapPtr(addr, uintptr(byteAmount)); err != nil {
		return fmt.Errorf("munmap at fixed address failed: %w", err)
	}
	return nil
}

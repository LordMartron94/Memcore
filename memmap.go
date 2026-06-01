package memcore

import (
	"fmt"
	"sync"
	"unsafe"
)

var mmapRegistry sync.Map

/*
MemoryMap is a byte slice header over an mmap-backed region tracked by memcore.
*/
type MemoryMap []byte

/*
MemmapRequest creates a new mapping of byteAmount bytes.

[Parameters]
protection - PROT_* flags for the mapping.
flags - MAP_* flags (anonymous mappings typically use MAP_ANON_PRIVATE).

[Returns]
A MemoryMap view and nil error on success.

[Errors]
Returns the platform mmap error when mapping fails.

[Side Effects]
Registers the mapping base in mmapRegistry for leak tracking.
*/
func MemmapRequest(byteAmount int, protection MemoryProtectionFlag, flags MemoryMapFlag) (MemoryMap, error) {
	m, err := platformMemmapRequest(byteAmount, protection, flags)
	if err == nil && len(m) > 0 {
		mmapRegistry.Store(uintptr(unsafe.Pointer(&m[0])), byteAmount)
	}
	return m, err
}

/*
MemmapUnmap releases a mapping created via MemmapRequest or MemmapRequestFromFile.
*/
func MemmapUnmap(memmap MemoryMap) error {
	err := platformMemmapUnmap(memmap)
	if err == nil && len(memmap) > 0 {
		mmapRegistry.Delete(uintptr(unsafe.Pointer(&memmap[0])))
	}
	return err
}

/*
MemmapProtect changes protection on an existing mapping.
*/
func MemmapProtect(memmap MemoryMap, protection MemoryProtectionFlag) error {
	return platformMemmapProtect(memmap, protection)
}

/*
MemmapLock locks the mapping's pages in RAM (mlock).
*/
func MemmapLock(memmap MemoryMap) error {
	return platformMemmapLock(memmap)
}

/*
MemmapUnlock unlocks pages previously locked with MemmapLock.
*/
func MemmapUnlock(memmap MemoryMap) error {
	return platformMemmapUnlock(memmap)
}

/*
MemmapLockAll locks or unlocks all mapped pages per flags (mlockall).
*/
func MemmapLockAll(flags MemoryLockAllFlag) error {
	return platformMemmapLockAll(flags)
}

/*
MemmapUnlockAll unlocks all pages locked via MemmapLockAll.
*/
func MemmapUnlockAll() error {
	return platformMemmapUnlockAll()
}

/*
MemmapAdvise applies madvise hints to a mapping.
*/
func MemmapAdvise(memmap MemoryMap, advice MemoryAdviceFlag) error {
	return platformMemmapAdvise(memmap, advice)
}

/*
MemmapSync flushes mapped pages to backing storage per syncFlags (msync).
*/
func MemmapSync(memmap MemoryMap, syncFlags MemorySyncFlag) error {
	return platformMemmapSync(memmap, syncFlags)
}

/*
MemmapRemap resizes an existing mapping in place when the platform supports it.

[Returns]
The new MemoryMap slice and nil error on success.

[Side Effects]
Updates mmapRegistry when the mapping base or size changes.
*/
func MemmapRemap(memmap MemoryMap, newSize int, flags MemoryRemapFlag) (MemoryMap, error) {
	m, err := platformMemmapRemap(memmap, newSize, flags)
	if err == nil && len(m) > 0 && len(memmap) > 0 {
		mmapRegistry.Delete(uintptr(unsafe.Pointer(&memmap[0])))
		mmapRegistry.Store(uintptr(unsafe.Pointer(&m[0])), newSize)
	}
	return m, err
}

/*
MemmapRemapAt resizes the mapping at addr, optionally moving it (MREMAP_MAYMOVE).

[Parameters]
addr - Base of the existing mapping.
oldSize, newSize - Previous and requested mapping sizes in bytes.

[Side Effects]
Updates mmapRegistry on success.
*/
func MemmapRemapAt(addr unsafe.Pointer, oldSize, newSize int, flags MemoryRemapFlag) (MemoryMap, error) {
	m, err := platformMemmapRemapAt(addr, oldSize, newSize, flags)
	if err == nil && len(m) > 0 {
		mmapRegistry.Delete(uintptr(addr))
		mmapRegistry.Store(uintptr(unsafe.Pointer(&m[0])), newSize)
	}
	return m, err
}

/*
MemmapRequestAt maps byteAmount bytes at a fixed addr (MAP_FIXED semantics on Unix).

[Returns]
addr on success when the platform returns the requested address.

[Side Effects]
Registers addr in mmapRegistry on success.
*/
func MemmapRequestAt(addr unsafe.Pointer, byteAmount int, protection MemoryProtectionFlag, flags MemoryMapFlag) (unsafe.Pointer, error) {
	mmap, err := platformMemmapRequestAt(addr, byteAmount, protection, flags)
	if err == nil {
		mmapRegistry.Store(uintptr(addr), byteAmount)
	}
	return mmap, err
}

/*
MemmapUnmapAt unmaps byteAmount bytes starting at addr.
*/
func MemmapUnmapAt(addr unsafe.Pointer, byteAmount int) error {
	err := platformMemmapUnmapAt(addr, byteAmount)
	if err == nil {
		mmapRegistry.Delete(uintptr(addr))
	}

	return err
}

/*
MemmapUnmapAllRegions forcibly unmaps every region still listed in mmapRegistry.

[Context]
Used after panic or abrupt benchmark teardown to avoid leaking mmap regions.

[Side Effects]
Iterates the registry and calls MemmapUnmapAt for each tracked base.
*/
func MemmapUnmapAllRegions() {
	mmapRegistry.Range(func(k, v any) bool {
		size := v.(int)
		_ = MemmapUnmapAt(unsafe.Pointer(k.(uintptr)), size)
		mmapRegistry.Delete(k)
		return true
	})
}

/*
MemmapPageSizeGet returns the system page size in bytes.

[Context]
Required for page-aligned file offsets when using MemmapRequestFromFile and MemmapAlignOffset.

[Returns]
Typically 4096 on common Linux systems; may differ on some ARM hosts. Always a power of two.

[Complexity]
Time: O(1). Space: O(1).

[Side Effects]
Pure; value is fixed for the process lifetime.
*/
func MemmapPageSizeGet() int {
	return platformMemmapPageSizeGet()
}

/*
MemmapAlignOffset rounds offset up to the next page boundary.

[Parameters]
offset - File offset in bytes; must be >= 0.

[Returns]
The smallest page-aligned offset >= offset.

[Complexity]
Time: O(1). Space: O(1).

[Example]

	pageSize := MemmapPageSizeGet()
	aligned := MemmapAlignOffset(5000) // 8192 when page size is 4096
*/
func MemmapAlignOffset(offset int64) int64 {
	pageSize := int64(MemmapPageSizeGet())
	mask := pageSize - 1
	return (offset + mask) &^ mask
}

/*
MemmapRequestFromFile maps length bytes from an open file at a page-aligned offset.

[Parameters]
fd - Valid OS file descriptor with permissions matching protection.
offset - Must be page-aligned (use MemmapAlignOffset).
length - Bytes to map; must be > 0.
protection, flags - mmap protection and MAP_* flags; must not include MAP_ANONYMOUS.

[Returns]
A MemoryMap over the file region.

[Errors]
Returns an error when length <= 0, MAP_ANONYMOUS is set, or the platform mmap fails.

[Side Effects]
Registers the mapping in mmapRegistry. The fd must stay open for the mapping lifetime.
*/
func MemmapRequestFromFile(fd int, offset int64, length int, protection MemoryProtectionFlag, flags MemoryMapFlag) (MemoryMap, error) {
	if length <= 0 {
		return nil, fmt.Errorf("invalid mapping length: %d", length)
	}

	if flags&MAP_ANONYMOUS != 0 {
		return nil, fmt.Errorf("file-backed mapping cannot use MAP_ANONYMOUS flag")
	}

	m, err := platformMemmapRequestFromFile(fd, offset, length, protection, flags)
	if err == nil && len(m) > 0 {
		mmapRegistry.Store(uintptr(unsafe.Pointer(&m[0])), length)
	}
	return m, err
}

/*
MemmapFileResize sets the file size to sizeBytes (ftruncate / platform equivalent).

[Context]
Call before MemmapRequestFromFile when creating or growing a file-backed store.

[Parameters]
fd - File descriptor opened for write (O_RDWR or O_WRONLY).
sizeBytes - New file size; truncates or zero-extends.

[Errors]
Returns an error when sizeBytes < 0 or the platform call fails.
*/
func MemmapFileResize(fd int, sizeBytes int64) error {
	if sizeBytes < 0 {
		return fmt.Errorf("invalid file size: %d (must be >= 0)", sizeBytes)
	}
	return platformMemmapFileResize(fd, sizeBytes)
}

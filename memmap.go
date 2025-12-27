// Package memcore provides low-level core infrastructure for memory management.
package memcore

import (
	"fmt"
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

/*
MemmapPageSizeGet returns the system page size in bytes.

The page size is typically 4096 bytes on most systems, but can vary (e.g., 65536 bytes
on some ARM systems). This value is required for aligning file offsets when creating
file-backed memory mappings.

Use cases:
- Aligning file offsets for mmap operations
- Calculating page-aligned buffer sizes
- Memory layout optimization

Time complexity: O(1) - cached or single system call
Space complexity: O(1) - no allocations

Prerequisites:
- None

Edge cases:
- Page size is constant for the lifetime of the process
- May vary between different systems or architectures
- Always a power of two

The returned value is suitable for use with MemmapAlignOffset to align file offsets.
*/
func MemmapPageSizeGet() int {
	return platformMemmapPageSizeGet()
}

/*
MemmapAlignOffset rounds a file offset up to the nearest page boundary.

File-backed memory mappings require offsets to be aligned to the system page size.
This function ensures proper alignment by rounding up to the nearest multiple of
the page size.

Use cases:
- Preparing file offsets for MemmapRequestFromFile
- Ensuring proper alignment for memory-mapped files
- Vector store file layout calculations

Time complexity: O(1) - simple bitwise arithmetic
Space complexity: O(1) - no allocations

Prerequisites:
- offset must be >= 0
- Page size must be a power of two (guaranteed by system)

Edge cases:
- Returns 0 if offset is 0
- Returns page size if offset is 1
- Handles large offsets correctly (up to int64 max)

Formula: aligned = (offset + pageSize - 1) &^ (pageSize - 1)

Example:

	pageSize := MemmapPageSizeGet() // 4096
	aligned := MemmapAlignOffset(5000) // returns 8192
*/
func MemmapAlignOffset(offset int64) int64 {
	pageSize := int64(MemmapPageSizeGet())
	mask := pageSize - 1
	return (offset + mask) &^ mask
}

/*
MemmapRequestFromFile maps a file-backed memory region.

The file descriptor must be valid and opened with appropriate permissions matching
the protection flags. For read-only mappings, open the file with O_RDONLY. For
read-write mappings, open the file with O_RDWR.

The offset must be page-aligned (use MemmapAlignOffset to ensure proper alignment).
The length determines how many bytes to map from the file starting at the offset.

Use cases:
- Zero-copy file I/O for vector stores
- Persistent data structures with O(1) access
- Database-like storage engines
- Large file processing without loading into RAM

Time complexity: O(1) - single system call
Space complexity: O(1) - no additional allocations beyond the mapping

Prerequisites:
- fd must be a valid file descriptor from os.File.Fd()
- offset must be page-aligned (use MemmapAlignOffset)
- length must be > 0
- file must be opened with permissions matching the protection flags
- flags must NOT include MAP_ANONYMOUS (file-backed mappings cannot be anonymous)
- file must be large enough to contain offset+length bytes

Edge cases:
- Returns error if file is too small for offset+length
- MAP_SHARED: writes are visible to other processes and persisted to disk
- MAP_PRIVATE: creates copy-on-write mapping, changes not visible to others
- File descriptor must remain open for the lifetime of the mapping
- Unmapping does not close the file descriptor (caller responsibility)

The mapped region is automatically registered in the mmap registry and can be used
with all existing memcore functions (MemmapSync, MemmapUnmap, etc.).
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
MemmapFileResize sets the size of a file to the specified number of bytes.

This function is required before mapping a file with MemmapRequestFromFile if the file
does not already exist or is smaller than the desired mapping size. The file must be
opened with write permissions (O_RDWR or O_WRONLY).

Use cases:
- Initializing new files for memory-mapped vector stores
- Pre-allocating space for persistent data structures
- Resizing existing mapped files before remapping

Time complexity: O(1) - single system call
Space complexity: O(1) - no allocations

Prerequisites:
- fd must be a valid file descriptor from os.File.Fd()
- file must be opened with write permissions (O_RDWR or O_WRONLY)
- sizeBytes must be >= 0

Edge cases:
- If sizeBytes is smaller than current file size, file is truncated
- If sizeBytes is larger than current file size, file is extended (filled with zeros)
- On Unix, uses ftruncate(2)
- On Windows, uses Ftruncate which internally uses SetFilePointer + SetEndOfFile
- Returns error if file descriptor is invalid or lacks write permissions

The file size must be set before calling MemmapRequestFromFile with a length
that exceeds the current file size.
*/
func MemmapFileResize(fd int, sizeBytes int64) error {
	if sizeBytes < 0 {
		return fmt.Errorf("invalid file size: %d (must be >= 0)", sizeBytes)
	}
	return platformMemmapFileResize(fd, sizeBytes)
}

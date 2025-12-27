//go:build windows

package memcore

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// platformMemmapRequest creates an anonymous read/write mapping.
// Equivalent to unix.Mmap(-1, 0, size, prot, flags).
func platformMemmapRequest(byteAmount int, protection MemoryProtectionFlag, flags MemoryMapFlag) (MemoryMap, error) {
	if byteAmount <= 0 {
		return nil, fmt.Errorf("invalid allocation size: %d", byteAmount)
	}

	// Windows protection and access constants
	var pageProt uint32
	var fileAccess uint32

	switch protection {
	case PROT_READ:
		pageProt = windows.PAGE_READONLY
		fileAccess = windows.FILE_MAP_READ
	case PROT_WRITE, PROT_READWRITE:
		pageProt = windows.PAGE_READWRITE
		fileAccess = windows.FILE_MAP_WRITE | windows.FILE_MAP_READ
	case PROT_EXEC:
		pageProt = windows.PAGE_EXECUTE_READ
		fileAccess = windows.FILE_MAP_READ | windows.FILE_MAP_EXECUTE
	case PROT_ALL:
		pageProt = windows.PAGE_EXECUTE_READWRITE
		fileAccess = windows.FILE_MAP_READ | windows.FILE_MAP_WRITE | windows.FILE_MAP_EXECUTE
	default:
		pageProt = windows.PAGE_READWRITE
		fileAccess = windows.FILE_MAP_WRITE | windows.FILE_MAP_READ
	}

	// Create an anonymous file mapping in the system paging file
	h, err := windows.CreateFileMapping(windows.InvalidHandle, nil, pageProt, 0, uint32(byteAmount), nil)
	if err != nil {
		return nil, fmt.Errorf("CreateFileMapping failed: %w", err)
	}
	defer windows.CloseHandle(h)

	addr, err := windows.MapViewOfFile(h, fileAccess, 0, 0, uintptr(byteAmount))
	if err != nil {
		return nil, fmt.Errorf("MapViewOfFile failed: %w", err)
	}

	// Slice over the mapped memory
	data := unsafe.Slice((*byte)(unsafe.Pointer(addr)), byteAmount)
	return MemoryMap(data), nil
}

// platformMemmapUnmap unmaps the mapped region.
func platformMemmapUnmap(memmap MemoryMap) error {
	if len(memmap) == 0 {
		return nil
	}
	addr := uintptr(unsafe.Pointer(&memmap[0]))
	if err := windows.UnmapViewOfFile(addr); err != nil {
		return fmt.Errorf("UnmapViewOfFile failed: %w", err)
	}
	return nil
}

// Memory protection changes — Windows uses VirtualProtect.
func platformMemmapProtect(memmap MemoryMap, protection MemoryProtectionFlag) error {
	if len(memmap) == 0 {
		return nil
	}

	var newProt uint32
	switch protection {
	case PROT_READ:
		newProt = windows.PAGE_READONLY
	case PROT_WRITE, PROT_READWRITE:
		newProt = windows.PAGE_READWRITE
	case PROT_EXEC:
		newProt = windows.PAGE_EXECUTE_READ
	case PROT_ALL:
		newProt = windows.PAGE_EXECUTE_READWRITE
	default:
		newProt = windows.PAGE_READWRITE
	}

	var oldProt uint32
	base := uintptr(unsafe.Pointer(&memmap[0]))
	if err := windows.VirtualProtect(base, uintptr(len(memmap)), newProt, &oldProt); err != nil {
		return fmt.Errorf("VirtualProtect failed: %w", err)
	}
	return nil
}

// Lock/unlock memory with VirtualLock / VirtualUnlock.
func platformMemmapLock(memmap MemoryMap) error {
	if len(memmap) == 0 {
		return nil
	}
	if err := windows.VirtualLock(uintptr(unsafe.Pointer(&memmap[0])), uintptr(len(memmap))); err != nil {
		return fmt.Errorf("VirtualLock failed: %w", err)
	}
	return nil
}

func platformMemmapUnlock(memmap MemoryMap) error {
	if len(memmap) == 0 {
		return nil
	}
	if err := windows.VirtualUnlock(uintptr(unsafe.Pointer(&memmap[0])), uintptr(len(memmap))); err != nil {
		return fmt.Errorf("VirtualUnlock failed: %w", err)
	}
	return nil
}

// Windows has no global mlockall equivalent — no-op.
func platformMemmapLockAll(flags MemoryLockAllFlag) error                  { return nil }
func platformMemmapUnlockAll() error                                       { return nil }
func platformMemmapAdvise(memmap MemoryMap, advice MemoryAdviceFlag) error { return nil }
func platformMemmapSync(memmap MemoryMap, syncFlags MemorySyncFlag) error  { return nil }

// Remap is simulated via unmap + map new region + copy
func platformMemmapRemap(memmap MemoryMap, newSize int, flags MemoryRemapFlag) (MemoryMap, error) {
	if newSize <= 0 {
		return nil, fmt.Errorf("invalid new size: %d", newSize)
	}

	newMap, err := platformMemmapRequest(newSize, PROT_READWRITE, MAP_ANON_PRIVATE)
	if err != nil {
		return nil, err
	}
	copy(newMap, memmap)
	_ = platformMemmapUnmap(memmap)
	return newMap, nil
}

func platformMemmapRemapAt(addr unsafe.Pointer, oldSize, newSize int, flags MemoryRemapFlag) (MemoryMap, error) {
	if newSize <= 0 {
		return nil, fmt.Errorf("invalid new size: %d", newSize)
	}

	newMap, err := platformMemmapRequest(newSize, PROT_READWRITE, MAP_ANON_PRIVATE)
	if err != nil {
		return nil, err
	}

	oldSlice := unsafe.Slice((*byte)(addr), oldSize)
	copy(newMap, oldSlice)

	if err := platformMemmapUnmap(oldSlice); err != nil {
		return nil, err
	}

	return newMap, nil
}

// Fixed address mapping equivalents (rarely used on Windows)
func platformMemmapRequestAt(addr unsafe.Pointer, byteAmount int, protection MemoryProtectionFlag, flags MemoryMapFlag) (unsafe.Pointer, error) {
	// Windows doesn’t allow user-specified addresses for anonymous mappings easily.
	// We just allocate normally.
	m, err := platformMemmapRequest(byteAmount, protection, flags)
	if err != nil {
		return nil, err
	}
	return unsafe.Pointer(&m[0]), nil
}

func platformMemmapUnmapAt(addr unsafe.Pointer, byteAmount int) error {
	if err := windows.UnmapViewOfFile(uintptr(addr)); err != nil {
		return fmt.Errorf("UnmapViewOfFile failed: %w", err)
	}
	return nil
}

func platformMemmapRequestFromFile(fd int, offset int64, length int, protection MemoryProtectionFlag, flags MemoryMapFlag) (MemoryMap, error) {
	if length <= 0 {
		return nil, fmt.Errorf("invalid mapping length: %d", length)
	}

	// Convert file descriptor to Windows handle
	fileHandle := windows.Handle(fd)

	// Map protection flags to Windows page protection constants
	var pageProt uint32
	var fileAccess uint32

	switch protection {
	case PROT_READ:
		pageProt = windows.PAGE_READONLY
		fileAccess = windows.FILE_MAP_READ
	case PROT_WRITE, PROT_READWRITE:
		pageProt = windows.PAGE_READWRITE
		fileAccess = windows.FILE_MAP_WRITE | windows.FILE_MAP_READ
	case PROT_EXEC:
		pageProt = windows.PAGE_EXECUTE_READ
		fileAccess = windows.FILE_MAP_READ | windows.FILE_MAP_EXECUTE
	case PROT_ALL:
		pageProt = windows.PAGE_EXECUTE_READWRITE
		fileAccess = windows.FILE_MAP_READ | windows.FILE_MAP_WRITE | windows.FILE_MAP_EXECUTE
	default:
		pageProt = windows.PAGE_READWRITE
		fileAccess = windows.FILE_MAP_WRITE | windows.FILE_MAP_READ
	}

	// Calculate high and low DWORDs for offset
	offsetHigh := uint32(offset >> 32)
	offsetLow  := uint32(offset & 0xFFFFFFFF)

	// Create file mapping object
	// Passing 0,0 for maxSize uses the actual file size
	h, err := windows.CreateFileMapping(fileHandle, nil, pageProt, 0, 0, nil)
	if err != nil {
		return nil, fmt.Errorf("CreateFileMapping failed: %w", err)
	}
	defer windows.CloseHandle(h)

	// Map view of file with specified offset
	addr, err := windows.MapViewOfFile(h, fileAccess, offsetHigh, offsetLow, uintptr(length))
	if err != nil {
		return nil, fmt.Errorf("MapViewOfFile failed: %w", err)
	}

	// Create slice over the mapped memory
	data := unsafe.Slice((*byte)(unsafe.Pointer(addr)), length)
	return MemoryMap(data), nil
}

func platformMemmapPageSizeGet() int {
	return windows.Getpagesize()
}

func platformMemmapFileResize(fd int, sizeBytes int64) error {
	fileHandle := windows.Handle(fd)
	if err := windows.Ftruncate(fileHandle, sizeBytes); err != nil {
		return fmt.Errorf("Ftruncate failed: %w", err)
	}
	return nil
}

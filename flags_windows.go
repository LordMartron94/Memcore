//go:build windows

package memcore

/*
Windows definitions for mmap-related flag types used by the public memcore API.

[Context]
Constants mirror Unix names where possible; platformMemmap* in memmap_windows.go maps them to
VirtualAlloc, VirtualProtect, and related Win32 calls.
*/
type (
	MemoryProtectionFlag int
	MemoryMapFlag        int
	MemoryAdviceFlag     int
	MemorySyncFlag       int
	MemoryLockAllFlag    int
	MemoryRemapFlag      int
)

const (
	PROT_NONE      MemoryProtectionFlag = 0
	PROT_READ      MemoryProtectionFlag = 1
	PROT_WRITE     MemoryProtectionFlag = 2
	PROT_EXEC      MemoryProtectionFlag = 4
	PROT_READWRITE MemoryProtectionFlag = PROT_READ | PROT_WRITE
	PROT_ALL       MemoryProtectionFlag = PROT_READ | PROT_WRITE | PROT_EXEC

	MAP_PRIVATE      MemoryMapFlag = 1
	MAP_ANONYMOUS    MemoryMapFlag = 2
	MAP_SHARED       MemoryMapFlag = 4
	MAP_FIXED        MemoryMapFlag = 8
	MAP_ANON         MemoryMapFlag = MAP_ANONYMOUS
	MAP_ANON_PRIVATE MemoryMapFlag = MAP_PRIVATE | MAP_ANONYMOUS
	MAP_ANON_SHARED  MemoryMapFlag = MAP_SHARED | MAP_ANONYMOUS

	MS_ASYNC      MemorySyncFlag = 1
	MS_SYNC       MemorySyncFlag = 4
	MS_INVALIDATE MemorySyncFlag = 2

	// Remap flag values are not used by the kernel on Windows; memmap_windows.go emulates mremap.
	MREMAP_MAYMOVE   MemoryRemapFlag = 1
	MREMAP_FIXED     MemoryRemapFlag = 2
	MREMAP_DONTUNMAP MemoryRemapFlag = 4
)

//go:build windows

package memcore

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

	// Dummy remap flags
	MREMAP_MAYMOVE   MemoryRemapFlag = 1
	MREMAP_FIXED     MemoryRemapFlag = 2
	MREMAP_DONTUNMAP MemoryRemapFlag = 4
)

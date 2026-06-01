//go:build darwin

package memcore

import "golang.org/x/sys/unix"

/*
MemoryProtectionFlag mirrors mmap PROT_* protection bits on Darwin.
*/
type MemoryProtectionFlag int

/*
MemoryMapFlag mirrors mmap MAP_* flags on Darwin.
*/
type MemoryMapFlag int

/*
MemoryAdviceFlag mirrors madvise MADV_* flags on Darwin.
*/
type MemoryAdviceFlag int

/*
MemorySyncFlag mirrors msync MS_* flags on Darwin.
*/
type MemorySyncFlag int

/*
MemoryLockAllFlag mirrors mlockall MCL_* flags on Darwin.
*/
type MemoryLockAllFlag int

/*
MemoryRemapFlag is accepted by MemmapRemap APIs; Darwin emulates resize in memmap_darwin.go.
*/
type MemoryRemapFlag int

const (
	PROT_NONE      MemoryProtectionFlag = unix.PROT_NONE
	PROT_READ      MemoryProtectionFlag = unix.PROT_READ
	PROT_WRITE     MemoryProtectionFlag = unix.PROT_WRITE
	PROT_EXEC      MemoryProtectionFlag = unix.PROT_EXEC
	PROT_READWRITE MemoryProtectionFlag = PROT_READ | PROT_WRITE
	PROT_ALL       MemoryProtectionFlag = PROT_READ | PROT_WRITE | PROT_EXEC
)

const (
	MAP_SHARED       MemoryMapFlag = unix.MAP_SHARED
	MAP_PRIVATE      MemoryMapFlag = unix.MAP_PRIVATE
	MAP_FIXED        MemoryMapFlag = unix.MAP_FIXED
	MAP_ANONYMOUS    MemoryMapFlag = unix.MAP_ANON
	MAP_NORESERVE    MemoryMapFlag = unix.MAP_NORESERVE
	MAP_ANON         MemoryMapFlag = MAP_ANONYMOUS
	MAP_ANON_PRIVATE MemoryMapFlag = MAP_PRIVATE | MAP_ANONYMOUS
	MAP_ANON_SHARED  MemoryMapFlag = MAP_SHARED | MAP_ANONYMOUS
)

const (
	MADV_NORMAL     MemoryAdviceFlag = unix.MADV_NORMAL
	MADV_RANDOM     MemoryAdviceFlag = unix.MADV_RANDOM
	MADV_SEQUENTIAL MemoryAdviceFlag = unix.MADV_SEQUENTIAL
	MADV_WILLNEED   MemoryAdviceFlag = unix.MADV_WILLNEED
	MADV_DONTNEED   MemoryAdviceFlag = unix.MADV_DONTNEED
	MADV_FREE       MemoryAdviceFlag = unix.MADV_FREE
	MADV_PAGEOUT    MemoryAdviceFlag = unix.MADV_PAGEOUT
)

const (
	MS_ASYNC      MemorySyncFlag = unix.MS_ASYNC
	MS_SYNC       MemorySyncFlag = unix.MS_SYNC
	MS_INVALIDATE MemorySyncFlag = unix.MS_INVALIDATE
)

const (
	MCL_CURRENT MemoryLockAllFlag = unix.MCL_CURRENT
	MCL_FUTURE  MemoryLockAllFlag = unix.MCL_FUTURE
)

/*
Memory remap flags accepted by MemmapRemap; Darwin implements resize in memmap_darwin.go.
*/
const (
	MREMAP_MAYMOVE   MemoryRemapFlag = 1
	MREMAP_FIXED     MemoryRemapFlag = 2
	MREMAP_DONTUNMAP MemoryRemapFlag = 4
)

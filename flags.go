package memcore

import "golang.org/x/sys/unix"

// MemoryProtectionFlag represents mmap protection flags (PROT_*).
type MemoryProtectionFlag int

// MemoryMapFlag represents mmap mapping flags (MAP_*).
type MemoryMapFlag int

// MemoryAdviceFlag represents madvise(2) flags.
type MemoryAdviceFlag int

// MemorySyncFlag represents msync(2) flags.
type MemorySyncFlag int

// MemoryLockAllFlag represents mlockall(2) flags.
type MemoryLockAllFlag int

// MemoryRemapFlag represents mremap(2) flags (MREMAP_*).
type MemoryRemapFlag int

// Protection flags (mmap PROT_*). These determine the permitted operations
// on a mapped memory region. They correspond directly to the PROT_* flags
// accepted by mmap(2) and mprotect(2).
const (
	// PROT_NONE marks pages as inaccessible. Any access will result in a SIGSEGV.
	PROT_NONE MemoryProtectionFlag = unix.PROT_NONE

	// PROT_READ allows pages to be read.
	PROT_READ MemoryProtectionFlag = unix.PROT_READ

	// PROT_WRITE allows pages to be written. Must usually be combined with PROT_READ.
	PROT_WRITE MemoryProtectionFlag = unix.PROT_WRITE

	// PROT_EXEC allows execution of instructions in the mapped region.
	PROT_EXEC MemoryProtectionFlag = unix.PROT_EXEC

	// PROT_READWRITE is a convenience alias for PROT_READ|PROT_WRITE.
	PROT_READWRITE MemoryProtectionFlag = PROT_READ | PROT_WRITE

	// PROT_ALL enables read, write, and execute access.
	PROT_ALL MemoryProtectionFlag = PROT_READ | PROT_WRITE | PROT_EXEC
)

// Memory flags (mmap MAP_*). These control how a mapping is created and behaves.
const (
	// MAP_SHARED creates a mapping that is shared with other processes mapping the same
	// file. Writes are visible to others and may be written back to the file.
	MAP_SHARED MemoryMapFlag = unix.MAP_SHARED

	// MAP_PRIVATE creates a copy-on-write mapping. Writes are private and not visible to
	// other mappings of the same file and are not written back to disk.
	MAP_PRIVATE MemoryMapFlag = unix.MAP_PRIVATE

	// MAP_SHARED_VALIDATE is like MAP_SHARED but enforces stricter checking of flags.
	// Available on newer Linux kernels.
	MAP_SHARED_VALIDATE MemoryMapFlag = unix.MAP_SHARED_VALIDATE

	// MAP_FIXED places the mapping exactly at the requested address. Any existing mapping
	// in the range is unmapped. Use with caution.
	MAP_FIXED MemoryMapFlag = unix.MAP_FIXED

	// MAP_FIXED_NOREPLACE is like MAP_FIXED but fails if the requested address range is
	// already mapped, instead of unmapping it. Available on newer Linux kernels.
	MAP_FIXED_NOREPLACE MemoryMapFlag = unix.MAP_FIXED_NOREPLACE

	// MAP_ANONYMOUS creates an anonymous mapping not backed by any file. The fd argument
	// to mmap must be -1.
	MAP_ANONYMOUS MemoryMapFlag = unix.MAP_ANON

	// MAP_DENYWRITE was intended to prevent writes to the underlying file; ignored on Linux.
	MAP_DENYWRITE MemoryMapFlag = 0x800

	// MAP_GROWSDOWN marks the mapping as a stack, growing downward.
	MAP_GROWSDOWN MemoryMapFlag = unix.MAP_GROWSDOWN

	// MAP_LOCKED locks pages into RAM, preventing them from being swapped out.
	MAP_LOCKED MemoryMapFlag = unix.MAP_LOCKED

	// MAP_NORESERVE prevents the kernel from reserving swap space for the mapping.
	MAP_NORESERVE MemoryMapFlag = unix.MAP_NORESERVE

	// MAP_POPULATE pre-faults pages in the mapping, populating page tables immediately.
	MAP_POPULATE MemoryMapFlag = unix.MAP_POPULATE

	// MAP_NONBLOCK performs MAP_POPULATE asynchronously, without blocking.
	MAP_NONBLOCK MemoryMapFlag = unix.MAP_NONBLOCK

	// MAP_STACK indicates that the mapping is a stack. The kernel may apply special
	// behavior (such as guard pages) to stack mappings.
	MAP_STACK MemoryMapFlag = unix.MAP_STACK

	// MAP_HUGETLB allocates the mapping using huge pages. The address and length must
	// be huge-page aligned. Requires appropriate kernel configuration.
	MAP_HUGETLB MemoryMapFlag = unix.MAP_HUGETLB

	// MAP_SYNC enables write ordering between the mapping and underlying storage for DAX
	// or shared memory files. Typically used with MAP_SHARED_VALIDATE.
	MAP_SYNC MemoryMapFlag = unix.MAP_SYNC

	// MAP_UNINITIALIZED is a nonstandard arm64 extension that allows mapping of
	// uninitialized memory. It is generally unsafe and should not be used.
	MAP_UNINITIALIZED MemoryMapFlag = 0x4000000

	// MAP_ANON_PRIVATE creates a private anonymous mapping (the most common kind).
	// Equivalent to MAP_PRIVATE | MAP_ANONYMOUS.
	MAP_ANON_PRIVATE MemoryMapFlag = MAP_PRIVATE | MAP_ANONYMOUS

	// MAP_ANON_SHARED creates an anonymous mapping that is shared between processes,
	// e.g. for shared memory segments. Equivalent to MAP_SHARED | MAP_ANONYMOUS.
	MAP_ANON_SHARED MemoryMapFlag = MAP_SHARED | MAP_ANONYMOUS

	// MAP_ANON_POPULATE creates a private anonymous mapping and immediately populates
	// page tables. Equivalent to MAP_PRIVATE | MAP_ANONYMOUS | MAP_POPULATE.
	MAP_ANON_POPULATE MemoryMapFlag = MAP_PRIVATE | MAP_ANONYMOUS | MAP_POPULATE

	// MAP_SHARED_POPULATE creates a shared file-backed mapping with prefaulting.
	// Equivalent to MAP_SHARED | MAP_POPULATE.
	MAP_SHARED_POPULATE MemoryMapFlag = MAP_SHARED | MAP_POPULATE

	// MAP_ANON_HUGEPAGE creates a private anonymous mapping backed by huge pages.
	// Useful for large arenas where hugepage support is enabled. Requires hugepage alignment.
	MAP_ANON_HUGEPAGE MemoryMapFlag = MAP_PRIVATE | MAP_ANONYMOUS | MAP_HUGETLB

	// MAP_ANON_LOCKED creates a locked anonymous private mapping that is pinned in RAM
	// (requires CAP_IPC_LOCK or appropriate limits). Equivalent to MAP_PRIVATE | MAP_ANONYMOUS | MAP_LOCKED.
	MAP_ANON_LOCKED MemoryMapFlag = MAP_PRIVATE | MAP_ANONYMOUS | MAP_LOCKED
)

// Memory advice flags (for madvise(2)). These provide the kernel with hints about
// expected memory usage patterns, which can influence paging and caching behavior.
const (
	// MADV_NORMAL specifies the default kernel paging behavior.
	MADV_NORMAL MemoryAdviceFlag = unix.MADV_NORMAL

	// MADV_RANDOM advises the kernel that page references are likely to be random,
	// disabling read-ahead.
	MADV_RANDOM MemoryAdviceFlag = unix.MADV_RANDOM

	// MADV_SEQUENTIAL advises the kernel that pages will be accessed sequentially,
	// enabling aggressive read-ahead and faster reclamation of previous pages.
	MADV_SEQUENTIAL MemoryAdviceFlag = unix.MADV_SEQUENTIAL

	// MADV_WILLNEED advises the kernel that the pages will be needed soon. The kernel
	// may prefetch them.
	MADV_WILLNEED MemoryAdviceFlag = unix.MADV_WILLNEED

	// MADV_DONTNEED advises the kernel that the pages are no longer needed. The kernel
	// can reclaim them immediately.
	MADV_DONTNEED MemoryAdviceFlag = unix.MADV_DONTNEED

	// MADV_FREE marks pages as lazily reclaimable. They can be discarded under memory
	// pressure without being written back.
	MADV_FREE MemoryAdviceFlag = unix.MADV_FREE

	// MADV_REMOVE attempts to free up memory and associated backing store, like punching
	// a hole in the file for file-backed mappings.
	MADV_REMOVE MemoryAdviceFlag = unix.MADV_REMOVE

	// MADV_DONTFORK prevents the mapping from being inherited by child processes after fork().
	MADV_DONTFORK MemoryAdviceFlag = unix.MADV_DONTFORK

	// MADV_DOFORK restores the default behavior of inheriting the mapping across fork().
	MADV_DOFORK MemoryAdviceFlag = unix.MADV_DOFORK

	// MADV_HWPOISON injects a hardware memory error on the given pages, simulating a failure.
	MADV_HWPOISON MemoryAdviceFlag = unix.MADV_HWPOISON

	// MADV_MERGEABLE enables Kernel Samepage Merging (KSM) for the mapping, allowing
	// identical pages to be deduplicated.
	MADV_MERGEABLE MemoryAdviceFlag = unix.MADV_MERGEABLE

	// MADV_UNMERGEABLE disables KSM for the mapping.
	MADV_UNMERGEABLE MemoryAdviceFlag = unix.MADV_UNMERGEABLE

	// MADV_HUGEPAGE enables transparent huge pages for this region (if supported).
	MADV_HUGEPAGE MemoryAdviceFlag = unix.MADV_HUGEPAGE

	// MADV_NOHUGEPAGE disables transparent huge pages for this region.
	MADV_NOHUGEPAGE MemoryAdviceFlag = unix.MADV_NOHUGEPAGE

	// MADV_DONTDUMP excludes the mapping from core dumps.
	MADV_DONTDUMP MemoryAdviceFlag = unix.MADV_DONTDUMP

	// MADV_DODUMP includes the mapping in core dumps (default).
	MADV_DODUMP MemoryAdviceFlag = unix.MADV_DODUMP

	// MADV_WIPEONFORK causes pages to be zeroed in child processes after fork().
	MADV_WIPEONFORK MemoryAdviceFlag = unix.MADV_WIPEONFORK

	// MADV_KEEPONFORK disables MADV_WIPEONFORK, preserving memory contents after fork().
	MADV_KEEPONFORK MemoryAdviceFlag = unix.MADV_KEEPONFORK
)

// MemorySyncFlag represents msync(2) flags, which control how changes to a mapping
// are synchronized with the underlying file.
const (
	// MS_ASYNC schedules writes back to storage asynchronously and returns immediately.
	MS_ASYNC MemorySyncFlag = unix.MS_ASYNC

	// MS_SYNC writes changes back to storage synchronously before returning.
	MS_SYNC MemorySyncFlag = unix.MS_SYNC

	// MS_INVALIDATE invalidates cached pages, forcing them to be re-read from storage
	// on the next access.
	MS_INVALIDATE MemorySyncFlag = unix.MS_INVALIDATE
)

// MemoryLockAllFlag represents flags for mlockall(2).
const (
	// MCL_CURRENT locks all pages currently mapped into the calling process's address space.
	MCL_CURRENT MemoryLockAllFlag = unix.MCL_CURRENT

	// MCL_FUTURE causes all future mappings to be locked automatically.
	MCL_FUTURE MemoryLockAllFlag = unix.MCL_FUTURE

	// MCL_ONFAULT locks pages on first access rather than immediately. Linux 4.4+.
	MCL_ONFAULT MemoryLockAllFlag = unix.MCL_ONFAULT
)

// MemoryRemapFlag represents mremap(2) flags, controlling how an existing mapping is resized.
const (
	// MREMAP_MAYMOVE allows the kernel to relocate the mapping to a new address if
	// in-place expansion is not possible.
	MREMAP_MAYMOVE MemoryRemapFlag = unix.MREMAP_MAYMOVE

	// MREMAP_FIXED moves the mapping to the address specified in the mremap call.
	// Must be used together with MREMAP_MAYMOVE.
	MREMAP_FIXED MemoryRemapFlag = unix.MREMAP_FIXED

	// MREMAP_DONTUNMAP (Linux ≥ 5.7) keeps the old mapping around after relocation.
	// Both the old and new addresses then refer to the same memory until explicitly unmapped.
	MREMAP_DONTUNMAP MemoryRemapFlag = unix.MREMAP_DONTUNMAP
)

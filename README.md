# memcore

Low-level core infrastructure for manual memory management in Go.

## Overview

`memcore` provides the foundational primitives for systems programming with manual memory control, bypassing Go's garbage collector. It offers platform-agnostic access to memory mapping operations, a region-based pointer system (`MarkRaw`), and utilities for safe manipulation of manually managed memory.

## Core Concepts

### Memory Mapping

`memcore` wraps platform-specific memory mapping operations (`mmap`/`VirtualAlloc`) to allocate raw memory regions outside the Go heap. This memory:

- Never triggers garbage collection
- Can be precisely controlled (protection, synchronization, locking)
- Must never contain Go pointers (to avoid GC corruption)
- Is tracked in an internal registry for safety

### Mark System

Instead of raw pointers, `memcore` uses `MarkRaw` values that reference memory through a region ID and offset:

```go
type MarkRaw struct {
    regionID uint32
    offset   uintptr
}
```

This design enables:
- Safe relocation of memory regions
- Snapshot/restore capabilities
- Region-based memory management
- Zero-cost abstraction (marked as `//go:inline`)

### Regions

Memory regions are registered with `memcore` to enable the mark system:
- `MemcoreRegionRegister` - Register a memory region and get a region ID
- `MemcoreRegionUnregister` - Unregister a region
- `MemcoreRegionBaseUpdate` - Update a region's base address after remapping

## Key Features

### Memory Mapping Operations

- **`MemmapRequest`** - Map anonymous memory with specified protection and flags
- **`MemmapRequestFromFile`** - Map a file-backed memory region for zero-copy access
- **`MemmapUnmap`** - Unmap a memory region
- **`MemmapProtect`** - Change protection flags (read/write/execute)
- **`MemmapLock`/`MemmapUnlock`** - Lock pages in RAM (prevent swapping)
- **`MemmapLockAll`/`MemmapUnlockAll`** - Lock/unlock all process memory
- **`MemmapAdvise`** - Provide hints to the kernel about memory usage patterns
- **`MemmapSync`** - Synchronize memory with backing store (for file-backed mappings)
- **`MemmapRemap`** - Resize an existing mapping (using `mremap` on Linux)
- **`MemmapRequestAt`** - Map memory at a specific address (with `MAP_FIXED`)
- **`MemmapPageSizeGet`** - Get the system page size (required for file offset alignment)
- **`MemmapAlignOffset`** - Align a file offset to the nearest page boundary
- **`MemmapFileResize`** - Set or resize a file to a specific size (required before mapping new files)

### Mark Operations

- **`MemcoreMarkCreate`** - Create a mark from region ID and offset
- **`MemcoreMarkOffsetFrom`** - Create a mark relative to another mark
- **`MemcoreMarkAlignedOffsetFrom`** - Create an aligned mark relative to another
- **`MemcoreMarkDereference`** - Convert a mark to `unsafe.Pointer`
- **`MemcoreMarkDereferenceObject`** - Dereference a mark as a typed object pointer

### Memory Utilities

- **`SizeOf[T]()`** - Get the size of any type
- **`AlignOf[T]()`** - Get the alignment requirement of any type
- **`AlignUp`** - Round up a value to the nearest multiple (power-of-two alignment)
- **`NextPowerOfTwo`** - Get the next power of two (useful for alignment)
- **`MemoryMoveNoHeapPointers`** - Move memory without triggering GC scans
- **`MemoryClear`** - Zero out a memory region
- **`MemoryCompare`** - Compare two memory regions

### Type Handling

- **`MemcoreTypeRetrieve[T]()`** - Get a stable type ID for a Go type
- Used for storing type information without keeping Go pointers

### Prefetching

- **`MemoryPrefetch`** - Prefetch memory into CPU cache (platform-specific)
- Optimized implementations for AMD64 with AVX2 support

## Constants

Useful size constants are provided:
- `Byte`, `KiloByte`, `MegaByte`, `GigaByte`, `TeraByte`
- `PetaByte` (for completeness)

## Safety Guidelines

⚠️ **Critical Warnings:**

1. **No Go Pointers**: Manually managed memory must never contain Go pointers. This will corrupt the garbage collector and cause undefined behavior.

2. **Region Lifetime**: Marks become invalid if their region is unmapped or remapped. Always ensure regions outlive their marks.

3. **Thread Safety**: Memory mapping operations are not thread-safe. Coordinate access appropriately.

4. **Platform Differences**: Some operations (like `mremap`) are Linux-specific. Use platform-specific code paths when necessary.

## Example Usage

### Anonymous Memory Mapping

```go
import "memcore"

// Map 1MB of anonymous memory
memory, err := memcore.MemmapRequest(
    int(memcore.MegaByte),
    memcore.PROT_READ|memcore.PROT_WRITE,
    memcore.MAP_ANONYMOUS|memcore.MAP_PRIVATE,
)
if err != nil {
    panic(err)
}
defer memcore.MemmapUnmap(memory)

// Register the region
regionID := memcore.MemcoreRegionRegister(
    uintptr(unsafe.Pointer(&memory[0])),
    uint64(len(memory)),
)

// Create a mark to the start of the region
mark := memcore.MemcoreMarkCreate(regionID, 0)

// Dereference as a pointer
ptr := memcore.MemcoreMarkDereference(mark)

// Use the memory...
```

### File-Backed Memory Mapping

File-backed mappings enable zero-copy, O(1) access to binary files, essential for vector stores and persistent data structures:

**Initializing a new file for mapping:**

```go
import (
    "os"
    "memcore"
    "unsafe"
)

// Create or open file for read-write access
file, err := os.OpenFile("vector_store.bin", os.O_RDWR|os.O_CREATE, 0644)
if err != nil {
    panic(err)
}
defer file.Close()

// Initialize file to desired size (e.g., 100MB for vector store)
desiredSize := int64(100 * memcore.MegaByte)
if err := memcore.MemmapFileResize(int(file.Fd()), desiredSize); err != nil {
    panic(err)
}

// Align offset to page boundary
pageSize := memcore.MemmapPageSizeGet()
alignedOffset := memcore.MemmapAlignOffset(0)

// Map the file into memory
fileMap, err := memcore.MemmapRequestFromFile(
    int(file.Fd()),
    alignedOffset,
    int(desiredSize) - int(alignedOffset),
    memcore.PROT_READWRITE,
    memcore.MAP_SHARED, // Changes are visible to other processes and persisted
)
if err != nil {
    panic(err)
}
defer memcore.MemmapUnmap(fileMap)

// Register as a region for use with memstruct
regionID := memcore.MemcoreRegionRegister(
    uintptr(unsafe.Pointer(&fileMap[0])),
    uint64(len(fileMap)),
)

// Create a mark to the start of the mapped file
baseMark := memcore.MemcoreMarkCreate(regionID, 0)

// Access data structures directly in the file (zero-copy, O(1))
// Changes are automatically synced to disk with MAP_SHARED

// Explicitly sync changes to disk when needed
if err := memcore.MemmapSync(fileMap, memcore.MS_SYNC); err != nil {
    panic(err)
}
```

**Opening an existing file:**

```go
// Open existing file for read-write access
file, err := os.OpenFile("vector_store.bin", os.O_RDWR, 0644)
if err != nil {
    panic(err)
}
defer file.Close()

// Get file size and align offset to page boundary
fileInfo, _ := file.Stat()
fileSize := int(fileInfo.Size())
pageSize := memcore.MemmapPageSizeGet()
alignedOffset := memcore.MemmapAlignOffset(0)

// Map the file into memory
fileMap, err := memcore.MemmapRequestFromFile(
    int(file.Fd()),
    alignedOffset,
    fileSize - int(alignedOffset),
    memcore.PROT_READWRITE,
    memcore.MAP_SHARED,
)
if err != nil {
    panic(err)
}
defer memcore.MemmapUnmap(fileMap)

// Use the mapped file...
```

**Important Notes:**
- File offsets must be page-aligned (use `MemmapAlignOffset`)
- File descriptor must remain open for the lifetime of the mapping
- `MAP_SHARED`: writes are visible to other processes and persisted to disk
- `MAP_PRIVATE`: creates copy-on-write mapping, changes not visible to others
- Use `MemmapSync` to ensure writes are flushed to disk

## Dependencies

- Platform-specific implementations require `golang.org/x/sys/unix` (for Unix) or Windows syscalls (for Windows)
- Build tags control platform-specific code (`//go:build unix` and `//go:build windows`)

## Relationship to Other Libraries

`memcore` is the foundation for:
- **memforge** - Uses `MemmapRequest` for allocator backends
- **memstruct** - Uses marks and memory utilities for data structures
- **memarch** - Combines memforge allocators with memstruct data structures
- **blaze** - Uses memstruct for numerical computations

All manual memory libraries in this ecosystem depend on `memcore` for basic operations.

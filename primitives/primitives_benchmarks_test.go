package primitives

import (
	"memcore"
	"runtime"
	"testing"
	"unsafe"
)

var sink unsafe.Pointer

// -----------------------------------------------------------
// Structs for different payload sizes
// -----------------------------------------------------------
type small struct{ a, b, c, d uint64 }              // 32 B
type medium struct{ a, b, c, d, e, f, g, h uint64 } // 64 B
type large struct{ data [128]byte }                 // 128 B
type huge struct{ data [512]byte }                  // 512 B
type enormous struct{ data [1024]byte }             // 1 KiB

// -----------------------------------------------------------
// Copy primitives
// -----------------------------------------------------------
func setByMoveTest[T any](dst unsafe.Pointer, value *T) {
	memcore.MemoryMoveNoHeapPointers(dst, unsafe.Pointer(value), unsafe.Sizeof(*value))
}
func setByAssignTest[T any](dst unsafe.Pointer, value *T) {
	*(*T)(dst) = *value
}

// -----------------------------------------------------------
// Bench harness (OOM-safe)
// -----------------------------------------------------------
func benchSet[T any](b *testing.B, value T, useMove bool) {
	const arenaSize = 256 * 1024 // 256 KiB total buffer — small but plenty
	mem := make([]byte, arenaSize)
	base := unsafe.Pointer(&mem[0])
	val := value
	valSize := unsafe.Sizeof(val)

	// stride large enough to avoid cache aliasing but small enough for reuse
	step := uintptr(64)

	b.ReportAllocs()
	runtime.GC()
	b.ResetTimer()

	// reuse same arena cyclically — no growth, no new allocations
	for i := 0; i < b.N; i++ {
		offset := (uintptr(i) * step) % (arenaSize - uintptr(valSize))
		dst := unsafe.Add(base, offset)

		// mutate first byte every few iterations to prevent elimination
		if i&15 == 0 {
			(*(*[1]byte)(unsafe.Pointer(&val)))[0] ^= 0xFF
		}

		if useMove {
			setByMoveTest(dst, &val)
		} else {
			setByAssignTest(dst, &val)
		}
	}

	sink = base
	runtime.KeepAlive(val)
}

// -----------------------------------------------------------
// Generic wrapper for each struct type
// -----------------------------------------------------------
func runBench[T any](b *testing.B, name string, sample T) {
	b.Run(name+"/Assign", func(b *testing.B) { benchSet(b, sample, false) })
	b.Run(name+"/Memmove", func(b *testing.B) { benchSet(b, sample, true) })
}

// -----------------------------------------------------------
// Benchmark suite
// -----------------------------------------------------------
func BenchmarkArraySetMethods(b *testing.B) {
	// Run sequentially, no concurrent sub-benches
	b.SetParallelism(1)

	runBench(b, "Small(32B)", small{})
	runtime.GC()

	runBench(b, "Medium(64B)", medium{})
	runtime.GC()

	runBench(b, "Large(128B)", large{})
	runtime.GC()

	runBench(b, "Huge(512B)", huge{})
	runtime.GC()

	runBench(b, "Enormous(1024B)", enormous{})
	runtime.GC()
}

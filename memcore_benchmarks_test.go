package memcore

import (
	"fmt"
	"runtime"
	"testing"
	"unsafe"
)

var sink unsafe.Pointer

// -----------------------------------------------------------
// Manual copy path (mirrors MemoryMoveNoHeapPointers fast path)
// -----------------------------------------------------------
func manualCopy(dst, src unsafe.Pointer, n uintptr) {
	if n == 0 || dst == src {
		return
	}
	d := uintptr(dst)
	s := uintptr(src)
	if s < d && s+n > d {
		// Overlapping regions: copy backward
		for i := n; i > 0; i-- {
			*(*byte)(unsafe.Pointer(d + i - 1)) =
				*(*byte)(unsafe.Pointer(s + i - 1))
		}
	} else {
		// Non-overlapping (or forward-safe)
		for i := uintptr(0); i < n; i++ {
			*(*byte)(unsafe.Pointer(d + i)) =
				*(*byte)(unsafe.Pointer(s + i))
		}
	}
}

// -----------------------------------------------------------
// Bench harness
// -----------------------------------------------------------
func benchMove(b *testing.B, size uintptr, mode string) {
	mem := make([]byte, size*2+128) // ensure buffer for overlap cases
	src := unsafe.Pointer(&mem[0])
	dst := unsafe.Add(src, size+32) // 32B gap for non-overlapping copy

	// Initialize bytes so the CPU really moves data
	for i := uintptr(0); i < size; i++ {
		*(*byte)(unsafe.Add(src, i)) = byte(i)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		switch mode {
		case "manual":
			manualCopy(dst, src, size)
		case "memmove":
			memmoveInternal(dst, src, size)
		case "overlap":
			manualCopy(unsafe.Add(src, size/2), src, size)
		}
	}

	runtime.KeepAlive(src)
	runtime.KeepAlive(dst)
	sink = dst
}

// -----------------------------------------------------------
// Benchmark suite
// -----------------------------------------------------------
func BenchmarkMemoryMovePaths(b *testing.B) {
	sizes := []uintptr{
		1, 4, 8, 12, 16, 24, 32, 40, 48, 64, 96, 128, 256, 512, 1024, 4096,
	}

	for _, sz := range sizes {
		b.Run(fmt.Sprintf("Manual/%dB", sz),
			func(b *testing.B) { benchMove(b, sz, "manual") })

		b.Run(fmt.Sprintf("Memmove/%dB", sz),
			func(b *testing.B) { benchMove(b, sz, "memmove") })

		b.Run(fmt.Sprintf("ManualOverlap/%dB", sz),
			func(b *testing.B) { benchMove(b, sz, "overlap") })
	}
}

package memcore

import (
	"reflect"
	"unsafe"
)

/*
SizeOf returns the size in bytes of T using unsafe.Sizeof.

[Complexity]
Time: O(1). Space: O(1).

[Side Effects]
Pure at compile time; result is constant for a given T.

[Edge Cases]
On exotic architectures, uint64 conversion from unsafe.Sizeof could theoretically differ from native size_t expectations.
*/
//go:inline
//go:nosplit
func SizeOf[T any]() uint64 {
	var zero T
	return (uint64)(unsafe.Sizeof(zero))
}

/*
AlignOf returns the required alignment of T using unsafe.Alignof.
*/
//go:inline
//go:nosplit
func AlignOf[T any]() uint64 {
	var zero T
	return (uint64)(unsafe.Alignof(zero))
}

/*
TypeOf returns reflect.TypeFor[T].
*/
//go:inline
//go:nosplit
func TypeOf[T any]() reflect.Type {
	return reflect.TypeFor[T]()
}

/*
AlignUp rounds n up to the next multiple of alignment using bit masking.

[Parameters]
alignment - Must be a power of two.

[Returns]
The smallest value >= n divisible by alignment.

[Example]
	AlignUp(13, 8) => 16
	AlignUp(32, 8) => 32

[Complexity]
Time: O(1). Space: O(1).

[Side Effects]
Pure function.
*/
//go:inline
//go:nosplit
func AlignUp(n, alignment uint64) uint64 {
	mask := alignment - 1
	return (n + mask) &^ mask
}

/*
NextPowerOfTwo returns the smallest power of two greater than or equal to x.

[Returns]
1 when x is 0; x unchanged when x is already a power of two.

[Example]
	NextPowerOfTwo(5)  => 8
	NextPowerOfTwo(64) => 64

[Complexity]
Time: O(1). Space: O(1).

[Edge Cases]
For x in (2^63, 2^64-1] the bit-twiddling path overflows to 0.
*/
//go:inline
//go:nosplit
func NextPowerOfTwo(x uint64) uint64 {
	if x == 0 {
		return 1
	}
	x--
	x |= x >> 1
	x |= x >> 2
	x |= x >> 4
	x |= x >> 8
	x |= x >> 16
	x |= x >> 32
	return x + 1
}

/*
IsAligned reports whether ptr's address is divisible by alignment.

[Parameters]
alignment - Must be a power of two.

[Errors]
Panics if alignment is not a power of two.

[Side Effects]
Pure address check; does not read object contents.
*/
//go:inline
//go:nosplit
func IsAligned(ptr unsafe.Pointer, alignment uint64) bool {
	if alignment&(alignment-1) != 0 {
		panic("alignment must be power of two")
	}

	return uintptr(ptr)&uintptr(alignment-1) == 0
}

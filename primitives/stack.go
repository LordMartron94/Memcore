package primitives

import (
	"fmt"
	"unsafe"
)

// Stack is a custom stack implementation build on top of the array primitive.
// It contains an unsafe Pointer internally and therefore can NOT be stored in custom allocated memory.
type Stack[T any] struct {
	data   *Array[T]
	length uint64
}

// StackCreateAt creates an instance of a stack for type T at a specific memory address.
// Ensure the address is properly aligned and has the right size.
//
// ⚠️ capacity is in elements, not bytes.
// When using memory returned from a byte allocator,
// convert using: capacity = bytes / memcore.SizeOf[T]()
//
// ⚠️ Important: Do NOT allocate this struct itself inside manually-managed memory.
// The struct contains Go pointers and must remain
// visible to the Go garbage collector.
//
// You may, however, point its internal data (the `ptr` field) to memory that was
// manually allocated (e.g. via mmap or a custom allocator). In other words:
//
//	✅ Safe:   header on Go heap, data in manual memory
//	❌ Unsafe: header and data both in manual memory
func StackCreateAt[T any](addr unsafe.Pointer, capacity uint64) *Stack[T] {
	array := ArrayCreateAt[T](addr, capacity)

	return &Stack[T]{
		data:   array,
		length: 0,
	}
}

// StackPush pushes an item into the stack and does boundary validation.
func StackPush[T any](instance *Stack[T], item T) error {
	if instance.length >= ArrayCapacityGet(instance.data) {
		return fmt.Errorf("stack overflow: capacity %d", ArrayCapacityGet(instance.data))
	}

	ArraySetAtUnsafe(instance.data, instance.length, item)

	instance.length++
	return nil
}

// StackPushUnsafe pushes an item into the stack.
// It does not validate boundaries.
func StackPushUnsafe[T any](instance *Stack[T], item T) {
	ArraySetAtUnsafe(instance.data, instance.length, item)

	instance.length++
}

// StackPop retrieves an element from the stack while performing boundary validation.
func StackPop[T any](instance *Stack[T]) (T, error) {
	if instance.length == 0 {
		var zero T
		return zero, fmt.Errorf("stack underflow: stack empty")
	}

	if item, err := ArrayItemGetAt(instance.data, instance.length-1); err != nil {
		return item, err
	} else {
		instance.length--
		return item, nil
	}
}

// StackPopUnsafe retrieves an element from the stack.
// It does not validate boundaries.
func StackPopUnsafe[T any](instance *Stack[T]) T {
	item := ArrayItemGetAtUnsafe(instance.data, instance.length-1)
	instance.length--
	return item
}

// StackPeek peeks at the last pushed item while validating boundaries.
func StackPeek[T any](instance *Stack[T]) (T, error) {
	if item, err := ArrayItemGetAt(instance.data, instance.length-1); err != nil {
		return item, err
	} else {
		return item, nil
	}
}

// StackPeekUnsafe peeks at the last pushed item.
// It does not validate boundaries.
func StackPeekUnsafe[T any](instance *Stack[T]) T {
	item := ArrayItemGetAtUnsafe(instance.data, instance.length-1)
	return item
}

// StackClear clears the stack, allowing for reuse.
// It does NOT zero the memory.
func StackClear[T any](instance *Stack[T]) {
	instance.length = 0
}

// StackClearAndZero clears the stack, allowing for reuse.
// It does zero the memory.
func StackClearAndZero[T any](instance *Stack[T]) {
	ArrayClear(instance.data)
	instance.length = 0
}

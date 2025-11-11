//go:build !amd64

package memcore

import "unsafe"

func platformPrefetchReadAsm(ptr unsafe.Pointer) {}

//go:build amd64

package memcore

import "unsafe"

//go:noescape
func platformPrefetchReadAsm(ptr unsafe.Pointer)

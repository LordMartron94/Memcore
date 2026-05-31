//go:build !amd64 && !arm64

package memcore

import "unsafe"

/*
platformPrefetchReadAsm is a no-op on architectures that do not have a dedicated assembly prefetch
implementation.

[Context]
amd64 routes to PREFETCHT0 via prefetch_amd64.go and prefetch.s, and arm64 uses prefetch_arm64.go.
Every other GOARCH falls through to this stub. The filename deliberately avoids the `_amd64.go`
suffix so Go's implicit GOARCH filename constraint does not combine with the explicit `!amd64`
build tag to exclude this file from every build.
*/
func platformPrefetchReadAsm(ptr unsafe.Pointer) {}

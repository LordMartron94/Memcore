package memcore

import (
	"testing"
	"unsafe"
)

func TestMemcoreRegionRegisterSubRegionDereference(t *testing.T) {
	MemcoreMarkManagementStateReset(false)

	buffer := make([]byte, 256)
	parentRegionID := MemcoreRegionRegister(uintptr(unsafe.Pointer(&buffer[0])), uint64(len(buffer)))
	subRegionID := MemcoreRegionRegisterSubRegion(parentRegionID, 64, 32)

	childMark := MemcoreMarkCreate(subRegionID, 8)
	resolved := MemcoreMarkResolveToRoot(childMark)

	if MemcoreMarkRegionIDGet(resolved) != parentRegionID {
		t.Fatalf("resolved region ID = %d, want %d", MemcoreMarkRegionIDGet(resolved), parentRegionID)
	}

	if MemcoreMarkOffsetGet(resolved) != uintptr(72) {
		t.Fatalf("resolved offset = %d, want 72", MemcoreMarkOffsetGet(resolved))
	}

	destination := MemcoreMarkDereference(childMark)
	if uintptr(destination) != uintptr(unsafe.Pointer(&buffer[72])) {
		t.Fatalf("dereference address = %p, want %p", destination, unsafe.Pointer(&buffer[72]))
	}

	MemcoreRegionUnregister(subRegionID)
	MemcoreRegionUnregister(parentRegionID)
}

func TestMemcoreMarkResolveToRootNestedChain(t *testing.T) {
	MemcoreMarkManagementStateReset(false)

	buffer := make([]byte, 512)
	rootRegionID := MemcoreRegionRegister(uintptr(unsafe.Pointer(&buffer[0])), uint64(len(buffer)))
	levelOneID := MemcoreRegionRegisterSubRegion(rootRegionID, 128, 256)
	levelTwoID := MemcoreRegionRegisterSubRegion(levelOneID, 64, 64)

	childMark := MemcoreMarkCreate(levelTwoID, 16)
	resolved := MemcoreMarkResolveToRoot(childMark)

	if MemcoreMarkRegionIDGet(resolved) != rootRegionID {
		t.Fatalf("nested resolved region ID = %d, want %d", MemcoreMarkRegionIDGet(resolved), rootRegionID)
	}

	if MemcoreMarkOffsetGet(resolved) != uintptr(208) {
		t.Fatalf("nested resolved offset = %d, want 208", MemcoreMarkOffsetGet(resolved))
	}

	MemcoreRegionUnregister(levelTwoID)
	MemcoreRegionUnregister(levelOneID)
	MemcoreRegionUnregister(rootRegionID)
}

func TestMemcoreRegionRegisterSubRegionOpaqueInheritance(t *testing.T) {
	MemcoreMarkManagementStateReset(false)

	parentRegionID := MemcoreRegionRegisterOpaque(4096)
	subRegionID := MemcoreRegionRegisterSubRegion(parentRegionID, 0, 1024)

	if !MemcoreRegionIsOpaque(subRegionID) {
		t.Fatal("sub-region did not inherit opaque flag")
	}

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic when dereferencing opaque sub-region mark")
		}
		MemcoreRegionUnregister(subRegionID)
		MemcoreRegionUnregister(parentRegionID)
	}()

	MemcoreMarkDereference(MemcoreMarkCreate(subRegionID, 0))
}

func TestMemcoreRegionUnregisterParentWithActiveSubRegionPanics(t *testing.T) {
	MemcoreMarkManagementStateReset(false)

	parentRegionID := MemcoreRegionRegisterOpaque(4096)
	subRegionID := MemcoreRegionRegisterSubRegion(parentRegionID, 0, 1024)

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic when unregistering parent with active sub-region")
		}
		MemcoreRegionUnregister(subRegionID)
		MemcoreRegionUnregister(parentRegionID)
	}()

	MemcoreRegionUnregister(parentRegionID)
}

func TestMemcoreMarkOffsetInBoundsSubRegionWindow(t *testing.T) {
	MemcoreMarkManagementStateReset(false)

	parentRegionID := MemcoreRegionRegisterOpaque(4096)
	subRegionID := MemcoreRegionRegisterSubRegion(parentRegionID, 512, 128)

	inBounds := MemcoreMarkCreate(subRegionID, 64)
	outOfBounds := MemcoreMarkCreate(subRegionID, 96)

	if !MemcoreMarkOffsetInBounds(inBounds, 32) {
		t.Fatal("expected in-bounds sub-region mark")
	}

	if MemcoreMarkOffsetInBounds(outOfBounds, 64) {
		t.Fatal("expected out-of-bounds sub-region mark")
	}

	MemcoreRegionUnregister(subRegionID)
	MemcoreRegionUnregister(parentRegionID)
}

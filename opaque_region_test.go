package memcore

import (
	foundationTesting "foundation/testing"
	"testing"
)

func TestMemcoreRegionRegisterOpaque(t *testing.T) {
	MemcoreMarkManagementStateReset(false)

	regionID := MemcoreRegionRegisterOpaque(4096)
	foundationTesting.Assert(MemcoreRegionIsOpaque(regionID), "region not opaque", "opaque region", t)

	mark := MemcoreMarkCreate(regionID, 128)
	foundationTesting.Assert(MemcoreMarkRegionIsOpaque(mark), "mark region not opaque", "opaque mark", t)
	foundationTesting.Assert(MemcoreMarkOffsetInBounds(mark, 256), "in bounds failed", "in bounds", t)
	foundationTesting.Assert(!MemcoreMarkOffsetInBounds(mark, 5000), "out of bounds accepted", "out of bounds rejected", t)

	didPanic := false
	func() {
		defer func() {
			if recover() != nil {
				didPanic = true
			}
		}()
		_ = MemcoreMarkDereference(mark)
	}()
	foundationTesting.Assert(didPanic, "opaque dereference did not panic", "opaque dereference panicked", t)

	MemcoreRegionUnregister(regionID)
	foundationTesting.Assert(!MemcoreMarkIsValid(mark), "mark valid after unregister", "mark invalid after unregister", t)
}

func TestMemcoreAddressBelongsToActiveRegionOpaque(t *testing.T) {
	MemcoreMarkManagementStateReset(false)

	regionID := MemcoreRegionRegisterOpaque(1024)
	foundationTesting.Assert(!MemcoreAddressBelongsToActiveRegion(0x1000), "opaque counted as mapped", "opaque excluded", t)
	MemcoreRegionUnregister(regionID)
}

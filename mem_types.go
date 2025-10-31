package memcore

type MemoryUnitBytes uint64

const (
	Byte     MemoryUnitBytes = 1
	KiloByte MemoryUnitBytes = 1024
	MegaByte MemoryUnitBytes = KiloByte * 1024
	GigaByte MemoryUnitBytes = MegaByte * 1024
)
